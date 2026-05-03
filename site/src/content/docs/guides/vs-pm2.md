---
title: Lynx process manager vs PM2
description: Lynx process manager vs PM2 — benchmark comparison (47x faster cold start, 4.5x less memory), feature differences, and migration guide for Linux.
---

Lynx is a systemd-native process manager for Linux written in Go. PM2 is a Node.js-based process manager. This page compares them across performance, architecture, security, and day-to-day usage.

## Performance benchmarks

Numbers from [CI bench](https://github.com/Jaro-c/Lynx/actions/workflows/bench.yml) — Ubuntu 24.04, kernel 6.17, idle daemon supervising 10 noop processes.

| Metric | Lynx | PM2 |
|--------|------|-----|
| Cold start | **7.8 ms** | 366 ms |
| Idle RSS | **14.7 MB** | 66.7 MB |
| RSS w/ 10 processes | **22.8 MB** | 69.3 MB |
| Daemon binary | **7.2 MB** | Node.js + deps |

Lynx starts **47× faster** and uses **4.5× less memory** at idle.

## Architecture differences

### Runtime

PM2 is a Node.js application — to run PM2, you need Node.js installed on the host. Lynx is a compiled Go binary with no runtime dependencies. Copy the `.deb` or binary and it runs.

### Process supervision

PM2 runs its own custom daemon that supervises your processes. If PM2 crashes or is killed, the apps it manages die with it.

Lynx delegates supervision to systemd. Your apps run as systemd transient services. If `lynxd` stops, the apps keep running. Systemd takes care of crash recovery, restarts, and logging — it already does this for the rest of your system.

### Crash resilience

```
PM2 crash → all managed apps die
Lynx daemon crash → apps keep running (systemd holds them)
```

### Config format

PM2 uses `ecosystem.config.js` — a JavaScript file. Lynx uses either the CLI directly or a `Lynxfile.yml`:

```yaml
# Lynxfile.yml
version: 1
processes:
  api:
    command: node server.js
    restart: always
    env:
      PORT: "3000"
```

## Security

PM2 runs processes under the current user with no additional isolation. Lynx uses systemd's `DynamicUser=yes` plus Linux landlock to restrict filesystem access. Secrets can be passed via systemd credentials — they never appear in `/proc/<pid>/environ` or `ps` output.

## Feature comparison

| Feature | Lynx | PM2 |
|---------|------|-----|
| Process supervision | systemd | Custom daemon |
| Apps outlive the CLI | ✓ | ✗ |
| Sandboxing | DynamicUser + landlock | User-space only |
| Secrets in env | Never in /proc | Exposed in /proc |
| Config | CLI or YAML | JS file |
| Namespaces | `--namespace prod` | Ecosystem files |
| Cluster mode | `--instances N` | `--instances N` |
| Log rotation | Built-in | Built-in |
| Runtime required | None (Go binary) | Node.js |
| Linux only | ✓ | ✗ (cross-platform) |

## When to choose PM2

- You are on macOS or Windows (Lynx is Linux-only)
- You need the PM2 ecosystem integrations (Keymetrics, PM2 Plus)
- You are already deeply invested in a PM2 workflow on a non-systemd system

## When to choose Lynx

- You deploy to Linux servers with systemd (Debian, Ubuntu, RHEL, Arch)
- You want your apps to survive daemon crashes or restarts
- You care about memory footprint (containers, low-resource VMs)
- You want real sandboxing without configuring it manually
- You need secrets that never touch environment variable lists

## Migrating from PM2

### Export your current processes

```bash
# PM2 — save current process list
pm2 save
# Output is ~/.pm2/dump.pm2 (JSON)
```

### Recreate with Lynx

```bash
# Start equivalent processes
lynxpm start "node server.js" --name api --restart always
lynxpm start "node worker.js" --name worker --restart always

# Or write a Lynxfile.yml and apply it
lynxpm apply Lynxfile.yml
```

### Stop PM2

```bash
pm2 kill
# Remove PM2 from startup
pm2 unstartup
```

### Add Lynx to startup

```bash
lynxpm startup install
```

### Verify

```bash
lynxpm list
# ┌──────────┬────────┬──────────┬─────────┬────────┐
# │ id       │ name   │ namespace│ status  │ pid    │
# ├──────────┼────────┼──────────┼─────────┼────────┤
# │ ▸ 019dbd │ api    │ default  │ running │ 1234   │
# └──────────┴────────┴──────────┴─────────┴────────┘
```

## See also

- [Lynx vs Supervisor](./vs-supervisor/)
- [Install Lynx](../start/install/)
- [Quickstart](../start/quickstart/)
