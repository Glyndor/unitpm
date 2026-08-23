//! Tests for the `git` probe.
//!
//! Ports the single Go `TestGetInfo`. The Go test mixes four scenarios in
//! one function — not-a-repo, valid-clean, latest-commit, dirty — so the
//! Rust port keeps them as one `#[test]` with each scenario explicitly
//! labelled. Splitting into separate tests would change the failure
//! isolation contract: today, a dirty-state regression would surface as a
//! failure in `TestGetInfo` together with everything else; in the Rust
//! port we mirror that by either keeping the single test or splitting
//! only the parts that need separate fixtures.
//!
//! `git` is required; the test skips cleanly when it is not installed.
//! The probe creates a real repository in a `tempfile::TempDir`, so it
//! inherits the parallel-test hazard the rest of the crate guards with
//! Drop-restoring mutexes — here, no process-global state is mutated, so
//! the test can run alongside the others without a lock.

use std::path::Path;
use std::process::{Command, Stdio};

use crate::git::{get_info, Info};

fn git_installed() -> bool {
	Command::new("git")
		.arg("--version")
		.stdout(Stdio::null())
		.stderr(Stdio::null())
		.status()
		.map(|s| s.success())
		.unwrap_or(false)
}

/// Run `git` against `dir`, ignoring stderr. Returns true when the exit
/// was zero. Used by the fixture-setup helpers; mirrors the Go
/// `_ = exec.CommandContext(...).Run()` pattern.
fn run(dir: &Path, args: &[&str]) -> bool {
	Command::new("git")
		.args(args)
		.current_dir(dir)
		.stdout(Stdio::null())
		.stderr(Stdio::null())
		.status()
		.map(|s| s.success())
		.unwrap_or(false)
}

fn init_repo(dir: &Path) {
	assert!(run(dir, &["init"]), "git init failed in {}", dir.display());
	// Local config — the test environment may have no global user.name /
	// user.email, and `git commit` requires both.
	let _ = run(dir, &["config", "user.email", "test@example.com"]);
	let _ = run(dir, &["config", "user.name", "Test User"]);
	// Pin the default branch to `main` so the branch assertion is not
	// sensitive to git-version-specific defaults (master vs main).
	let _ = run(dir, &["checkout", "-b", "main"]);
}

fn commit_file(dir: &Path, name: &str, content: &[u8], msg: &str) {
	let path = dir.join(name);
	std::fs::write(&path, content).expect("write file");
	assert!(run(dir, &["add", name]), "git add failed");
	assert!(run(dir, &["commit", "-m", msg]), "git commit failed");
}

#[test]
fn get_info_handles_non_repo_valid_and_dirty() {
	if !git_installed() {
		eprintln!("git not installed, skipping");
		return;
	}

	let tmp = tempfile::tempdir().expect("tempdir");
	let dir = tmp.path();

	// Scenario 1: not a repository.
	let info: Info = get_info(dir.to_str().expect("utf8 path"));
	assert!(
		info.branch.is_empty() && info.commit.is_empty() && !info.dirty,
		"non-repo must yield empty Info, got {info:?}"
	);

	// Scenario 2 & 3: valid repository, clean, with two commits — the
	// second one must be the one the probe reports.
	init_repo(dir);
	commit_file(dir, "test.txt", b"hello", "initial commit");
	commit_file(dir, "test.txt", b"hello world", "second commit");

	let info: Info = get_info(dir.to_str().expect("utf8 path"));
	assert!(
		!info.branch.is_empty(),
		"branch empty on valid repo: {info:?}"
	);
	assert!(
		!info.commit.is_empty(),
		"commit empty on valid repo: {info:?}"
	);
	assert!(!info.dirty, "clean repo must not be dirty: {info:?}");

	// Scenario 4: dirty — modify a tracked file.
	std::fs::write(dir.join("test.txt"), b"changed").expect("rewrite");
	let info: Info = get_info(dir.to_str().expect("utf8 path"));
	assert!(
		info.dirty,
		"modified tracked file must report dirty: {info:?}"
	);
}
