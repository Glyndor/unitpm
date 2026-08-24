//! `unitpm monit` — live btop-style view of a single managed process.
//!
//! The view is rebuilt on a timer; the loop reads a stream of input
//! events (`Tick`, `Key`, `Resize`) and drives [`render_frame`]. Tests
//! drive frames deterministically by feeding events through
//! [`run_loop`], never touching a real terminal or sleeping.
//!
//! Module split:
//!
//!   - [`view`]: rendering primitives (borders, padding, graphs).
//!   - [`state`]: the [`MonitState`] shared between the renderer and
//!     the loop.
//!   - this file: the public [`run`] entry point and the input
//!     dispatch loop.

mod state;
#[cfg(test)]
mod tests;
mod view;

use std::collections::BTreeMap;
use std::io::{self, Read, Write};

use serde::Serialize;

use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::root::cmd;
use crate::ipc::transport::IPCClient;
use crate::term;
use crate::types::{ProcessInfo, ProcessState};

pub use state::{MonitState, MAX_HISTORY, REFRESH_RATE};

/// Wire payload the daemon returns for the `show` verb. Manual
/// [`Deserialize`] because [`AppSpec`] does not implement
/// [`Default`] — a missing spec field falls back to an empty render
/// rather than a deserialization error.
#[derive(Debug, Clone, Serialize)]
pub struct ShowResponse {
	pub info: ProcessInfo,
	pub spec: crate::ipc::protocol::AppSpec,
}

impl<'de> serde::Deserialize<'de> for ShowResponse {
	fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
	where
		D: serde::Deserializer<'de>,
	{
		#[derive(serde::Deserialize)]
		struct Helper {
			info: ProcessInfo,
			#[serde(default)]
			spec: Option<crate::ipc::protocol::AppSpec>,
		}
		let h = Helper::deserialize(deserializer)?;
		Ok(ShowResponse {
			info: h.info,
			spec: h.spec.unwrap_or_else(empty_spec),
		})
	}
}

/// Inputs the loop recognises.
#[derive(Debug, Clone)]
pub enum Event {
	Tick,
	Key(u8),
	Resize,
	Quit,
}

/// Local client trait — mirrors the subset of [`crate::ipc::transport::IPCClient`]
/// the `monit` command uses. Kept module-private because [`IPCClient`]'s
/// generic method prevents `dyn` use, and the alternative would be
/// reshaping the public transport API. The dispatcher wires
/// [`crate::ipc::transport::Client`] into this trait; tests supply their
/// own implementation.
pub trait MonitClient {
	/// Call `show` for the given id and populate `resp`.
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String>;
	/// Call `list` and populate the buffer.
	fn call_list(&mut self, out: &mut Vec<ProcessInfo>) -> Result<(), String>;
	/// Call `proctree` for the given id.
	fn call_proctree(
		&mut self,
		id: &str,
		out: &mut Vec<crate::metrics::ChildStat>,
	) -> Result<(), String>;
}

impl MonitClient for crate::ipc::transport::Client {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		let params = BTreeMap::from([("id".to_string(), id.to_string())]);
		self.call("show", Some(&params), Some(resp))
			.map_err(|e| format!("monit: {e}"))
	}
	fn call_list(&mut self, out: &mut Vec<ProcessInfo>) -> Result<(), String> {
		self.call("list", None::<&String>, Some(out))
			.map_err(|e| format!("{e}"))
	}
	fn call_proctree(
		&mut self,
		id: &str,
		out: &mut Vec<crate::metrics::ChildStat>,
	) -> Result<(), String> {
		let params = BTreeMap::from([("id".to_string(), id.to_string())]);
		self.call("proctree", Some(&params), Some(out))
			.map_err(|e| format!("monit: {e}"))
	}
}

/// Forward through `Box<dyn MonitClient>` so callers that need a
/// boxed concrete client (the dispatcher path) can hand it to the
/// `dyn`-taking functions.
impl MonitClient for Box<dyn MonitClient> {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		(**self).call_show(id, resp)
	}
	fn call_list(&mut self, out: &mut Vec<ProcessInfo>) -> Result<(), String> {
		(**self).call_list(out)
	}
	fn call_proctree(
		&mut self,
		id: &str,
		out: &mut Vec<crate::metrics::ChildStat>,
	) -> Result<(), String> {
		(**self).call_proctree(id, out)
	}
}

