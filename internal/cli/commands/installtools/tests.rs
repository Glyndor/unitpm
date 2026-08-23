//! Tests for the install-tools command.
//!
//! 9 cases ported from `internal/cli/commands/installtools/cmd_test.go`.

use std::os::unix::fs::symlink;

use crate::cli::commands::installtools;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn get_spec_has_system_flag() {
	let s = installtools::spec();
	assert_eq!(s.name, "install-tools");
	assert!(!s.description.is_empty(), "description must be non-empty");
	let has_system = s.options.iter().any(|o| o.long.contains("--system"));
	assert!(has_system, "--system flag must be in options");
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	installtools::run(&mut buf, &["--help".to_string()], None).expect("ok");
}

#[test]
fn run_system_without_root_errors() {
	let _g = lock_term();
	if std::process::Command::new("id")
		.args(["-u"])
		.output()
		.map(|o| String::from_utf8_lossy(&o.stdout).trim() == "0")
		.unwrap_or(false)
	{
		// skip when running as root; the Go test does the same.
		return;
	}
	let mut buf = Vec::new();
	let rc = installtools::run(&mut buf, &["--system".to_string(), "-y".to_string()], None);
	let err = rc.expect_err("--system without root must error");
	assert!(
		err.to_string().contains("requires root"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_user_mode_creates_local_bin() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let mut buf = Vec::new();
	let rc = installtools::run(&mut buf, &["-y".to_string()], None);
	rc.expect("user mode ok");
	let local_bin = home.path().join(".local").join("bin");
	assert!(
		std::path::Path::new(&local_bin).is_dir(),
		"~/.local/bin should exist"
	);
}

#[test]
fn run_user_mode_long_yes() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let mut buf = Vec::new();
	installtools::run(&mut buf, &["--yes".to_string()], None).expect("ok");
}

/// Stage fake tools on PATH so the planner finds candidates.
fn stage_fake_tools() -> Option<tempfile::TempDir> {
	let tmp = tempfile::tempdir().ok()?;
	let src = "/bin/true";
	for name in ["bun", "node", "python3"] {
		let dst = tmp.path().join(name);
		if symlink(src, &dst).is_err() {
			return None;
		}
	}
	let cur_path = std::env::var_os("PATH").unwrap_or_default();
	let new_path = std::env::join_paths(
		std::iter::once(tmp.path().to_path_buf()).chain(std::env::split_paths(&cur_path)),
	)
	.ok()?;
	std::env::set_var("PATH", new_path);
	Some(tmp)
}

#[test]
fn run_user_mode_links_tools() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let _stage = stage_fake_tools();
	let mut buf = Vec::new();
	installtools::run(&mut buf, &["-y".to_string()], None).expect("ok");
	for name in ["bun", "node", "python3"] {
		let link = home.path().join(".local").join("bin").join(name);
		let meta = std::fs::symlink_metadata(&link).expect("link meta");
		assert!(meta.file_type().is_symlink(), "{name} is not a symlink");
	}
}

#[test]
fn run_user_mode_prompt_deny() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let _stage = stage_fake_tools();
	let mut buf = Vec::new();
	installtools::run(&mut buf, &[], Some("n\n")).expect("ok");
	let entries = std::fs::read_dir(home.path().join(".local").join("bin"))
		.map(|rd| rd.count())
		.unwrap_or(0);
	assert_eq!(entries, 0, "expected no links after deny");
}

#[test]
fn run_user_mode_prompt_choose_all_no() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let _stage = stage_fake_tools();
	let mut buf = Vec::new();
	let stdin = format!("choose\n{}", "n\n".repeat(32));
	installtools::run(&mut buf, &[], Some(&stdin)).expect("ok");
	let entries = std::fs::read_dir(home.path().join(".local").join("bin"))
		.map(|rd| rd.count())
		.unwrap_or(0);
	assert_eq!(entries, 0, "expected no links after rejecting all");
}

#[test]
fn run_user_mode_prompt_default_yes() {
	let _g = lock_term();
	let home = tempfile::tempdir().expect("tempdir");
	std::env::set_var("HOME", home.path());
	let _stage = stage_fake_tools();
	let mut buf = Vec::new();
	installtools::run(&mut buf, &[], Some("\n")).expect("ok");
	let entries = std::fs::read_dir(home.path().join(".local").join("bin"))
		.map(|rd| rd.count())
		.unwrap_or(0);
	assert!(
		entries > 0,
		"expected default-yes prompt to create symlinks"
	);
}
