# Lynx

**The Secure, Systemd-Native Process Manager for Linux.**

Lynx is a lightweight, secure alternative to PM2 or Supervisor, designed specifically for Debian/Ubuntu systems. It leverages `systemd` for robust process supervision while providing a developer-friendly CLI for easy management.

## Prerequisites

*   **OS**: Linux (Debian/Ubuntu recommended). Windows/macOS not supported.
*   **Go**: Version 1.25.6+ (as defined in `go.mod`).
*   **Path**: The `lynx` binary must be in the system `PATH` for the daemon to function correctly (required for internal helpers).

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
*   **Accurate Metrics**: Aggregates CPU and Memory usage for entire process trees by scanning `/proc` (Proctree) or Cgroups V2.

## Access Model

Lynx supports two modes of operation:

### 1. System Mode (Default)
- **Daemon**: Runs as a system service (`lynx.lynxd`), managed by `systemd`.
- **User**: `lynx` (system user).
- **Socket**: `/run/lynx/lynx.sock`.
- **Permissions**: Restricted to `root` and members of the `lynxadm` group (mode `0660`).
- **Environment**: Does **not** inherit system environment variables (to prevent leaking secrets). Whitelists safe variables (`PATH`, `LANG`, `XDG_*`, `LC_*`).
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
- **Environment**: Inherits the full user environment.
- **Use Case**: Development environments or per-user service management.

## Commands

| Command | Description | Documentation |
|---------|-------------|---------------|
| `start` | Start a new process with monitoring, scheduling, and restart policies. | [Docs](docs/commands/start.md) |
| `list` | List all managed processes with real-time status and metrics. | [Docs](docs/commands/list.md) |
| `logs` | View and follow process logs (stdout/stderr). | [Docs](docs/commands/logs.md) |
| `show` | Show detailed information about a process. | [Docs](docs/commands/show.md) |
| `stop` | Stop one or more running processes. | [Docs](docs/commands/stop.md) |
| `restart` | Restart one or more processes. | [Docs](docs/commands/restart.md) |
| `reload` | Reload process configuration and restart. | [Docs](docs/commands/reload.md) |
| `flush` | Truncate process log files. | [Docs](docs/commands/flush.md) |
| `delete` | Delete one or more processes and their configurations. | [Docs](docs/commands/delete.md) |
| `apply` | Apply a declarative Lynxfile.yml and start apps. | [Docs](docs/commands/apply.md) |
| `export` | Export a namespace to Lynxfile.yml. | [Docs](docs/commands/export.md) |
| `startup` | Enable system startup for the daemon (systemd). | [Docs](docs/commands/startup.md) |
| `version` | Display CLI, Daemon, and Protocol version information. | [Docs](docs/commands/version.md) |
| `update` | Check for updates and apply them. | [Docs](docs/commands/update.md) |
| `help` | Show help for any command. | [Docs](docs/commands/help.md) |

## Releases

Pre-built `.deb` packages for Debian/Ubuntu are available on the [GitHub Releases](https://github.com/Jaro-c/Lynx/releases) page.

## Installation (Debian/Ubuntu)

### Option A: Install Prebuilt .deb
```bash
# Download from Releases and install
sudo dpkg -i lynx_<version>_amd64.deb

# Add your user to the admin group for CLI access to the system daemon
sudo usermod -aG lynxadm $USER
newgrp lynxadm

# Enable and start the system daemon
sudo systemctl enable --now lynx.lynxd

# Verify the service and socket
systemctl status lynx.lynxd
ls -l /run/lynx/lynx.sock
```

### Option B: Build and Install from Source
```bash
# Dependencies
sudo apt-get update
sudo apt-get install -y build-essential debhelper

# Build package
dpkg-buildpackage -us -uc -b

# Install
sudo dpkg -i ../lynx_*.deb

# Enable daemon
sudo systemctl enable --now lynx.lynxd
```

### User Mode (Optional)
If you prefer per-user isolation without system-wide privileges:
```bash
# Start the daemon as your user (in a separate terminal)
systemd --user &
# Then use lynx with default user-mode socket ($XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock)
lynx list
```

## Deployment Guide (Debian/Ubuntu)

1. Install Lynx using either Option A or B above.
2. Add your user to `lynxadm` (system mode) and re-login or run `newgrp lynxadm`.
3. Verify daemon logs:
   ```bash
   journalctl -u lynx.lynxd -f
   ```
4. Start your application:
   ```bash
   lynx start app.js --name my-api --restart on-failure
   ```
5. Use secure isolation for production:
   ```bash
   lynx start app.js --name my-api --isolation dynamic --env-file .env
   ```
6. Inspect and follow logs:
   ```bash
   lynx logs my-api --follow
   ```
7. Manage lifecycle:
   ```bash
   lynx list
   lynx show my-api
   lynx reload my-api
   lynx restart my-api
   lynx stop my-api
   lynx delete --purge my-api
   ```

## Getting Started Tutorial

### 1. Install Lynx
Use the prebuilt `.deb` and enable `lynx.lynxd` as shown in the Installation section.

### 2. Start a simple app
```bash
lynx start "node server.js" --name hello --restart on-failure
```

### 3. Scale instances
```bash
lynx start "node server.js" --name hello --scale 3 --shell
```
Note: `--shell` enables variable expansion. Ensure each instance binds to a unique port.

### 4. Monitor and logs
```bash
lynx monit
lynx logs hello --follow
```

### 5. Secure isolation and env
```bash
echo "PORT=8080" > .env
lynx start server.js --name secure-hello --isolation dynamic --env-file .env
```

### 6. Export and re-apply configurations
```bash
lynx export --namespace default > Lynxfile.yml
lynx apply Lynxfile.yml
```

## Development

**Note for Windows Developers**:
Since Lynx is Linux-only, we recommend using **VS Code Remote-WSL**.
If you are editing on Windows, you may see false positive errors (e.g., "build constraints exclude all Go files").
To fix this in your editor settings, set the environment variable:
`GOOS=linux`

## Packaging

Lynx is designed to be installed as a native Debian package. See the **Quickstart** section above for build instructions.