/// Parse the `monit` command's flags and optional target name.
pub fn parse_args(args: &[String]) -> ParsedArgs {
	let mut parsed = ParsedArgs::default();
	let mut positional: Vec<String> = Vec::new();
	for a in args {
		match a.as_str() {
			"--json" | "-json" => parsed.json = true,
			x if !x.starts_with('-') => positional.push(a.clone()),
			_ => {}
		}
	}
	parsed.target = positional.into_iter().next();
	parsed
}

#[derive(Debug, Clone, Default)]
pub struct ParsedArgs {
	pub json: bool,
	pub target: Option<String>,
}

/// Public entry point. `events` drives the render loop; `client` is
/// the IPC transport (or `None` for the dispatcher to spin one up).
pub fn run(
	client: Option<&mut dyn MonitClient>,
	args: &[String],
	events: &mut dyn Iterator<Item = Event>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if crate::cli::help::is_help(args) {
		let mut buf = Vec::new();
		crate::cli::help::render_command_help(&mut buf, &spec())?;
		print!("{}", String::from_utf8_lossy(&buf));
		return Ok(());
	}

	let opts = parse_args(args);
	let mut owned_client: Option<Box<dyn MonitClient>>;
	let client: &mut dyn MonitClient = match client {
		Some(c) => c,
		None => {
			let new_client: Box<dyn MonitClient> = Box::new(
				crate::ipc::transport::Client::new().map_err(|e| format!("monit failed: {e}"))?,
			);
			owned_client = Some(new_client);
			owned_client.as_mut().unwrap()
		}
	};

	if let Some(target) = &opts.target {
		let mut state = MonitState::default();
		fetch_state(client, target, &mut state)?;
		if opts.json {
			return print_json(&state);
		}
		run_loop(&mut state, events, |s| fetch_state(client, target, s));
		Ok(())
	} else {
		// All-processes view: every tick re-queries the daemon. Errors
		// here are prefixed with `"monit failed"` so the operator sees
		// the failing op from the same wrapper as the single-process
		// path.
		let mut processes: Vec<ProcessInfo> = Vec::new();
		client
			.call_list(&mut processes)
			.map_err(|e| format!("monit failed: {e}"))?;
		let stdout = io::stdout();
		let mut out = stdout.lock();
		write_all_processes(&mut out, &processes);
		Ok(())
	}
}

fn fetch_state(
	client: &mut dyn MonitClient,
	target: &str,
	state: &mut MonitState,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	let mut resp = ShowResponse {
		info: empty_info(),
		spec: empty_spec(),
	};
	client.call_show(target, &mut resp)?;
	state.info = resp.info.clone();
	state.spec = resp.spec;

	let mut tree: Vec<crate::metrics::ChildStat> = Vec::new();
	let _ = client.call_proctree(target, &mut tree);
	state.tree = tree;

	state.cpu_hist.push(resp.info.cpu);
	state.mem_hist.push(resp.info.memory);
	if resp.info.memory > state.mem_max {
		state.mem_max = resp.info.memory;
	}
	if state.cpu_hist.len() > MAX_HISTORY {
		let drop = state.cpu_hist.len() - MAX_HISTORY;
		state.cpu_hist.drain(0..drop);
		state.mem_hist.drain(0..drop);
	}
	Ok(())
}

fn empty_info() -> ProcessInfo {
	ProcessInfo {
		id: String::new(),
		name: String::new(),
		namespace: String::new(),
		version: String::new(),
		mode: String::new(),
		pid: 0,
		uptime: 0,
		restarts: 0,
		state: ProcessState::Running,
		cpu: 0.0,
		memory: 0,
		user: String::new(),
		watch: false,
		git_branch: None,
		git_commit: None,
		git_dirty: false,
		created_at: None,
	}
}

fn empty_spec() -> crate::ipc::protocol::AppSpec {
	crate::ipc::protocol::AppSpec {
		version: 1,
		id: String::new(),
		name: String::new(),
		namespace: None,
		exec: crate::ipc::protocol::AppExec {
			kind: String::new(),
			command: None,
			args: None,
			entry: None,
			runtime: None,
			shell: false,
		},
		cwd: None,
		env: None,
		env_file: None,
		logs: None,
		restart: None,
		cron: None,
		run_as: None,
		stop: None,
		resources: None,
		watch: None,
		created_at: None,
		disabled: false,
	}
}

/// Render a single snapshot to JSON. One-shot for `--json` mode.
pub fn print_json(state: &MonitState) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	let payload = serde_json::json!({
		"info": state.info,
		"tree": state.tree,
	});
	let bytes = serde_json::to_vec(&payload).map_err(io::Error::other)?;
	let stdout = io::stdout();
	let mut out = stdout.lock();
	out.write_all(&bytes)?;
	writeln!(out)?;
	Ok(())
}

