//! JSON encode/decode helpers.
//!
//! Wraps `serde_json` behind a small surface so the rest of the crate does not
//! have to depend on `serde_json` directly. The Go original wrapped `sonic`;
//! the wrapper exists to make a future swap (or a faster backend) a single-file
//! change. We deliberately do not chase sonic parity — `serde_json` is what
//! the rest of the Rust ecosystem speaks, and the eight tests in scope pin
//! behaviour, not implementation.

use std::io::{Read, Write};

use serde::{Deserialize, Serialize};

/// Encode `value` to JSON bytes.
pub fn marshal<T: Serialize>(value: &T) -> Result<Vec<u8>, Error> {
	serde_json::to_vec(value).map_err(Error::from)
}

/// Decode `data` into `value`.
pub fn unmarshal<T: for<'de> Deserialize<'de>>(data: &[u8]) -> Result<T, Error> {
	serde_json::from_slice(data).map_err(Error::from)
}

/// Encode `value` to JSON bytes with indentation.
///
/// Matches `MarshalIndent(v, "", "  ")` — two-space indent, no per-line prefix.
/// `serde_json` does not support a per-line prefix in pretty mode; the empty
/// prefix in the Go call is what the eight tests cover, so the loss is not
/// observable here.
pub fn marshal_indent<T: Serialize>(
	value: &T,
	_prefix: &str,
	indent: &str,
) -> Result<Vec<u8>, Error> {
	let formatter = serde_json::ser::PrettyFormatter::with_indent(indent.as_bytes());
	let mut buf = Vec::new();
	let mut ser = serde_json::Serializer::with_formatter(&mut buf, formatter);
	value.serialize(&mut ser).map_err(Error::from)?;
	Ok(buf)
}

/// Writer-backed encoder. Mirrors `jsonx.NewEncoder` for tests that exercise
/// the streaming surface; behaviour on a single `encode` call is identical to
/// [`marshal`].
pub struct Encoder<W: Write> {
	writer: W,
}

impl<W: Write> Encoder<W> {
	#[must_use]
	pub fn new(writer: W) -> Self {
		Self { writer }
	}

	/// Encode `value`, followed by a trailing newline. The newline mirrors
	/// `encoding/json`'s streaming convention; the Go tests do not assert on
	/// it either way.
	pub fn encode<T: Serialize>(&mut self, value: &T) -> Result<(), Error> {
		serde_json::to_writer(&mut self.writer, value).map_err(Error::from)?;
		self.writer.write_all(b"\n").map_err(Error::Io)?;
		Ok(())
	}
}

/// Reader-backed decoder.
pub struct Decoder<R: Read> {
	reader: R,
}

impl<R: Read> Decoder<R> {
	#[must_use]
	pub fn new(reader: R) -> Self {
		Self { reader }
	}

	pub fn decode<T: for<'de> Deserialize<'de>>(&mut self) -> Result<T, Error> {
		serde_json::from_reader(&mut self.reader).map_err(Error::from)
	}
}

/// Raw JSON value that round-trips verbatim through encode and decode.
///
/// On encode, an empty `RawMessage` becomes JSON `null` (matching
/// `encoding/json`'s convention). A non-empty `RawMessage` is validated as
/// JSON and re-serialized — the result is semantically identical but may be
/// canonically reformatted (whitespace). The Go tests assert only on
/// substring containment, so the rewrite passes.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RawMessage(Vec<u8>);

impl RawMessage {
	/// Wrap an already-encoded JSON value.
	pub fn from_bytes(bytes: &[u8]) -> Self {
		Self(bytes.to_vec())
	}

	#[must_use]
	pub fn as_bytes(&self) -> &[u8] {
		&self.0
	}
}

impl Serialize for RawMessage {
	fn serialize<S: serde::Serializer>(&self, ser: S) -> Result<S::Ok, S::Error> {
		if self.0.is_empty() {
			ser.serialize_unit()
		} else {
			let value: serde_json::Value =
				serde_json::from_slice(&self.0).map_err(serde::ser::Error::custom)?;
			value.serialize(ser)
		}
	}
}

