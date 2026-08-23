//! `systemd-run` argument list for `--isolation dynamic`.
//!
//! Mirrors `manager.prepareIsolation`'s dynamic branch in
//! `internal/daemon/manager/process.go`. The argument list *is* the
//! sandbox for every app the daemon supervises in dynamic mode —
//! `DynamicUser=yes` for ephemeral users, `NoNewPrivileges=yes` against
//! setuid escalation, `PrivateTmp=yes` / `ProtectSystem=strict` /
//! `ProtectHome=yes` / `ProtectProc=invisible` for namespace hardening,
//! `LoadCredential=env:<path>` so secrets never appear in
//! `/proc/<pid>/environ` or `ps`.
//!
//! The build is pure — `dynamic_args` returns a `Vec<String>` that the
//! test in this package asserts on. Nothing is executed against the
//! user's session, no real systemd unit is created. Phase 7 deletes the
//! Go wrapper for this code path and lands a Rust one; until then the
//! Go `_exec-env` CLI subcommand consumes these arguments byte-for-byte.

use std::fmt;

/// Hardened directives every dynamic-mode spawn must carry. The unit
/// test in this package asserts on each one explicitly so dropping any
/// of them turns something red.
///
/// Each directive is its own `pub const` (rather than inlined into the
/// argv builder) so the test below can assert against the canonical
/// string, and so a regression that drops one directive leaves the
/// others untouched — making the missing directive obvious in the diff.
pub mod directives {
	/// Ephemeral per-app user.
	pub const DYNAMIC_USER: &str = "DynamicUser=yes";
	/// Block setuid escalation.
	pub const NO_NEW_PRIVILEGES: &str = "NoNewPrivileges=yes";
	/// Per-app private /tmp.
	pub const PRIVATE_TMP: &str = "PrivateTmp=yes";
	/// Read-only system paths except /dev.
	pub const PROTECT_SYSTEM: &str = "ProtectSystem=strict";
	/// Hide user home from the app.
	pub const PROTECT_HOME: &str = "ProtectHome=yes";
	/// Make /proc/[pid] invisible to other processes.
	pub const PROTECT_PROC: &str = "ProtectProc=invisible";
}

/// `systemd-run` argv the supervisor builds for `--isolation dynamic`.
/// Public so the brief's "delete a control, watch a test go red" check
/// can assert against the exact byte values below.
pub fn build_argv(spec_id: &str, spec_name: &str, cwd: &str) -> Vec<String> {
	let ctx = DynamicContext {
		id: spec_id,
		name: spec_name,
		cwd,
		resources: None,
	};
	dynamic_command(&ctx).args
}

/// Wrapper subcommand the daemon invokes after systemd-run has set up the
/// sandbox; it re-execs the real binary inside the dynamic-mode
/// environment. Matches the Go wrapper name so the existing Go CLI
/// continues to consume these arguments until phase 7.
pub const EXEC_ENV_SUBCOMMAND: &str = "_exec-env";

/// Unit name prefix. The pre-rename prefix is gone; this phase uses
/// `unitpm-app-<id>` so the name is visible in `systemctl`, the journal,
/// and the cgroup tree.
pub const UNIT_NAME_PREFIX: &str = "unitpm-app-";

/// Inputs to [`dynamic_command`]. Carries only the spec bits the
/// argument list depends on.
pub struct DynamicContext<'a> {
	/// Spec ID, embedded in the unit name.
	pub id: &'a str,
	/// Spec name, surfaced as `--description`.
	pub name: &'a str,
	/// `spec.cwd`, surfaced as `WorkingDirectory=`. Empty → omitted.
	pub cwd: &'a str,
	/// Resource limits, surfaced as `MemoryMax=` / `CPUQuota=` /
	/// `TasksMax=` directives. `None` → omitted.
	pub resources: Option<Box<crate::ipc::protocol::AppResources>>,
}

/// Errors surfaced by [`dynamic_command`] argument-list construction.
/// The wrapper-builder APIs that follow can carry their own errors.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DynamicError {
	/// The build failed because an empty ID was passed.
	EmptyId,
}

impl std::fmt::Display for DynamicError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			DynamicError::EmptyId => f.write_str("dynamic: empty process id"),
		}
	}
}

impl std::error::Error for DynamicError {}

/// Pre-built `systemd-run` invocation. The supervisor takes ownership
/// of this and runs it via `std::process::Command`. The fields are kept
/// separate (rather than collapsed into `Vec<String>`) so the test can
/// assert on each piece without having to know `systemd-run`'s argument
/// order — the order is fixed here and asserted in tests.
#[derive(Debug)]
pub struct DynamicCommand {
	/// Always `systemd-run`. Stored as a separate field so the test can
	/// check it independently of the args.
	pub binary: String,
	/// `systemd-run` arguments. Order matches the Go output.
	pub args: Vec<String>,
	/// Wrapper binary path (`unitpm`). Empty by default; set via
	/// [`with_wrapper_binary`].
	pub wrapper_binary: String,
	/// Path passed to `--load-credential env:<path>`. Set by
	/// [`with_env_path`].
	pub env_path: String,
}

