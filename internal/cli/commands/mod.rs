//! The command tree.
//!
//! Phases 6b, 6c and 6d ported these in parallel, in separate worktrees, so
//! this file is where the three lanes meet. Each lane declared its own
//! commands here and the merge unions them rather than choosing a side.

pub mod list;
pub mod logs;
pub mod monit;
pub mod restart;
pub mod show;
pub mod start;
pub mod stop;
