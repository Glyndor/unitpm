//! `unitpm show` — print detailed information about a single process.
//!
//! Mirrors `internal/cli/commands/show/cmd.go` (the 426-line Go file).
//! Public entry point: [`run`]. The render helpers split per section so
//! each one is small and focused.

use std::collections::BTreeMap;
use std::io::{self, Write};

use serde::Serialize;

use crate::cli::errs::UsageError;
use crate::cli::format;
use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::root::cmd;
use crate::cli::table::{self, KvRow};
use crate::ipc::protocol::AppSpec;
use crate::ipc::transport::IPCClient;
use crate::jsonx;
use crate::term;
use crate::types::{ProcessInfo, ProcessState};

/// Wire payload the daemon returns for the `show` verb. We carry the
/// spec explicitly instead of deriving `Deserialize` because
/// [`AppSpec`] does not implement `Default` and the daemon may omit the
/// field on older versions.
#[derive(Debug, Clone, Serialize)]
pub struct ShowResponse {
	pub info: ProcessInfo,
	pub spec: AppSpec,
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
			spec: Option<AppSpec>,
		}
		let h = Helper::deserialize(deserializer)?;
		Ok(ShowResponse {
			info: h.info,
			spec: h.spec.unwrap_or_else(empty_spec),
		})
	}
}

/// Parsed command-line options.
#[derive(Debug, Clone, Default)]
pub struct Options {
	pub json: bool,
	pub target: Option<String>,
}

/// Local client trait — mirrors [`IPCClient::call`] with `Box<dyn>`
/// erased types so it stays dyn-compatible. The dispatcher uses the
/// concrete [`crate::ipc::transport::Client`]; tests can substitute
/// any mock that implements this trait. Kept module-private because
/// three commands need the same shape, and the alternative would be
/// to refactor the public [`IPCClient`] trait.
pub trait DynClient {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String>;
}

impl DynClient for crate::ipc::transport::Client {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		let params = BTreeMap::from([("id".to_string(), id.to_string())]);
		self.call("show", Some(&params), Some(resp))
			.map_err(|e| format!("show failed: {e}"))
	}
}

impl DynClient for Box<dyn DynClient> {
	fn call_show(&mut self, id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		(**self).call_show(id, resp)
	}
}

/// Public entry point. `client` may be `None`; in that case the
/// dispatcher has already created a concrete transport client.
pub fn run(
	client: Option<&mut dyn DynClient>,
	args: &[String],
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
	if crate::cli::help::is_help(args) {
		let mut buf = Vec::new();
		render_help(&mut buf)?;
		print!("{}", String::from_utf8_lossy(&buf));
		return Ok(());
	}
	let opts = parse_args(args)?;
	let id = opts
		.target
		.ok_or_else(|| UsageError::new("missing process ID or name"))?;

	let mut owned_client: Option<Box<dyn DynClient>>;
	let client: &mut dyn DynClient = match client {
		Some(c) => c,
		None => {
			let new_client: Box<dyn DynClient> = Box::new(
				crate::ipc::transport::Client::new().map_err(|e| format!("show failed: {e}"))?,
			);
			owned_client = Some(new_client);
			owned_client.as_mut().unwrap()
		}
	};

	let mut resp = ShowResponse {
		info: ProcessInfo {
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
		},
		spec: empty_spec(),
	};
	client.call_show(&id, &mut resp)?;

	if opts.json {
		let bytes = jsonx::marshal(&resp).map_err(jsonx_to_io)?;
		let stdout = io::stdout();
		let mut out = stdout.lock();
		writeln!(out, "{}", String::from_utf8_lossy(&bytes))?;
		return Ok(());
	}

	render_to(&mut io::stdout().lock(), &resp);
	Ok(())
}

fn empty_spec() -> AppSpec {
	AppSpec {
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

fn jsonx_to_io(e: jsonx::Error) -> io::Error {
	io::Error::new(io::ErrorKind::InvalidData, e)
}

/// Parse the command's flag arguments. Mirrors the Go hand-rolled
/// parser: `--json` flips JSON output; the first non-flag argument is
/// the target.
pub fn parse_args(args: &[String]) -> Result<Options, Box<dyn std::error::Error + Send + Sync>> {
	let mut opts = Options::default();
	let mut positional: Vec<String> = Vec::new();
	for a in args {
		match a.as_str() {
			"--json" => opts.json = true,
			_ => positional.push(a.clone()),
		}
	}
	opts.target = positional.into_iter().next();
	Ok(opts)
}

/// Render the full show page to stdout. Convenience wrapper around
/// [`render_to`] for the production path; tests use [`render_to`]
/// with their own writer.
pub fn render(resp: &ShowResponse) {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	render_to(&mut out, resp);
}

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

/// `CommandSpec` returned to the dispatcher at registration time.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: cmd::SHOW.to_string(),
		aliases: vec!["info".to_string(), "describe".to_string()],
		usage: format!(
			"unitpm {}|info|describe <id|name|namespace:name> [--json]",
			cmd::SHOW
		),
		description: "Show detailed information about a process.".to_string(),
		options: vec![
			HelpOption {
				short: "-h".into(),
				long: "--help".into(),
				description: "Show this help message.".into(),
			},
			HelpOption {
				short: "".into(),
				long: "--json".into(),
				description: "Emit the raw daemon response as JSON on stdout.".into(),
			},
		],
		examples: vec![
			format!("unitpm {} my-api", cmd::SHOW),
			format!("unitpm info prod:my-api"),
			format!("unitpm describe 019d9a04"),
			format!("unitpm {} my-api --json | jq '.spec.env'", cmd::SHOW),
		],
		hidden: false,
	}
}

