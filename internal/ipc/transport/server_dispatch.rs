//! Connection dispatch and response helpers for the IPC server.
//!
//! Extracted from [`crate::ipc::transport::server`] so the core server
//! surface stays small. Three entry points are used by the accept loop:
//! [`dispatch`] for standard requests, [`dispatch_start`] for the legacy
//! `start` envelope, and [`send_error`] for protocol-level error replies.

use std::io::Write;

use crate::ipc::protocol::{
	self, Error as ProtocolError, MismatchData, Request, Response, StartError, StartResponse,
	StartResponseData, STATUS_ERROR, STATUS_SUCCESS,
};
use crate::ipc::transport::server::{HandlersSnapshot, RequestContext, UniversalRequest};
use crate::jsonx;

/// Dispatch a standard (non-start) request.
pub(crate) fn dispatch(
	univ: &UniversalRequest,
	handlers: &HandlersSnapshot,
	identity: &crate::ipc::transport::Identity,
) -> Response {
	let req = Request {
		version: univ.version,
		id: univ.id.clone(),
		command: univ.command.clone(),
		params: univ.params.clone(),
	};

	let mut resp = Response {
		version: protocol::VERSION,
		id: req.id.clone(),
		status: STATUS_SUCCESS.into(),
		result: None,
		error: None,
	};

	if req.version != protocol::VERSION {
		resp.status = STATUS_ERROR.into();
		resp.error = Some(Box::new(ProtocolError {
			code: "PROTOCOL_MISMATCH".into(),
			message: format!(
				"Protocol mismatch: server v{}, client v{}",
				protocol::VERSION,
				req.version,
			),
			data: Some(json_or_value(MismatchData {
				supported: protocol::VERSION,
				received: req.version,
			})),
		}));
		return resp;
	}

	let handler = handlers.0.get(&req.command).cloned();
	let handler = match handler {
		Some(h) => h,
		None => {
			resp.status = STATUS_ERROR.into();
			resp.error = Some(Box::new(ProtocolError {
				code: "UNKNOWN_COMMAND".into(),
				message: "Command not found".into(),
				data: None,
			}));
			return resp;
		}
	};

	let ctx = RequestContext {
		identity: identity.clone(),
	};
	let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
		handler(ctx, req.params.clone().unwrap_or_default())
	}));
	match result {
		Ok(Ok(raw)) => {
			resp.status = STATUS_SUCCESS.into();
			resp.result = Some(raw);
		}
		Ok(Err(e)) => {
			resp.status = STATUS_ERROR.into();
			resp.error = Some(Box::new(ProtocolError {
				code: "INTERNAL_ERROR".into(),
				message: e,
				data: None,
			}));
		}
		Err(_) => {
			resp.status = STATUS_ERROR.into();
			resp.error = Some(Box::new(ProtocolError {
				code: "INTERNAL_ERROR".into(),
				message: "handler panicked".into(),
				data: None,
			}));
		}
	}
	resp
}

/// Dispatch a `start` payload.
pub(crate) fn dispatch_start(
	univ: &UniversalRequest,
	handlers: &HandlersSnapshot,
	identity: &crate::ipc::transport::Identity,
) -> StartResponse {
	let mut resp = StartResponse {
		protocol_version: protocol::VERSION,
		kind: "start_result".into(),
		request_id: univ.request_id.clone(),
		ok: false,
		data: None,
		error: None,
	};

	if univ.protocol_version != protocol::VERSION {
		resp.error = Some(Box::new(StartError {
			code: "PROTOCOL_MISMATCH".into(),
			message: format!(
				"Protocol mismatch: server v{}, client v{}",
				protocol::VERSION,
				univ.protocol_version,
			),
		}));
		return resp;
	}

	let handler = handlers.0.get("start").cloned();
	let handler = match handler {
		Some(h) => h,
		None => {
			resp.error = Some(Box::new(StartError {
				code: "UNKNOWN_COMMAND".into(),
				message: "Command start not found".into(),
			}));
			return resp;
		}
	};

	let spec_bytes = univ.spec.clone().unwrap_or_default();
	let app_spec: Result<protocol::AppSpec, _> = jsonx::unmarshal(spec_bytes.as_bytes());
	let ctx = RequestContext {
		identity: identity.clone(),
	};
	let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
		handler(ctx, spec_bytes.clone())
	}));

	match (result, app_spec) {
		(Ok(Ok(raw)), Ok(_spec)) => {
			let data: Result<StartResponseData, _> = jsonx::unmarshal(raw.as_bytes());
			match data {
				Ok(d) => {
					resp.ok = true;
					resp.data = Some(Box::new(d));
				}
				Err(_) => {
					resp.error = Some(Box::new(StartError {
						code: "INTERNAL_ERROR".into(),
						message: "Failed to encode response data".into(),
					}));
				}
			}
		}
		(Ok(Err(e)), _) => {
			resp.error = Some(Box::new(StartError {
				code: "INTERNAL_ERROR".into(),
				message: e,
			}));
		}
		(Err(_), _) => {
			resp.error = Some(Box::new(StartError {
				code: "INTERNAL_ERROR".into(),
				message: "handler panicked".into(),
			}));
		}
		(_, Err(_)) => {
			resp.error = Some(Box::new(StartError {
				code: "INTERNAL_ERROR".into(),
				message: "Failed to decode spec".into(),
			}));
		}
	}
	resp
}

/// Send a structured error response on `writer`. Used when the request body
/// could not even be parsed.
pub(crate) fn send_error<W: Write>(writer: &mut W, req_id: &str, code: &str, message: &str) {
	let resp = Response {
		version: protocol::VERSION,
		id: req_id.to_string(),
		status: STATUS_ERROR.into(),
		result: None,
		error: Some(Box::new(ProtocolError {
			code: code.to_string(),
			message: message.to_string(),
			data: None,
		})),
	};
	if let Ok(bytes) = jsonx::marshal(&resp) {
		let _ = writer.write_all(&bytes);
		let _ = writer.write_all(b"\n");
		let _ = writer.flush();
	}
}

fn json_or_value<T: serde::Serialize>(v: T) -> serde_json::Value {
	serde_json::to_value(v).unwrap_or(serde_json::Value::Null)
}
