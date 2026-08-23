//! JSON-lines audit log for destructive daemon actions.
//!
//! Records every `start`, `stop`, `delete`, `reload`, `restart`, `reset` and
//! `flush` for compliance and post-mortem forensics. The log is **on in system
//! mode** (`/var/log/glyndor/unitpm/audit.log`) and **off in user mode**,
//! where the daemon is already scoped to a single user.
//!
//! Mirrors `internal/daemon/audit/audit.go`.

// The Go audit package conventionally names its source `audit.go`; the Rust
// port preserves that shape so the directory layout mirrors the Go tree
// one-to-one. The clippy `module-inception` lint is allow-listed for this
// file only.
#[allow(clippy::module_inception)]
mod audit;

pub use audit::{Event, Logger};
