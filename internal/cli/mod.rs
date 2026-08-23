//! CLI-specific types.
//!
//! Phase 6a carries the CLI infrastructure packages. Phases 6b–6d add the
//! commands; 6d ported 14 of them.

pub mod batch;
pub mod commands;
pub mod errs;
pub mod expand;
pub mod format;
pub mod help;
pub mod registry;
pub mod root;
pub mod table;