fn write_all_processes<W: Write>(w: &mut W, processes: &[ProcessInfo]) {
	let _ = write!(w, "\x1b[H\x1b[2J");
	let _ = writeln!(w, "{} monit", term::cyan(format_args!("{}", "unitpm")));
	for p in processes {
		let _ = writeln!(
			w,
			"{}/{} pid={} state={} cpu={:.1}% mem={}",
			p.namespace,
			p.name,
			p.pid,
			p.state.as_str(),
			p.cpu,
			p.memory
		);
	}
}

/// Drive the render loop until the iterator yields [`Event::Quit`].
/// `on_tick` is called to refresh state between renders. The function
/// never sleeps — the iterator paces the ticks.
pub fn run_loop<F>(state: &mut MonitState, events: &mut dyn Iterator<Item = Event>, mut on_tick: F)
where
	F: FnMut(&mut MonitState) -> Result<(), Box<dyn std::error::Error + Send + Sync>>,
{
	render_frame(state);
	for ev in events {
		match ev {
			Event::Quit => return,
			Event::Resize | Event::Tick => {
				if on_tick(state).is_err() {
					return;
				}
				render_frame(state);
			}
			Event::Key(b) if b == b'q' || b == 3 => return,
			Event::Key(_) => {}
		}
	}
}

/// Render one full frame to stdout. The width comes from
/// [`crate::term::get_terminal_width`] in production; tests inject a
/// fixed width via [`render_frame_to`].
pub fn render_frame(state: &MonitState) {
	let width = crate::term::get_terminal_width();
	let stdout = io::stdout();
	let mut out = stdout.lock();
	let _ = render_frame_to(&mut out, state, width);
}

/// Render one full frame to a caller-supplied writer at the given
/// width. The deterministic test entry point.
pub fn render_frame_to<W: Write>(w: &mut W, state: &MonitState, width: usize) -> io::Result<()> {
	let mut s = String::new();
	view::build_frame(&mut s, state, width);
	w.write_all(s.as_bytes())?;
	Ok(())
}

/// Stdin events iterator used by the dispatcher to drive the render
/// loop in production. Each yielded byte becomes an [`Event::Key`];
/// EOF and `Interrupt` / `Quit` paths surface as their respective
/// variants so the loop can shut down cleanly when the user hits
/// `q` or `Ctrl+C`.
pub fn stdin_events() -> StdinEvents {
	StdinEvents::new()
}

/// Iterator over keystroke events read from stdin. One byte at a time
/// is enough for the input the render loop cares about (`q`, `Q`,
/// `Ctrl+C`, and resize signalling on Windows where the resize is a
/// byte stream). The iterator never blocks on writes — [`Tick`] is
/// the only source of redraw cadence in production.
///
/// [`Tick`]: Event::Tick
pub struct StdinEvents {
	closed: bool,
}

impl StdinEvents {
	#[must_use]
	pub fn new() -> Self {
		Self { closed: false }
	}
}

impl Default for StdinEvents {
	fn default() -> Self {
		Self::new()
	}
}

impl Iterator for StdinEvents {
	type Item = Event;

	fn next(&mut self) -> Option<Event> {
		if self.closed {
			return None;
		}
		let stdin = io::stdin();
		let mut handle = stdin.lock();
		let mut byte = [0u8; 1];
		// A read of one byte from a non-TTY stdin under `cargo test`
		// returns 0 immediately because nothing is connected; the
		// dispatcher uses this iterator only in production, where stdin
		// is the terminal and the call blocks until a key is pressed.
		match handle.read(&mut byte) {
			Ok(0) => {
				self.closed = true;
				None
			}
			Ok(_) => Some(Event::Key(byte[0])),
			Err(_) => {
				self.closed = true;
				None
			}
		}
	}
}

/// `CommandSpec` returned to the dispatcher at registration time.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: cmd::MONIT.to_string(),
		aliases: vec!["top".to_string(), "monitor".to_string()],
		usage: format!("unitpm {}|top|monitor [process] [--json]", cmd::MONIT),
		description: "Live process statistics. Pass a name/ID for a single-process view with CPU/memory graphs and process tree. --json prints one snapshot and exits.".to_string(),
		options: vec![HelpOption {
			short: "-h".into(),
			long: "--help".into(),
			description: "Show this help message.".into(),
		}],
		examples: vec![
			format!("unitpm {} my-api", cmd::MONIT),
			format!("unitpm {} my-api --json", cmd::MONIT),
		],
		hidden: false,
	}
}