/// Print the command help block to `w`.
pub fn render_help<W: Write>(w: &mut W) -> io::Result<()> {
	crate::cli::help::render_command_help(w, &spec())
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::ipc::protocol::{AppLogs, AppResources, AppRestart, AppStop, AppWatch, RunAsPolicy};

	fn spec_with_logs() -> AppSpec {
		AppSpec {
			logs: Some(Box::new(AppLogs {
				mode: "file".into(),
				dir: Some("/var/log".into()),
				stdout: Some("out.log".into()),
				stderr: Some("err.log".into()),
				format: None,
				timestamp: None,
			})),
			..empty_spec()
		}
	}

	#[test]
	fn parse_args_missing_target() {
		let opts = parse_args(&[]).unwrap();
		assert!(opts.target.is_none());
	}

	#[test]
	fn parse_args_target_set() {
		let opts = parse_args(&["abc-123".into()]).unwrap();
		assert_eq!(opts.target.as_deref(), Some("abc-123"));
	}

	#[test]
	fn parse_args_json_flag() {
		let opts = parse_args(&["--json".into(), "abc".into()]).unwrap();
		assert!(opts.json);
		assert_eq!(opts.target.as_deref(), Some("abc"));
	}

	#[test]
	fn spec_name_and_aliases() {
		let s = spec();
		assert_eq!(s.name, "show");
		assert!(s.aliases.contains(&"info".to_string()));
		assert!(s.aliases.contains(&"describe".to_string()));
	}

	#[test]
	fn help_renders_without_panic() {
		let mut buf = Vec::new();
		render_help(&mut buf).unwrap();
		let plain = format::strip_ansi(&String::from_utf8(buf).unwrap());
		assert!(plain.contains("Usage:"));
		assert!(plain.contains("--json"));
	}

	#[test]
	fn color_state_categorises() {
		assert!(color_state(ProcessState::Running).contains("running"));
		assert!(color_state(ProcessState::Online).contains("online"));
		assert!(color_state(ProcessState::Stopped).contains("stopped"));
		assert!(color_state(ProcessState::Failed).contains("failed"));
		assert!(color_state(ProcessState::Restarting).contains("restarting"));
		assert!(color_state(ProcessState::Exited).contains("exited"));
	}

	#[test]
	fn pid_str_formats() {
		assert!(pid_str(0).contains("-"));
		assert_eq!(pid_str(42), "42");
	}

	#[test]
	fn git_str_branch_commit_dirty() {
		let plain = git_str(&ProcessInfo {
			git_branch: Some("main".into()),
			git_commit: Some("abc".into()),
			..empty_info()
		});
		assert!(plain.contains("main"));
		assert!(plain.contains("abc"));

		let dirty = git_str(&ProcessInfo {
			git_branch: Some("main".into()),
			git_commit: Some("abc".into()),
			git_dirty: true,
			..empty_info()
		});
		assert!(dirty.contains('*'));

		let empty = git_str(&empty_info());
		assert!(empty.contains('-'));
	}

	#[test]
	fn watch_str_formats() {
		assert!(watch_str(true).contains("enabled"));
		assert!(watch_str(false).contains("disabled"));
	}

	#[test]
	fn bool_dimmed_formats() {
		assert!(bool_dimmed(true).contains("true"));
		assert!(bool_dimmed(false).contains("false"));
	}

	#[test]
	fn join_args_quotes_whitespace() {
		assert_eq!(join_args(&[]), "");
		assert_eq!(join_args(&["a".into(), "b".into()]), "a b");
		assert_eq!(join_args(&["a b".into(), "c".into()]), "\"a b\" c");
	}

	#[test]
	fn join_log_path_resolves_relative() {
		assert_eq!(join_log_path("", ""), "");
		assert_eq!(join_log_path("/var/log", ""), "");
		assert_eq!(join_log_path("", "stdout.log"), "stdout.log");
		assert_eq!(join_log_path("/var/log", "/etc/abs.log"), "/etc/abs.log");
		assert_eq!(
			join_log_path("/var/log", "stdout.log"),
			"/var/log/stdout.log"
		);
	}

	#[test]
	fn int_or_dash_formats() {
		assert!(int_or_dash(0).contains('-'));
		assert_eq!(int_or_dash(5), "5");
	}

	#[test]
	fn int_or_unlimited_formats() {
		assert!(int_or_unlimited(0).contains("unlimited"));
		assert_eq!(int_or_unlimited(7), "7");
	}

	#[test]
	fn mem_or_unlimited_formats() {
		assert!(mem_or_unlimited(0).contains("unlimited"));
		let s = mem_or_unlimited(2 * 1024 * 1024);
		assert!(!s.is_empty());
	}

	#[test]
	fn cpu_or_unlimited_formats() {
		assert!(cpu_or_unlimited(0).contains("unlimited"));
		assert!(cpu_or_unlimited(150).contains("150%"));
	}

	#[test]
	fn non_empty_picks_first() {
		assert_eq!(non_empty("", "b"), "b");
		assert_eq!(non_empty("a", "b"), "a");
	}

	#[test]
	fn mask_secret_hides_sensitive() {
		assert!(mask_secret("API_TOKEN", "abc").contains('*'));
		assert_eq!(mask_secret("PORT", ""), "");
		assert_eq!(mask_secret("PORT", "8080"), "8080");
		for k in ["PASSWORD", "PASSWD", "MY_KEY", "CREDENTIALS", "PRIVATE_KEY"] {
			assert!(mask_secret(k, "v").contains('*'));
		}
	}

	#[test]
	fn render_restart_full_and_nil() {
		let spec = AppSpec {
			restart: Some(Box::new(AppRestart {
				policy: "always".into(),
				max_retries: Some(3),
				backoff_ms: Some(1000),
				backoff_type: Some("expo".into()),
				stop_on_exit: Some(vec![0, 2]),
			})),
			..empty_spec()
		};
		let mut buf = Vec::new();
		render_restart(&mut buf, &spec);
		render_restart(&mut buf, &empty_spec());
	}

	#[test]
	fn render_env_full_and_nil() {
		let mut env = BTreeMap::new();
		env.insert("FOO".into(), "bar".into());
		env.insert("API_TOKEN".into(), "xyz".into());
		let mut spec = empty_spec();
		spec.env_file = Some("/tmp/env".into());
		spec.env = Some(env);
		let mut buf = Vec::new();
		render_env(&mut buf, &spec);
		render_env(&mut buf, &empty_spec());
	}

	#[test]
	fn render_logs_present_and_absent() {
		let mut buf = Vec::new();
		render_logs(&mut buf, &spec_with_logs());
		render_logs(&mut buf, &empty_spec());
	}

	#[test]
	fn render_resources_present_and_absent() {
		let mut buf = Vec::new();
		render_resources(
			&mut buf,
			&AppSpec {
				resources: Some(Box::new(AppResources {
					memory_max_bytes: Some(512 * 1024 * 1024),
					cpu_max_percent: Some(200),
					tasks_max: Some(100),
				})),
				..empty_spec()
			},
		);
		render_resources(
			&mut buf,
			&AppSpec {
				resources: Some(Box::new(AppResources {
					memory_max_bytes: None,
					cpu_max_percent: None,
					tasks_max: None,
				})),
				..empty_spec()
			},
		);
		render_resources(&mut buf, &empty_spec());
	}

	#[test]
	fn render_stop_present_and_absent() {
		let mut buf = Vec::new();
		render_stop(
			&mut buf,
			&AppSpec {
				stop: Some(Box::new(AppStop {
					signal: Some("SIGTERM".into()),
					timeout_ms: Some(1000),
				})),
				..empty_spec()
			},
		);
		render_stop(&mut buf, &empty_spec());
	}

	#[test]
	fn render_isolation_present_and_absent() {
		let mut buf = Vec::new();
		render_isolation(
			&mut buf,
			&AppSpec {
				run_as: Some(Box::new(RunAsPolicy {
					mode: "self".into(),
				})),
				..empty_spec()
			},
		);
		render_isolation(&mut buf, &empty_spec());
	}

	#[test]
	fn render_schedule_present_and_absent() {
		let mut buf = Vec::new();
		render_schedule(
			&mut buf,
			&AppSpec {
				cron: Some("* * * * *".into()),
				..empty_spec()
			},
		);
		render_schedule(&mut buf, &empty_spec());
	}

	#[test]
	fn render_watch_present_and_absent() {
		let mut buf = Vec::new();
		render_watch(
			&mut buf,
			&AppSpec {
				watch: Some(Box::new(AppWatch {
					enabled: true,
					ignore: Some(vec!["node_modules".into()]),
				})),
				..empty_spec()
			},
		);
		render_watch(
			&mut buf,
			&AppSpec {
				watch: Some(Box::new(AppWatch {
					enabled: false,
					ignore: Some(vec![]),
				})),
				..empty_spec()
			},
		);
		render_watch(&mut buf, &empty_spec());
	}

	#[test]
	fn render_exec_uses_entry() {
		let mut spec = empty_spec();
		spec.exec.kind = "entry".into();
		spec.exec.entry = Some("npm:start".into());
		let mut buf = Vec::new();
		render_exec(&mut buf, &spec);
	}

	#[test]
	fn render_process_uses_info_namespace_fallback() {
		let mut info = empty_info();
		info.namespace = String::new();
		let mut spec = empty_spec();
		spec.namespace = Some("prod".into());
		let mut buf = Vec::new();
		render_process(&mut buf, &info, &spec);
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
}
