//! `unitpm` core library.
//!
//! Linux process manager — systemd-native PM2 alternative. This crate is the
//! Rust rewrite's foundation layer; each module here has a direct Go counterpart
//! under `internal/<name>` and ports the same test cases. The Go tree stays
//! buildable alongside this one until phase 7 deletes the old code.

pub mod cli;
pub mod daemon;
pub mod env;
pub mod git;
pub mod ipc;
pub mod jsonx;
pub mod manifest;
pub mod metrics;
pub mod paths;
pub mod spec;
pub mod term;
pub mod types;
pub mod updater;
pub mod version;
