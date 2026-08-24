//! Render the `show` page.
//!
//! One helper per [`AppSpec`] section plus a small set of
//! formatting helpers (state colouring, "0 → dash", secret masking).
//! [`render_to`] is the orchestrator — it walks the response once and
//! calls each section renderer in the order the Go side uses.
//!
//! Mirrors `internal/cli/commands/show/cmd.go`'s `renderProcess`,
//! `renderExec`, `renderEnv`, `renderLogs`, `renderRestart`,
//! `renderStop`, `renderResources`, `renderIsolation`, `renderSchedule`,
//! and `renderWatch`.

use std::io::{self, Write};

use crate::cli::format;
use crate::cli::table::{self, KvRow};
use crate::ipc::protocol::AppSpec;
use crate::term;
use crate::types::{ProcessInfo, ProcessState};

use super::ShowResponse;

/// Render the full show page to stdout. Convenience wrapper around
/// [`render_to`] for the production path; tests use [`render_to`]
/// with their own writer.
pub fn render(resp: &ShowResponse) {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	render_to(&mut out, resp);
}

/// Orchestrator. Walks the response once and calls each section
/// renderer in the order the Go side uses, with a blank line between
/// sections that opt into one.
pub fn render_to<W: Write>(w: &mut W, resp: &ShowResponse) {
	let info = &resp.info;
	let spec = &resp.spec;
	let _ = writeln!(
		w,
		"{} {} {}",
		term::bold(format_args!("{}", "Process")),
		term::cyan(format_args!("{}", non_empty(&info.name, &spec.name))),
		term::dim(format_args!("({})", non_empty(&info.id, &spec.id))),
	);
	let _ = writeln!(w);
	render_process(w, info, spec);
	let _ = writeln!(w);
	render_exec(w, spec);
	let _ = writeln!(w);
	render_env(w, spec);
	render_logs(w, spec);
	render_restart(w, spec);
	render_stop(w, spec);
	render_resources(w, spec);
	render_isolation(w, spec);
	render_schedule(w, spec);
	render_watch(w, spec);
}

fn render_process<W: Write>(w: &mut W, info: &ProcessInfo, spec: &AppSpec) {
	let ns = if !info.namespace.is_empty() {
		info.namespace.clone()
	} else {
		spec.namespace.clone().unwrap_or_default()
	};
	let rows = vec![
		KvRow::from(["state".to_string(), color_state(info.state)]),
		KvRow::from(["pid".to_string(), pid_str(info.pid)]),
		KvRow::from(["namespace".to_string(), ns]),
		KvRow::from(["version".to_string(), info.version.clone()]),
		KvRow::from(["mode".to_string(), info.mode.clone()]),
		KvRow::from(["uptime".to_string(), format::uptime_exact(info.uptime)]),
		KvRow::from(["restarts".to_string(), info.restarts.to_string()]),
		KvRow::from(["cpu".to_string(), format::percent(info.cpu)]),
		KvRow::from(["memory".to_string(), format::bytes_exact(info.memory)]),
		KvRow::from(["user".to_string(), info.user.clone()]),
		KvRow::from([
			"created at".to_string(),
			format::timestamp(&info.created_at.clone().unwrap_or_default()),
		]),
		KvRow::from(["git".to_string(), git_str(info)]),
		KvRow::from(["watch".to_string(), watch_str(info.watch)]),
		KvRow::from(["disabled".to_string(), bool_dimmed(spec.disabled)]),
	];
	let _ = table::kv(w, "Process", &rows);
}

fn render_exec<W: Write>(w: &mut W, spec: &AppSpec) {
	let mut cmd = spec.exec.command.clone().unwrap_or_default();
	if spec.exec.kind == "entry" {
		cmd = spec.exec.entry.clone().unwrap_or_default();
	}
	let rows = vec![
		KvRow::from(["type".to_string(), spec.exec.kind.clone()]),
		KvRow::from([
			"runtime".to_string(),
			spec.exec.runtime.clone().unwrap_or_default(),
		]),
		KvRow::from(["command".to_string(), cmd]),
		KvRow::from([
			"args".to_string(),
			join_args(spec.exec.args.as_deref().unwrap_or(&[])),
		]),
		KvRow::from(["shell".to_string(), bool_dimmed(spec.exec.shell)]),
		KvRow::from(["cwd".to_string(), spec.cwd.clone().unwrap_or_default()]),
	];
	let _ = table::kv(w, "Exec", &rows);
}

