// Binary entry point for the `unitpm` CLI.
//
// hand argv off to the dispatcher and exit
// with its code. The dispatcher lives in `internal::cli::root` and is
// covered by the same test suite that ports the Go `Execute` cases.
//
// The crate name (`unitpm`) and library path (`internal/lib.rs`) are
// fixed in `Cargo.toml` so this binary keeps the org's "product code
// under `internal/`" layout. Keeping the binary entry point in `cmd/`
// matches the standard Go layout the previous tree used, which keeps
// the existing Debian packaging and systemd unit working with minimal
// change.

use std::process::ExitCode;

use unitpm::cli::root::{self, TransportDispatcherClient};

fn main() -> ExitCode {
	// pass `argv[1:]` to the dispatcher
	// and exit with its code. The dispatcher handles help, unknown
	// commands, and the global `--quiet`/`-q` flag.
	let mut argv = std::env::args();
	argv.next();
	let args: Vec<String> = argv.collect();

	// Install a transport client before dispatching. The dispatcher
	// takes it via the global slot (it cannot take ownership of a
	// value through the entry point's slice), then consumes it on the
	// first lifecycle command. The client dials lazily — a missing
	// daemon only matters for the four commands that need it
	// (`list`/`start`/`stop`/`restart`), so `unitpm --help` and
	// `unitpm version` never touch the socket.
	root::install_dispatcher_client(Box::new(TransportDispatcherClient::lazy()));

	ExitCode::from(root::execute(&args) as u8)
}
