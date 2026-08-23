#![no_main]
//! Fuzzes the updater's parsing primitives against arbitrary bytes.
//!
//! Release metadata comes off the network, so it is untrusted input. The
//! parser must reject any byte sequence without panicking — release tag
//! names are user-visible and could be crafted. Parse errors and validation
//! errors are expected; what this catches is the parser or the version
//! comparison crashing outright.

use libfuzzer_sys::fuzz_target;

use unitpm::updater;

fuzz_target!(|data: &[u8]| {
	if let Ok(s) = std::str::from_utf8(data) {
		// parse_version and is_newer are the public parsing primitives
		// exercised by release metadata. Both must be panic-free under
		// arbitrary input.
		let _ = updater::parse_version(s);
		let _ = updater::is_newer(s, s);
	}
	// decode_signature must also be panic-free; the signature is base64
	// (or raw 64 bytes), but the wire format is whatever the publisher
	// sent, and that publisher could be hostile.
	let _ = updater::decode_signature(data);
});
