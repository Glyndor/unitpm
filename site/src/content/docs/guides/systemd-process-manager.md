---
title: systemd-native process manager for Linux
description: A systemd-native process manager delegates supervision to systemd instead of running its own watchdog. Learn why this matters for crash resilience, security, and resource limits on Linux.
---

Most Linux process managers reinvent what systemd already does well. They run their own daemon, their own watchdog loop, their own restart logic — and when that daemon crashes, every app it was supervising dies with it. A **systemd-native process manager** takes a different approach: it generates systemd units and lets the kernel's init system do the supervision.

unitpm is a systemd-native process manager. This page explains what that means, why it matters, and how it compares to PM2 and Supervisor.

## How traditional process managers work

PM2 and Supervisor run a persistent daemon process. That daemon:

1. Forks your app as a child process
2. Monitors it with a polling loop
3. Restarts it when it crashes

The problem: **your app's lifetime is tied to the daemon's lifetime**. If PM2 is killed — by an OOM event, a segfault in the daemon itself, or `kill -9` — every process PM2 was managing dies immediately. There is no fallback.

## How a systemd-native process manager works

A systemd-native process manager is a thin coordinator. It:

1. Translates your command (`node server.js --name api`) into a systemd transient unit
2. Registers that unit with the running systemd instance via D-Bus
3. Lets systemd supervise, restart, log, and resource-limit the process

Your app runs under systemd's supervision — not under unitpm's supervision. If `unitpmd` is restarted, updated, or killed, **your apps keep running**. Systemd holds them. The daemon is just the control plane, not the supervisor.

```bash
# Start a process — unitpm registers it as a systemd transient unit
unitpm start "node server.js" --name api --restart always

# The app survives a daemon restart
sudo systemctl restart unitpmd
unitpm list  # api still running
```

## Crash resilience comparison

| Scenario | PM2 | Supervisor | unitpm |
|----------|-----|-----------|------|
| App crashes | Restarts app ✓ | Restarts app ✓ | Restarts app ✓ |
| Daemon crashes | All apps die ✗ | All apps die ✗ | Apps keep running ✓ |
| Daemon OOM-killed | All apps die ✗ | All apps die ✗ | Apps keep running ✓ |
| System update, daemon restart | All apps die ✗ | All apps die ✗ | Apps keep running ✓ |

## Systemd features unitpm exposes

Because unitpm uses systemd for supervision, you get the entire systemd feature set for free:

### Journal logging

Every managed process writes to the systemd journal automatically. `unitpm logs api --follow` reads directly from the journal. No custom log rotation daemon required — unitpm configures journal-based log rotation out of the box.

### Cgroup resource limits

Systemd uses Linux cgroups to enforce resource limits. unitpm exposes them directly:

```bash
unitpm start "python worker.py" --name worker \
  --memory-max 512M \
  --cpu-max 100 \
  --tasks-max 64
```

These map directly to `MemoryMax=`, `CPUQuota=`, and `TasksMax=` in the generated unit. There is no polling or userspace enforcement — the kernel enforces them.

### DynamicUser isolation

With `--isolation dynamic`, each process gets its own ephemeral user ID allocated by systemd's `DynamicUser=` feature. The UID exists only while the process runs and owns nothing on disk. Combined with landlock filesystem restrictions, this gives true per-process isolation without containers.

```bash
unitpm start "node api.js" --name api --isolation dynamic
# api runs as a fresh ephemeral UID
# files owned by that UID are cleaned up on stop
```

### Startup restoration

When `unitpmd` starts (on boot or after a restart), it reads its process registry and restores all registered apps. The apps are re-registered with systemd and begin supervising again. You do not lose your process list across reboots.

## Why not just write systemd unit files manually?

You can — and for permanent production services, that may be the right answer. unitpm is designed for the middle ground:

- **More dynamic than hand-authored units**: start, stop, scale, reload from the CLI without editing files
- **Less complex than containers**: no OCI images, no registry, no runtime overhead
- **Namespace-aware**: manage entire tiers with `--namespace prod` or `prod:*` glob
- **Exportable**: `unitpm export --namespace prod > unitpm.yml` captures the current state as a declarative YAML you can commit

If you already manage dozens of unit files, unitpm replaces the manual bookkeeping with a CLI while keeping systemd as the actual supervisor.

## Setting up unitpm as a systemd service

The `.deb` package installs `unitpmd` as a system-mode service automatically:

```bash
sudo apt install ./unitpm_*_amd64.deb
sudo systemctl enable --now unitpmd
```

For user-mode (per-UID daemon):

```bash
unitpm startup   # installs ~/.config/systemd/user/unitpmd.service
```

## What Linux distributions are supported?

Any Linux distribution running systemd. Tested in CI against:

- Debian 12 (bookworm)
- Debian 13 (trixie)
- Ubuntu 22.04 LTS
- Ubuntu 24.04 LTS

The binary is statically linked — `CGO_ENABLED=0`. Copy it to any amd64 or arm64 Linux host with systemd and it runs.

## See also

- [Install unitpm](../start/install/)
- [unitpm vs PM2](./vs-pm2/) — detailed comparison with benchmarks
- [unitpm vs Supervisor](./vs-supervisor/) — detailed comparison with benchmarks
- [Security model](../reference/security/) — DynamicUser, landlock, systemd credentials
- [Access model](../start/access-model/) — system-mode vs user-mode daemon
