//! CLI-specific types.
//!
//! Phase 6a carries the CLI infrastructure packages (no commands).
//! Phase 6b ports the lifecycle commands (`list`, `start`, `stop`,
//! `restart`); phases 6c and 6d land the remaining ones.

pub mod batch;
pub mod commands;
pub mod errs;
pub mod expand;
pub mod format;
pub mod help;
pub mod registry;
pub mod root;
pub mod table;
