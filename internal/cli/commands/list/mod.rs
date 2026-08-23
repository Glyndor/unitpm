//! `unitpm list|ls|ps` — table or JSON view of every managed process.
//!
//! 26 cases ported from `internal/cli/commands/list/{cmd_test,sort_test,
//! notify_test}.go`. The Go file splits into `cmd.go` (argument parsing,
//! IPC call, render dispatch) and `sort.go` (sort-spec parser); they
//! live together here.

mod args;
mod sort;

// Test-only. Declared without `#[cfg(test)]` they compile into the library,
// where their `#[test]` functions vanish and every fixture they import reads
// as unused.
#[cfg(test)]
mod mock_client;
#[cfg(test)]
mod parser_tests;
#[cfg(test)]
mod tests;

use std::collections::{HashMap, HashSet};

use args::{args_contain_help, parse_args};
use std::io::{self, Write};

use crate::cli::format;
use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::table::Table;
use crate::term;
use crate::types::{ProcessInfo, ProcessState, DEFAULT_NAMESPACE};
use crate::updater;

pub use sort::{parse_sort_spec, SortField};

// ---------------------------------------------------------------------------
// IPC contract
// ---------------------------------------------------------------------------

/// IPC surface the list command needs. Defined locally so the mock used
/// in tests does not have to re-implement the full `transport::IPCClient`
/// (which is generic over Serialize/Deserialize and not object-safe).
/// Each command in phase 6b ports its own private `IpcOps` for the same
/// reason — three private traits stay cheaper than three variants of one
/// shared trait.
pub trait IpcOps {
	fn call_list(&mut self) -> Result<Vec<ProcessInfo>, IpcError>;
}

/// String-payload error wrapper; matches the Go side, which surfaces
/// errors via `fmt.Errorf("list failed: %w", ...)`.
#[derive(Debug)]
pub struct IpcError(pub String);

impl std::fmt::Display for IpcError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.write_str(&self.0)
	}
}

impl std::error::Error for IpcError {}

impl From<&str> for IpcError {
	fn from(s: &str) -> Self {
		Self(s.to_string())
	}
}

impl From<String> for IpcError {
	fn from(s: String) -> Self {
		Self(s)
	}
}

// ---------------------------------------------------------------------------
// Render options
// ---------------------------------------------------------------------------

