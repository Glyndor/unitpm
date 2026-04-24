# 🦁 Lynx

<div align="center">
  <h3>The Secure, Systemd-Native Process Manager for Linux</h3>
  <p>A lean, hardened alternative to PM2 and Supervisor — built directly on top of <code>systemd</code>.</p>

  <img src="https://img.shields.io/badge/OS-Linux%20Only-informational?style=for-the-badge&logo=linux&color=2ecc71" alt="Linux Only" />
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/github/v/release/Jaro-c/Lynx?style=for-the-badge&color=ff69b4" alt="Release" />
  <img src="https://img.shields.io/github/actions/workflow/status/Jaro-c/Lynx/ci.yml?branch=main&style=for-the-badge&logo=github&label=CI" alt="CI" />
  <img src="https://img.shields.io/codecov/c/github/Jaro-c/Lynx?style=for-the-badge&logo=codecov" alt="Coverage" />
  <a href="https://scorecard.dev/viewer/?uri=github.com/Jaro-c/Lynx"><img src="https://img.shields.io/ossf-scorecard/github.com/Jaro-c/Lynx?style=for-the-badge&label=OpenSSF%20Scorecard" alt="OpenSSF Scorecard" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=for-the-badge" alt="License: Apache 2.0" /></a>
</div>

---

## Why Lynx?

| Feature | 🦁 Lynx | 🐢 PM2 | 🦖 Supervisor |
| :--- | :--- | :--- | :--- |
| **Runtime** | Compiled Go, native | Node.js (V8) | Python (interpreted) |
| **Base RAM** | **~10 MB** | ~60–100 MB | ~50 MB |
| **Supervisor** | **`systemd`** | Custom daemon | `supervisord` |
| **Crash resilience** | Apps outlive the CLI | Apps die with PM2 | Apps die with the daemon |
| **Sandboxing** | **`DynamicUser` + landlock** | User-space only | User-space only |
| **Config** | CLI flags or `Lynxfile.yml` | `ecosystem.config.js` | INI files |

---

## The Zero-Privilege Deploy

One command spawns an API with no access to `/home`, no new privileges, and
secrets delivered through systemd credentials instead of environment disk:

```bash
lynxpm start api.js \
    --name api \
    --isolation dynamic \
    --env-file .env.production
```

Secrets never appear in `/proc/<pid>/environ`, `ps`, or the on-disk spec.

---

## Quickstart

### Install — `.deb` (recommended)

```bash
# Grab the latest .deb from https://github.com/Jaro-c/Lynx/releases
sudo apt install ./lynxpm_*_amd64.deb
sudo usermod -aG lynxadm "$USER" && newgrp lynxadm
sudo systemctl enable --now lynxd
sudo lynxpm install-tools   # optional: expose bun/node/go/… to the daemon
```

### Install — prebuilt binary

```bash
gh release download --repo Jaro-c/Lynx --pattern 'lynxpm_linux_amd64'
install -m 0755 lynxpm_linux_amd64 ~/.local/bin/lynxpm
```

### Run something

```bash
lynxpm start "node server.js" --name api --namespace prod --restart always
lynxpm list
lynxpm logs api --follow
```

### Operate on a whole namespace

Every lifecycle command (`stop`, `restart`, `reload`, `reset`, `delete`,
`flush`) accepts `--namespace <ns>` or the `<ns>:*` selector — no more
`xargs` loops:

```bash
lynxpm restart --namespace prod    # roll the prod tier
lynxpm stop 'staging:*'            # halt everything in staging (quote the glob)
lynxpm delete --namespace old --purge
```

---

## Documentation

📘 **Full docs site: <https://jaro-c.github.io/Lynx/>** — searchable,
with the landing page, quickstart, runtimes, tutorials, and every
command's flag reference.

| Topic | Link |
|-------|------|
| Runtime recipes — Node / Bun / Python / Go / Rust / Ruby / JVM / … | [`docs/RUNTIMES.md`](docs/RUNTIMES.md) |
| Tutorials — Next.js, FastAPI, Django, production hardening, Lynxfile | [`docs/TUTORIALS.md`](docs/TUTORIALS.md) |
| Commands reference — `start`, `list`, `apply`, `export`, … | [`docs/commands/`](docs/commands/) |
| FAQ — "Can I…?" / "Why does X fail?" | [`docs/FAQ.md`](docs/FAQ.md) |
| Architecture overview | [`ARCHITECTURE.md`](ARCHITECTURE.md) |
| Security model + threat model | [`SECURITY.md`](SECURITY.md) |

---

## Access model

- **System mode** (default with the `.deb`) — daemon runs as the `lynx`
  system user under `systemd`, socket at `/run/lynxd/lynx.sock` (`0660`,
  group `lynxadm`). Does **not** inherit the caller's env. Use for
  production.
- **User mode** — daemon runs under `systemd --user`, socket at
  `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock` (`0600`). Inherits your env.
  Use for dev.

Launch user mode ad-hoc with `lynxd &`, or `sudo lynxpm startup` to
wire the systemd unit at boot. Details in the [FAQ](docs/FAQ.md).

---

## Supported runtimes

Anything you can spawn as a Linux process: Node, Bun, Deno, Python
(system / venv / `uv` / `uvx`), Go, Rust, Ruby, Java/JVM, PHP, Lua,
Erlang, shell, and more. Per-runtime recipes in
[`docs/RUNTIMES.md`](docs/RUNTIMES.md).

---

## Troubleshooting

| Symptom | Where to look |
|---------|---------------|
| `cannot reach the Lynx daemon` | `lynxd &` (user) or `sudo systemctl start lynxd` (system) |
| Daemon won't start / unit errors | `journalctl -u lynxd -f` |
| `--isolation dynamic` rejected | Needs the system-mode daemon (polkit rule is shipped in the `.deb`) |
| Generic usage / naming / env issues | [`docs/FAQ.md`](docs/FAQ.md) |

---

## Development

Lynx is **Linux-only**. Contributors on macOS/Windows should use a
Linux VM or VS Code Remote-WSL — local editors flag false-positive
errors without `GOOS=linux`.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full workflow, and
[`ARCHITECTURE.md`](ARCHITECTURE.md) for the internals.

---

## License

Lynx is open source under the **[Apache License 2.0](LICENSE)** —
commercial use, modification, distribution, and the explicit patent
grant all included. Preserve the copyright notice and ship a copy of
the license with any redistribution.