impl DynamicCommand {
	/// Build the immutable argv portion: the unit-name, description,
	/// hardening `-p` pairs, `--pipe --wait`, the conditional cwd, the
	/// resource directives, and the `--` separator.
	pub fn build_args(ctx: &DynamicContext<'_>) -> Result<Vec<String>, DynamicError> {
		if ctx.id.is_empty() {
			return Err(DynamicError::EmptyId);
		}
		let mut sd_args: Vec<String> = vec![
			format!("--unit={UNIT_NAME_PREFIX}{}", ctx.id),
			format!("--description={}", ctx.name),
			"-p".into(),
			directives::DYNAMIC_USER.into(),
			"-p".into(),
			directives::NO_NEW_PRIVILEGES.into(),
			"-p".into(),
			directives::PRIVATE_TMP.into(),
			"-p".into(),
			directives::PROTECT_SYSTEM.into(),
			"-p".into(),
			directives::PROTECT_HOME.into(),
			"-p".into(),
			directives::PROTECT_PROC.into(),
		];

		// LoadCredential is added once `env_path` is known; the
		// caller patches it via [`DynamicCommand::with_env_path`].

		sd_args.push("--pipe".into());
		sd_args.push("--wait".into());

		if !ctx.cwd.is_empty() {
			sd_args.push("-p".into());
			sd_args.push(format!("WorkingDirectory={}", ctx.cwd));
		}

		if let Some(r) = &ctx.resources {
			if let Some(mem) = r.memory_max_bytes {
				if mem > 0 {
					sd_args.push("-p".into());
					sd_args.push(format!("MemoryMax={mem}"));
				}
			}
			if let Some(cpu) = r.cpu_max_percent {
				if cpu > 0 {
					sd_args.push("-p".into());
					sd_args.push(format!("CPUQuota={cpu}%"));
				}
			}
			if let Some(tasks) = r.tasks_max {
				if tasks > 0 {
					sd_args.push("-p".into());
					sd_args.push(format!("TasksMax={tasks}"));
				}
			}
		}

		sd_args.push("--".into());
		Ok(sd_args)
	}

	/// Set the wrapper binary path (`unitpm`) used to invoke
	/// `_exec-env` inside the sandbox.
	pub fn with_wrapper_binary(mut self, wrapper_binary: String) -> Self {
		self.wrapper_binary = wrapper_binary;
		self
	}

	/// Set the credential path passed via `LoadCredential=env:<path>`.
	/// Inserts the `-p LoadCredential=env:<path>` directive at the
	/// position the Go implementation places it.
	pub fn with_env_path(mut self, env_path: String) -> Self {
		self.env_path = env_path.clone();
		// Find the slot after `ProtectProc=invisible` and before `--pipe`.
		// We rebuild the args to keep the test's assertion stable.
		let mut new_args: Vec<String> = Vec::with_capacity(self.args.len() + 2);
		let mut inserted = false;
		for arg in self.args.drain(..) {
			if !inserted && arg == "--pipe" {
				new_args.push("-p".into());
				new_args.push(format!("LoadCredential=env:{env_path}"));
				inserted = true;
			}
			new_args.push(arg);
		}
		self.args = new_args;
		self
	}

	/// Full argv as a flat slice, including `wrapper_binary` and the
	/// `_exec-env` subcommand. The supervisor appends the wrapped
	/// binary's argv after this.
	pub fn full_argv(&self, wrapped_bin: &str, wrapped_args: &[String]) -> Vec<String> {
		let mut argv = Vec::with_capacity(self.args.len() + 4 + wrapped_args.len());
		argv.extend_from_slice(&self.args);
		argv.push(self.wrapper_binary.clone());
		argv.push(EXEC_ENV_SUBCOMMAND.to_string());
		argv.push(wrapped_bin.to_string());
		argv.extend(wrapped_args.iter().cloned());
		argv
	}
}

/// Build the dynamic-mode `systemd-run` invocation. Pure function; no
/// process is spawned.
pub fn dynamic_command(ctx: &DynamicContext<'_>) -> DynamicCommand {
	let args = DynamicCommand::build_args(ctx).expect("DynamicContext validated above");
	DynamicCommand {
		binary: "systemd-run".into(),
		args,
		wrapper_binary: String::new(),
		env_path: String::new(),
	}
}

