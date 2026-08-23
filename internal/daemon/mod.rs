//! Daemon-side subsystems: authorization, process isolation, audit, and the
//! IPC handlers that consume them.
//!
//! Phases of the Go → Rust rewrite in this module:
//!
//! - Phase 4a: [`policy`] — authorization rules for the `start` IPC verb.
//!   [`runtime`] — per-process isolation primitives (landlock, rlimit, the
//!   unprivileged sandbox wrapper, and the syscall-attribute setup the
//!   `self` / reserved modes share).
//! - Phase 4b: [`manager`] — the registry, lifecycle, isolation, rotation,
//!   and supervision for one managed application.
//! - Phase 4c: [`audit`] — JSON-lines audit log for destructive actions,
//!   and [`handlers`] — the request layer that wires the manager to the
//!   IPC transport.
//!
//! The Rust IPC layer under [`crate::ipc`] covers the transport and wire
//! format. [`handlers::register_handlers`] is what a `unitpmd` entry point
//! (phase 6 / 7) calls to wire everything onto a [`crate::ipc::transport::Server`].

pub mod audit;
pub mod handlers;
pub mod manager;
pub mod policy;

#[cfg(target_os = "linux")]
pub mod runtime;