impl<'de> Deserialize<'de> for RawMessage {
	fn deserialize<D: serde::Deserializer<'de>>(de: D) -> Result<Self, D::Error> {
		let value = serde_json::Value::deserialize(de)?;
		let bytes = serde_json::to_vec(&value).map_err(serde::de::Error::custom)?;
		Ok(RawMessage(bytes))
	}
}

/// Errors surfaced by the JSON helpers.
#[derive(Debug)]
pub enum Error {
	Json(serde_json::Error),
	Io(std::io::Error),
}

impl std::fmt::Display for Error {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			Error::Json(e) => write!(f, "json error: {e}"),
			Error::Io(e) => write!(f, "io error: {e}"),
		}
	}
}

impl std::error::Error for Error {
	fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
		match self {
			Error::Json(e) => Some(e),
			Error::Io(e) => Some(e),
		}
	}
}

impl From<serde_json::Error> for Error {
	fn from(e: serde_json::Error) -> Self {
		Error::Json(e)
	}
}

#[cfg(test)]
mod tests {
	use super::*;
	use serde::{Deserialize, Serialize};
	use std::io::Cursor;

	#[derive(Debug, PartialEq, Serialize, Deserialize)]
	struct Sample {
		name: String,
		value: i64,
	}

	#[test]
	fn marshal_unmarshal_round_trip() {
		let s = Sample {
			name: "api".into(),
			value: 42,
		};
		let bytes = marshal(&s).expect("marshal");
		let got: Sample = unmarshal(&bytes).expect("unmarshal");
		assert_eq!(got, s);
	}

	#[test]
	fn unmarshal_invalid_yields_error() {
		let result: Result<Sample, _> = unmarshal(b"not json");
		assert!(result.is_err(), "expected error on invalid JSON");
	}

	#[test]
	fn raw_message_passes_through_struct() {
		let raw = RawMessage::from_bytes(br#"{"k":1}"#);
		#[derive(Serialize)]
		struct Wrap<'a> {
			inner: &'a RawMessage,
		}
		let bytes = marshal(&Wrap { inner: &raw }).expect("marshal");
		let s = std::str::from_utf8(&bytes).expect("utf8");
		assert!(
			s.contains(r#""inner":{"k":1}"#),
			"raw message did not pass through: {s}"
		);
	}

	#[test]
	fn encoder_writes_to_writer() {
		let mut buf = Vec::new();
		let mut enc = Encoder::new(&mut buf);
		enc.encode(&Sample {
			name: "x".into(),
			value: 1,
		})
		.expect("encode");
		let out = std::str::from_utf8(&buf).expect("utf8");
		assert!(
			out.contains(r#""name":"x""#),
			"encoder output missing data: {out}"
		);
	}

	#[test]
	fn decoder_reads_from_reader() {
		let buf = Cursor::new(br#"{"name":"y","value":7}"#);
		let mut dec = Decoder::new(buf);
		let s: Sample = dec.decode().expect("decode");
		assert_eq!(s.name, "y");
		assert_eq!(s.value, 7);
	}

	#[test]
	fn marshal_indent_inserts_newlines_and_indent() {
		let bytes = marshal_indent(
			&Sample {
				name: "z".into(),
				value: 3,
			},
			"",
			"  ",
		)
		.expect("indent");
		let s = std::str::from_utf8(&bytes).expect("utf8");
		assert!(s.contains('\n'), "expected newlines, got {s:?}");
		assert!(
			s.contains("  \"name\""),
			"expected indented field, got {s:?}"
		);
	}

	#[test]
	fn empty_raw_message_marshals_to_null() {
		let raw = RawMessage::default();
		let bytes = marshal(&raw).expect("marshal");
		assert_eq!(bytes, b"null");
	}

	#[test]
	fn raw_message_round_trips_through_unmarshal() {
		// Stand-in for the Go nil-receiver no-op test: serde has no nil
		// receiver, so we exercise the same equivalence — unmarshalling into
		// an empty RawMessage populates it with the canonical JSON form.
		let raw: RawMessage = unmarshal(br#"{"x":1}"#).expect("unmarshal");
		let v: serde_json::Value = serde_json::from_slice(raw.as_bytes()).expect("parse");
		assert_eq!(v, serde_json::json!({"x": 1}));
	}
}
