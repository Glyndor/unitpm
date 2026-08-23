//! The `install-tools` command.
//!
//! 9 cases ported from `internal/cli/commands/installtools/cmd_test.go`.
//!
//! Symlinks a curated list of common developer tools (bun, node, go,
//! etc.) into `~/.local/bin` (or `/usr/local/bin` under `--system`)
//! so the daemon can find them when it spawns user processes. Without
//! `--system` the operation is interactive; `--yes` skips the prompt.

use std::io::Write;
use std::os::unix::fs::symlink;
use std::path::{Path, PathBuf};

use crate::cli::help::CommandSpec;
use crate::paths;

/// Tool names the command searches for on PATH and offers to link.
pub const COMMON_TOOLS: &[&str] = &[
	"bun", "node", "npm", "pnpm", "yarn", "go", "python", "python3", "pip", "pip3", "ruby", "gem",
	"rustc", "cargo", "java", "javac", "deno",
];

/// Run the `install-tools` command. `w` receives the rendered plan and
/// status lines. `stdin` lets tests drive the interactive prompt.
pub fn run<W: Write>(
	w: &mut W,
	args: &[String],
	stdin: Option<&str>,
) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let mut auto_yes = false;
	let mut system_mode = false;
	for arg in args {
		match arg.as_str() {
			"-y" | "--yes" => auto_yes = true,
			"--system" => system_mode = true,
			_ => {}
		}
	}

	if system_mode && !paths::is_root() {
		return Err(Box::<dyn std::error::Error>::from(
			"--system requires root privileges (run with sudo)",
		));
	}

	let dest_dir = if system_mode {
		PathBuf::from("/usr/local/bin")
	} else {
		let home = dirs_home().ok_or_else(|| -> Box<dyn std::error::Error> {
			Box::<dyn std::error::Error>::from("failed to determine home directory")
		})?;
		let d = PathBuf::from(home).join(".local").join("bin");
		std::fs::create_dir_all(&d).map_err(|e| -> Box<dyn std::error::Error> {
			Box::<dyn std::error::Error>::from(format!("failed to create {}: {}", d.display(), e))
		})?;
		d
	};

	let sudo_user = if system_mode {
		std::env::var("SUDO_USER").unwrap_or_default()
	} else {
		String::new()
	};

	if system_mode && sudo_user.is_empty() {
		let _ = writeln!(
			w,
			"{} SUDO_USER not set. Scanning root's PATH only.",
			term_yellow("!")
		);
		let _ = writeln!(w, "  Ideally run as: sudo unitpm install-tools --system");
	}

	let _ = writeln!(
		w,
		"Scanning for development tools to link to {}...",
		dest_dir.display()
	);

	let mut plan: Vec<PlannedLink> = Vec::new();
	for tool in COMMON_TOOLS {
		let dest_path = dest_dir.join(tool);
		if dest_path.exists() || dest_path.is_symlink() {
			continue;
		}
		let src_path = if system_mode && !sudo_user.is_empty() {
			find_via_runuser(&sudo_user, tool)
		} else {
			which(tool)
		};
		let Some(src) = src_path else {
			continue;
		};
		if !src.is_file() {
			continue;
		}
		// Skip circular symlinks: if the resolved src equals the dest
		// we're about to create, the link would loop.
		if let Ok(resolved) = std::fs::canonicalize(&src) {
			if resolved == dest_path {
				let _ = writeln!(
					w,
					"{} Skipping {}: resolves to destination (loop)",
					term_yellow("!"),
					tool
				);
				continue;
			}
		}
		plan.push(PlannedLink {
			tool,
			src,
			dest: dest_path,
		});
	}

	if plan.is_empty() {
		let _ = writeln!(
			w,
			"{} No new tools found to link. Everything up to date.",
			term_green("✓")
		);
		return Ok(());
	}

	let _ = writeln!(w, "\nPlan of execution:");
	for p in &plan {
		let _ = writeln!(
			w,
			"  {} {} -> {}",
			term_green("+"),
			term_cyan(p.tool),
			p.src.display()
		);
	}
	let _ = writeln!(w);

	let accepted = if auto_yes {
		plan
	} else {
		let prompt = stdin.unwrap_or("");
		let mut lines = prompt.lines();
		let head = lines.next().unwrap_or("").trim().to_lowercase();
		match head.as_str() {
			"n" | "no" => {
				let _ = writeln!(w, "Aborted.");
				return Ok(());
			}
			"c" | "choose" => {
				// Rebuild a slice starting at the second line so
				// `choose_per_tool` can iterate per-tool answers.
				let rest: String = lines.collect::<Vec<_>>().join("\n");
				let rest = if rest.is_empty() { "\n" } else { &rest };
				choose_per_tool(&plan, rest, w)?
			}
			_ => plan, // default yes
		}
	};

	let mut count = 0usize;
	for p in &accepted {
		let _ = write!(w, "  Linking {}... ", p.tool);
		match symlink(&p.src, &p.dest) {
			Ok(()) => {
				let _ = writeln!(w, "{}", term_green("✓"));
				count += 1;
			}
			Err(e) => {
				let _ = writeln!(w, "{} {}", term_red("✗"), e);
			}
		}
	}

	let _ = writeln!(
		w,
		"\n{} Linked {} tools to {}",
		term_green("✓"),
		count,
		dest_dir.display()
	);

	if !system_mode {
		if let (Ok(path), Ok(home)) = (std::env::var("PATH"), std::env::var("HOME")) {
			let local_bin = PathBuf::from(&home).join(".local").join("bin");
			if !path
				.split(':')
				.any(|p| p == local_bin.display().to_string())
			{
				let _ = writeln!(
					w,
					"\n{} Add {} to your PATH:",
					term_yellow("!"),
					local_bin.display()
				);
				let _ = writeln!(
					w,
					"  echo 'export PATH=\"{}{}:$PATH\"' >> ~/.bashrc",
					local_bin.display(),
					if path.is_empty() { "" } else { ":" }
				);
			}
		}
	}

	Ok(())
}

