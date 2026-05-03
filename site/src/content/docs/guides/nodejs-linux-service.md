---
title: How to run a Node.js app as a Linux service
description: Run a Node.js application as a persistent Linux service with auto-restart, log management, and boot persistence. Using Lynx process manager, plain systemd, and PM2.
---

Running a Node.js application as a **Linux service** means it starts on boot, restarts on crash, writes logs to disk, and stays running when you disconnect from SSH. This guide covers three approaches: Lynx process manager (recommended), plain systemd unit files, and PM2.

## Prerequisites

- Linux with systemd (Debian, Ubuntu, RHEL, Arch, etc.)
- Node.js installed and in PATH
- Your app accessible at a known path (e.g., `/srv/api/server.js`)

## Option 1: Lynx (recommended)

Lynx is a systemd-native process manager — it registers your app as a systemd transient unit, so Node.js survives even if the Lynx daemon restarts.

### Install Lynx

```bash
# Download latest .deb from GitHub releases
sudo apt install ./lynxpm_*_amd64.deb
sudo usermod -aG lynxadm "$USER" && newgrp lynxadm
sudo systemctl enable --now lynxd
```

### Start your Node.js app

```bash
lynxpm start "node /srv/api/server.js" \
  --name api \
  --restart always \
  --cwd /srv/api
```

### Pass environment variables

```bash
# Using an .env file (recommended)
lynxpm start "node server.js" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file /srv/api/.env.production
```

### Set resource limits

```bash
lynxpm start "node server.js" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file .env \
  --memory-max 512M \
  --cpu-max 100
```

### Verify it's running

```bash
lynxpm list
# ┌──────────┬──────┬──────────┬─────────┬─────────┐
# │ id       │ name │ namespace│ status  │ pid     │
# ├──────────┼──────┼──────────┼─────────┼─────────┤
# │ ▸ 019dbd │ api  │ default  │ running │ 2336612 │
# └──────────┴──────┴──────────┴─────────┴─────────┘

lynxpm logs api --follow
```

### Enable on boot

```bash
sudo lynxpm startup
```

Lynx installs a systemd service that starts `lynxd` on boot and restores all registered processes automatically.

### Declare it as code

Export the current configuration to a `Lynxfile.yml` you can commit:

```bash
lynxpm export api > Lynxfile.yml
```

```yaml
# Lynxfile.yml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production
    memory_max: 512M
    cpu_max: 100
```

Re-apply on any server:

```bash
lynxpm apply Lynxfile.yml
```

## Option 2: Plain systemd unit file

Writing a unit file gives you direct control but requires manual file management.

### Create the unit file

```ini
# /etc/systemd/system/api.service
[Unit]
Description=Node.js API
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/srv/api
EnvironmentFile=/srv/api/.env.production
ExecStart=/usr/bin/node server.js
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=api

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now api
sudo systemctl status api
sudo journalctl -u api -f
```

**Tradeoffs**: Full control, but you must edit files and reload systemd for every change. No CLI for bulk operations across multiple services.

## Option 3: PM2

PM2 is the most commonly documented approach but has significant drawbacks on Linux servers.

```bash
npm install -g pm2
pm2 start server.js --name api --cwd /srv/api
pm2 save
pm2 startup
```

**Key limitation**: PM2 requires Node.js on the server permanently, uses 66 MB idle RAM, and your app dies if PM2 crashes or is restarted. With Lynx, Node.js is only required for your app — not the process manager itself.

## Running multiple Node.js apps

With Lynx, use namespaces to group related services:

```bash
lynxpm start "node api.js"     --name api     --namespace prod --restart always
lynxpm start "node worker.js"  --name worker  --namespace prod --restart always
lynxpm start "node scheduler.js" --name cron  --namespace prod --restart always

# Restart the entire tier
lynxpm restart --namespace prod

# Stop for maintenance
lynxpm stop --namespace prod
```

## Using Bun or other Node.js runtimes

Swap `node` for `bun`, `deno`, or any other runtime:

```bash
lynxpm start "bun run server.ts" --name api --restart always --cwd /srv/api
lynxpm start "deno run --allow-net server.ts" --name api --restart always
```

## Logs and debugging

```bash
# Live output
lynxpm logs api --follow

# Last 100 lines of stderr only
lynxpm logs api --stderr --lines 100

# Truncate if disk is full
lynxpm flush api
```

## See also

- [Install Lynx](../start/install/)
- [Runtimes guide](./runtimes/) — Node.js, Bun, Deno specifics
- [How to manage multiple Node.js apps on a VPS](./manage-multiple-nodejs-apps-vps/)
- [How to set environment variables for a Linux service](./linux-service-environment-variables/)
- [Auto-restart on crash](./auto-restart-on-crash/)