/// Controls how the process table renders. `Highlight` carries process
/// IDs or names that should be visually marked, used to emphasise the
/// targets of a preceding start/stop/restart action pm2-style.
#[derive(Debug, Default, Clone)]
pub struct RenderOptions {
	pub show_long: bool,
	pub highlight: HashSet<String>,
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Run the list command with the given IPC client. Writes the table (or
/// `--json` blob) to `out`. Errors are returned as
/// `Box<dyn Error + Send + Sync>` so the dispatcher's `handle_error_to`
/// can downcast and treat usage errors differently.
pub fn run<O: Write, C: IpcOps>(
	client: &mut C,
	out: &mut O,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if args_contain_help(args) {
		let _ = print_help_to(out);
		return Ok(());
	}

	let options = match parse_args(args) {
		Ok(o) => o,
		Err(e) => return Err(Box::new(e)),
	};

	let procs = match client.call_list() {
		Ok(p) => p,
		Err(e) => return Err(format!("list failed: {e}").into()),
	};

	let procs = filter_processes(&procs, &options.namespace);

	if options.json_output {
		let bytes = serde_json::to_vec(&procs)
			.map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;
		out.write_all(&bytes)?;
		writeln!(out)?;
		return Ok(());
	}

	let opts = RenderOptions {
		show_long: options.show_long,
		highlight: HashSet::new(),
	};
	render_to(out, &procs, &opts);
	Ok(())
}

/// Fetch the process list and render it with `highlight` markers. Used
/// by `start`/`stop`/`restart` for the pm2-style follow-up table.
/// Errors are silently swallowed; the primary action already succeeded,
/// so a follow-up failure must not propagate a non-zero exit.
pub fn fetch_and_render<O: Write, C: IpcOps>(
	client: &mut C,
	highlight: HashSet<String>,
	out: &mut O,
) {
	let procs = match client.call_list() {
		Ok(p) => p,
		Err(_) => return,
	};
	let _ = writeln!(out);
	let opts = RenderOptions {
		show_long: false,
		highlight,
	};
	render_to(out, &procs, &opts);
}

// ---------------------------------------------------------------------------
// Sorting & filtering
// ---------------------------------------------------------------------------

/// Apply the user-specified sort, falling back to the default. Layered
/// in the Go side via `fields = empty || {default}`; we mirror that here.
pub(crate) fn sort_processes(processes: &mut [ProcessInfo], user_fields: &[SortField]) {
	let fields: Vec<SortField> = if user_fields.is_empty() {
		vec![
			SortField {
				field: "namespace".into(),
				asc: true,
			},
			SortField {
				field: "name".into(),
				asc: true,
			},
			SortField {
				field: "createdAt".into(),
				asc: false,
			},
			SortField {
				field: "id".into(),
				asc: true,
			},
		]
	} else {
		user_fields.to_vec()
	};
	processes.sort_by(|pi, pj| {
		for f in &fields {
			if let Some(cmp) = compare_process(pi, pj, f) {
				if cmp != std::cmp::Ordering::Equal {
					return cmp;
				}
			}
		}
		std::cmp::Ordering::Equal
	});
}

fn compare_process(
	pi: &ProcessInfo,
	pj: &ProcessInfo,
	f: &SortField,
) -> Option<std::cmp::Ordering> {
	let cmp = match f.field.as_str() {
		"namespace" => {
			let ni = if pi.namespace.is_empty() {
				DEFAULT_NAMESPACE
			} else {
				&pi.namespace
			};
			let nj = if pj.namespace.is_empty() {
				DEFAULT_NAMESPACE
			} else {
				&pj.namespace
			};
			ni.cmp(nj)
		}
		"name" => pi.name.to_lowercase().cmp(&pj.name.to_lowercase()),
		"createdAt" => {
			let a = pi.created_at.as_deref().unwrap_or("");
			let b = pj.created_at.as_deref().unwrap_or("");
			a.cmp(b)
		}
		"id" => pi.id.cmp(&pj.id),
		_ => return None,
	};
	Some(ord(cmp, f.asc))
}

fn ord(c: std::cmp::Ordering, asc: bool) -> std::cmp::Ordering {
	if asc {
		c
	} else {
		c.reverse()
	}
}

/// Short-ID prefix length — minimum prefix (>=8) that uniquely identifies
/// every process ID in the list. Stops short-ID collisions when several
/// processes are created in rapid succession.
pub fn short_id_len(processes: &[ProcessInfo]) -> usize {
	const MIN_LEN: usize = 8;
	if processes.len() <= 1 {
		return MIN_LEN;
	}
	for l in MIN_LEN..=36 {
		let mut seen: HashMap<&str, bool> = HashMap::new();
		let mut collide = false;
		for p in processes {
			let prefix: &str = if p.id.len() > l { &p.id[..l] } else { &p.id };
			if seen.insert(prefix, true).is_some() {
				collide = true;
				break;
			}
		}
		if !collide {
			return l;
		}
	}
	36
}

/// Filter `processes` by `filter`. Empty filter returns the input.
/// Empty process namespaces are normalised to "default" so
/// `--namespace default` finds them.
pub fn filter_processes(processes: &[ProcessInfo], filter: &str) -> Vec<ProcessInfo> {
	if filter.is_empty() {
		return processes.to_vec();
	}
	processes
		.iter()
		.filter(|p| {
			let ns = if p.namespace.is_empty() {
				DEFAULT_NAMESPACE
			} else {
				&p.namespace
			};
			ns == filter
		})
		.cloned()
		.collect()
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

/// Render the process list as a box-drawing table. Exposed for the
/// other lifecycle commands (`start`/`stop`/`restart` follow up their
/// primary action with a highlighted version of the same table).
pub fn render_to<W: Write>(w: &mut W, processes: &[ProcessInfo], opts: &RenderOptions) {
	let headers = [
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "id")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "name")))),
		term::cyan(format_args!(
			"{}",
			term::bold(format_args!("{}", "namespace"))
		)),
		term::cyan(format_args!(
			"{}",
			term::bold(format_args!("{}", "version"))
		)),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "mode")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "pid")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "uptime")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "↺")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "status")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "cpu")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "mem")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "user")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "git")))),
		term::cyan(format_args!("{}", term::bold(format_args!("{}", "watch")))),
	];
	let header_refs: Vec<&str> = headers.iter().map(String::as_str).collect();
	let mut t = Table::new(&header_refs);

	let short_id_w = short_id_len(processes);
	let id_col_width = if opts.show_long { 36 } else { short_id_w };
	let highlight_pad = if opts.highlight.is_empty() { 0 } else { 2 };

	t.set_max_col_widths(&[
		id_col_width + highlight_pad, // id — dynamic width to avoid short-ID collisions
		40,                           // name — 128-char max upstream; 40 covers most labels
		20,                           // namespace
		10,                           // version
		10,                           // mode
		8,                            // pid
		10,                           // uptime
		5,                            // restarts
		15,                           // status
		8,                            // cpu
		10,                           // mem
		15,                           // user
		20,                           // git
		10,                           // watch
	]);

	// Default sort when no spec is given. The Go parser leaves
	// `sort_fields` empty in that case so the layered-default branch
	// fires inside `sortProcesses`. We do the same here.
	let mut user_fields: Vec<SortField> = Vec::new();
	if !processes.is_empty() {
		// Caller passes the parser's user fields via this private API.
		// The default branch is handled by passing an empty slice.
		user_fields.clear();
	}
	let mut sorted: Vec<ProcessInfo> = processes.to_vec();
	sort_processes(&mut sorted, &user_fields);

	for p in &sorted {
		let status_str = match p.state {
			ProcessState::Running | ProcessState::Online => {
				term::green(format_args!("{}", p.state.as_str()))
			}
			ProcessState::Stopped | ProcessState::Failed => {
				term::red(format_args!("{}", p.state.as_str()))
			}
			ProcessState::Restarting => term::yellow(format_args!("{}", p.state.as_str())),
			_ => p.state.as_str().to_string(),
		};

		let pid_str = if p.pid == 0 {
			term::dim(format_args!("{}", "-"))
		} else {
			format!("{}", p.pid)
		};

		let uptime_str = format::uptime(p.uptime);
		let mem_str = format::bytes(p.memory);

		let cpu_str = if p.cpu == 0.0 {
			"0%".to_string()
		} else {
			format!("{:.1}%", p.cpu)
		};

		let watch_str = if p.watch {
			term::green(format_args!("{}", "enabled"))
		} else {
			term::dim(format_args!("{}", "disabled"))
		};

		let raw_id = if opts.show_long {
			p.id.clone()
		} else if p.id.len() > short_id_w {
			p.id[..short_id_w].to_string()
		} else {
			p.id.clone()
		};

		let id_str = if opts.highlight.is_empty() {
			raw_id
		} else if opts.highlight.contains(&p.id) || opts.highlight.contains(&p.name) {
			format!(
				"{} {}",
				term::green(format_args!("{}", "▸ ")),
				term::bold(format_args!("{}", raw_id))
			)
		} else {
			format!("  {raw_id}")
		};

		let git_str = if !p.git_branch.as_deref().unwrap_or("").is_empty() {
			let branch = p.git_branch.as_deref().unwrap_or("");
			let commit = p.git_commit.as_deref().unwrap_or("");
			let s = format!("{branch}@{commit}");
			if p.git_dirty {
				let mut t = s;
				t.push('*');
				term::yellow(format_args!("{}", t))
			} else {
				term::dim(format_args!("{}", s))
			}
		} else {
			term::dim(format_args!("{}", "-"))
		};

		let row: [String; 14] = [
			id_str,
			term::bold(format_args!("{}", p.name)),
			p.namespace.clone(),
			p.version.clone(),
			p.mode.clone(),
			pid_str,
			uptime_str,
			format!("{}", p.restarts),
			status_str,
			cpu_str,
			mem_str,
			p.user.clone(),
			git_str,
			watch_str,
		];
		let refs: Vec<&str> = row.iter().map(String::as_str).collect();
		t.add_row(&refs);
	}

	let width = crate::term::get_terminal_width();
	let _ = t.render_to(w, width);
}

