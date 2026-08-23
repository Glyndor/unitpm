//! Centralised help renderer for the CLI.
//!
//! 8 cases ported from `internal/cli/help/help_test.go`.
//!
//! The binary name "unitpm" appears here in usage lines; it is the same
//! constant the root command uses when it prints errors so a rename only
//! happens in one place.

use std::io::{self, Write};

use crate::term;

/// A flag/option as it appears in `--help`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Option {
	pub short: String,
	pub long: String,
	pub description: String,
}

/// Metadata for one CLI command.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CommandSpec {
	pub name: String,
	pub aliases: Vec<String>,
	pub usage: String,
	pub description: String,
	pub options: Vec<Option>,
	pub examples: Vec<String>,
	pub hidden: bool,
}

/// Render the help block for a single command to `w`.
pub fn render_command_help<W: Write>(w: &mut W, spec: &CommandSpec) -> io::Result<()> {
	let mut options = spec.options.clone();

	let has_help = options
		.iter()
		.any(|o| o.short == "-h" || o.long == "--help");
	if !has_help {
		options.push(Option {
			short: "-h".into(),
			long: "--help".into(),
			description: "Show this help message.".into(),
		});
	}

	// Pad the usage with `[options]` unless the author already declared a
	// placeholder, so a bare "unitpm x" still reads like a complete usage.
	let mut usage = spec.usage.clone();
	if !options.is_empty() && !usage.contains("[options]") && !usage.contains("[flags]") {
		usage.push_str(" [options]");
	}

	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "Usage:")))?;
	writeln!(w, "  {usage}")?;

	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "Description:")))?;
	for line in spec.description.split('\n') {
		writeln!(w, "  {line}")?;
	}

	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "Options:")))?;

	let labels: Vec<String> = options.iter().map(flag_label).collect();
	let max_len = labels.iter().map(String::len).max().unwrap_or(0);

	for (label, opt) in labels.iter().zip(options.iter()) {
		let pad = " ".repeat(max_len - label.len() + 4);
		writeln!(
			w,
			"  {}{pad}{}",
			term::bold(format_args!("{}", label)),
			opt.description
		)?;
	}
	writeln!(w)?;

	if !spec.examples.is_empty() {
		writeln!(w, "{}", term::cyan(format_args!("{}", "Examples:")))?;
		for ex in &spec.examples {
			writeln!(w, "  {ex}")?;
		}
		writeln!(w)?;
	}
	Ok(())
}

/// Render the help block for the root command. `show_commands=false` hides
/// the per-command list so the same renderer can produce the `--help` page
/// and a quieter "no command given" hint.
pub fn render_root_help<W: Write>(
	w: &mut W,
	specs: &[CommandSpec],
	show_commands: bool,
) -> io::Result<()> {
	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "Usage:")))?;
	writeln!(w, "  unitpm <command> [flags]")?;

	if show_commands {
		writeln!(w)?;
		writeln!(w, "{}", term::cyan(format_args!("{}", "Commands:")))?;

		let visible: Vec<&CommandSpec> = specs.iter().filter(|s| !s.hidden).collect();

		let mut display_names: Vec<String> = Vec::with_capacity(visible.len());
		let mut max_len = 0;
		for spec in &visible {
			let name = if spec.aliases.is_empty() {
				spec.name.clone()
			} else {
				format!("{}, {}", spec.name, spec.aliases.join(", "))
			};
			if name.len() > max_len {
				max_len = name.len();
			}
			display_names.push(name);
		}

		for (name, spec) in display_names.iter().zip(visible.iter()) {
			let pad = " ".repeat(max_len - name.len() + 3);
			writeln!(
				w,
				"  {}{pad}{}",
				term::bold(format_args!("{}", name)),
				spec.description
			)?;
		}
	}

	writeln!(w)?;
	writeln!(w, "{}", term::cyan(format_args!("{}", "Get Help:")))?;
	writeln!(w, "  unitpm --help")?;
	writeln!(w, "  unitpm <command> --help")?;
	Ok(())
}

/// Returns true when `args` contains `-h`, `--help`, or the legacy `-help`.
#[must_use]
pub fn is_help(args: &[String]) -> bool {
	args.iter()
		.any(|a| a == "-h" || a == "--help" || a == "-help")
}

