//! Daemon IPC handlers — the request layer that wires [`manager`](crate::daemon::manager)
//! and [`policy`](crate::daemon::policy) to the IPC transport.
//!
//! Phase 4c of the Go → Rust rewrite. Mirrors `internal/daemon/handlers`
//! and the dispatcher in `internal/daemon/handlers.go`. The file layout
//! follows the request families the Go code eventually grew:
//!
//! - [`service`] — validation, policy gate, the `start_process` entry.
//! - [`start`] — the `start` IPC verb (parses `StartRequest`, calls
//!   `start_process`, packages `StartResponseData`).
//! - [`audit`] — audit-event emission and `processMeta`/`wasRunning`
//!   helpers used by every destructive verb.
//! - [`lifecycle`] — `restart` / `reload` / `reset` (the simple `{id}`
//!   template).
//! - [`control`] — `stop`, `delete`, `show`, `scale`, `flush`, `proctree`,
//!   `list`, `version`, `ping`.
//! - [`register`] — [`register_handlers`], the single entry point that
//!   wires every verb onto an [`ipc::transport::Server`].
//!
//! Every destructive verb funnels through [`audit::audit_event`] so that
//! removing the audit call turns a test red.

pub mod audit;
pub mod control;
pub mod flush;
pub mod lifecycle;
pub mod query;
pub mod register;
pub mod service;
pub mod start;

pub use register::register_handlers;
pub use service::SharedManager;

#[cfg(all(test, target_os = "linux"))]
mod tests;
