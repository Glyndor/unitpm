//! The command tree.
//!
//! Phases 6b, 6c and 6d ported these in parallel, in separate worktrees, so
//! this file is where the three lanes meet. Each lane declared its own
//! commands here and the merge unions them rather than choosing a side.

pub mod apply;
pub mod completion;
pub mod delete;
pub mod execenv;
pub mod execsandbox;
pub mod export;
pub mod flush;
pub mod installtools;
pub mod list;
pub mod logs;
pub mod monit;
pub mod reload;
pub mod reset;
pub mod restart;
pub mod scale;
pub mod show;
pub mod start;
pub mod startup;
pub mod stop;
pub mod update;
pub mod version;
