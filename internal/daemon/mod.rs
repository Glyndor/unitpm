//! Daemon-side subsystems: authorization, process isolation, and the IPC
//! handlers that consume them.
//!
//! Phase 4a of the Go -> Rust rewrite ports:
//!
//! - [`policy`] — authorization rules for the `start` IPC verb.
//! - [`runtime`] — per-process isolation primitives (landlock, rlimit, the
//!   unprivileged sandbox wrapper, and the syscall-attribute setup the
//!   `self` / reserved modes share).
//!
//! The Rust IPC layer under [`crate::ipc`] already covers the transport and
//! wire format. The handlers that consume `policy::authorize_start` and the
//! `runtime::WrapSandbox` builder land on a later phase; for now these
//! modules exist so that they can be exercised in isolation.

pub mod policy;

#[cfg(target_os = "linux")]
pub mod runtime;
