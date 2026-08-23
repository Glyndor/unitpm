//! Per-process isolation primitives used when the daemon spawns children.
//!
//! Three modes are handled here. `self` is a plain `SysProcAttr` (only
//! `Setpgid` is set, so `Stop` can kill the whole group). `dynamic` is
//! delegated to the systemd-run wrapper inside `manager.prepareIsolation`
//! and is **not** in this package. `sandbox` is the unprivileged
//! user-namespace + Landlock + rlimit sandbox assembled by [`wrap_sandbox`].
//!
//! The two underlying kernel primitives live in their own sub-modules:
//!
//! - [`landlock`] — direct Landlock syscall wrapper.
//! - [`rlimit`] — `setrlimit(2)` wrapper for the sandbox rlimit caps.
//!
//! Both are gated to Linux (the kernel interfaces are Linux-only), as are
//! [`start`] and [`sandbox`] above them. On other platforms the modules
//! disappear entirely — the Go counterparts carry the same `//go:build linux`
//! constraint.

#[cfg(target_os = "linux")]
pub mod landlock;

#[cfg(target_os = "linux")]
pub mod rlimit;

#[cfg(target_os = "linux")]
mod sandbox;

#[cfg(target_os = "linux")]
mod start;

#[cfg(target_os = "linux")]
pub use sandbox::{wrap_sandbox, SandboxOptions, WrappedCommand};
#[cfg(target_os = "linux")]
pub use start::configure_process_isolation;

#[cfg(all(test, target_os = "linux"))]
mod sandbox_tests;
#[cfg(all(test, target_os = "linux"))]
mod start_tests;