fn render_env<W: Write>(w: &mut W, spec: &AppSpec) {
	if spec.env_file.as_deref().unwrap_or("").is_empty()
		&& spec.env.as_ref().map(|e| e.is_empty()).unwrap_or(true)
	{
		return;
	}
	let mut rows: Vec<KvRow> = Vec::new();
	if let Some(p) = &spec.env_file {
		if !p.is_empty() {
			rows.push(KvRow::from(["env-file".to_string(), p.clone()]));
		}
	}
	if let Some(env) = &spec.env {
		let mut keys: Vec<&String> = env.keys().collect();
		keys.sort();
		for k in keys {
			let v = env.get(k).cloned().unwrap_or_default();
			rows.push(KvRow::from([k.clone(), mask_secret(k, &v)]));
		}
	}
	let _ = table::kv(w, "Environment", &rows);
	let _ = writeln!(w);
}

fn render_logs<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(l) = &spec.logs else {
		return;
	};
	let dir = l.dir.clone().unwrap_or_default();
	let rows = vec![
		KvRow::from(["mode".to_string(), l.mode.clone()]),
		KvRow::from(["dir".to_string(), dir.clone()]),
		KvRow::from([
			"stdout".to_string(),
			join_log_path(&dir, l.stdout.as_deref().unwrap_or("")),
		]),
		KvRow::from([
			"stderr".to_string(),
			join_log_path(&dir, l.stderr.as_deref().unwrap_or("")),
		]),
		KvRow::from(["format".to_string(), l.format.clone().unwrap_or_default()]),
		KvRow::from([
			"timestamp".to_string(),
			l.timestamp.clone().unwrap_or_default(),
		]),
	];
	let _ = table::kv(w, "Logs", &rows);
	let _ = writeln!(w);
}

fn render_restart<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(r) = &spec.restart else {
		return;
	};
	let mut backoff = String::new();
	if r.backoff_type.is_some() || r.backoff_ms.is_some() {
		let kind = r.backoff_type.clone().unwrap_or_else(|| "expo".into());
		let ms = r.backoff_ms.unwrap_or(0);
		backoff = format!("{} ({})", kind, format::uptime(ms as i64));
	}
	let mut stop_on = String::new();
	if let Some(codes) = &r.stop_on_exit {
		let parts: Vec<String> = codes.iter().map(|c| c.to_string()).collect();
		stop_on = parts.join(", ");
	}
	let rows = vec![
		KvRow::from(["policy".to_string(), r.policy.clone()]),
		KvRow::from([
			"maxRetries".to_string(),
			int_or_dash(r.max_retries.unwrap_or(0)),
		]),
		KvRow::from(["backoff".to_string(), backoff]),
		KvRow::from(["stopOnExit".to_string(), stop_on]),
	];
	let _ = table::kv(w, "Restart", &rows);
	let _ = writeln!(w);
}

fn render_stop<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(s) = &spec.stop else {
		return;
	};
	let rows = vec![
		KvRow::from(["signal".to_string(), s.signal.clone().unwrap_or_default()]),
		KvRow::from([
			"timeout".to_string(),
			format::uptime_exact(s.timeout_ms.unwrap_or(0) as i64),
		]),
	];
	let _ = table::kv(w, "Stop", &rows);
	let _ = writeln!(w);
}

fn render_resources<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(r) = &spec.resources else {
		return;
	};
	if r.memory_max_bytes.unwrap_or(0) == 0
		&& r.cpu_max_percent.unwrap_or(0) == 0
		&& r.tasks_max.unwrap_or(0) == 0
	{
		return;
	}
	let rows = vec![
		KvRow::from([
			"memory max".to_string(),
			mem_or_unlimited(r.memory_max_bytes.unwrap_or(0)),
		]),
		KvRow::from([
			"cpu max".to_string(),
			cpu_or_unlimited(r.cpu_max_percent.unwrap_or(0)),
		]),
		KvRow::from([
			"tasks max".to_string(),
			int_or_unlimited(r.tasks_max.unwrap_or(0)),
		]),
	];
	let _ = table::kv(w, "Resources", &rows);
	let _ = writeln!(w);
}

fn render_isolation<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(ra) = &spec.run_as else {
		return;
	};
	if ra.mode.is_empty() {
		return;
	}
	let rows = vec![KvRow::from(["mode".to_string(), ra.mode.clone()])];
	let _ = table::kv(w, "Isolation", &rows);
	let _ = writeln!(w);
}

fn render_schedule<W: Write>(w: &mut W, spec: &AppSpec) {
	if spec.cron.as_deref().unwrap_or("").is_empty() {
		return;
	}
	let rows = vec![KvRow::from([
		"cron".to_string(),
		spec.cron.clone().unwrap_or_default(),
	])];
	let _ = table::kv(w, "Schedule", &rows);
	let _ = writeln!(w);
}

