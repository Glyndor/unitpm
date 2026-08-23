//! Transport-wide limits.

/// Maximum number of concurrent connections the server accepts.
#[allow(non_upper_case_globals)]
pub const MaxConnections: usize = 100;
/// Per-request read timeout.
#[allow(non_upper_case_globals)]
pub const ReadTimeout: std::time::Duration = std::time::Duration::from_secs(5);
/// Per-request write timeout.
#[allow(non_upper_case_globals)]
pub const WriteTimeout: std::time::Duration = std::time::Duration::from_secs(5);
/// Hard ceiling on a single message. A peer sending more is rejected.
#[allow(non_upper_case_globals)]
pub const MaxMsgSize: usize = 1024 * 1024;
