---
title: How to manage multiple Node.js apps on a VPS
description: Run and manage multiple Node.js applications on a single Linux VPS using unitpm process manager. Covers namespaces, resource limits, Nginx reverse proxy, env files, and declarative config.
---

Running multiple Node.js applications on a single VPS is a common cost optimization. This guide covers how to **manage multiple Node.js apps on a Linux VPS** using unitpm process manager — with namespaces, resource limits, Nginx proxying, and declarative config.

## The problem with running multiple apps

When you run several apps on one server, the main risks are:

- **Port conflicts**: each app must bind to a different port
- **Resource contention**: one app consuming all RAM or CPU degrades others
- **Blast radius**: one crashing app should not affect others
- **Config sprawl**: managing separate unit files or PM2 configs per app

unitpm handles all four: each process is a separate systemd unit with configurable CPU/memory limits, and namespaces group related apps for bulk operations.

## Architecture

A typical VPS setup:

```
Internet → Nginx (443/80)
              ├── / → :3000 (Next.js frontend)
              ├── /api → :4000 (Express API)
              └── /admin → :5000 (admin panel)

unitpm manages:
  ├── frontend  (namespace: prod)
  ├── api       (namespace: prod)
  └── admin     (namespace: prod)
```

All apps bind to `127.0.0.1` (not `0.0.0.0`). Nginx handles TLS and public traffic.

## Start multiple apps with unitpm

```bash
# Frontend
unitpm start "node /srv/frontend/server.js" \
  --name frontend \
  --namespace prod \
  --restart always \
  --cwd /srv/frontend \
  --env-file /srv/frontend/.env.production \
  --memory-max 512M

# API
unitpm start "node /srv/api/server.js" \
  --name api \
  --namespace prod \
  --restart always \
  --cwd /srv/api \
  --env-file /srv/api/.env.production \
  --memory-max 256M

# Admin panel
unitpm start "node /srv/admin/server.js" \
  --name admin \
  --namespace prod \
  --restart always \
  --cwd /srv/admin \
  --env-file /srv/admin/.env.production \
  --memory-max 128M
```

### View all apps

```bash
unitpm list
# ┌──────────┬──────────┬──────────┬─────────┬─────────┐
# │ id       │ name     │ namespace│ status  │ pid     │
# ├──────────┼──────────┼──────────┼─────────┼─────────┤
# │ ▸ 019dbd │ frontend │ prod     │ running │ 1234    │
# │ ▸ 019dbe │ api      │ prod     │ running │ 1235    │
# │ ▸ 019dbf │ admin    │ prod     │ running │ 1236    │
# └──────────┴──────────┴──────────┴─────────┴─────────┘
```

### Namespace bulk operations

```bash
# Restart entire production namespace (rolling, one at a time)
unitpm restart --namespace prod

# Stop all for maintenance
unitpm stop --namespace prod

# Resume
unitpm start --namespace prod
```

## Declarative config (recommended for VPS)

Define all apps in a single file you can commit to version control:

```yaml
# /srv/unitpm.yml
version: 1
processes:
  frontend:
    command: node server.js
    cwd: /srv/frontend
    restart: always
    env_file: /srv/frontend/.env.production
    memory_max: 512M
    cpu_max: 200
    namespace: prod

  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: /srv/api/.env.production
    memory_max: 256M
    cpu_max: 150
    namespace: prod

  api-worker:
    command: node worker.js
    cwd: /srv/api
    restart: on-failure
    env_file: /srv/api/.env.production
    memory_max: 128M
    namespace: prod

  admin:
    command: node server.js
    cwd: /srv/admin
    restart: always
    env_file: /srv/admin/.env.production
    memory_max: 128M
    namespace: prod
```

Apply or update:

```bash
unitpm apply /srv/unitpm.yml
```

unitpm only restarts processes whose config changed. `apply` is idempotent — safe to run on every deploy.

## Nginx reverse proxy

Install Nginx and configure a site per app:

```bash
sudo apt install nginx
```

```nginx
# /etc/nginx/sites-available/apps
server {
    listen 443 ssl http2;
    server_name example.com;

    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    # Frontend
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }

    # API
    location /api/ {
        proxy_pass http://127.0.0.1:4000/;
        proxy_http_version 1.1;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Host $host;
    }

    # Admin
    location /admin/ {
        proxy_pass http://127.0.0.1:5000/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        allow 10.0.0.0/8;  # restrict to VPN
        deny all;
    }
}

server {
    listen 80;
    server_name example.com;
    return 301 https://$host$request_uri;
}
```

```bash
sudo ln -s /etc/nginx/sites-available/apps /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

## Deploy workflow

A minimal deploy script for a VPS with multiple apps:

```bash
#!/bin/bash
# /usr/local/bin/deploy
set -e

APP=$1
SRV="/srv/$APP"

echo "Deploying $APP..."
cd "$SRV"
git pull origin main
npm ci --production
unitpm restart "$APP"

echo "Done. Logs:"
unitpm logs "$APP" --lines 20
```

```bash
# Deploy just the API
deploy api

# Or apply the full unitpm.yml
cd /srv && git pull
unitpm apply unitpm.yml
```

## Enable boot persistence

```bash
sudo unitpm startup
```

On boot: `unitpmd` starts, reads its state file, registers all processes with systemd. All apps come back without manual intervention.

## Monitoring all apps

```bash
# Live dashboard — all apps, CPU + RSS per process
unitpm monit

# JSON for scripting/alerting
unitpm list --json | jq '.[] | {name, status, cpu_pct, rss_bytes}'
```

## Resource planning

Rule of thumb for a 2 GB VPS:

| Component | Reserve |
|-----------|---------|
| OS + kernel | 200 MB |
| unitpm daemon | 15 MB |
| Nginx | 5 MB |
| Per Node.js app | 50-200 MB |
| Buffer (peak) | 200 MB |

With `--memory-max` on each app, you guarantee the buffer stays available. Without limits, one app can OOM the entire VPS.

## See also

- [Install unitpm](../start/install/)
- [How to run a Node.js app as a Linux service](./nodejs-linux-service/)
- [Zero-downtime deployment on Linux](./zero-downtime-deployment-linux/)
- [How to set environment variables for a Linux service](./linux-service-environment-variables/)
- [Monitor process memory and CPU on Linux](./monitor-process-memory-cpu-linux/)
