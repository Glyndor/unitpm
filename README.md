# Lynx

Lynx is a secure process manager for Debian/Ubuntu systems, designed as a systemd-friendly alternative to PM2 or Supervisor. It allows you to manage and supervise processes through a lightweight daemon and a command-line interface.

## Supported Platforms

*   **OS:** Linux (Debian/Ubuntu)
*   **Init System:** systemd

## Components

*   **lynxd**: The background daemon that supervises processes. It must be running for the CLI to work.
*   **lynx**: The command-line client used to interact with the daemon (start, stop, and list processes).

## Key Features

*   **Secure by Design:** Strictly controls process isolation and permissions. No `sudo` required for app management.
*   **App Spec v1:** JSON-based application specification storage using XDG standards.
*   **Stable Identifiers:** Processes are identified by stable UUID v4 identifiers.
*   **Systemd Integration:** Daemon runs as a proper systemd service.

## Quick Start

1.  **Start the daemon**
    If installed via package, the service should be running:
    ```bash
    systemctl status lynxd
    ```

    Or run manually for dev:
    ```bash
    ./lynxd
    ```

2.  **Manage processes**
    ```bash
    # Start a simple command
    lynx start --name "my-worker" --cwd ./worker -- node index.js

    # Check status
    lynx list
    ```

## Commands

For detailed documentation on each command, please refer to the links below:

*   [start](docs/commands/start.md) - Start a new process managed by Lynx.
*   [list](docs/commands/list.md) - List all processes managed by Lynx.
*   [startup](docs/commands/startup.md) - Generate and install the system startup script.
*   [version](docs/commands/version.md) - Show Lynx version information.

## Storage

Lynx stores application specifications in your user configuration directory (XDG_CONFIG_HOME):
*   **Linux:** `~/.config/lynx/apps/`

Files are stored securely with restricted permissions (0600).

## Packaging (Debian/Ubuntu)

Lynx is designed to be packaged as a `.deb` file.

### Build .deb Package

1.  **Install prerequisites:**
    ```bash
    sudo apt install build-essential debhelper golang-go
    ```

2.  **Build the package:**
    Run the following command in the project root:
    ```bash
    dpkg-buildpackage -us -uc -b
    ```

3.  **Install:**
    ```bash
    sudo dpkg -i ../lynx_*.deb
    ```

### Systemd Usage

The package installs a systemd unit `lynx.lynxd.service`.

*   **Start/Restart:** `sudo systemctl restart lynxd`
*   **Status:** `sudo systemctl status lynxd`
*   **Logs:** `journalctl -u lynxd -f`
