//! Tests for the update command.
//!
//! 10 cases ported from `internal/cli/commands/update/{cmd_test.go,
//! find_deb_test.go}`.

use crate::cli::commands::update::{self, find_deb_asset};
use crate::updater::{Asset, Release};

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

#[test]
fn get_spec_matches_name() {
	let s = update::spec();
	assert_eq!(s.name, "update");
}

#[test]
fn run_invalid_flags_errors() {
	// The Go side reports a Go-flag-library "flag provided but not
	// defined" error which we don't reproduce under Rust's match-based
	// parser. We surface the rejection as an "Unknown flag" message
	// via the positional check.
	let _g = lock_term();
	let mut buf = Vec::new();
	// `--not-a-flag` doesn't match any recognised token so the parser
	// falls into the positional bucket and `Unexpected arguments` is
	// surfaced. The Go test fed the same shape and asserted on
	// "Unknown flag"; verify we reject the input regardless.
	let rc = update::run(&mut buf, &["--not-a-flag".to_string()]);
	let err = rc.expect_err("unknown flag must error");
	assert!(
		err.to_string().contains("Unknown flag")
			|| err.to_string().contains("Unexpected arguments"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	update::run(&mut buf, &["--help".to_string()]).expect("ok");
}

#[test]
fn run_unexpected_args_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = update::run(&mut buf, &["extra-positional-arg".to_string()]);
	let err = rc.expect_err("unexpected args");
	assert!(
		err.to_string().contains("Unexpected arguments"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_managed_apply_without_force_errors() {
	// Skip when the test binary isn't package-managed; the Go test
	// does the same.
	if !update::is_managed() {
		return;
	}
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = update::run(&mut buf, &["--apply".to_string()]);
	let err = rc.expect_err("managed apply must error");
	assert!(
		err.to_string().contains("system package manager"),
		"unexpected error: {err}"
	);
}

/// A package-managed install must refuse `--apply`, and an unmanaged one must
/// not.
///
/// Two tests stood here before and neither could fail on this machine. One
/// asserted that `mod.rs` *contained* the line `if apply && is_managed() &&
/// !force` — grepping your own source is not a behaviour test: it survives the
/// guard being moved after the network call, and it breaks on a reformat that
/// changes nothing. It broke on exactly that, when the verdict became an
/// argument. The other returned early unless the test binary itself was
/// package-managed, so it passed by doing nothing.
///
/// `run_with` takes the verdict now, so both sides are reachable anywhere.
#[test]
fn managed_install_refuses_apply_and_unmanaged_does_not() {
	let mut out = Vec::new();
	let err = update::run_with(&mut out, true, false, false, false, true)
		.expect_err("a package-managed apply must be refused");
	assert!(
		err.to_string().contains("system package manager"),
		"the refusal must say why, got: {err}"
	);

	// The accepting half. Without it a guard that refused every apply would
	// satisfy the assertion above and ship an update command that can never
	// update.
	let mut out2 = Vec::new();
	let refused = update::run_with(&mut out2, true, false, false, false, false)
		.err()
		.map(|e| e.to_string().contains("system package manager"))
		.unwrap_or(false);
	assert!(
		!refused,
		"an unmanaged install must not be refused for packaging reasons"
	);
}

/// `--force` is the documented override, so it has to work.
#[test]
fn force_overrides_the_package_manager_guard() {
	let mut out = Vec::new();
	let refused = update::run_with(&mut out, true, false, true, false, true)
		.err()
		.map(|e| e.to_string().contains("system package manager"))
		.unwrap_or(false);
	assert!(!refused, "--force must override the guard");
}

#[test]
fn run_quiet_silences() {
	let _g = crate::term::tests::lock_term();
	crate::term::set_quiet(true);
	let mut buf = Vec::new();
	// Network call to updater.check may fail; that's fine. We only
	// check that nothing was written to the buffer when quiet.
	let _ = update::run(&mut buf, &[]);
	assert_eq!(
		buf.len(),
		0,
		"quiet mode should silence stdout; got {:?}",
		String::from_utf8_lossy(&buf)
	);
}

#[test]
fn find_deb_asset_prefers_arch_match() {
	let arch = std::env::consts::ARCH;
	let release = Release {
		tag_name: "v0.1.0".into(),
		assets: vec![
			Asset {
				name: "unitpm_0.1.0_other.deb".to_string(),
				browser_download_url: "https://example/other.deb".into(),
			},
			Asset {
				name: format!("unitpm_0.1.0_{arch}.deb"),
				browser_download_url: "https://example/arch.deb".into(),
			},
			Asset {
				name: "unitpm_0.1.0_other2.deb".into(),
				browser_download_url: "https://example/other2.deb".into(),
			},
		],
		body: String::new(),
		html_url: String::new(),
	};
	let got = find_deb_asset(&release);
	assert_eq!(got.as_deref(), Some("https://example/arch.deb"));
}

#[test]
fn find_deb_asset_fallback_any_deb() {
	let release = Release {
		tag_name: "v0.1.0".into(),
		assets: vec![
			Asset {
				name: "unitpm_0.1.0_unknownarch.deb".into(),
				browser_download_url: "https://example/any.deb".into(),
			},
			Asset {
				name: "checksums.txt".into(),
				browser_download_url: "https://example/checksums.txt".into(),
			},
		],
		body: String::new(),
		html_url: String::new(),
	};
	let got = find_deb_asset(&release);
	let s = got.expect("expected .deb URL");
	assert!(s.ends_with(".deb"), "expected fallback .deb URL, got {s}");
}

#[test]
fn find_deb_asset_none_found() {
	let release = Release {
		tag_name: "v0.1.0".into(),
		assets: vec![
			Asset {
				name: "checksums.txt".into(),
				browser_download_url: "https://example/checksums.txt".into(),
			},
			Asset {
				name: "unitpm_0.1.0_amd64.tar.gz".into(),
				browser_download_url: "https://example/tarball".into(),
			},
		],
		body: String::new(),
		html_url: String::new(),
	};
	assert_eq!(find_deb_asset(&release), None);
}

#[test]
fn find_deb_asset_empty_assets() {
	let release = Release {
		tag_name: "v0.1.0".into(),
		assets: Vec::new(),
		body: String::new(),
		html_url: String::new(),
	};
	assert_eq!(find_deb_asset(&release), None);
}
