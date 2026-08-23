//! Phase-6b lifecycle commands.
//!
//! Four commands live here: `list`, `start`, `stop`, `restart`. They are
//! one lane because `start`/`stop`/`restart` all import `list` for the
//! post-action table. The rest of the command tree (apply, completion,
//! delete, execenv, execsandbox, export, flush, installtools, logs,
//! monit, reload, reset, scale, show, startup, update, version) is
//! ported in phases 6c and 6d.

pub mod list;
pub mod restart;
pub mod start;
pub mod stop;
