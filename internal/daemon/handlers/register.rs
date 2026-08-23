//! Daemon-side handler registry.
//!
//! Mirrors `RegisterHandlers` from `internal/daemon/handlers.go`. Each
//! IPC verb gets a closure that:
//!
//! 1. parses its parameters;
//! 2. drives the [`SharedManager`];
//! 3. emits an audit record on every exit path when the verb is destructive.
//!
//! `privileged` is the daemon's own mode (`true` for the system instance,
//! `false` for a user-mode one). The `policy::authorize_start` check
//! inside the `start` handler refuses `shell` execution and gates
//! `run_as=dynamic` on it — so this flag is a security parameter, not a
//! configuration one, and is forwarded verbatim.

use std::sync::Arc;

use crate::daemon::audit::Logger;
use crate::daemon::handlers::control::{delete_handler, scale_handler, stop_handler};
use crate::daemon::handlers::flush::flush_handler;
use crate::daemon::handlers::lifecycle::{register_id_verb, reload, reset, restart};
use crate::daemon::handlers::query::{
	list_handler, ping, proctree_handler, show_handler, version_handler,
};
use crate::daemon::handlers::service::SharedManager;
use crate::daemon::handlers::start::start_handler;
use crate::ipc::transport::Server;

/// Register every daemon IPC verb against `server`. `auditor` is the audit
/// logger — pass [`Logger::disabled`](crate::daemon::audit::Logger::disabled)
/// for a user-mode daemon where the log is intentionally off.
pub fn register_handlers(
	server: &Server,
	mgr: SharedManager,
	privileged: bool,
	auditor: Arc<Logger>,
) {
	server.register("ping", ping);

	server.register("start", start_handler(Arc::clone(&mgr), privileged));

	server.register("stop", stop_handler(Arc::clone(&mgr), Arc::clone(&auditor)));

	server.register(
		"restart",
		register_id_verb(
			Arc::clone(&mgr),
			Arc::clone(&auditor),
			"restart",
			"restarted",
			restart,
		),
	);
	server.register(
		"delete",
		delete_handler(Arc::clone(&mgr), Arc::clone(&auditor)),
	);

	let mgr_for_show = Arc::clone(&mgr);
	server.register("show", move |ctx, params| {
		show_handler(Arc::clone(&mgr_for_show), ctx, params)
	});

	server.register(
		"reset",
		register_id_verb(
			Arc::clone(&mgr),
			Arc::clone(&auditor),
			"reset",
			"reset",
			reset,
		),
	);
	server.register(
		"reload",
		register_id_verb(
			Arc::clone(&mgr),
			Arc::clone(&auditor),
			"reload",
			"reloaded",
			reload,
		),
	);

	server.register(
		"scale",
		scale_handler(Arc::clone(&mgr), Arc::clone(&auditor)),
	);

	server.register(
		"flush",
		flush_handler(Arc::clone(&mgr), Arc::clone(&auditor)),
	);

	let mgr_for_proctree = Arc::clone(&mgr);
	server.register("proctree", move |ctx, params| {
		proctree_handler(Arc::clone(&mgr_for_proctree), ctx, params)
	});

	let mgr_for_list = Arc::clone(&mgr);
	server.register("list", move |ctx, params| {
		list_handler(Arc::clone(&mgr_for_list), ctx, params)
	});

	server.register("version", version_handler);
}

/// Re-export the verb list as a static slice so tests can pin it. Used by
/// the "every verb is wired" assertion that catches silent removal after a
/// refactor.
pub const REGISTERED_VERBS: &[&str] = &[
	"ping", "start", "stop", "restart", "reload", "reset", "flush", "delete", "list", "show",
	"version", "scale", "proctree",
];
