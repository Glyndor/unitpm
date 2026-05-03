---
title: Lynx process manager vs Supervisor (supervisord)
description: Lynx process manager vs Supervisor (supervisord) — benchmark comparison (32x faster cold start), feature differences, and migration guide for Linux.
---

Lynx is a systemd-native process manager for Linux written in Go. Supervisor (supervisord) is a Python-based process control system. This page compares them across performance, architecture, security, and configuration.

## Performance benchmarks

Numbers from [CI bench](https://github.com/Jaro-c/Lynx/actions/workflows/bench.yml) — Ubuntu 24.04, kernel 6.17, idle daemon supervising 10 noop processes.

| Metric | Lynx | Supervisor |
|--------|------|-----------|
| Cold start | **7.8 ms** | 252 ms |
| Idle RSS | **14.7 MB** | 27.1 MB |
| RSS w/ 10 processes | **22.8 MB** | 27.3 MB |
| Daemon binary | **7.2 MB** | Python + libs |

Lynx starts **32× faster** and uses **1.8× less memory** at idle.

## Architecture differences

### Runtime

Supervisor is a Python application — Python must be installed and maintained on the host. Lynx is a compiled Go binary with no runtime dependencies.

### Process supervision model

Supervisor runs its own daemon (`supervisord`) that manages processes. Like PM2, if `supervisord` is killed, the supervised apps are also killed.

Lynx uses systemd as the actual supervisor. Apps run as systemd transient services. `lynxd` is a thin coordinator — apps survive its restart.

### Configuration

Supervisor uses INI-style config files:

```ini
[program:api]
command=node /srv/api/server.js
autostart=true
autorestart=true
user=www-data
environment=PORT="3000"
stdout_logfile=/var/log/supervisor/api.log
```

Lynx uses a CLI or `Lynxfile.yml`:

```yaml
version: 1
processes:
  api:
    command: node /srv/api/server.js
    restart: always
    env:
      PORT: "3000"
```

Or directly from the terminal:

```bash
lynxpm start "node /srv/api/server.js" --name api --restart always
```

## Security

Supervisor runs processes as a specified `user=` but provides no additional kernel-level isolation. Lynx uses systemd's `DynamicUser=yes` and Linux landlock restrictions. Secrets can be injected via systemd credentials — they never appear in `/proc/<pid>/environ`.

## Feature comparison

| Feature | Lynx | Supervisor |
|---------|------|-----------|
| Process supervision | systemd | supervisord daemon |
| Apps outlive the CLI | ✓ | ✗ |
| Sandboxing | DynamicUser + landlock | User switching only |
| Secrets in env | Never in /proc | Exposed in /proc |
| Config format | CLI or YAML | INI files |
| Namespaces / groups | `--namespace prod` | Groups |
| Web UI | ✗ | ✓ (supervisorctl web) |
| XML-RPC API | ✗ | ✓ |
| Runtime required | None (Go binary) | Python |
| Log rotation | Built-in | External (logrotate) |
| Linux only | ✓ | ✗ (cross-platform) |

## When to choose Supervisor

- You need the supervisorctl web UI or XML-RPC API for integration with existing tooling
- You are on a non-systemd Linux system or non-Linux OS
- Your team has deep existing Supervisor expertise and config templates

## When to choose Lynx

- You deploy to Linux servers with systemd (Debian, Ubuntu, RHEL, Arch)
- You want apps to survive daemon crashes
- You want DynamicUser + landlock sandboxing without manual systemd unit authoring
- You prefer a single compiled binary with no Python dependency
- You want a modern CLI (`lynxpm list`, `lynxpm logs`, `lynxpm scale`)

## Migrating from Supervisor

### List current processes

```bash
supervisorctl status
```

### Recreate with Lynx

For each program in your `supervisord.conf`:

```bash
lynxpm start "<command>" --name <program_name> --restart always
```

Or write a `Lynxfile.yml` that mirrors your `[program:*]` blocks and apply it:

```bash
lynxpm apply Lynxfile.yml
```

### Stop Supervisor

```bash
sudo systemctl stop supervisor
sudo systemctl disable supervisor
```

### Add Lynx to startup

```bash
lynxpm startup install
```

### Verify

```bash
lynxpm list
```

## See also

- [Lynx vs PM2](./vs-pm2/)
- [Install Lynx](../start/install/)
- [Quickstart](../start/quickstart/)
