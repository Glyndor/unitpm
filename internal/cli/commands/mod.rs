//! CLI commands.
//!
//! Each sub-module here implements one CLI command. The phase-6d port
//! landed the 14 commands not covered by phases 6b/6c.

pub mod apply;
pub mod completion;
pub mod delete;
pub mod execenv;
pub mod execsandbox;
pub mod export;
pub mod flush;
pub mod installtools;
pub mod reload;
pub mod reset;
pub mod scale;
pub mod startup;
pub mod update;
pub mod version;
