//! IPC protocol and transport.
//!
//! The daemon listens on a Unix socket, authenticates the peer via
//! `SO_PEERCRED`, rate-limits it per UID, and bounds message size at 1 MB.
//! The protocol package defines the wire format; the transport package wires
//! it to a streaming connection.

pub mod protocol;
pub mod transport;
