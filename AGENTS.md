# Lynx Contributor Guide

## 1. Project Overview
Lynx is a process manager for Debian/Ubuntu systems, designed as a secure, systemd-friendly alternative to PM2 or Supervisor. It consists of:
- **`lynx`**: CLI tool for user interaction.
- **`lynxd`**: Background daemon managing processes.

**Scope**: Linux-only (Debian/Ubuntu). Windows and macOS are not supported.

## 2. Architecture
- **Entry Points**: `cmd/lynx` (CLI), `cmd/lynxd` (Daemon).
- **IPC**: Communication uses Unix domain sockets (`AF_UNIX`).
- **State**:
  - **Specs**: Stored in `$XDG_CONFIG_HOME/lynx/apps` (or `~/.config/lynx/apps`).
  - **Socket**: Stored in `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock`.

## 3. Security Rules
- **No Sudo**: The application logic must never invoke `sudo`. Privileged operations (e.g., service installation) are handled by package managers or systemd.
- **Shell Execution**: Must be opt-in via `--shell`. Default execution is direct (`execve`).
- **Path Validation**: All user-provided paths must be validated to prevent traversal.
- **Secrets**: Do not persist full environment variables by default. Use `envFile` with restricted permissions (0600).
- **Permissions**:
  - Directories: `0700` (User only).
  - Files (Specs/Secrets): `0600` (User only).

## 4. Filesystem Conventions
Lynx follows standard Linux filesystem hierarchies:
- **User Config**: `~/.config/lynx`
- **System Config**: `/etc/lynx`
- **State/Data**: `/var/lib/lynx`
- **Logs**: `/var/log/lynx`
- **Runtime**: `/run/lynxd` (managed by systemd `RuntimeDirectory`)

## 5. Coding Standards
- **Formatting**: `gofmt -s -w .` and `goimports`.
- **Linting**: Must pass `golangci-lint run`.
- **Build Tags**: Use `//go:build linux` for platform-specific code.
- **Errors**: Use explicit error wrapping (`fmt.Errorf("...: %w", err)`).
- **Logging**: Use structured logging.

## 6. Testing
- Run unit tests: `go test ./...`
- Run linter: `golangci-lint run`

## 7. PR Checklist
- [ ] Code compiles on Linux (`GOOS=linux go build ./...`).
- [ ] `go test ./...` passes.
- [ ] `golangci-lint run` passes.
- [ ] No security regressions (check Security Rules).
- [ ] Documentation updated if behavior changes.

## 8. Versioning Policy
- **SemVer**: We follow Semantic Versioning (vX.Y.Z).
- **Source**: Version is defined in `internal/version/version.go`.

## 9. Commit Conventions
- **Format**: Conventional Commits (type(scope): description).
- **Types**: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `build`, `ci`.
- **Scopes**: `ipc`, `spec`, `debian`, `cli`, `daemon`, `deps`.

## 10. Release Process
1. Update version in `internal/version/version.go`.
2. Update `CHANGELOG.md` with a summary of changes.
3. Create git tag `vX.Y.Z`.
4. (Optional) Create GitHub release if tooling is configured.

## 11. Documentation Rules
- **Commands**: Any new CLI command must be added to `README.md` under "Commands".
- **Doc Files**: Each command must have its own doc file in `docs/commands/<command>.md` with synopsis, flags, and examples.
- **Linking**: `README.md` must link to these doc files.
