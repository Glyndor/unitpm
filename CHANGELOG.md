# Changelog

All notable changes to this project will be documented in this file.

## [0.1.2] - 2026-01-29

### Features
- **start**: Added support for PM2-like flags: `--restart`, `--max-restarts`, `--backoff`, `--schedule`, `--log-dir`, `--stdout`, `--stderr`.
- **daemon**: Implemented restart policy (always, on-failure, never) and cron-based scheduling.
- **daemon**: Implemented log file management (stdout/stderr redirection) and directory isolation.
- **cli**: Standardized command documentation and improved help output.

## [0.1.1] - 2026-01-29

### Features
- **metrics**: Removed non-Linux shims and fixed build tags.
- **cli**: Show full IDs on start and added `--long` flag to list command.
- **docs**: Added examples and help command.

## [0.1.0] - 2025-01-29

### Features
- **ipc**: Switch JSON backend to `bytedance/sonic` via `internal/jsonx` wrapper for better performance.
- **spec**: Use `google/uuid` for robust AppSpec IDs.
- **daemon**: Allow root (uid 0) to manage system daemon via socket identity check.

### Build
- **debian**: Improved packaging with hardened systemd unit and correct postinst permissions.
- **debian**: Set up `postinst` to create `lynx` user and secure directories.

### Documentation
- Added command documentation in `docs/commands/`.
- Updated `AGENTS.md` with project workflow rules.
- Updated `README.md` with Commands and Packaging sections.
