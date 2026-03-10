# Changelog

All notable changes to this project will be documented in this file.

## [0.4.5] - 2026-03-10

### Features
- **env**: Implement robust custom environment variable parser.
  - Handles double and single quotes (`"`, `'`) correctly by stripping them from values.
  - Supports inline comments (`#`) and ignores them.
  - Fixes compatibility issues with Node.js/Next.js/Bun applications that failed when receiving quoted values from `.env` files.
- **cli**: Change default `start` logging mode from `inherit` to `file`.
  - New processes now automatically write to `stdout.log` and `stderr.log` in the standard log directory unless specified otherwise.
- **cli**: Allow spaces in process names (e.g., `lynx start --name "My App"`).
- **cli**: Add `log` as an alias for the `logs` command.

## [0.4.4] - 2026-03-10

### Features
- **cli**: Allow selective tool installation in `install-tools`.

## [0.4.3] - 2026-03-10

### Refactor
- **core**: Replace all occurrences of UUID v4 with UUID v7 for better sortability and time-ordered IDs.

## [0.4.2] - 2026-03-08

### Security
- **updater**: Fix `Chmod` called on closed file descriptor — binary updates now correctly receive executable permissions.
- **updater**: Add timeout (10 min) to download HTTP client to prevent indefinite hangs.
- **updater**: Add `io.LimitReader` (500MB) to prevent disk exhaustion from malicious responses.
- **updater**: Implement proper semver comparison to prevent accidental downgrades.

### Efficiency
- **daemon**: Eliminate unnecessary double mutex lock/unlock in restart handler.
- **daemon**: Replace O(n×m) environment whitelist loop with O(1) map lookup.
- **manager**: Remove `goto` in `ResolveID`, extract `resolveFromCandidates` helper.
- **paths**: Remove dead-code duplicate check in `withinRoot`.

### Chore
- Clean up stale binary artifacts from repository working directory.

## [0.4.1] - 2026-03-08

### Features
- **git**: Add support for Git metadata detection in managed applications.
- **cli**: Display Git branch, commit hash, and dirty status in `lynx list`.
- **daemon**: Capture Git metadata (branch/commit) at process startup time.

### Documentation
- **list**: Update documentation to include new Git information column and flags.

## [0.4.0] - 2026-02-27

### Bug Fixes
- **cli**: use correct systemd service name 'lynx.lynxd.service' in startup command.

## [0.3.9] - 2026-02-27

### Bug Fixes
- **systemd**: Fix service name mismatch (lynx.lynxd.service).
- **debian**: Ensure systemd service is correctly installed by debhelper.

## [0.3.8] - 2026-02-27

### Chore
- **paths**: rename system paths to /var/lib/lynx-pm and /var/log/lynx-pm to avoid conflicts with lynx browser package.

## [0.3.7] - 2026-02-27

### Features
- **isolation**: enhance dynamic mode with ProtectProc=invisible.

### Bug Fixes
- **daemon**: correctly detect privileged mode for handlers.

### Documentation
- **start**: add missing flags to help and documentation.
- **delete**: improve flag documentation format.

## [0.3.6] - 2026-02-25

### Bug Fixes
- **ipc**: allow non-root users in admin group to connect to daemon.
- **build**: disable git vcs stamping during debian rules compilation.

### Chore
- **build**: dynamically resolve wsl output directory in powershell script.

### Documentation
- **install**: rename downloaded deb file to lynxd.deb to prevent collisions.

## [0.3.0] - 2026-02-24

### Features
- **update**: Add `lynx update` command to check for and apply self-updates with GitHub releases integration.
- **logs**: Add `lynx logs` command with PM2-like output, log following, and centralized log paths.
- **namespaces**: Introduce namespace support to isolate applications.
- **config**: Add logrotate configuration integration for Lynx application logs.
- **community**: Add GitHub Sponsors funding configuration (`.github/FUNDING.yml`).
- **core**: Introduce new modules for log path resolution and application specification management.

### Security & Reliability
- **daemon**: Reset PID and ensure state consistency when processes are manually stopped out-of-band.
- **daemon**: Make `lynx stop` authoritative over automatic restart policies.
- **daemon**: Add robust state persistence and restore on startup.
- **security**: Improve environment handling, harden ID validations, and tighten system socket directory permissions.
- **build**: Resolve Linux build/test failures by heavily isolating lifecycle tests and removing Windows regressions.

### Refactor
- Update Go to 1.26.0 and adopt `yaml.v3`.
- Consolidate ignore patterns, fix extensive `golangci-lint` issues, and adopt best-practice `gosec` rules.

### Documentation
- Add comprehensive Godoc comments for all exported funcs and types.
- Add detailed CLI command manuals and update the `README.md` with PM2/Supervisor comparisons.
- Add comprehensive guide for building and releasing Ubuntu/Debian `.deb` packages via WSL.

## [0.2.0] - 2026-02-05

*(Internal unreleased bump or lost history, merging into 0.3.0)*

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
- Updated `AGENTS.md` with enhanced agent guidelines and project context.
- Updated `README.md` with Commands and Packaging sections.