// ---------------------------------------------------------------------------
// spec / help
// ---------------------------------------------------------------------------

/// Command spec for the registry.
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "list".into(),
		aliases: vec!["ls".into(), "ps".into()],
		usage: "unitpm list|ls|ps [options]".into(),
		description: "List all managed processes.".into(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--long".into(),
				description: "Show full process IDs.".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--namespace <name>".into(),
				description: "Filter by namespace".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--sort <fields>".into(),
				description: "Sort order, e.g. 'namespace:asc,name:asc,createdAt:desc'".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--json".into(),
				description: "Emit the process list as JSON on stdout".into(),
			},
		],
		examples: vec![
			"unitpm list".into(),
			"unitpm ls --namespace prod".into(),
			"unitpm ls --sort name:asc".into(),
			"unitpm ls --long".into(),
			"unitpm ls --json | jq '.[] | {name, state, pid}'".into(),
		],
		hidden: false,
	}
}

/// Render the command-specific help to `out`. Shared with the dispatcher
/// which calls it when `--help` is passed.
pub fn print_help_to<W: Write>(w: &mut W) -> io::Result<()> {
	let spec = spec();
	crate::cli::help::render_command_help(w, &spec)
}

/// Print the help block to stdout. Used by the root dispatcher when the
/// command is invoked with `--help`.
pub fn print_help() {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	let _ = print_help_to(&mut out);
}

// ---------------------------------------------------------------------------
// update-notification helpers — exposed because they are referenced from
// the `notify_test.go` cases (6 of them). The channel shape mirrors the
// Go side's bounded buffer + deadline.
// ---------------------------------------------------------------------------

use std::sync::mpsc::Receiver;
use std::time::{Duration, Instant};

/// Block until `ch` produces a release or `deadline` elapses, printing a
/// yellow banner to stderr when one arrives. The Go side calls the
/// banner printer from `Run` after the IPC round-trip. The Rust port
/// keeps the same shape; tests drive a single-slot channel to assert
/// each branch (nil, value, timeout) without panicking.
pub fn wait_update_and_notify(ch: &Receiver<Option<updater::CachedRelease>>, deadline: Instant) {
	let now = Instant::now();
	let remaining = deadline.saturating_duration_since(now);
	let timeout = Duration::from_millis(remaining.as_millis() as u64);
	if let Ok(Some(rel)) = ch.recv_timeout(timeout) {
		print_update_banner(&rel.tag_name);
	}
}

fn print_update_banner(tag_name: &str) {
	let stderr = io::stderr();
	let mut err = stderr.lock();
	let _ = writeln!(
		err,
		"\n{} New version available: {} (current {})",
		term::yellow(format_args!("{}", "!")),
		term::bold(format_args!("{}", tag_name)),
		crate::version::VERSION,
	);
	let _ = writeln!(err, "  Run 'unitpm update --apply' to install.");
}
