//! Proves the sandbox denies, rather than that the syscalls returned Ok.
//!
//! Every unit test around `landlock` stops short of `landlock_restrict_self`,
//! and for a good reason: the ruleset is **non-revocable**, so calling it
//! confines the test runner itself for the rest of its life. What the unit
//! tests can show is that the right syscall was assembled with the right
//! arguments. They cannot show that the kernel then refused a read.
//!
//! That gap is the whole risk of this module. A ruleset built with the wrong
//! bits does not error — it confines less than it claims to, and every unit
//! test still passes.
//!
//! So this test forks a child, confines *it*, and asks the kernel. The parent
//! is untouched.

use std::process::Command;

/// Set in the child so it takes the confined branch instead of re-spawning.
const CHILD_MARKER: &str = "UNITPM_LANDLOCK_CONFINE_CHILD";

#[test]
#[cfg_attr(not(target_os = "linux"), ignore = "landlock is a Linux interface")]
fn a_confined_child_cannot_read_outside_its_allowed_path() {
	if std::env::var_os(CHILD_MARKER).is_some() {
		child_body();
		return;
	}

	if !unitpm::daemon::runtime::landlock::supported() {
		eprintln!("landlock unsupported on this kernel; nothing to prove here");
		return;
	}

	let exe = std::env::current_exe().expect("current_exe");
	let out = Command::new(exe)
		.arg("--exact")
		.arg("a_confined_child_cannot_read_outside_its_allowed_path")
		.arg("--nocapture")
		.env(CHILD_MARKER, "1")
		.output()
		.expect("spawn confined child");

	let stdout = String::from_utf8_lossy(&out.stdout);
	eprintln!("--- child stdout ---\n{stdout}--- end ---");

	assert!(
		stdout.contains("ALLOWED-PATH-READABLE"),
		"the child could not read inside its own allowed path, so the run \
		 proves nothing about denial — it may have failed for another reason.\n{stdout}"
	);
	assert!(
		stdout.contains("OUTSIDE-PATH-DENIED"),
		"the child read a path the ruleset never allowed: the sandbox is not \
		 confining.\n{stdout}"
	);
}

fn child_body() {
	use unitpm::daemon::runtime::landlock::{apply, PathAccess, Ruleset};

	let dir = std::env::temp_dir().join(format!("unitpm-landlock-{}", std::process::id()));
	std::fs::create_dir_all(&dir).expect("create allowed dir");
	let inside = dir.join("inside.txt");
	std::fs::write(&inside, b"readable").expect("seed allowed file");

	let rs = Ruleset {
		allow: vec![PathAccess {
			path: dir.to_string_lossy().into_owned(),
			read: true,
			write: true,
			execute: false,
		}],
	};

	apply(&rs).expect("apply ruleset");

	// Inside the allow-list: must still work. Without this half, a ruleset that
	// denied everything would satisfy the denial assertion and prove nothing.
	if std::fs::read(&inside).is_ok() {
		println!("ALLOWED-PATH-READABLE");
	}

	// Outside it: the kernel must refuse. /etc/passwd is world-readable and was
	// readable in this same process a few lines ago.
	match std::fs::read("/etc/passwd") {
		Err(e) if e.kind() == std::io::ErrorKind::PermissionDenied => {
			println!("OUTSIDE-PATH-DENIED");
		}
		Err(e) => println!("OUTSIDE-PATH-OTHER-ERROR: {e}"),
		Ok(_) => println!("OUTSIDE-PATH-READ-SUCCEEDED"),
	}
}
