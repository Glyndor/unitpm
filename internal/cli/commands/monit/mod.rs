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

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::commands::monit::state::MAX_HISTORY as MAX_HISTORY_VAL;
	use std::time::Duration;

	#[test]
	fn parse_args_default() {
		let p = parse_args(&[]);
		assert!(!p.json);
		assert!(p.target.is_none());
	}

	#[test]
	fn parse_args_with_target() {
		let p = parse_args(&["api".into()]);
		assert_eq!(p.target.as_deref(), Some("api"));
	}

	#[test]
	fn parse_args_with_json() {
		let p = parse_args(&["--json".into(), "api".into()]);
		assert!(p.json);
		assert_eq!(p.target.as_deref(), Some("api"));
	}

	#[test]
	fn spec_includes_aliases() {
		let s = spec();
		assert_eq!(s.name, "monit");
		assert!(s.aliases.contains(&"top".to_string()));
		assert!(s.aliases.contains(&"monitor".to_string()));
	}

	#[test]
	fn render_frame_does_not_panic_full_state() {
		let mut s = MonitState::default();
		s.info.name = "svc".into();
		s.info.pid = 1234;
		s.info.state = ProcessState::Running;
		s.info.cpu = 12.5;
		s.info.memory = 4 * 1024 * 1024;
		s.info.uptime = 3_725_000;
		s.info.restarts = 3;
		s.info.git_branch = Some("main".into());
		s.info.git_commit = Some("abc1234".into());
		s.info.version = "1.0".into();
		s.info.mode = "cluster".into();
		s.info.user = "root".into();
		s.spec.exec.command = Some("/usr/bin/node".into());
		s.spec.exec.args = Some(vec!["server.js".into()]);
		s.cpu_hist = vec![0.0, 5.0, 10.0, 15.0, 20.0, 25.0, 12.5];
		s.mem_hist = vec![0, 1024 * 1024, 2 * 1024 * 1024, 4 * 1024 * 1024];
		s.mem_max = 4 * 1024 * 1024;
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 120).unwrap();
		assert!(!buf.is_empty());
	}

	#[test]
	fn render_frame_with_process_tree() {
		let mut s = MonitState::default();
		s.info.name = "svc".into();
		s.info.pid = 42;
		s.info.state = ProcessState::Running;
		s.cpu_hist = vec![10.0; 10];
		s.mem_hist = vec![1024 * 1024; 10];
		s.mem_max = 2 * 1024 * 1024;
		s.tree = vec![
			crate::metrics::ChildStat {
				pid: 42,
				comm: "node".into(),
				depth: 0,
				memory_bytes: 1024 * 1024,
			},
			crate::metrics::ChildStat {
				pid: 43,
				comm: "worker".into(),
				depth: 1,
				memory_bytes: 512 * 1024,
			},
		];
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 80).unwrap();
	}

	#[test]
	fn render_frame_stopped_state() {
		let mut s = MonitState::default();
		s.info.name = "svc".into();
		s.info.state = ProcessState::Stopped;
		s.cpu_hist = vec![0.0; 5];
		s.mem_hist = vec![0; 5];
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 80).unwrap();
	}

	#[test]
	fn render_frame_failed_state() {
		let mut s = MonitState::default();
		s.info.name = "svc".into();
		s.info.state = ProcessState::Failed;
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 80).unwrap();
	}

	#[test]
	fn render_frame_empty_history() {
		let mut s = MonitState::default();
		s.info.name = "empty".into();
		s.info.state = ProcessState::Running;
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 80).unwrap();
	}

	#[test]
	fn render_frame_no_git() {
		let mut s = MonitState::default();
		s.info.name = "svc".into();
		s.info.state = ProcessState::Running;
		s.spec.exec.command = Some("/bin/true".into());
		let mut buf = Vec::new();
		render_frame_to(&mut buf, &s, 80).unwrap();
	}

	#[test]
	fn run_loop_quits_on_event() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		let events: Vec<Event> = vec![Event::Quit];
		let mut it = events.into_iter();
		run_loop(&mut state, &mut it, |_| Ok(()));
	}

	#[test]
	fn run_loop_quits_on_q_key() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		let events: Vec<Event> = vec![Event::Key(b'q'), Event::Tick];
		let mut it = events.into_iter();
		run_loop(&mut state, &mut it, |_| Ok(()));
	}

	#[test]
	fn run_loop_quits_on_ctrl_c() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		let events: Vec<Event> = vec![Event::Key(3)];
		let mut it = events.into_iter();
		run_loop(&mut state, &mut it, |_| Ok(()));
	}

	#[test]
	fn run_loop_handles_other_keys() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		let events: Vec<Event> = vec![Event::Key(b'x'), Event::Resize, Event::Quit];
		let mut it = events.into_iter();
		run_loop(&mut state, &mut it, |_| Ok(()));
	}

	#[test]
	fn run_loop_tick_triggers_refresh_and_render() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		state.info.cpu = 1.0;
		let mut calls = 0;
		let events: Vec<Event> = vec![Event::Tick, Event::Quit];
		let mut it = events.into_iter();
		run_loop(&mut state, &mut it, |s| {
			calls += 1;
			s.info.cpu += 1.0;
			Ok(())
		});
		assert!(calls >= 1, "on_tick was not invoked");
		assert!(state.info.cpu > 1.0);
	}

	#[test]
	fn max_history_locked() {
		assert_eq!(MAX_HISTORY_VAL, 120);
		assert_eq!(REFRESH_RATE, Duration::from_secs(1));
	}

	#[test]
	fn print_json_writes_payload() {
		let mut state = MonitState::default();
		state.info.name = "svc".into();
		state.info.pid = 999;
		print_json(&state).unwrap();
	}

	#[test]
	fn write_all_processes_prints_rows() {
		let procs = vec![ProcessInfo {
			name: "api".into(),
			namespace: "default".into(),
			pid: 1,
			state: ProcessState::Running,
			cpu: 0.5,
			memory: 1024,
			..empty_info()
		}];
		let mut buf = Vec::new();
		write_all_processes(&mut buf, &procs);
		let plain = String::from_utf8_lossy(&buf);
		assert!(plain.contains("api"));
		assert!(plain.contains("default"));
	}

	/// Test-only mock that returns canned responses.
	struct MockMonitClient {
		show: Option<ShowResponse>,
		list: Vec<ProcessInfo>,
		proctree: Vec<crate::metrics::ChildStat>,
		list_err: Option<String>,
		show_err: Option<String>,
	}

	impl MonitClient for MockMonitClient {
		fn call_show(&mut self, _id: &str, resp: &mut ShowResponse) -> Result<(), String> {
			if let Some(e) = &self.show_err {
				return Err(e.clone());
			}
			if let Some(s) = &self.show {
				*resp = s.clone();
			}
			Ok(())
		}
		fn call_list(&mut self, out: &mut Vec<ProcessInfo>) -> Result<(), String> {
			if let Some(e) = &self.list_err {
				return Err(e.clone());
			}
			*out = self.list.clone();
			Ok(())
		}
		fn call_proctree(
			&mut self,
			_id: &str,
			out: &mut Vec<crate::metrics::ChildStat>,
		) -> Result<(), String> {
			*out = self.proctree.clone();
			Ok(())
		}
	}

	#[test]
	fn run_with_mock_json_mode() {
		let info = ProcessInfo {
			name: "svc".into(),
			pid: 999,
			state: ProcessState::Running,
			..empty_info()
		};
		let mut client = MockMonitClient {
			show: Some(ShowResponse {
				info,
				spec: empty_spec(),
			}),
			list: Vec::new(),
			proctree: Vec::new(),
			list_err: None,
			show_err: None,
		};
		let args = vec!["svc".to_string(), "--json".to_string()];
		let events: Vec<Event> = vec![Event::Quit];
		let mut it = events.into_iter();
		run(Some(&mut client), &args, &mut it).unwrap();
	}

	#[test]
	fn fetch_state_records_history() {
		let info = ProcessInfo {
			name: "testproc".into(),
			pid: 12345,
			state: ProcessState::Running,
			cpu: 1.5,
			memory: 1024 * 1024,
			..empty_info()
		};
		let mut client = MockMonitClient {
			show: Some(ShowResponse {
				info,
				spec: empty_spec(),
			}),
			list: Vec::new(),
			proctree: Vec::new(),
			list_err: None,
			show_err: None,
		};
		let mut state = MonitState::default();
		fetch_state(&mut client, "testproc", &mut state).unwrap();
		assert_eq!(state.info.name, "testproc");
		assert_eq!(state.cpu_hist.last().copied(), Some(1.5));
		assert_eq!(state.mem_max, 1024 * 1024);
	}

	#[test]
	fn fetch_state_trims_history_at_max() {
		let info = ProcessInfo {
			cpu: 50.0,
			..empty_info()
		};
		let mut client = MockMonitClient {
			show: Some(ShowResponse {
				info,
				spec: empty_spec(),
			}),
			list: Vec::new(),
			proctree: Vec::new(),
			list_err: None,
			show_err: None,
		};
		let mut state = MonitState::default();
		for _ in 0..(MAX_HISTORY + 10) {
			fetch_state(&mut client, "x", &mut state).unwrap();
		}
		assert_eq!(state.cpu_hist.len(), MAX_HISTORY);
		assert_eq!(state.mem_hist.len(), MAX_HISTORY);
	}

	#[test]
	fn run_list_all_processes() {
		let mut client = MockMonitClient {
			show: None,
			list: vec![ProcessInfo {
				name: "api".into(),
				pid: 1,
				..empty_info()
			}],
			proctree: Vec::new(),
			list_err: None,
			show_err: None,
		};
		let args = vec![];
		let events: Vec<Event> = vec![Event::Quit];
		let mut it = events.into_iter();
		run(Some(&mut client), &args, &mut it).unwrap();
	}

	#[test]
	fn run_ipc_error_propagates() {
		let mut client = MockMonitClient {
			show: None,
			list: Vec::new(),
			proctree: Vec::new(),
			list_err: Some("connection refused".into()),
			show_err: None,
		};
		let args = vec![];
		let events: Vec<Event> = vec![Event::Quit];
		let mut it = events.into_iter();
		let err = run(Some(&mut client), &args, &mut it).unwrap_err();
		assert!(err.to_string().contains("monit failed"), "got {err}");
	}
}
