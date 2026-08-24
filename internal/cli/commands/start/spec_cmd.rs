//! Command spec registration for the `start` command.
//!
//! The `CommandSpec` literal is large because the `start` command
//! exposes every flag the Go side parses; mirroring it as data keeps
//! the dispatcher's help page accurate. Lives in its own file so the
//! entry-point module stays focused on the runtime flow.

use crate::cli::help::{CommandSpec, Option as HelpOption};
use crate::cli::root::cmd;

/// Command spec for the registry.
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "start".into(),
		aliases: Vec::new(),
		usage: "unitpm start <command|file> [flags]".into(),
		description: "Start a new process.".into(),
		options: vec![
			HelpOption {
				short: String::new(),
				long: "--name <name>".into(),
				description: "Assign a name to the process".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--namespace <name>".into(),
				description: "Assign a namespace to the process".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--cwd <dir>".into(),
				description: "Working directory".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--shell".into(),
				description: "Execute command in shell".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--schedule <cron>".into(),
				description: "Cron schedule for restart (alias --cron)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--restart <policy>".into(),
				description: "Restart policy (never, on-failure, always)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--max-restarts <N>".into(),
				description: "Max restarts (default 10)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--restart-delay <ms>".into(),
				description: "Restart delay in ms (default 2000)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--backoff <type>".into(),
				description: "Backoff strategy (none, linear, expo)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--stop-on-exit <codes>".into(),
				description: "Exit codes to stop on (comma-separated)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--log-dir <path>".into(),
				description: "Directory for logs".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--stdout <file>".into(),
				description: "Stdout file (relative to log-dir)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--stderr <file>".into(),
				description: "Stderr file (relative to log-dir)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--log-format <fmt>".into(),
				description: "Log format (plain, json)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--log-timestamp <fmt>".into(),
				description: "Log timestamp (rfc3339, unix, none)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--runtime <rt>".into(),
				description: "Runtime for entry file (e.g., node, python)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--env-file <file>".into(),
				description: "Path to environment file".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--isolation <mode>".into(),
				description: "Isolation mode (self, dynamic, sandbox)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--scale <N>".into(),
				description: "Number of instances to start".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--stop-signal <name>".into(),
				description:
					"Signal sent on stop (SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2)"
						.into(),
			},
			HelpOption {
				short: String::new(),
				long: "--stop-timeout <ms>".into(),
				description: "Grace period before SIGKILL (default 10000, range 1000-300000)"
					.into(),
			},
			HelpOption {
				short: String::new(),
				long: "--memory-max <size>".into(),
				description: "Hard memory ceiling: 512M, 2G, or bytes".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--cpu-max <percent>".into(),
				description: "CPU cap as percent of one core (100 = 1 core, 200 = 2 cores)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--tasks-max <N>".into(),
				description: "Maximum number of tasks (threads + subprocesses)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--watch".into(),
				description: "Restart on file changes in cwd".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--watch-ignore <globs>".into(),
				description: "Extra ignore patterns (comma-separated)".into(),
			},
			HelpOption {
				short: "-n".into(),
				long: "--dry-run".into(),
				description: "Print the resolved spec without starting anything".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--json".into(),
				description: "Emit the start result as JSON on stdout".into(),
			},
			HelpOption {
				short: "-q".into(),
				long: "--quiet".into(),
				description: "Suppress success messages (errors still printed)".into(),
			},
			HelpOption {
				short: String::new(),
				long: "--no-list".into(),
				description: "Skip the process list printed after the action".into(),
			},
		],
		examples: vec![
			"unitpm start \"node server.js\" --name api".into(),
			"unitpm start app.py --runtime python3 --restart on-failure".into(),
			"unitpm start \"uv run main.py\" --name worker --cwd /srv/app".into(),
			"unitpm start \"bun run dev\" --name web --env-file .env".into(),
			"unitpm start ./target/release/api --name api --restart always".into(),
			"unitpm start worker.js --name w --scale 3".into(),
			"unitpm start server.js --isolation sandbox --cwd /srv/app".into(),
			"# Runtime recipes:  docs/RUNTIMES.md".into(),
		],
		hidden: false,
	}
}

#[allow(dead_code)]
pub(crate) fn _unused_cmd_ref() -> &'static str {
	cmd::START
}
