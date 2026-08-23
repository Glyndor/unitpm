#![no_main]
//! Fuzzes the manifest parser against arbitrary bytes.
//!
//! The manifest is the user-supplied input the daemon accepts to declare its
//! managed processes, so a malformed or hostile file must be rejected, never
//! panic the process. Parse errors and validation errors are expected; what
//! this catches is the parser or the converter crashing outright.

use libfuzzer_sys::fuzz_target;

use unitpm::manifest::ToAppSpecs;

fuzz_target!(|data: &[u8]| {
	if let Ok(file) = unitpm::manifest::parse(data) {
		// Conversion may legitimately return an error for invalid but
		// syntactically-valid configurations; we only care that it does
		// not panic.
		let _ = file.to_app_specs();
	}
});
