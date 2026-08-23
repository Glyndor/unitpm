//! Exec-shape decision for the `start` command.
//!
//! Determines whether the parsed command tokens form a single-entry
//! file (extension-inferred runtime), a quoted command-with-args
//! string, or a multi-token command line. Mirrors the Go
//! `resolveExec` helper.

use std::path::Path;

use crate::cli::commands::start::lexer::tokenize;
use crate::ipc::protocol::AppExec;

/// Decide whether the command tokens are an entry file (one token,
/// extension-inferred runtime), a quoted command-and-args string, or
/// a multi-token command.
pub fn resolve_exec(cmd_parts: &[String], runtime: &str, shell: bool, exec: &mut AppExec) {
	if cmd_parts.len() == 1 {
		let token = &cmd_parts[0];
		if !runtime.is_empty() {
			*exec = AppExec {
				kind: "entry".to_string(),
				command: None,
				args: None,
				entry: Some(token.clone()),
				runtime: Some(runtime.to_string()),
				shell,
			};
			return;
		}

		if let Ok(parts) = tokenize(token) {
			if parts.len() > 1 {
				let mut rest = parts;
				let first = rest.remove(0);
				*exec = AppExec {
					kind: "command".to_string(),
					command: Some(first),
					args: Some(rest),
					entry: None,
					runtime: None,
					shell,
				};
				return;
			}
		}

		let ext = Path::new(token)
			.extension()
			.and_then(|s| s.to_str())
			.unwrap_or("");
		let ext_dotted = if ext.is_empty() {
			String::new()
		} else {
			format!(".{ext}")
		};
		match ext_dotted.as_str() {
			".js" | ".mjs" | ".cjs" => {
				*exec = AppExec {
					kind: "entry".to_string(),
					command: None,
					args: None,
					entry: Some(token.clone()),
					runtime: Some("node".to_string()),
					shell,
				};
			}
			".go" => {
				*exec = AppExec {
					kind: "entry".to_string(),
					command: None,
					args: None,
					entry: Some(token.clone()),
					runtime: Some("go run".to_string()),
					shell,
				};
			}
			_ => {
				*exec = AppExec {
					kind: "command".to_string(),
					command: Some(token.clone()),
					args: None,
					entry: None,
					runtime: None,
					shell,
				};
			}
		}
	} else {
		let mut rest = cmd_parts.to_vec();
		let first = rest.remove(0);
		*exec = AppExec {
			kind: "command".to_string(),
			command: Some(first),
			args: Some(rest),
			entry: None,
			runtime: None,
			shell,
		};
	}
}
