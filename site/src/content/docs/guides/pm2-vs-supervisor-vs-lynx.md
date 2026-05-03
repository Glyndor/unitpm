---
title: PM2 vs Supervisor vs Lynx — process manager comparison
description: Three-way comparison of PM2, Supervisor (supervisord), and Lynx process managers for Linux. Benchmarks, architecture differences, feature matrix, and migration guidance.
---

Choosing a process manager for Linux comes down to three main options: **PM2**, **Supervisor (supervisord)**, and **Lynx**. This page compares all three across performance, architecture, security, and use cases so you can make an informed decision.

## TL;DR

| | Lynx | PM2 | Supervisor |
|--|------|-----|-----------|
| **Best for** | Linux servers, production | Node.js devs, cross-platform | Python apps, legacy setups |
| **Runtime required** | None (Go binary) | Node.js | Python |
| **Supervision model** | systemd (kernel) | Custom daemon | Custom daemon |
| **Apps survive daemon restart** | ✓ | ✗ | ✗ |
| **Linux only** | ✓ | ✗ | ✗ |

## Performance benchmarks

From [CI bench](https://github.com/Jaro-c/Lynx/actions/workflows/bench.yml) — Ubuntu 24.04, kernel 6.17, idle daemon supervising 10 noop processes:

| Metric | Lynx | PM2 | Supervisor |
|--------|------|-----|-----------|
| Cold start | **7.8 ms** | 366 ms | 252 ms |
| Idle RSS | **14.7 MB** | 66.7 MB | 27.1 MB |
| RSS with 10 processes | **22.8 MB** | 69.3 MB | 27.3 MB |
| Binary / install size | **7.2 MB** | Node + deps (~250 MB) | Python + libs |

Lynx starts **47× faster than PM2** and **32× faster than Supervisor**. At idle it uses **4.5× less memory than PM2** and **1.8× less than Supervisor**.

## Architecture: who holds your processes?

This is the most important difference between the three tools.

### PM2

PM2 is a Node.js daemon. It forks your apps as child processes. The process tree looks like:

```
systemd
└── pm2 daemon (Node.js, ~67 MB)
    ├── node server.js   ← your app
    └── python worker.py ← your app
```

If `pm2 daemon` is killed — by an OOM event, a `kill -9`, a system update — **every child process dies**. PM2 also requires the Node.js runtime to be installed and maintained on every host, even if your app is not Node.js.

### Supervisor

Supervisor is a Python daemon (supervisord). Same pattern:

```
systemd
└── supervisord (Python, ~27 MB)
    ├── node server.js
    └── python worker.py
```

Same weakness: if `supervisord` dies, so do your apps. Requires Python on every host.

### Lynx

Lynx registers your apps as **systemd transient units**. The kernel's init system holds them:

```
systemd
├── lynxd (Go, ~15 MB) ← control plane only
├── api.service (node server.js) ← held by systemd
└── worker.service (python worker.py) ← held by systemd
```

If `lynxd` is killed, restarted, or updated, **your apps keep running**. Systemd supervises them independently. Lynx is just the CLI and bookkeeping layer.

## Feature comparison

| Feature | Lynx | PM2 | Supervisor |
|---------|------|-----|-----------|
| Auto-restart on crash | ✓ | ✓ | ✓ |
| Restart policies (always/on-failure/never) | ✓ | ✓ | ✓ |
| Exponential backoff | ✓ | ✗ | ✗ |
| Apps outlive daemon restart | ✓ | ✗ | ✗ |
| Boot persistence | ✓ | ✓ | ✓ |
| Log capture + rotation | ✓ | ✓ | ✓ |
| Memory limits | ✓ | ✓ (soft) | ✗ |
| CPU limits | ✓ | ✗ | ✗ |
| DynamicUser sandboxing | ✓ | ✗ | ✗ |
| Landlock filesystem restriction | ✓ | ✗ | ✗ |
| Namespace bulk operations | ✓ | Partial | Partial |
| Declarative config | YAML | JS | INI |
| Web UI / dashboard | ✗ | ✓ (PM2 Plus) | ✓ |
| Cluster mode | ✓ | ✓ | ✗ |
| JSON output | ✓ | ✓ | ✗ |
| Cron scheduling | ✓ | ✓ | ✗ |
| Runtime required | None | Node.js | Python |
| Linux only | ✓ | ✗ | ✗ |

## Configuration comparison

Three tools, three config formats for the same app:

**Lynxfile.yml (Lynx)**
```yaml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production
    memory_max: 512M
```

**ecosystem.config.js (PM2)**
```js
module.exports = {
  apps: [{
    name: 'api',
    script: 'server.js',
    cwd: '/srv/api',
    restart_delay: 3000,
    env_file: '.env.production',
    max_memory_restart: '512M',
  }]
};
```

**supervisord.conf (Supervisor)**
```ini
[program:api]
command=node /srv/api/server.js
directory=/srv/api
autostart=true
autorestart=true
environment=NODE_ENV="production"
stdout_logfile=/var/log/supervisor/api.log
```

## When to choose each tool

### Choose Lynx when:
- You deploy to Linux servers with systemd
- You want apps to survive daemon crashes and system updates
- You care about memory (containers, low-resource VMs)
- You need per-process sandboxing without writing unit files
- Your team manages both Node.js and Python apps from one tool

### Choose PM2 when:
- You need macOS or Windows support
- You are deeply invested in PM2 Plus / Keymetrics monitoring
- Your team is Node.js-only and already knows PM2

### Choose Supervisor when:
- You have existing Supervisor configs you are not ready to migrate
- You need the supervisorctl web interface for non-technical stakeholders
- You are on a non-systemd Linux (rare) or legacy infrastructure

## Migration guides

- [Migrating from PM2 to Lynx](./vs-pm2/#migrating-from-pm2) — step by step
- [Migrating from Supervisor to Lynx](./vs-supervisor/#migrating-from-supervisor) — step by step

## See also

- [What is a Linux process manager?](./what-is-a-process-manager/)
- [Lynx vs PM2](./vs-pm2/) — detailed PM2 comparison
- [Lynx vs Supervisor](./vs-supervisor/) — detailed Supervisor comparison
- [Lightweight process manager for Linux](./lightweight-process-manager/)
- [systemd-native process manager](./systemd-process-manager/)
