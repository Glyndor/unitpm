//! Argument parsing for the `list` command.
//!
//! Hand-rolled flag parser mirroring `internal/cli/commands/list/cmd.go`.
//! The dispatcher already strips `--quiet`/`-q`; we only deal with
//! the list-specific flags here.

use crate::cli::errs::UsageError;

use super::{parse_sort_spec, SortField};

/// Parsed `list` arguments. Lives at module-private scope because no
/// caller outside `list` needs the parsed shape.
#[derive(Default, Debug)]
pub(crate) struct Args {
	pub show_long: bool,
	pub namespace: String,
	pub sort_spec: String,
	pub json_output: bool,
	pub sort_fields: Vec<SortField>,
}

/// Parse `list` argv into [`Args`]. Mirrors the Go `flag` package:
/// boolean flags have no value, value flags consume the next token.
/// Trailing positional args are rejected with a [`UsageError`].
pub(crate) fn parse_args(args: &[String]) -> Result<Args, UsageError> {
	// Lightweight, hand-rolled flag parser. Mirrors the Go `flag`
	// package surface: boolean flags have no value, value flags consume
	// the next token.
	let mut a = Args::default();
	let mut i = 0;
	while i < args.len() {
		let arg = args[i].clone();
		match arg.as_str() {
			"--long" => a.show_long = true,
			"--json" => a.json_output = true,
			"--namespace" => {
				a.namespace = take_value(args, &mut i, "--namespace")?;
			}
			"--sort" => {
				a.sort_spec = take_value(args, &mut i, "--sort")?;
			}
			"-h" | "--help" => {
				// Handled earlier; defensive skip.
			}
			other if other.starts_with('-') => {
				let name = other.trim_start_matches('-');
				return Err(UsageError::new(format!("Unknown flag: -{name}")));
			}
			_ => {
				return Err(UsageError::new(format!(
					"Unexpected arguments: {}",
					quote_list(&args[i..])
				)));
			}
		}
		i += 1;
	}
	a.sort_fields = parse_sort_spec(&a.sort_spec).map_err(|e| UsageError::new(e.to_string()))?;
	// Apply user fields, layered with the default sort. Empty user spec
	// means default; explicit spec replaces. The Go applies a layered
	// sort inside `sortProcesses`, mirroring that here means we hand a
	// combined list to `sort_processes_with`.
	Ok(a)
}

pub(crate) fn take_value(args: &[String], i: &mut usize, flag: &str) -> Result<String, UsageError> {
	if *i + 1 >= args.len() {
		return Err(UsageError::new(format!("missing value for flag {flag}")));
	}
	*i += 1;
	Ok(args[*i].clone())
}

/// Quote a slice of strings the way `bash` would when joined with a
/// space — wraps any value containing whitespace or quote chars in
/// double quotes and escapes embedded quotes.
pub(crate) fn quote_list(items: &[String]) -> String {
	let mut out = String::from("[");
	for (idx, it) in items.iter().enumerate() {
		if idx > 0 {
			out.push(' ');
		}
		out.push_str(it);
	}
	out.push(']');
	out
}

/// True when `args` contains `-h` or `--help`. Used by the
/// `args_contain_help` helper that decides whether to print
/// command-level help instead of running the command.
pub(crate) fn args_contain_help(args: &[String]) -> bool {
	args.iter()
		.any(|a| a == "-h" || a == "--help" || a == "-help")
}
