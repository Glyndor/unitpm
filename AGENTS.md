# Lynx Agent Guidelines & Project Context

This document provides critical context and operational rules for AI agents and developers working on the Lynx codebase.

## 1. System Context

**Lynx** is a secure, systemd-native process manager for Linux (Debian/Ubuntu).
It replaces PM2/Supervisor by leveraging `systemd` for process supervision while providing a modern CLI.

### Core Components
- **CLI (`cmd/lynx`)**: User interface. Communicates with daemon via Unix Socket.
- **Daemon (`cmd/lynxd`)**: Background service. Manages processes, logs, and monitoring.
- **IPC (`internal/ipc`)**: Custom JSON-over-Unix-Socket protocol.
- **State (`internal/spec`)**: JSON-based application specifications.

### Target Environment
- **OS**: Linux ONLY (Debian/Ubuntu focus).
- **No Windows/macOS Support**: Code using `syscall` or `golang.org/x/sys/unix` must be build-tagged `//go:build linux`.

## 2. Architectural Invariants

Agents must STRICTLY ADHERE to these rules. Violations will cause build failures or security breaches.

1.  **Zero-Sudo Policy**:
    - The `lynx` CLI and `lynxd` daemon logic must NEVER invoke `sudo` internally.
    - Privileged setup is handled by external package managers (`apt`, `dpkg`) or systemd units.

2.  **Filesystem Hierarchy**:
    - **User Config**: `$XDG_CONFIG_HOME/lynx` (`~/.config/lynx`)
    - **System Config**: `/etc/lynx`
    - **Runtime (Socket)**: `$XDG_RUNTIME_DIR/lynx-<uid>/` (User) or `/run/lynxd/` (System).
    - **Logs**: `$XDG_DATA_HOME/lynx/logs` or `/var/log/lynx-pm`.

3.  **Security Model**:
    - **Permissions**: All config/spec files MUST be `0600` (User Read/Write only).
    - **Socket**: `0600` (User) or `0660` (System/Group).
    - **Isolation**: Use `systemd-run --scope` or `DynamicUser=yes` for process isolation.

## 3. Codebase Map

| Path | Purpose | Key Packages |
|------|---------|--------------|
| `cmd/` | Entry points | `main` |
| `internal/cli/commands/` | CLI logic | `start`, `stop`, `list` |
| `internal/daemon/` | Daemon logic | `manager`, `handlers` |
| `internal/ipc/` | Communication | `transport`, `protocol` |
| `internal/spec/` | Data models | `AppSpec`, `JobSpec` |
| `debian/` | Packaging | `control`, `rules` |

## 4. Development Standards

### Code Style
- **Language**: Go 1.26+
- **Formatting**: Standard `gofmt`.
- **Errors**: Use `fmt.Errorf("context: %w", err)` for wrapping.
- **Logging**: Use structured logging (no `fmt.Println` in daemon).

### Testing
- **Unit Tests**: `go test ./...`
- **Integration**: Requires Linux environment. Mock `syscall` where possible.
- **Linting**: `golangci-lint` must pass.

## 5. Task Protocols

### Adding a CLI Command
1.  Create `internal/cli/commands/<name>/cmd.go`.
2.  Implement `cobra.Command`.
3.  Register in `internal/cli/root/root.go`.
4.  Add documentation in `docs/commands/<name>.md`.
5.  Update `README.md` command list.

### Modifying IPC
1.  Update `internal/ipc/protocol/types.go` with new request/response structs.
2.  Implement handler in `internal/daemon/handlers/`.
3.  Implement client method in `internal/ipc/transport/client.go`.

### Packaging
- Version is authoritative in `internal/version/version.go`.
- `debian/changelog` must match the version tag.

## 6. Agent Operational Constraints

- **Do NOT** suggest Windows-specific code.
- **Do NOT** modify `go.mod` unless explicitly requested.
- **ALWAYS** check for existing functions in `internal/` before writing helpers.
- **ALWAYS** run `go build ./...` (if environment permits) to verify syntax.
