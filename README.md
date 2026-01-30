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
# Download Go (example version)
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz

# Install to /usr/local
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz

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

# List running processes
lynx list

# Check daemon logs
journalctl -u lynx.lynxd -f
```

## Why Lynx?

*   **Systemd-First**: Doesn't reinvent the wheel. The daemon integrates natively with Linux init systems for robust reliability.
*   **Secure Defaults**: Strictly controls permissions. Spec files are 0600, no implicit `sudo`, and rigorous path validation prevents traversal attacks.
*   **Declarative Specs**: Every process is defined by a JSON specification stored in `~/.config/lynx/apps`, making it GitOps-friendly.
*   **Accurate Metrics**: Aggregates CPU and Memory usage for entire process trees using Cgroups V2 (where available).

## Commands

| Command | Description | Documentation |
|---------|-------------|---------------|
| `start` | Start a new process with monitoring, scheduling, and restart policies. | [Docs](docs/commands/start.md) |
| `list` | List all managed processes with real-time status and metrics. | [Docs](docs/commands/list.md) |
| `startup` | Generate and install system startup scripts. | [Docs](docs/commands/startup.md) |
| `version` | Display CLI, Daemon, and Protocol version information. | [Docs](docs/commands/version.md) |
| `help` | Show help for any command. | [Docs](docs/commands/help.md) |

## Packaging

Lynx is designed to be installed as a native Debian package. See the **Quickstart** section above for build instructions.

## Development

If you are developing on Windows, we recommend using **VS Code Remote-WSL**.
Since Lynx is Linux-only, Windows editors may show false positive errors (e.g., "build constraints exclude all Go files").
To fix this in your editor settings, set the environment variable:
`GOOS=linux`
