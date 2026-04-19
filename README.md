# 🦁 Lynx

<div align="center">
  <h3>The Secure, Systemd-Native Process Manager for Linux</h3>
  <p>A lightning-fast, highly secure alternative to PM2 or Supervisor—built specifically for modern Debian/Ubuntu servers.</p>

  <img src="https://img.shields.io/badge/OS-Linux%20Only-informational?style=for-the-badge&logo=linux&color=2ecc71" alt="Linux Only" />
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/github/v/release/Jaro-c/Lynx?style=for-the-badge&color=ff69b4" alt="Release" />
  <img src="https://img.shields.io/github/actions/workflow/status/Jaro-c/Lynx/ci.yml?branch=main&style=for-the-badge&logo=github&label=CI" alt="CI" />
  <img src="https://img.shields.io/codecov/c/github/Jaro-c/Lynx?style=for-the-badge&logo=codecov" alt="Coverage" />
  <a href="https://scorecard.dev/viewer/?uri=github.com/Jaro-c/Lynx"><img src="https://img.shields.io/ossf-scorecard/github.com/Jaro-c/Lynx?style=for-the-badge&label=OpenSSF%20Scorecard" alt="OpenSSF Scorecard" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=for-the-badge" alt="License: Apache 2.0" /></a>
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
lynxpm start api.js \
  --name max-security-api \
  --isolation dynamic \
  --env-file .env.production
```

It is impossible to achieve this level of security with one command in other managers.

---

## ⚡ 1-Minute Quickstart

### Option A: Download the binary (fastest)

```bash
curl -L -o lynxpm "$(curl -s https://api.github.com/repos/Jaro-c/Lynx/releases/latest \
  | grep browser_download_url | grep 'lynxpm_linux_amd64"' | cut -d '"' -f 4)"
chmod +x lynxpm
sudo mv lynxpm /usr/local/bin/     # system-wide
# or: mkdir -p ~/.local/bin && mv lynxpm ~/.local/bin/   # user-local
```

### Option B: Install the Debian package

```bash
sudo apt install ./lynxpm_*.deb
sudo usermod -aG lynxadm $USER && newgrp lynxadm
sudo systemctl enable --now lynxd
sudo lynxpm install-tools          # optional: make bun/node/go visible to lynxd
```

### Deploy

```bash
lynxpm start "node server.js" --name ultra-api --restart always
lynxpm list
lynxpm logs ultra-api --follow
```

---

## 📚 Documentation

| Topic | Link |
|-------|------|
| **Runtime recipes** (Node, Bun, Python, Go, Rust, Ruby, Java, …) | [`docs/RUNTIMES.md`](docs/RUNTIMES.md) |
| **Tutorials** (Next.js, FastAPI, Django, production deploy, Lynxfile) | [`docs/TUTORIALS.md`](docs/TUTORIALS.md) |
| **Commands reference** (`start`, `list`, `apply`, `export`, …) | [`docs/commands/`](docs/commands/) |
| **FAQ** — rapid-fire "Can I…?" / "Why does X fail?" | [`docs/FAQ.md`](docs/FAQ.md) |
| **Architecture overview** | [`ARCHITECTURE.md`](ARCHITECTURE.md) |

---

## 🔒 Access Model

Lynx runs in one of two modes — pick based on deployment scope:

- **System Mode** (default): daemon runs as system user `lynx` under `systemd`, socket at `/run/lynxd/lynx.sock` (`0660`, `lynxadm` group). Does **not** inherit user env to prevent secret leaks. For production.
- **User Mode**: daemon runs under `systemd --user`, socket at `$XDG_RUNTIME_DIR/lynx/lynx.sock` (`0600`). Inherits full user env. For dev.

Run `lynxd &` for ad-hoc user-mode, or `sudo lynxpm startup` to enable boot-time startup. Full details in the [FAQ](docs/FAQ.md).

---

## 🧩 Supported Runtimes

Lynx is language-agnostic — it runs anything you can spawn as a Linux process: Node, Bun, Deno, Python (system/venv/uv), Go, Rust, Ruby, Java/JVM, PHP, Lua, Erlang, shell, and more.

See [`docs/RUNTIMES.md`](docs/RUNTIMES.md) for per-runtime recipes, virtualenvs, env-file injection, clustering, and isolation picker.

---

## 👨‍💻 Development

Lynx is **Linux-only**. Contributors on macOS/Windows should use **VS Code Remote-WSL** or a Linux VM — local editors may show false-positive errors without `GOOS=linux`.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full workflow.

---

## ⚠️ Troubleshooting

| Symptom | Where to look |
|---------|---------------|
| Generic errors, permissions, naming, env vars | [`docs/FAQ.md`](docs/FAQ.md) |
| Daemon won't start / socket unreachable | `journalctl -u lynxd -f` |
| `--isolation dynamic` fails | Requires system-mode daemon (Polkit rule in `.deb`) |

---

## 📜 License

[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

Lynx is open source under the **[Apache License 2.0](LICENSE)**. You may use, modify, and distribute it freely, including in commercial products, provided you preserve the copyright notice and include a copy of the license. The Apache 2.0 grant also includes an explicit patent license.