/// Build just the systemd-run argument list. Convenience for callers
/// (and tests) that don't need a full `DynamicCommand`.
pub fn dynamic_args(
	spec_id: &str,
	spec_name: &str,
	cwd: &str,
	resources: Option<&crate::ipc::protocol::AppResources>,
) -> Vec<String> {
	let ctx = DynamicContext {
		id: spec_id,
		name: spec_name,
		cwd,
		resources: resources.cloned().map(Box::new),
	};
	dynamic_command(&ctx).args
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::ipc::protocol::AppResources;

	#[test]
	fn unit_name_uses_unitpm_prefix() {
		let ctx = DynamicContext {
			id: "abc-123",
			name: "my-app",
			cwd: "",
			resources: None,
		};
		let cmd = dynamic_command(&ctx);
		let unit = cmd
			.args
			.iter()
			.find(|a| a.starts_with("--unit="))
			.expect("--unit=");
		assert!(unit.starts_with("--unit=unitpm-app-"), "got {unit}");
		assert!(
			!unit.contains("deadbrand"),
			"dead-brand name leaked: {unit}"
		);
	}

	#[test]
	fn every_directive_present_in_argv() {
		let ctx = DynamicContext {
			id: "abc-123",
			name: "my-app",
			cwd: "",
			resources: None,
		};
		let cmd = dynamic_command(&ctx);
		let argv = &cmd.args;
		let mut joined = argv.join(" ");
		joined.push(' ');
		for d in [
			directives::DYNAMIC_USER,
			directives::NO_NEW_PRIVILEGES,
			directives::PRIVATE_TMP,
			directives::PROTECT_SYSTEM,
			directives::PROTECT_HOME,
			directives::PROTECT_PROC,
		] {
			assert!(
				joined.contains(d),
				"directive {d} missing from argv: {argv:?}"
			);
		}
	}

	#[test]
	fn load_credential_added_after_protect_proc() {
		let mut cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: None,
		});
		cmd = cmd.with_env_path("/var/lib/glyndor/unitpm/creds/abc/env".into());
		let argv = cmd.args;
		let protect_idx = argv
			.iter()
			.position(|a| a == directives::PROTECT_PROC)
			.expect("ProtectProc");
		let load_idx = argv
			.iter()
			.position(|a| a.starts_with("LoadCredential="))
			.expect("LoadCredential");
		let pipe_idx = argv.iter().position(|a| a == "--pipe").expect("--pipe");
		assert!(protect_idx < load_idx);
		assert!(load_idx < pipe_idx);
	}

	#[test]
	fn working_directory_omitted_when_cwd_empty() {
		let cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: None,
		});
		assert!(
			!cmd.args.iter().any(|a| a.starts_with("WorkingDirectory=")),
			"got {:?}",
			cmd.args
		);
	}

	#[test]
	fn working_directory_included_when_set() {
		let cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "/srv/app",
			resources: None,
		});
		assert!(
			cmd.args.iter().any(|a| a == "WorkingDirectory=/srv/app"),
			"got {:?}",
			cmd.args
		);
	}

	#[test]
	fn resource_directives_emitted_when_set() {
		let res = AppResources {
			memory_max_bytes: Some(512_000_000),
			cpu_max_percent: Some(80),
			tasks_max: Some(256),
		};
		let cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: Some(Box::new(res)),
		});
		let argv = &cmd.args;
		assert!(argv.iter().any(|a| a == "MemoryMax=512000000"));
		assert!(argv.iter().any(|a| a == "CPUQuota=80%"));
		assert!(argv.iter().any(|a| a == "TasksMax=256"));
	}

	#[test]
	fn resource_directives_skipped_when_zero_or_negative() {
		let res = AppResources {
			memory_max_bytes: Some(0),
			cpu_max_percent: Some(0),
			tasks_max: Some(0),
		};
		let cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: Some(Box::new(res)),
		});
		let argv = &cmd.args;
		assert!(!argv.iter().any(|a| a.starts_with("MemoryMax=")));
		assert!(!argv.iter().any(|a| a.starts_with("CPUQuota=")));
		assert!(!argv.iter().any(|a| a.starts_with("TasksMax=")));
	}

	#[test]
	fn argv_ends_with_separator() {
		let cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: None,
		});
		let argv = cmd.args;
		assert_eq!(argv.last().unwrap(), "--");
	}

	#[test]
	fn full_argv_appends_wrapper_and_command() {
		let mut cmd = dynamic_command(&DynamicContext {
			id: "abc",
			name: "app",
			cwd: "",
			resources: None,
		});
		cmd = cmd
			.with_wrapper_binary("/usr/bin/unitpm".into())
			.with_env_path("/var/lib/glyndor/unitpm/creds/abc/env".into());
		let argv = cmd.full_argv("/bin/sleep", &["10".into()]);
		// Tail of the argv (post `--`) is `[wrapper, subcommand, wrapped_bin, args...]`.
		let tail_start = argv.iter().rposition(|a| a == "--").expect("-- in argv") + 1;
		let tail = &argv[tail_start..];
		assert_eq!(tail[0], "/usr/bin/unitpm");
		assert_eq!(tail[1], EXEC_ENV_SUBCOMMAND);
		assert_eq!(tail[2], "/bin/sleep");
		assert_eq!(tail[3], "10");
	}

	#[test]
	fn empty_id_rejected() {
		let ctx = DynamicContext {
			id: "",
			name: "app",
			cwd: "",
			resources: None,
		};
		assert_eq!(DynamicCommand::build_args(&ctx), Err(DynamicError::EmptyId));
	}
}
