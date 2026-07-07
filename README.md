# Lynx

Systemd-native process manager for Linux — a lean alternative to PM2 and
Supervisor built directly on `systemd` rather than a custom supervisor daemon.

[![CI](https://github.com/Jaro-c/Lynx/actions/workflows/ci.yml/badge.svg)](https://github.com/Jaro-c/Lynx/actions/workflows/ci.yml)

Go 1.26+ · Linux only · License: Apache-2.0

## Architecture

| | |
| :--- | :--- |
| **Runtime** | Compiled Go, native |
| **Supervisor** | `systemd` — apps outlive the CLI |
| **Sandboxing** | `DynamicUser` + landlock |
| **Config** | CLI flags or `Lynxfile.yml` |

Startup/memory measurements and the reproduction harness:
[`scripts/bench`](./scripts/bench/) — run in CI on Ubuntu 24.04 (kernel 6.17)
against PM2 5.4.3 and supervisord 4.2.5, reproducible locally with
`docker build -f scripts/bench/Dockerfile -t lynx-bench . && docker run --rm lynx-bench`.

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

## Documentation

Rendered docs: <https://jaro-c.github.io/Lynx/>. In-repo:

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

| Mode | Socket | Use for |
|------|--------|---------|
| **System** (default with `.deb`) | `/run/lynxd/lynx.sock` (`0660`, group `lynxadm`) | Production |
| **User** | `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock` (`0600`) | Dev |

System mode does **not** inherit the caller's env. User mode does.
Launch user mode ad-hoc with `lynxd &`, or `sudo lynxpm startup` for boot persistence. Details in the [FAQ](docs/FAQ.md).

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

Lynx is **Linux-only**. Contributors on macOS/Windows should use a Linux VM or VS Code Remote-WSL.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full workflow and [`ARCHITECTURE.md`](ARCHITECTURE.md) for the internals.

---

## License

[Apache License 2.0](LICENSE) — commercial use, modification, and distribution permitted. Patent grant included.
