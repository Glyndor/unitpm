# 🦁 Lynx

<div align="center">
  <h3>The Secure, Systemd-Native Process Manager for Linux</h3>
  <p>A lightning-fast, highly secure alternative to PM2 or Supervisor—built specifically for modern Debian/Ubuntu servers.</p>

  <img src="https://img.shields.io/badge/OS-Linux%20Only-informational?style=for-the-badge&logo=linux&color=2ecc71" alt="Linux Only" />
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/github/v/release/Jaro-c/Lynx?style=for-the-badge&color=ff69b4" alt="Release" />
</div>

---

## ✨ Why Lynx? (vs PM2 & Supervisor)

Stop wrestling with complex configurations, high RAM overhead, and insecure wrappers. Lynx gives you superpowers by natively combining the rock-solid reliability of `systemd` with a beautiful, modern CLI.

| Feature | 🦁 Lynx | 🐢 PM2 | 🦖 Supervisor |
| :--- | :--- | :--- | :--- |
| **Technology** | `Compiled Go` (Native) | `Node.js` (V8 Engine) | `Python` (Interpreted) |
| **Base RAM Overhead** | **~10 MB** ⚡ | ~60-100+ MB 🐌 | ~50+ MB 🐢 |
| **Daemon Engine** | **Native OS (`systemd`)** 🛡️ | Custom (PM2 Daemon) ❌ | Custom (supervisord) ❌ |
| **Crash Resilience** | Perfect (Apps outlive CLI) | Poor (Apps die with PM2) | Poor (Apps die with daemon) |
| **Sandboxing & Security**| **DynamicUser isolation** 🔒 | Root / User-space ⚠️ | Root / User-space ⚠️ |
| **Config Format** | `CLI` / `Lynxfile.yml` | `Ecosystem.config.js` | `ini` files |

---

## 🔒 The "Zero-Privilege" Deploy

The most powerful feature of Lynx. Start an API fully isolated: **no access to `/home`**, **no new privileges**, and secrets are passed securely via systemd without writing them to global disk variables.

```bash
lynx start api.js \
  --name max-security-api \
  --isolation dynamic \
  --env-file .env.production
```

It is impossible to achieve this level of security with one command in other managers.

---

## ⚡ 1-Minute Quickstart

Getting your app running has never been this easy.

### 1. Install the Pre-built Package
Forget about compiling source code. Just download the Debian package!

```bash
# Download the package (check the Releases tab for the latest version)
curl -L -o lynx.deb https://github.com/Jaro-c/Lynx/releases/latest/download/lynx_0.3.1-1_amd64.deb

# Install it
sudo apt install ./lynx.deb

# Add yourself to the lynx admin group & enable the backend engine
sudo usermod -aG lynxadm $USER
newgrp lynxadm
sudo systemctl enable --now lynx.lynxd
```

### 2. Deploy your application!
That's it! Let's bring your app to life in seconds:

```bash
# Start your program and keep it running 24/7 forever
lynx start "node server.js" --name ultra-api --restart always

# Watch the magic happen with real-time stats
lynx list

# Check your application logs in real-time
lynx logs ultra-api --follow
```

---
## 🔒 Access Model

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
## 🛠️ Commands

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

## ⚙️ Advanced Installation & Build from Source

If you prefer modifying the source code or building the binaries yourself:

```bash
# 1. Clone repository
git clone https://github.com/Jaro-c/Lynx.git
cd Lynx

# 2. Build binaries manually
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -v -o lynx_linux_amd64 ./cmd/lynx

# 3. Build & Install Debian Package locally
sudo apt-get update && sudo apt-get install -y build-essential debhelper
dpkg-buildpackage -us -uc -b
sudo dpkg -i ../lynx_*.deb
```

### User Mode (Optional)
If you prefer per-user isolation without system-wide privileges:
```bash
# Start the daemon as your user (in a separate terminal)
systemd --user &
# Then use lynx with default user-mode socket ($XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock)
lynx list
```

## 🚀 Deployment Guide (Debian/Ubuntu)

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

## 📚 Getting Started Tutorial

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

## 👨‍💻 Development

**Note for Windows Developers**:
Since Lynx is Linux-only, we recommend using **VS Code Remote-WSL**.
If you are editing on Windows, you may see false positive errors (e.g., "build constraints exclude all Go files").
To fix this in your editor settings, set the environment variable:
`GOOS=linux`

## 📦 Packaging

Lynx is designed to be installed as a native Debian package. See `docs/BUILDING_UBUNTU_RELEASE.md` for full instructions.
