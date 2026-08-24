// Binary entry point for the `unitpmd` daemon.
//
// wire the manager, the IPC server, the
// handlers, the audit logger, and the signal-driven lifecycle. Every
// piece here already exists in the library; this file is the wiring,
// not the implementation.
//
// The daemon uses an `Arc<Manager>` so every handler (which lives on a
// per-connection thread spawned by `Server::start`) can hold a cheap
// clone and the rotate-loop thread in `Manager::new` can talk to the
// same registry. The audit logger is also `Arc`-shared — handler-side
// `audit_event` calls take a cheap reference and a no-op "disabled"
// sentinel can stand in for the user-mode daemon without breaking the
// shape of the API.
//
// Signal handling: SIGINT / SIGTERM trigger a clean shutdown; SIGHUP
// triggers the "stop and re-exec" path the Go daemon uses to roll
// updates without losing the registered process table. The Go path
// uses `syscall.Exec`; in Rust there is no direct equivalent in
// stable, so the closest we get is `std::process::Command::new(...).exec()`
// (still uses execvp under the hood), wrapped in a safe child path.

use std::os::unix::process::CommandExt;
use std::path::Path;
use std::process::ExitCode;
use std::sync::Arc;

use unitpm::daemon::audit;
use unitpm::daemon::handlers::{self, SharedManager};
use unitpm::daemon::manager::Manager;
use unitpm::ipc::transport::Server;
use unitpm::paths;

fn main() -> ExitCode {
	// Block SIGINT / SIGTERM / SIGHUP in the main thread BEFORE any
	// worker threads are spawned. Worker threads inherit the signal
	// mask, so once this call returns every thread created later —
	// the rotate loop in `Manager::new`, the accept loop in
	// `Server::start`, anything else — has these three signals
	// blocked and the kernel delivers them via `sigwait` instead of
	// falling through to the default handler (which would terminate
	// the daemon on SIGHUP, since the default SIGHUP action is to
	// terminate a non-leader process).
	let signal_mask = install_signal_mask();

	eprintln!("unitpmd starting...");

	let mgr = Arc::new(std::sync::Mutex::new(Manager::new()));
	let server = Server::new();

	let privileged = paths::is_system_mode();
	let auditor = audit::Logger::open(audit_path(privileged));

	handlers::register_handlers(&server, mgr_clone(&mgr), privileged, Arc::clone(&auditor));

	// Restore state. The Go side logs a warning on failure but does not
	// abort — half-restored state still beats no daemon.
	eprintln!("Restoring processes...");
	if let Err(e) = mgr.lock().expect("manager lock").restore() {
		eprintln!("Warning: failed to restore state: {e}");
	}

	let socket_path = match server.start() {
		Ok(p) => p,
		Err(e) => {
			eprintln!("Failed to start IPC server: {e}");
			return ExitCode::from(1);
		}
	};
	eprintln!("IPC server listening on {}", socket_path.display());

	match wait_for_signal(signal_mask) {
		SignalOutcome::Hangup => {
			// Re-exec path: gracefully stop children so Restore() can
			// bring them back after the new image is in place. Mirrors
			// the Go mgr.Shutdown() before syscall.Exec.
			eprintln!("SIGHUP received — stopping processes and re-executing...");
			mgr.lock().expect("manager lock").shutdown();
			// Give the OS a moment to release ports and PIDs.
			std::thread::sleep(std::time::Duration::from_millis(500));
			server.close();
			auditor.close();
			exec_self();
			// If exec returns (it shouldn't), fall through to clean shutdown.
		}
		SignalOutcome::Terminate => {}
	}

	eprintln!("Shutting down...");
	mgr.lock().expect("manager lock").shutdown();
	drop(server);
	auditor.close();
	ExitCode::SUCCESS
}

/// Where the JSON-lines audit log should be written. Empty string
/// disables audit — used by user-mode daemons where the daemon is
/// already scoped to a single user.
fn audit_path(system_daemon: bool) -> std::path::PathBuf {
	if !system_daemon {
		return std::path::PathBuf::new();
	}
	Path::new(paths::LOG_ROOT).join("audit.log")
}

fn mgr_clone(mgr: &SharedManager) -> SharedManager {
	Arc::clone(mgr)
}

/// Outcome of the signal wait. Matches the two branches the Go daemon
/// distinguishes: SIGHUP re-execs; SIGINT/SIGTERM terminate cleanly.
enum SignalOutcome {
	Hangup,
	Terminate,
}

#[cfg(unix)]
fn install_signal_mask() -> libc::sigset_t {
	let mut mask: libc::sigset_t = unsafe { std::mem::zeroed() };
	unsafe {
		libc::sigemptyset(&mut mask);
		libc::sigaddset(&mut mask, libc::SIGINT);
		libc::sigaddset(&mut mask, libc::SIGTERM);
		libc::sigaddset(&mut mask, libc::SIGHUP);
		let rc = libc::pthread_sigmask(libc::SIG_BLOCK, &mask, std::ptr::null_mut());
		if rc != 0 {
			// `pthread_sigmask` returning non-zero here is fatal: if we
			// can't mask the signals we can't catch them, so a SIGINT
			// would still terminate the process. Bail loudly.
			eprintln!(
				"unitpmd: pthread_sigmask failed: {}",
				std::io::Error::from_raw_os_error(rc)
			);
			std::process::exit(1);
		}
	}
	mask
}

#[cfg(unix)]
fn wait_for_signal(mask: libc::sigset_t) -> SignalOutcome {
	use std::os::raw::c_int;

	// The signal mask was already installed by `install_signal_mask`
	// at the top of `main`, before any worker thread was spawned, so
	// every thread in this process has these three signals blocked.
	// The kernel holds them pending until this dedicated thread calls
	// `sigwait` and consumes the next one.
	let handle = std::thread::spawn(move || -> c_int {
		let mut sig: c_int = 0;
		let rc = unsafe { libc::sigwait(&mask, &mut sig) };
		if rc != 0 {
			return -rc;
		}
		sig
	});

	let sig = handle.join().expect("sigwait thread panicked");
	if sig == libc::SIGHUP {
		SignalOutcome::Hangup
	} else {
		SignalOutcome::Terminate
	}
}

#[cfg(not(unix))]
fn install_signal_mask() -> libc::sigset_t {
	std::ptr::null_mut()
}

#[cfg(not(unix))]
fn wait_for_signal(_mask: libc::sigset_t) -> SignalOutcome {
	// Non-unix builds cannot SIGHUP — fall back to a busy loop that
	// never returns, matching the Go behaviour on platforms without
	// signals. Phase 7a's binary is linux-only, so this branch is a
	// safety net for `cargo check` on macOS hosts.
	loop {
		std::thread::sleep(std::time::Duration::from_secs(3600));
	}
}

/// Replace the running process with a fresh copy of the current
/// binary, preserving argv and the environment. Mirrors the Go
/// `syscall.Exec(exe, os.Args, os.Environ)` the daemon uses on
/// SIGHUP. If the exec fails, log and fall through; the caller
/// performs the regular shutdown sequence.
fn exec_self() {
	let exe = match std::env::current_exe() {
		Ok(p) => p,
		Err(e) => {
			eprintln!("re-exec: cannot resolve executable: {e}");
			return;
		}
	};
	let args: Vec<String> = std::env::args().collect();
	// `exec` on a Command replaces the current process image. It only
	// returns on failure.
	let err = std::process::Command::new(&exe).args(&args[1..]).exec();
	eprintln!("re-exec failed: {err} — daemon exiting");
}
