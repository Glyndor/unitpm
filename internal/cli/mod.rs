//! CLI-specific types.
//!
//! Phase 6a carries the CLI infrastructure packages (no commands).
//! Phases 6b, 6c and 6d port the command tree in parallel: 6b the
//! lifecycle commands (`list`, `start`, `stop`, `restart`), 6c the
//! output-heavy ones (`logs`, `monit`, `show`), 6d the remaining fourteen.

pub mod batch;
pub mod commands;
pub mod errs;
pub mod expand;
pub mod format;
pub mod help;
pub mod registry;
pub mod root;
pub mod table;
