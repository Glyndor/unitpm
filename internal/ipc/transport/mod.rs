//! IPC transport layer.
//!
//! The server listens on a Unix socket, authenticates the peer via
//! `SO_PEERCRED`, rate-limits it per UID, and bounds message size at
//! [`limits::MaxMsgSize`]. The client dials the socket and exchanges
//! newline-delimited JSON request/response envelopes.

mod client;
mod decode;
mod dial;
mod identity;
#[cfg(unix)]
mod identity_unix;
mod limits;
#[cfg(unix)]
mod listener_unix;
mod ratelimit;
mod server;
mod server_dispatch;
mod server_loop;
#[cfg(unix)]
mod socket_unix;

pub use client::{Client, IPCClient, TransportError};
pub use decode::{new_response_decoder, ResponseDecoder};
pub use identity::{ContextKeyIdentity, Identity};
pub use limits::{MaxConnections, MaxMsgSize, ReadTimeout, WriteTimeout};
pub use ratelimit::{new_rate_limiter, RateLimiter};
pub use server::{CommandHandler, RequestContext, Server, UniversalRequest};

#[cfg(unix)]
pub use identity_unix::{validate_identity, IdentityError};
#[cfg(unix)]
pub use listener_unix::listen;
#[cfg(unix)]
pub use socket_unix::{get_socket_path, SocketPathError};

#[cfg(all(test, target_os = "linux"))]
mod server_tests;
#[cfg(all(test, target_os = "linux"))]
mod server_tests_security;