struct PlannedLink<'a> {
	tool: &'a str,
	src: PathBuf,
	dest: PathBuf,
}

fn choose_per_tool<'a, W: Write>(
	plan: &'a [PlannedLink<'a>],
	stdin: &str,
	w: &mut W,
) -> Result<Vec<PlannedLink<'a>>, Box<dyn std::error::Error>> {
	let mut lines = stdin.lines();
	// The first line was already consumed at the top-level prompt;
	// here we read per-tool answers only.
	let mut accepted: Vec<PlannedLink<'a>> = Vec::new();
	for p in plan {
		let _ = write!(
			w,
			"Link {} ({})? [Y/n] ",
			term_cyan(p.tool),
			p.src.display()
		);
		let ans = lines.next().unwrap_or("").trim().to_lowercase();
		if ans != "n" && ans != "no" {
			accepted.push(PlannedLink {
				tool: p.tool,
				src: p.src.clone(),
				dest: p.dest.clone(),
			});
		}
	}
	if accepted.is_empty() {
		let _ = writeln!(w, "Nothing selected. Aborting.");
	}
	Ok(accepted)
}

/// Run a command as another user via `runuser -l USER -c which TOOL`.
/// Best-effort: the Go test treats failures as "tool not found".
fn find_via_runuser(user: &str, tool: &str) -> Option<PathBuf> {
	let out = std::process::Command::new("runuser")
		.args(["-l", user, "-c", &format!("which {tool}")])
		.output()
		.ok()?;
	if !out.status.success() {
		return None;
	}
	let path = String::from_utf8_lossy(&out.stdout).trim().to_string();
	if path.is_empty() || !Path::new(&path).is_absolute() {
		return None;
	}
	Some(PathBuf::from(path))
}

/// Tiny PATH-driven `which` replacement.
fn which(tool: &str) -> Option<PathBuf> {
	let path = std::env::var_os("PATH")?;
	for dir in std::env::split_paths(&path) {
		let candidate = dir.join(tool);
		if candidate.is_file() {
			return Some(candidate);
		}
	}
	None
}

fn dirs_home() -> Option<String> {
	std::env::var("HOME").ok().filter(|v| !v.is_empty())
}

// Tiny ANSI helpers so we don't pull a dependency on the global
// `term` module's quiet gate (the install-tools output is interactive
// and must always show).

fn term_red(s: &str) -> String {
	crate::term::red(format_args!("{}", s))
}
fn term_green(s: &str) -> String {
	crate::term::green(format_args!("{}", s))
}
fn term_yellow(s: &str) -> String {
	crate::term::yellow(format_args!("{}", s))
}
fn term_cyan(s: &str) -> String {
	crate::term::cyan(format_args!("{}", s))
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "install-tools".to_string(),
		aliases: Vec::new(),
		usage: "unitpm install-tools [options]".to_string(),
		description: "Symlink common dev tools (bun, node, go, etc.) into ~/.local/bin so the unitpm daemon can find them. Use --system for a system-wide install.".to_string(),
		options: vec![
			crate::cli::help::Option {
				short: String::new(),
				long: "--system".to_string(),
				description: "Install to /usr/local/bin instead (requires sudo)".to_string(),
			},
			crate::cli::help::Option {
				short: "-y".to_string(),
				long: "--yes".to_string(),
				description: "Automatically confirm all prompts".to_string(),
			},
			crate::cli::help::Option {
				short: "-h".to_string(),
				long: "--help".to_string(),
				description: "Show this help message".to_string(),
			},
		],
		examples: Vec::new(),
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