fn render_watch<W: Write>(w: &mut W, spec: &AppSpec) {
	let Some(wa) = &spec.watch else {
		return;
	};
	let mut rows = vec![KvRow::from([
		"enabled".to_string(),
		bool_dimmed(wa.enabled),
	])];
	if let Some(ignore) = &wa.ignore {
		if !ignore.is_empty() {
			rows.push(KvRow::from(["ignore".to_string(), ignore.join(", ")]));
		}
	}
	let _ = table::kv(w, "Watch", &rows);
	let _ = writeln!(w);
}

fn color_state(state: ProcessState) -> String {
	let s = state.as_str();
	match state {
		ProcessState::Running | ProcessState::Online => term::green(format_args!("{}", s)),
		ProcessState::Stopped | ProcessState::Failed => term::red(format_args!("{}", s)),
		ProcessState::Restarting => term::yellow(format_args!("{}", s)),
		ProcessState::Exited => term::yellow(format_args!("{}", s)),
	}
}

fn pid_str(pid: i64) -> String {
	if pid == 0 {
		term::dim(format_args!("{}", "-"))
	} else {
		pid.to_string()
	}
}

fn git_str(info: &ProcessInfo) -> String {
	if info.git_branch.as_deref().unwrap_or("").is_empty() {
		return term::dim(format_args!("{}", "-"));
	}
	let branch = info.git_branch.clone().unwrap_or_default();
	let commit = info.git_commit.clone().unwrap_or_default();
	let s = format!("{}@{}", branch, commit);
	if info.git_dirty {
		term::yellow(format_args!("{}*", s))
	} else {
		s
	}
}

fn watch_str(on: bool) -> String {
	if on {
		term::green(format_args!("{}", "enabled"))
	} else {
		term::dim(format_args!("{}", "disabled"))
	}
}

fn bool_dimmed(v: bool) -> String {
	if v {
		term::green(format_args!("{}", "true"))
	} else {
		term::dim(format_args!("{}", "false"))
	}
}

fn join_args(args: &[String]) -> String {
	if args.is_empty() {
		return String::new();
	}
	let mut quoted: Vec<String> = Vec::with_capacity(args.len());
	for a in args {
		if a.contains(|c: char| [' ', '\t', '"', '\''].contains(&c)) {
			// Quote anything containing whitespace or quotes.
			let mut s = String::with_capacity(a.len() + 2);
			s.push('"');
			for ch in a.chars() {
				if ch == '"' || ch == '\\' {
					s.push('\\');
				}
				s.push(ch);
			}
			s.push('"');
			quoted.push(s);
		} else {
			quoted.push(a.clone());
		}
	}
	quoted.join(" ")
}

fn join_log_path(dir: &str, file: &str) -> String {
	if file.is_empty() {
		return String::new();
	}
	let abs = std::path::Path::new(file).is_absolute();
	if abs || dir.is_empty() {
		return file.to_string();
	}
	let mut p = std::path::PathBuf::from(dir);
	p.push(file);
	p.to_string_lossy().into_owned()
}

fn int_or_dash(v: i32) -> String {
	if v == 0 {
		term::dim(format_args!("{}", "-"))
	} else {
		v.to_string()
	}
}

fn int_or_unlimited(v: i32) -> String {
	if v == 0 {
		term::dim(format_args!("{}", "unlimited"))
	} else {
		v.to_string()
	}
}

fn mem_or_unlimited(b: i64) -> String {
	if b == 0 {
		term::dim(format_args!("{}", "unlimited"))
	} else {
		format::bytes_exact(b)
	}
}

fn cpu_or_unlimited(pct: i32) -> String {
	if pct == 0 {
		term::dim(format_args!("{}", "unlimited"))
	} else {
		format!("{}% ({:.2} cores)", pct, pct as f64 / 100.0)
	}
}

fn non_empty(a: &str, b: &str) -> String {
	if !a.is_empty() {
		a.to_string()
	} else {
		b.to_string()
	}
}

/// Mask values for keys that look sensitive (cosmetic only — the daemon
/// already keeps `--env-file` values off disk; this is shoulder-surfing
/// defence in depth).
fn mask_secret(key: &str, val: &str) -> String {
	if val.is_empty() {
		return String::new();
	}
	let upper = key.to_uppercase();
	for needle in [
		"TOKEN",
		"SECRET",
		"PASSWORD",
		"PASSWD",
		"KEY",
		"CREDENTIAL",
		"PRIVATE",
	] {
		if upper.contains(needle) {
			return term::dim(format_args!("{}", "********"));
		}
	}
	val.to_string()
}

#[cfg(test)]
mod tests;
