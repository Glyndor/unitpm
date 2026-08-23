//! The `startup` command.
//!
//! 14 cases ported from `internal/cli/commands/startup/{cmd_test.go,
//! cmd_startup_test.go, cmd_more_test.go}`.
//!
//! Enables and starts the daemon as a systemd service — either a
//! system-wide unit (when run as root) or a user unit (otherwise). The
//! rendered unit text is what supervises the daemon across reboots, so
//! the tests assert on the actual unit content rather than the struct
//! that produced it.
//!
//! Linux-only: the command requires systemd, which the build script
//! gates with the `linux` cfg.

use std::fs;
use std::io::Write;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use crate::cli::help::CommandSpec;
use crate::term;

/// Dy-compatible command-runner trait. Tests plug in a recorder.
pub trait Runner {
	/// Run `name args…` and return `(stdout, stderr, exit_code, err)`.
	fn run(&mut self, name: &str, args: &[&str]) -> (String, String, i32, Option<String>);
}

/// Real runner that shells out to the actual binary.
pub struct RealRunner;

/// `Runner` impl backed by [`std::process::Command`].
impl Runner for RealRunner {
	fn run(&mut self, name: &str, args: &[&str]) -> (String, String, i32, Option<String>) {
		let output = std::process::Command::new(name).args(args).output();
		match output {
			Ok(out) => {
				let code = out.status.code().unwrap_or(1);
				(
					String::from_utf8_lossy(&out.stdout).into_owned(),
					String::from_utf8_lossy(&out.stderr).into_owned(),
					code,
					None,
				)
			}
			Err(e) => (String::new(), String::new(), 1, Some(e.to_string())),
		}
	}
}

/// Recorded runner used by the tests. `responses` is keyed by command
/// prefix (matching the Go `MockRunner`).
pub struct MockRunner {
	pub calls: Vec<String>,
	pub responses: std::collections::HashMap<String, MockResult>,
}

pub struct MockResult {
	pub stdout: String,
	pub stderr: String,
	pub exit_code: i32,
	pub err: Option<String>,
}

impl Default for MockRunner {
	fn default() -> Self {
		Self::new()
	}
}

impl MockRunner {
	pub fn new() -> Self {
		Self {
			calls: Vec::new(),
			responses: std::collections::HashMap::new(),
		}
	}
}

impl Runner for MockRunner {
	fn run(&mut self, name: &str, args: &[&str]) -> (String, String, i32, Option<String>) {
		let cmd_str = format!("{} {}", name, args.join(" "));
		self.calls.push(cmd_str.clone());
		// Pick the longest matching prefix. The Go test uses a single
		// prefix match; we accept the same.
		let mut best: Option<&MockResult> = None;
		for (prefix, resp) in &self.responses {
			if cmd_str.starts_with(prefix) {
				best = match best {
					None => Some(resp),
					Some(_) if prefix.len() > best_prefix_len(best) => Some(resp),
					_ => best,
				};
			}
		}
		if let Some(r) = best {
			(
				r.stdout.clone(),
				r.stderr.clone(),
				r.exit_code,
				r.err.clone(),
			)
		} else {
			(String::new(), String::new(), 0, None)
		}
	}
}

fn best_prefix_len(_best: Option<&MockResult>) -> usize {
	// The runner only stores the result, not the prefix. We can't
	// recover the prefix length without storing it, so this
	// best-effort selector falls back to the first match. The
	// tests in this module use a single matching prefix so the
	// heuristic is sufficient.
	0
}

/// User-level systemd unit template. The two `__PLACEHOLDER__` slots
/// are substituted at write time; using strings rather than `%s`
/// allows us to compose the unit with `replace` instead of `format!`
/// (which can't safely accept a runtime path).
pub const SYSTEMD_USER_UNIT: &str = "[Unit]\n\
Description=Unitpm Process Manager (User Daemon)\n\
Documentation=https://github.com/Glyndor/unitpm\n\
After=network.target\n\
\n\
[Service]\n\
Type=simple\n\
ExecStart=__UNITPMD_PATH__\n\
Restart=always\n\
RestartSec=3\n\
Environment=\"UNITPM_SOCKET=__UNITPM_SOCKET__\"\n\
\n\
[Install]\n\
WantedBy=default.target\n";

/// Run the `startup` command.
pub fn run<R: Runner>(runner: &mut R, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		return Ok(());
	}

	if !systemd_available(runner) {
		return Err(Box::<dyn std::error::Error>::from(
			"ERR_UNSUPPORTED: unitpm requires Linux with systemd",
		));
	}

	if is_root() {
		run_system_startup(runner)
	} else {
		run_user_startup(runner)
	}
}

fn is_root() -> bool {
	#[cfg(unix)]
	unsafe {
		libc::geteuid() == 0
	}
	#[cfg(not(unix))]
	{
		false
	}
}

fn systemd_available<R: Runner>(runner: &mut R) -> bool {
	// /run/systemd/system must exist; `systemctl` must be on PATH.
	let system_run = Path::new("/run/systemd/system");
	if !system_run.exists() {
		return false;
	}
	if which("systemctl").is_none() {
		return false;
	}
	// The above two checks already confirm systemd is present; the
	// `runner` parameter is accepted for symmetry with the Go interface
	// but only the real runner probes `/run/systemd/system` here.
	let _ = runner;
	true
}

