//! Injects the version, commit and build date the binaries report.
//!
//! The Go build passed these with `-ldflags`. Phase 1 ported the fields as
//! hard-coded strings and left this for later, which left two sources of
//! truth disagreeing: `Cargo.toml` said 0.0.0 while `version::VERSION` said
//! 0.13.1, and `unitpm version` reported the second.
//!
//! `CARGO_PKG_VERSION` is the manifest's, so the manifest is now the single
//! source. `UNITPM_COMMIT` and `UNITPM_BUILD_DATE` are optional: a packaging
//! build sets them, a developer build gets the same "none"/"unknown" the Go
//! side used when its ldflags were absent.

fn main() {
	println!("cargo:rerun-if-env-changed=UNITPM_COMMIT");
	println!("cargo:rerun-if-env-changed=UNITPM_BUILD_DATE");

	let commit = std::env::var("UNITPM_COMMIT").unwrap_or_else(|_| "none".into());
	let date = std::env::var("UNITPM_BUILD_DATE").unwrap_or_else(|_| "unknown".into());

	println!("cargo:rustc-env=UNITPM_COMMIT={commit}");
	println!("cargo:rustc-env=UNITPM_BUILD_DATE={date}");
}
