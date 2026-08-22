//! Response decoder — a thin wrapper around the streaming [`crate::jsonx::Decoder`]
//! kept for parity with the Go `ResponseDecoder`. The trait object lets the
//! dispatcher swap encoders at the seams without dragging the generic into
//! every callsite.

use std::io::Read;

use crate::ipc::protocol;
use crate::jsonx;

/// Trait satisfied by anything that can decode a [`protocol::Response`] from a
/// stream. Mirrors `jsonx.Decoder` for the response envelope.
pub trait ResponseDecoder {
	fn decode_response(&mut self) -> Result<protocol::Response, jsonx::Error>;
}

struct JsonxDecoder<R: Read>(jsonx::Decoder<R>);

impl<R: Read + Send> ResponseDecoder for JsonxDecoder<R> {
	fn decode_response(&mut self) -> Result<protocol::Response, jsonx::Error> {
		self.0.decode()
	}
}

/// Build a decoder over `reader`.
pub fn new_response_decoder<R: Read + Send + 'static>(
	reader: R,
) -> Box<dyn ResponseDecoder + Send> {
	Box::new(JsonxDecoder(jsonx::Decoder::new(reader)))
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::ipc::protocol::Response;

	#[test]
	fn decode_round_trip() {
		let buf: &[u8] = br#"{"version":1,"id":"x","status":"success","result":{"id":"abc"}}"#;
		let mut dec = new_response_decoder(buf);
		let resp: Response = dec.decode_response().expect("decode");
		assert_eq!(resp.version, 1);
		assert_eq!(resp.id, "x");
		assert_eq!(resp.status, "success");
	}
}