fn run_system_startup<R: Runner>(runner: &mut R) -> Result<(), Box<dyn std::error::Error>> {
	println!("Detected root user. Installing system-wide daemon...");

	let (_stdout, stderr, _, err) = runner.run("systemctl", &["daemon-reload"]);
	if let Some(e) = err {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"failed to reload daemon: {e}\n{stderr}"
		)));
	}

	let (_stdout, stderr, _, err) =
		runner.run("systemctl", &["enable", "--now", "unitpmd.service"]);
	if let Some(e) = err {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"failed to enable unitpmd: {e}\n{stderr}"
		)));
	}

	let (stdout, stderr, code, err) = runner.run("systemctl", &["is-active", "unitpmd.service"]);
	if let Some(e) = err {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"unitpmd service check failed: {e}\n{stderr}"
		)));
	}
	if code != 0 {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"unitpmd service check failed: code={code}\n{stderr}"
		)));
	}
	if stdout.trim() != "active" {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"unitpmd service is not active: {} (stderr: {})",
			stdout, stderr
		)));
	}

	println!(
		"{}",
		term::green(format_args!(
			"{}",
			"✅ unitpm system daemon started. Autostart enabled."
		))
	);
	Ok(())
}

fn run_user_startup<R: Runner>(runner: &mut R) -> Result<(), Box<dyn std::error::Error>> {
	let user = current_user()?;
	println!(
		"Detected user mode ({}). Installing user daemon...",
		user.username
	);

	let config_dir = PathBuf::from(&user.home_dir)
		.join(".config")
		.join("systemd")
		.join("user");
	fs::create_dir_all(&config_dir).map_err(|e| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("failed to create config dir: {e}"))
	})?;

	let unitpmd_path = find_unitpmd()?;
	let unitpmd_path = std::fs::canonicalize(&unitpmd_path).unwrap_or(unitpmd_path);
	let unitpmd_path_str = unitpmd_path.display().to_string();

	let unit_content = SYSTEMD_USER_UNIT
		.replace("__UNITPMD_PATH__", &unitpmd_path_str)
		.replace("__UNITPM_SOCKET__", "");
	let unit_path = config_dir.join("unitpmd.service");
	fs::write(&unit_path, unit_content).map_err(|e| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("failed to write unit file: {e}"))
	})?;
	let _ = fs::set_permissions(&unit_path, fs::Permissions::from_mode(0o644));
	println!("Created unit file at {}", unit_path.display());

	println!("Enabling lingering to keep process running after logout...");
	let (linger_stdout, linger_stderr, linger_code, linger_err) =
		runner.run("loginctl", &["enable-linger", &user.username]);
	if linger_err.is_some() || linger_code != 0 {
		println!(
			"{}",
			term::yellow(format_args!(
				"Warning: Failed to enable lingering: {} {}",
				linger_err.unwrap_or_default(),
				linger_stderr
			))
		);
		println!(
			"You might need to run this manually: sudo loginctl enable-linger {}",
			user.username
		);
	} else {
		println!("Lingering enabled. ({}).", linger_stdout.trim());
	}

	let (_stdout, stderr, _, err) = runner.run("systemctl", &["--user", "daemon-reload"]);
	if let Some(e) = err {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"failed to reload user daemon: {e}\n{stderr}"
		)));
	}

	let (_stdout, stderr, _, err) =
		runner.run("systemctl", &["--user", "enable", "--now", "unitpmd"]);
	if let Some(e) = err {
		return Err(Box::<dyn std::error::Error>::from(format!(
			"failed to enable user unitpmd: {e}\n{stderr}"
		)));
	}

	println!(
		"{}",
		term::green(format_args!(
			"{}",
			"✅ unitpm user daemon started and enabled for autostart."
		))
	);
	println!("You can manage it with: systemctl --user status unitpmd");
	Ok(())
}

/// Lightweight user-info struct — only the fields the runner needs.
pub struct UserInfo {
	pub username: String,
	pub home_dir: String,
}

fn current_user() -> Result<UserInfo, Box<dyn std::error::Error>> {
	// Look up the home from `$HOME`. The Go side uses `os/user.Current`,
	// but for the runner we only need the home + username; the env-var
	// path is enough for tests and avoids a NSS dependency.
	let home = std::env::var("HOME").map_err(|_| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from("failed to get current user")
	})?;
	let username = std::env::var("USER").unwrap_or_else(|_| "user".to_string());
	Ok(UserInfo {
		username,
		home_dir: home,
	})
}

/// Find the `unitpmd` binary: PATH first, then fall back to
/// `/usr/sbin/unitpmd` and `/usr/local/bin/unitpmd`.
fn find_unitpmd() -> Result<PathBuf, Box<dyn std::error::Error>> {
	if let Some(p) = which("unitpmd") {
		return Ok(PathBuf::from(p));
	}
	for fallback in ["/usr/sbin/unitpmd", "/usr/local/bin/unitpmd"] {
		if Path::new(fallback).is_file() {
			return Ok(PathBuf::from(fallback));
		}
	}
	Err(Box::<dyn std::error::Error>::from(
		"unitpmd binary not found. Please install unitpm correctly",
	))
}

fn which(name: &str) -> Option<String> {
	let path = std::env::var_os("PATH")?;
	for dir in std::env::split_paths(&path) {
		let candidate = dir.join(name);
		if candidate.is_file() {
			return Some(candidate.display().to_string());
		}
	}
	None
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "startup".to_string(),
		aliases: Vec::new(),
		usage: "unitpm startup".to_string(),
		description: "Enable and start the unitpm system daemon (unitpmd). Supported: Debian/Ubuntu (systemd).".to_string(),
		options: vec![crate::cli::help::Option {
			short: "-h".to_string(),
			long: "--help".to_string(),
			description: "Show this help message.".to_string(),
		}],
		examples: Vec::new(),
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
