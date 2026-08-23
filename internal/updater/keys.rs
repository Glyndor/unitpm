//! Release signing key and signature decoding.
//!
//! The release public key is an embedded constant. It is the trust anchor:
//! every signature is verified against THIS key, and never against anything
//! read from disk, an environment variable, or a remote source. If the build
//! does not carry a key, `apply` refuses every update.
//!
//! `ErrSignatureRequired` exists because a release without a signature must
//! be refused, not warned about. Every error path that refuses must refuse.

use std::fmt;

use base64::Engine;

use ed25519_dalek::{Signature, Verifier, VerifyingKey};

/// Embedded ed25519 public key used to verify release signatures.
///
/// Base64 (standard alphabet) encoding of the 32-byte public key. This value
/// is the trust anchor — it MUST NOT become configurable, an env var, or
/// something read from disk. If the build doesn't ship this constant, no
/// release will be accepted.
pub const RELEASE_PUBLIC_KEY_B64: &str = "HFv7vg5FCY7YyKUDbJhaQSfB9SboJGSblJtFbLmLHzM=";

/// Refusal sentinel: release is not signed, or no release key is configured.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ErrSignatureRequired;

impl fmt::Display for ErrSignatureRequired {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		f.write_str("update refused: release is not signed")
	}
}

impl std::error::Error for ErrSignatureRequired {}

/// Decode the embedded release public key.
pub fn load_release_public_key() -> Result<VerifyingKey, KeyError> {
	let raw = base64::engine::general_purpose::STANDARD
		.decode(RELEASE_PUBLIC_KEY_B64)
		.map_err(|e| KeyError::Decode(e.to_string()))?;
	let bytes: [u8; 32] = raw
		.as_slice()
		.try_into()
		.map_err(|_| KeyError::WrongSize(raw.len()))?;
	VerifyingKey::from_bytes(&bytes).map_err(|e| KeyError::Invalid(e.to_string()))
}

/// Errors raised by the key loader.
#[derive(Debug)]
pub enum KeyError {
	Decode(String),
	WrongSize(usize),
	Invalid(String),
}

impl fmt::Display for KeyError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			KeyError::Decode(e) => write!(f, "decode pubkey: {e}"),
			KeyError::WrongSize(got) => {
				write!(f, "pubkey wrong size: got {got}, want 32")
			}
			KeyError::Invalid(e) => write!(f, "pubkey invalid: {e}"),
		}
	}
}

impl std::error::Error for KeyError {}

/// Decode a downloaded signature. Accepts raw 64 bytes OR any of the four
/// base64 encodings (standard / standard-raw / URL / URL-raw). Mirrors the
/// four-encoding fallback in the Go `downloadSignature`.
pub fn decode_signature(raw: &[u8]) -> Result<[u8; 64], SigError> {
	if raw.len() == 64 {
		let mut out = [0u8; 64];
		out.copy_from_slice(raw);
		return Ok(out);
	}
	for engine in [
		base64::engine::general_purpose::STANDARD,
		base64::engine::general_purpose::STANDARD_NO_PAD,
		base64::engine::general_purpose::URL_SAFE,
		base64::engine::general_purpose::URL_SAFE_NO_PAD,
	] {
		if let Ok(decoded) = engine.decode(raw) {
			if decoded.len() == 64 {
				let mut out = [0u8; 64];
				out.copy_from_slice(&decoded);
				return Ok(out);
			}
		}
	}
	Err(SigError::Malformed { bytes: raw.len() })
}

/// Errors raised by signature decoding.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum SigError {
	Malformed { bytes: usize },
}

impl fmt::Display for SigError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			SigError::Malformed { bytes } => {
				write!(f, "signature malformed: {bytes} bytes")
			}
		}
	}
}

impl std::error::Error for SigError {}

/// Verify an ed25519 signature over `body` against `key`. A `false` return
/// is a refusal — it must propagate up and stop the update.
pub fn verify_signature(key: &VerifyingKey, body: &[u8], sig_bytes: &[u8; 64]) -> bool {
	let sig = match Signature::from_slice(sig_bytes) {
		Ok(s) => s,
		Err(_) => return false,
	};
	key.verify(body, &sig).is_ok()
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn load_release_public_key_succeeds() {
		let key = load_release_public_key().expect("key");
		// Sanity: the key bytes round-trip through b64.
		let raw = base64::engine::general_purpose::STANDARD
			.decode(RELEASE_PUBLIC_KEY_B64)
			.expect("b64");
		let expected =
			VerifyingKey::from_bytes(raw.as_slice().try_into().unwrap()).expect("expected");
		assert_eq!(key.as_bytes(), expected.as_bytes());
	}
}
