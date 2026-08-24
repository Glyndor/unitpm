---
title: How to auto-restart a service on crash in Linux
description: Configure automatic process restart on crash in Linux using unitpm, systemd, or PM2. Set restart policies, exponential backoff, and crash loop protection.
---

When a Linux service crashes, you have two choices: restart it manually, or configure automatic restart before the crash ever happens. This guide covers how to set up **auto-restart on crash in Linux** using unitpm process manager, with comparisons to plain systemd and PM2.

## Restart policies

Most process managers support at least three restart policies:

| Policy | Behavior |
|--------|---------|
| `always` | Restart on any exit, including clean exit (code 0) |
| `on-failure` | Restart only on non-zero exit code (default in most tools) |
| `never` | Never restart automatically |

Choose `on-failure` for most services — it avoids restart loops when a process exits cleanly (e.g., a one-shot migration script). Use `always` only for processes that should never stop.

## Auto-restart with unitpm

### Basic restart on crash

```bash
unitpm start "node server.js" --name api --restart on-failure
```

### Always restart (including clean exits)

```bash
unitpm start "node server.js" --name api --restart always
```

### Check restart count

```bash
unitpm show api
# Shows: Restarts: 3, Status: running
```

### Reset the restart counter

```bash
unitpm reset api
```

## Exponential backoff

Blind restart loops — where a crashing process is restarted immediately, crashes again, and is restarted again — can amplify problems. unitpm uses exponential backoff by default:

```bash
unitpm start "node server.js" --name api \
  --restart on-failure \
  --backoff expo
```

With `--backoff expo`, wait time between restarts doubles on each crash: 1s, 2s, 4s, 8s, … up to a configured maximum. This prevents a crashing service from consuming all available resources.

### Limit total restart attempts

```bash
unitpm start "python worker.py" --name worker \
  --restart on-failure \
  --max-restarts 10
```

After 10 restarts, the process moves to `failed` state and stops restarting. Set `--max-restarts 0` for unlimited.

### Stop on specific exit codes

Some applications use exit codes to signal intentional shutdown. Tell unitpm not to restart on those:

```bash
unitpm start "./app" --name app \
  --restart always \
  --stop-on-exit 0,143,15
```

Exit codes 0 (clean), 143 (SIGTERM), and 15 (SIGTERM numeric) won't trigger a restart.

## Auto-restart with plain systemd

For comparison, a basic systemd unit with restart-on-failure:

```ini
# /etc/systemd/system/myapp.service
[Unit]
Description=My App

[Service]
ExecStart=/usr/bin/node /srv/app/server.js
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now myapp
```

unitpm generates equivalent unit configuration automatically — you don't need to write the unit file.

## Detect and respond to crash loops

A crash loop is a process that crashes, restarts, crashes, restarts — repeatedly. Signs:

```bash
unitpm show api
# Restarts: 47
# Uptime:   0s

unitpm logs api --lines 50
# [ERR] Cannot connect to database: connection refused
```

Common causes:
- Missing environment variable or config file
- Port already in use
- Dependency (database, cache) not yet available

Fix the root cause, then clear the counter:

```bash
unitpm reset api
```

## Configure a stop timeout

If a process ignores SIGTERM, unitpm sends SIGKILL after a timeout. Control it:

```bash
unitpm start "./app" --name app --stop-timeout 30000
# 30 seconds before SIGKILL
```

This matters during rolling restarts — you want the process to finish in-flight requests before dying.

## Monitor restart events

```bash
# Watch status in real time
unitpm monit

# Follow logs to see crash output
unitpm logs api --follow --stderr
```

## See also

- [unitpm start](../reference/commands/start/) — full flag reference
- [unitpm reset](../reference/commands/reset/) — clear restart counter
- [unitpm monit](../reference/commands/monit/) — live dashboard
- [Quickstart](../start/quickstart/)
- [Zero-downtime deployment on Linux](./zero-downtime-deployment-linux/)
