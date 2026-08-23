//! The `update` command.
//!
//! 10 cases ported from `internal/cli/commands/update/{cmd_test.go,
//! find_deb_test.go}`.
//!
//! Checks GitHub for a newer release, optionally applies it. The
//! self-update path lives in [`crate::updater`]; this module is the
//! CLI shell around it, plus the package-managed-install guard.
//!
//! **Security requirement (phase 5b couldn't cover):** when the
//! running binary is owned by `dpkg` / `rpm` / `pacman`, the `--apply`
//! path is refused. The guard lives at the top of [`run`] so that
//! removing it is observable — the test `run_managed_apply_without_force`
//! goes red if the check is bypassed.

use std::io::Write;

use crate::cli::help::CommandSpec;
use crate::term;
use crate::updater::{self, ApplyOptions, Release};
use crate::version;

/// Re-exported so tests can hit the predicate without going through the
/// full updater module path.
pub use crate::updater::is_managed_by_package_system as is_managed;

/// Run the `update` command. `apply` defaults to `false` (the Go flag
/// defaults to true, but the Rust port inverts the default per the
/// phase-6d plan: the user must opt in to actually overwrite the
/// binary).
pub fn run<W: Write>(w: &mut W, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	let mut apply = false;
	let mut check = true;
	let mut force = false;
	let mut insecure_skip_sig = false;
	let mut positional: Vec<String> = Vec::new();

	for a in args {
		match a.as_str() {
			"--apply" => {
				apply = true;
				check = false;
			}
			"--check" => check = true,
			"--force" => force = true,
			"--insecure-skip-signature" => insecure_skip_sig = true,
			_ => positional.push(a.clone()),
		}
	}

	if !positional.is_empty() {
		return Err(unexpected_args(&positional));
	}

	// Quiet: redirect progress to a sink so the caller's writer
	// receives nothing. Errors still bubble via the returned `Result`.
	if term::is_quiet() {
		let mut sink = Vec::new();
		return run_with(
			&mut sink,
			apply,
			check,
			force,
			insecure_skip_sig,
			is_managed(),
		);
	}

	run_with(w, apply, check, force, insecure_skip_sig, is_managed())
}

/// `managed` is the package-manager verdict, taken as an argument rather than
/// read here.
///
/// The guard below is the one thing in this command that must not regress, and
/// `is_managed()` reads the real dpkg/rpm state — so a test that let it call
/// out could only ever exercise whichever answer the machine happens to give.
/// Injecting it makes both sides reachable, which is the difference between a
/// test that pins behaviour and one that greps the source for a line of code.
pub(crate) fn run_with<O: Write>(
	out: &mut O,
	apply: bool,
	check: bool,
	force: bool,
	insecure_skip_sig: bool,
	managed: bool,
) -> Result<(), Box<dyn std::error::Error>> {
	let _ = check;

	// Security guard: refuse self-update on package-managed installs.
	// Removing this check would let `unitpm update --apply` clobber the
	// apt/rpm/pacman-managed binary, which is exactly the scenario the
	// Go side added this guard against.
	if apply && managed && !force {
		return Err(Box::<dyn std::error::Error>::from(
			"unitpm is managed by system package manager (dpkg). \
			 Please download the latest .deb release and install it using \
			 'sudo apt install ./unitpm_<version>_amd64.deb'. \
			 Use --force to override (not recommended)",
		));
	}

	let _ = writeln!(out, "Checking for updates...");

	let release = updater::check().map_err(|e| -> Box<dyn std::error::Error> {
		Box::<dyn std::error::Error>::from(format!("failed to check for updates: {e}"))
	})?;

	let Some(release) = release else {
		let _ = writeln!(
			out,
			"{} You are using the latest version ({})",
			term::green(format_args!("{}", "✓")),
			version::VERSION
		);
		return Ok(());
	};

	let _ = writeln!(
		out,
		"{} New version available: {}",
		term::yellow(format_args!("{}", "!")),
		term::bold(format_args!("{}", release.tag_name))
	);
	let _ = writeln!(out, "  Release notes: {}", release.html_url);

	if apply {
		let _ = writeln!(out, "Downloading and installing update...");
		updater::apply(
			&release,
			ApplyOptions {
				allow_unsigned: insecure_skip_sig,
			},
		)
		.map_err(|e| -> Box<dyn std::error::Error> {
			Box::<dyn std::error::Error>::from(format!("update failed: {e}"))
		})?;
		let _ = writeln!(
			out,
			"{} Successfully updated to {}",
			term::green(format_args!("{}", "✓")),
			release.tag_name
		);
		let _ = writeln!(
			out,
			"Please restart the daemon manually if needed: 'systemctl restart unitpmd' or 'unitpm reload'"
		);
		return Ok(());
	}

	if is_managed() {
		let deb_url = find_deb_asset(&release);
		if let Some(deb_url) = deb_url {
			let deb_file = deb_url.rsplit('/').next().unwrap_or(&deb_url);
			let _ = writeln!(out, "\nTo update, run:");
			let _ = writeln!(out, "  wget {}", deb_url);
			let _ = writeln!(out, "  sudo apt install ./{}", deb_file);
		} else {
			let _ = writeln!(
				out,
				"\nTo update, download the latest .deb release from {}",
				release.html_url
			);
			let _ = writeln!(out, "and run:\n  sudo apt install ./<downloaded_deb_file>");
		}
	} else {
		let _ = writeln!(out, "\nTo update, run:\n  unitpm update --apply");
	}
	Ok(())
}

fn unexpected_args(args: &[String]) -> Box<dyn std::error::Error> {
	let quoted: Vec<String> = args.iter().map(|s| format!("\"{s}\"")).collect();
	Box::<dyn std::error::Error>::from(format!("Unexpected arguments: {}", quoted.join(" ")))
}

/// Pick the asset whose name ends in `.deb` and contains the running
/// architecture. Falls back to any `.deb` when no arch match exists.
/// Mirrors `findDebAsset` from the Go side.
pub fn find_deb_asset(release: &Release) -> Option<String> {
	let arch = std::env::consts::ARCH;
	for asset in &release.assets {
		if asset.name.ends_with(".deb") && asset.name.contains(arch) {
			return Some(asset.browser_download_url.clone());
		}
	}
	for asset in &release.assets {
		if asset.name.ends_with(".deb") {
			return Some(asset.browser_download_url.clone());
		}
	}
	None
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "update".to_string(),
		aliases: vec!["upgrade".to_string()],
		usage: "unitpm update|upgrade [flags]".to_string(),
		description: "Check for updates and apply them.".to_string(),
		options: vec![
			crate::cli::help::Option {
				short: "-a".to_string(),
				long: "--apply".to_string(),
				description: "Download and apply the update.".to_string(),
			},
			crate::cli::help::Option {
				short: "-c".to_string(),
				long: "--check".to_string(),
				description: "Check for updates without applying (default).".to_string(),
			},
			crate::cli::help::Option {
				short: "-f".to_string(),
				long: "--force".to_string(),
				description: "Force update even if managed by system package manager.".to_string(),
			},
			crate::cli::help::Option {
				short: String::new(),
				long: "--insecure-skip-signature".to_string(),
				description: "Accept unsigned releases. Dangerous: skips integrity/authenticity verification.".to_string(),
			},
			crate::cli::help::Option {
				short: "-h".to_string(),
				long: "--help".to_string(),
				description: "Show this help message.".to_string(),
			},
		],
		examples: Vec::new(),
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
