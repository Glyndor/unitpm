# Lynx

**The Secure, Systemd-Native Process Manager for Linux.**

Lynx is a lightweight, secure alternative to PM2 or Supervisor, designed specifically for Debian/Ubuntu systems. It leverages `systemd` for robust process supervision while providing a developer-friendly CLI for easy management.

## Quickstart

### 1. Build from Source
```bash
# Requires Go 1.22+
go build ./cmd/lynx ./cmd/lynxd
```

### 2. Install via .deb (Recommended)
```bash
# Build the package
dpkg-buildpackage -us -uc -b

# Install
sudo dpkg -i ../lynx_*.deb
```

### 3. Start the Daemon
```bash
# Start the system service
sudo systemctl enable --now lynx.lynxd

# Check status
systemctl status lynx.lynxd
```

### 4. Run Your First App
```bash
# Start a node app with restart policy
lynx start main.js --name my-api --restart always

# List running processes
lynx list
```

## Why Lynx?

*   **Systemd-First**: Doesn't reinvent the wheel. The daemon integrates natively with Linux init systems for rock-solid reliability.
*   **Secure Defaults**: Strictly controls permissions. Spec files are 0600, no implicit `sudo`, and rigorous path validation prevents traversal attacks.
*   **Declarative Specs**: Every process is defined by a JSON specification stored in `~/.config/lynx/apps`, making it GitOps-friendly.
*   **Accurate Metrics**: Aggregates CPU and Memory usage for entire process trees using Cgroups V2 (where available), ensuring no child process goes unnoticed.

## Commands

| Command | Description | Documentation |
|---------|-------------|---------------|
| `start` | Start a new process with monitoring, scheduling, and restart policies. | [Docs](docs/commands/start.md) |
| `list` | List all managed processes with real-time status and metrics. | [Docs](docs/commands/list.md) |
| `startup` | Generate and install system startup scripts. | [Docs](docs/commands/startup.md) |
| `version` | Display CLI, Daemon, and Protocol version information. | [Docs](docs/commands/version.md) |
| `help` | Show help for any command. | [Docs](docs/commands/help.md) |

## Packaging

Lynx is designed to be installed as a native Debian package.

**Build Requirements:**
```bash
sudo apt install build-essential debhelper golang-go
```

**Build & Install:**
```bash
# 1. Build package
dpkg-buildpackage -us -uc -b

# 2. Install
sudo dpkg -i ../lynx_*.deb
```

**Service Management:**
The package installs a systemd unit named `lynx.lynxd.service`.
*   **Logs**: `journalctl -u lynx.lynxd -f`
*   **Restart**: `sudo systemctl restart lynx.lynxd`
