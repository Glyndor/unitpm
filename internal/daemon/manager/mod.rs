//! Process manager — registry, lifecycle, isolation, rotation.
//!
//! Phase 4b of the Go -> Rust rewrite. Each file under this directory owns
//! one responsibility, mirroring the four-way split that the brief asks for
//! out of the original 1108-line `process.go`:
//!
//! - [`spawn`] — build the exec command, env, log files, and isolation
//!   wrapper from an [`AppSpec`](crate::ipc::protocol::AppSpec).
//! - [`supervise`] — `Start` / `Restart` / `autoRestart`, the monitor
//!   goroutine, the auto-restart backoff loop, the cron scheduler.
//! - [`stop`] — graceful stop (signal + timeout + SIGKILL), the descendant
//!   discovery over `/proc`, signal/kill trees, binary lookup.
//! - [`systemd`] — the `systemd-run` argument list assembled for
//!   `--isolation dynamic` mode. Owns every hardening directive; the tests
//!   in this package treat each directive as a hard requirement.
//!
//! The helper modules are smaller and stand on their own:
//!
//! - [`shell`] — single-quote shell escaping for `sh -c` command lines.
//! - [`logwriter`] — timestamp-prefixed line writer and the 3-line
//!   lifecycle banner.
//! - [`rotate`] — log rotation with the `delaycompress` semantics.
//! - [`watcher`] — recursive file watcher used by `--watch`.
//! - [`version_detect`] — read a `package.json` / `Cargo.toml` version.
//!
//! The [`manager`] module ties everything together: the `Manager` registry,
//! `Restore`, the per-process `Process` struct, and the daemon-wide log
//! rotation loop.

// Phase 4b ports the manager; phase 4c ports the daemon loop that calls it,
// and phase 6 the CLI. Until those land, most of this module is reachable only
// from its own tests, and `dead_code` cannot tell "nothing uses this" from
// "nothing uses this yet" — it would have us delete the port one function at a
// time and write it again next phase.
//
// Scoped to this module rather than the crate, and it comes off when 4c lands.
// If anything here is still unreachable once the daemon calls into it, that is
// a genuine finding and the code should go.
#![allow(dead_code)]

// The inner `manager` module shares its name with the parent. Go's
// `manager.go` is conventionally the "main" file in a Go package; the
// Rust port preserves that shape so the directory layout mirrors the Go
// tree one-to-one. The clippy `module-inception` lint is allow-listed for
// this file only.
#[allow(clippy::module_inception)]
mod logwriter;
#[allow(clippy::module_inception)]
mod manager;
mod process;
mod rotate;
mod spawn;
mod stop;
mod supervise;
mod systemd;
mod version_detect;
mod watcher;

mod helpers;
mod lifecycle;

pub use helpers::ManagerError;
pub use manager::{Manager, ScaleSnapshot, APP_LIMIT, ROTATE_TICK_ENV, TRIM_HEAP_ENV};
pub use spawn::{resolve_command, shell_quote, SpawnError};
pub use stop::DEFAULT_STOP_TIMEOUT;
pub use supervise::{RestartPolicy, STOP_SIGNALS};
pub use systemd::{dynamic_args, DynamicCommand, EXEC_ENV_SUBCOMMAND, UNIT_NAME_PREFIX};

// The daemon loop (phase 4c) and the CLI (phase 6) are the consumers of this
// module's internal surface, and neither exists in Rust yet. Re-exporting it
// now keeps the shape the Go package had, so the next phase finds it where it
// expects rather than rediscovering it — and `dead_code` cannot tell
// "nothing uses this" from "nothing uses this yet".
//
// This attribute comes off when phase 4c lands. If it is still here after
// that, the re-export is genuinely unused and should be deleted instead.
#[allow(unused_imports)]
pub(crate) use logwriter::{timestamp_writer::TimestampWriter, write_banner};
#[allow(unused_imports)]
pub(crate) use process::Process;
#[allow(unused_imports)]
pub(crate) use rotate::{current_rotate_config, rotate_now_cfg, RotateConfig};
#[allow(unused_imports)]
pub(crate) use spawn::prepare_env;
#[allow(unused_imports)]
pub(crate) use stop::{graceful_kill, signal_tree, walk_descendants, KillError};
#[allow(unused_imports)]
pub(crate) use supervise::ProcessInternal;
#[allow(unused_imports)]
pub(crate) use systemd::DynamicContext;
#[allow(unused_imports)]
pub(crate) use version_detect::detect_project_version;
#[allow(unused_imports)]
pub(crate) use watcher::{file_watcher, FileWatcher};

/// Helpers shared between the public surface and the tests.
#[cfg(test)]
pub mod test_api {
	pub use super::logwriter::timestamp_writer::TimestampWriter as TimestampWriterExport;
	pub use super::rotate::{rotate_if_large, rotate_if_large_cfg};
	pub use super::spawn::prepare_env;
	pub use super::spawn::process_binary as process_binary_pub;
}
