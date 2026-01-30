# Lynx

**The Secure, Systemd-Native Process Manager for Linux.**

Lynx is a lightweight, secure alternative to PM2 or Supervisor, designed specifically for Debian/Ubuntu systems. It leverages `systemd` for robust process supervision while providing a developer-friendly CLI for easy management.

## Prerequisites

*   **OS**: Linux (Debian/Ubuntu recommended). Windows/macOS not supported.
*   **Go**: Version 1.25.6+ (as defined in `go.mod`).

## Quickstart

### 1. Install Go (Debian/Ubuntu)
If you don't have Go installed, use the official tarball (replace version with latest stable if needed):

```bash
# Download Go
wget https://go.dev/dl/go1.25.6.linux-amd64.tar.gz

# Install to /usr/local
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.25.6.linux-amd64.tar.gz

# Add to PATH
export PATH=$PATH:/usr/local/go/bin

# Verify
go version
```

### 2. Build from Source
```bash
# Clone repository
git clone https://github.com/Jaro-c/Lynx.git
cd Lynx

# Build binaries
go build -v ./cmd/lynx ./cmd/lynxd
```

### 3. Build & Install Debian Package (Recommended)
This creates a native `.deb` package and integrates with systemd.

```bash
# 1. Install build dependencies
sudo apt-get update
sudo apt-get install -y build-essential debhelper

# 2. Build the package
dpkg-buildpackage -us -uc -b

# 3. Install the generated package
sudo dpkg -i ../lynx_*.deb

# 4. Enable and start the daemon
sudo systemctl enable --now lynx.lynxd
```

### 4. Usage
```bash
# Start an application
lynx start app.js --name my-api --restart always

# Start with DynamicUser isolation (Secure)
lynx start app.js --isolation dynamic

# List running processes
lynx list

# Stop a process
lynx stop my-api

# Check daemon logs
journalctl -u lynx.lynxd -f
```

## Why Lynx?

*   **Systemd-First**: Doesn't reinvent the wheel. The daemon integrates natively with Linux init systems for robust reliability.
*   **Secure Defaults**: Strictly controls permissions. Spec files are 0600, no implicit `sudo`, and rigorous path validation prevents traversal attacks.
*   **Declarative Specs**: Every process is defined by a JSON specification stored in `~/.config/lynx/apps`, making it GitOps-friendly.
*   **Accurate Metrics**: Aggregates CPU and Memory usage for entire process trees using Cgroups V2 (where available).

## Access Model

Lynx supports two modes of operation:

### 1. System Mode (Default)
- **Daemon**: Runs as a system service (`lynx.lynxd`), managed by `systemd`.
- **User**: `lynx` (system user).
- **Socket**: `/run/lynx/lynx.sock`.
- **Permissions**: Restricted to `root` and members of the `lynxadm` group.
- **Use Case**: Production servers where a central daemon manages services.
- **Setup**: Add your user to the `lynxadm` group:
  ```bash
  sudo usermod -aG lynxadm $USER
  newgrp lynxadm
  ```

### 2. User Mode
- **Daemon**: Runs as a user service (`systemd --user`).
- **User**: The current logged-in user.
- **Socket**: `$XDG_RUNTIME_DIR/lynx/lynx.sock`.
- **Permissions**: Restricted to the owner (`0600`).
- **Use Case**: Development environments or per-user service management.

## Commands

| Command | Description | Documentation |
|---------|-------------|---------------|
| `start` | Start a new process with monitoring, scheduling, and restart policies. | [Docs](docs/commands/start.md) |
| `list` | List all managed processes with real-time status and metrics. | [Docs](docs/commands/list.md) |
| `stop` | Stop one or more running processes. | [Docs](docs/commands/stop.md) |
| `restart` | Restart one or more processes. | [Docs](docs/commands/restart.md) |
| `delete` | Delete one or more processes and their configurations. | [Docs](docs/commands/delete.md) |
| `startup` | Generate and install system startup scripts. | [Docs](docs/commands/startup.md) |
| `version` | Display CLI, Daemon, and Protocol version information. | [Docs](docs/commands/version.md) |
| `help` | Show help for any command. | [Docs](docs/commands/help.md) |

## Releases

Pre-built `.deb` packages for Debian/Ubuntu are available on the [GitHub Releases](https://github.com/Jaro-c/Lynx/releases) page.

## Development

**Note for Windows Developers**:
Since Lynx is Linux-only, we recommend using **VS Code Remote-WSL**.
If you are editing on Windows, you may see false positive errors (e.g., "build constraints exclude all Go files").
To fix this in your editor settings, set the environment variable:
`GOOS=linux`

## Packaging

Lynx is designed to be installed as a native Debian package. See the **Quickstart** section above for build instructions.