fn flag_label(opt: &Option) -> String {
	match (opt.short.is_empty(), opt.long.is_empty()) {
		(false, false) => format!("{}, {}", opt.short, opt.long),
		(false, true) => opt.short.clone(),
		(true, false) => format!("    {}", opt.long),
		(true, true) => String::new(),
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn is_help_recognises_flags() {
		let cases = [
			(vec![], false),
			(vec!["start".to_string()], false),
			(vec!["-h".to_string()], true),
			(vec!["--help".to_string()], true),
			(vec!["-help".to_string()], true),
			(vec!["start".to_string(), "-h".to_string()], true),
			(
				vec![
					"--name".to_string(),
					"foo".to_string(),
					"--help".to_string(),
				],
				true,
			),
		];
		for (input, want) in cases {
			let got = is_help(&input);
			assert_eq!(got, want, "is_help({input:?}) = {got}, want {want}");
		}
	}

	#[test]
	fn render_command_help_includes_all_sections() {
		let spec = CommandSpec {
			name: "test-cmd".to_string(),
			aliases: vec![],
			usage: "unitpm test-cmd [flags]".to_string(),
			description: "A test command description.".to_string(),
			options: vec![Option {
				short: "-f".into(),
				long: "--flag".into(),
				description: "A flag description".into(),
			}],
			examples: vec![],
			hidden: false,
		};
		let mut buf = Vec::new();
		render_command_help(&mut buf, &spec).expect("render");
		let out = String::from_utf8(buf).expect("utf8");

		// The Go test checks for substring presence; the colour-decision
		// branch (term::should_use_color) doesn't matter because we only
		// assert on the uncoloured payload.
		let plain = strip_ansi(&out);
		for want in [
			"Usage:",
			"unitpm test-cmd [flags]",
			"Description:",
			"A test command description.",
			"Options:",
			"-f, --flag",
			"A flag description",
		] {
			assert!(
				plain.contains(want),
				"output missing {want:?}; got:\n{plain}"
			);
		}
	}

	#[test]
	fn render_command_help_appends_help_flag_when_absent() {
		let spec = CommandSpec {
			name: "x".to_string(),
			aliases: vec![],
			usage: "unitpm x".to_string(),
			description: "d".to_string(),
			options: vec![],
			examples: vec![],
			hidden: false,
		};
		let mut buf = Vec::new();
		render_command_help(&mut buf, &spec).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		assert!(
			out.contains("-h, --help"),
			"expected auto-appended -h/--help, got {out:?}"
		);
		assert!(
			out.contains("[options]"),
			"expected usage augmented with [options], got {out:?}"
		);
	}

	#[test]
	fn render_command_help_keeps_existing_help_and_usage_placeholder() {
		let spec = CommandSpec {
			name: "x".to_string(),
			aliases: vec![],
			usage: "unitpm x [flags]".to_string(),
			description: "d".to_string(),
			options: vec![Option {
				short: "-h".into(),
				long: "--help".into(),
				description: "custom".into(),
			}],
			examples: vec![],
			hidden: false,
		};
		let mut buf = Vec::new();
		render_command_help(&mut buf, &spec).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		assert_eq!(
			out.matches("--help").count(),
			1,
			"expected exactly one --help, got {}",
			out.matches("--help").count()
		);
		assert!(
			out.contains("custom"),
			"expected custom description preserved, got {out:?}"
		);
		assert!(
			!out.contains("[options]"),
			"usage already declared [flags], should not append [options]"
		);
	}

	#[test]
	fn render_command_help_short_only_and_long_only_labels() {
		let spec = CommandSpec {
			name: "x".to_string(),
			aliases: vec![],
			usage: "unitpm x".to_string(),
			description: "d".to_string(),
			options: vec![
				Option {
					short: "-v".into(),
					long: String::new(),
					description: "short only".into(),
				},
				Option {
					short: String::new(),
					long: "--verbose".into(),
					description: "long only".into(),
				},
			],
			examples: vec![],
			hidden: false,
		};
		let mut buf = Vec::new();
		render_command_help(&mut buf, &spec).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		assert!(
			!out.contains(", --verbose"),
			"long-only option should not have a leading comma"
		);
		assert!(
			out.contains("    --verbose"),
			"long-only option should be padded to align with short forms"
		);
	}

	#[test]
	fn render_command_help_with_examples() {
		let spec = CommandSpec {
			name: "x".to_string(),
			aliases: vec![],
			usage: "unitpm x".to_string(),
			description: "d".to_string(),
			options: vec![],
			examples: vec!["unitpm x foo".to_string(), "unitpm x bar".to_string()],
			hidden: false,
		};
		let mut buf = Vec::new();
		render_command_help(&mut buf, &spec).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		assert!(out.contains("Examples:"), "expected Examples section");
		assert!(
			out.contains("unitpm x foo"),
			"expected example line 1, got {out:?}"
		);
		assert!(
			out.contains("unitpm x bar"),
			"expected example line 2, got {out:?}"
		);
	}

	#[test]
	fn render_root_help_hides_hidden_commands() {
		let specs = vec![
			CommandSpec {
				name: "start".into(),
				aliases: vec![],
				usage: String::new(),
				description: "Start app".into(),
				options: vec![],
				examples: vec![],
				hidden: false,
			},
			CommandSpec {
				name: "stop".into(),
				aliases: vec!["halt".into()],
				usage: String::new(),
				description: "Stop app".into(),
				options: vec![],
				examples: vec![],
				hidden: false,
			},
			CommandSpec {
				name: "_hidden".into(),
				aliases: vec![],
				usage: String::new(),
				description: "Internal".into(),
				options: vec![],
				examples: vec![],
				hidden: true,
			},
		];
		let mut buf = Vec::new();
		render_root_help(&mut buf, &specs, true).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		for want in [
			"Usage:",
			"Commands:",
			"start",
			"Start app",
			"stop, halt",
			"Get Help:",
		] {
			assert!(out.contains(want), "missing {want:?} in output");
		}
		assert!(
			!out.contains("_hidden"),
			"hidden command leaked into root help"
		);
	}

	#[test]
	fn render_root_help_hides_commands_section_when_disabled() {
		let mut buf = Vec::new();
		render_root_help(&mut buf, &[], false).expect("render");
		let out = String::from_utf8(buf).expect("utf8");
		assert!(
			!out.contains("Commands:"),
			"Commands section should be hidden when showCommands=false"
		);
		assert!(out.contains("Get Help:"), "expected Get Help section");
	}

	// --- helpers ------------------------------------------------------------

	fn strip_ansi(s: &str) -> String {
		crate::cli::format::strip_ansi(s)
	}
}
