---
title: How to run a Go binary as a systemd service
description: Deploy a compiled Go binary as a persistent Linux service with auto-restart, resource limits, and sandboxing using Lynx process manager and systemd. Covers build, deploy, and update workflows.
---

A compiled Go binary is one of the easiest things to deploy as a Linux service — no runtime, no dependencies, just a single file. This guide covers how to run a Go binary as a **systemd service** using Lynx process manager (recommended) and plain systemd unit files.

## Why Go binaries are ideal for Linux services

- **Single static binary**: no runtime, no shared libraries, no version conflicts
- **Fast start**: typical Go service starts in < 50 ms — compatible with socket activation and on-demand startup
- **Low memory footprint**: Go's runtime is lean; a minimal HTTP server uses 5-15 MB RSS
- **Graceful shutdown**: Go's `os.Signal` + `context` pattern handles SIGTERM cleanly

## Build for Linux

Cross-compile from any platform:

```bash
# Linux AMD64 (most servers)
GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

# Linux ARM64 (AWS Graviton, Raspberry Pi 4)
GOOS=linux GOARCH=arm64 go build -o bin/server ./cmd/server

# Fully static (no libc dependency)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server
```

For production, strip debug info to reduce binary size:

```bash
go build -ldflags="-s -w" -o bin/server ./cmd/server
```

## Deploy

Copy the binary and set permissions:

```bash
# Copy to server
scp bin/server user@host:/srv/api/server

# On the server
sudo chown root:root /srv/api/server
sudo chmod 755 /srv/api/server
```

## Option 1: Lynx (recommended)

### Install Lynx

```bash
sudo apt install ./lynxpm_*_amd64.deb
sudo usermod -aG lynxadm "$USER" && newgrp lynxadm
sudo systemctl enable --now lynxd
```

### Start the service

```bash
lynxpm start "/srv/api/server" \
  --name api \
  --restart on-failure \
  --cwd /srv/api
```

### Pass environment variables

```bash
lynxpm start "/srv/api/server" \
  --name api \
  --restart on-failure \
  --cwd /srv/api \
  --env-file /srv/api/.env.production
```

`.env.production`:

```bash
HTTP_ADDR=:8080
DATABASE_URL=postgres://user:pass@localhost/app
LOG_FORMAT=json
METRICS_ADDR=:9090
```

### Set resource limits

```bash
lynxpm start "/srv/api/server" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file .env \
  --memory-max 256M \
  --cpu-max 200
```

### Enable sandboxing

Lynx supports systemd's `DynamicUser` for zero-privilege deployment — the process runs as a generated UID with no persistent identity:

```bash
lynxpm start "/srv/api/server" \
  --name api \
  --restart on-failure \
  --env-file .env \
  --sandbox
```

Or via Lynxfile.yml:

```yaml
version: 1
processes:
  api:
    command: /srv/api/server
    cwd: /srv/api
    restart: on-failure
    env_file: .env.production
    memory_max: 256M
    dynamic_user: true
```

### Enable on boot

```bash
sudo lynxpm startup
```

### Verify

```bash
lynxpm list
lynxpm logs api --follow
```

## Option 2: Plain systemd unit file

```ini
# /etc/systemd/system/api.service
[Unit]
Description=Go API Server
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/srv/api
EnvironmentFile=/srv/api/.env.production
ExecStart=/srv/api/server
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Hardening (optional but recommended)
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/srv/api/data

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

### File descriptor limits

Go services often need high `NOFILE` limits for connection-heavy servers. Set `LimitNOFILE=65536` in the unit or via Lynx:

```bash
lynxpm start "/srv/api/server" --name api --restart on-failure --fd-limit 65536
```

## Zero-downtime binary updates

Go binaries can be hot-swapped using an atomic rename:

```bash
# Copy new binary alongside old one
scp bin/server user@host:/srv/api/server.new

# Atomic replace (on same filesystem)
mv /srv/api/server.new /srv/api/server

# Restart with Lynx (graceful: sends SIGTERM, waits, starts new)
lynxpm restart api
```

For true zero-downtime (no dropped connections), implement graceful shutdown in the Go binary:

```go
srv := &http.Server{Addr: ":8080", Handler: mux}

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

Then use Lynx's stop timeout:

```bash
lynxpm start "/srv/api/server" \
  --name api \
  --restart always \
  --stop-timeout 30000
```

## Multiple Go services

```bash
lynxpm start "/srv/api/server"      --name api      --namespace backend --restart on-failure --env-file /srv/api/.env
lynxpm start "/srv/worker/worker"   --name worker   --namespace backend --restart on-failure --env-file /srv/worker/.env
lynxpm start "/srv/metrics/metrics" --name metrics  --namespace backend --restart on-failure

# Deploy all
lynxpm restart --namespace backend
```

## Declarative config

```yaml
# Lynxfile.yml
version: 1
processes:
  api:
    command: /srv/api/server
    restart: on-failure
    env_file: /srv/api/.env.production
    memory_max: 256M
    namespace: backend

  worker:
    command: /srv/worker/worker
    restart: on-failure
    env_file: /srv/worker/.env.production
    namespace: backend
```

```bash
lynxpm apply Lynxfile.yml
```

## Common issues

### SIGTERM not caught (30-second delay before SIGKILL)

Default `stop-timeout` is 30 s. If your binary ignores SIGTERM, adjust:

```bash
lynxpm start "/srv/api/server" --name api --stop-timeout 5000
```

Or add signal handling in the binary (see graceful shutdown above).

### `bind: permission denied` on port 80/443

Ports < 1024 require root or `CAP_NET_BIND_SERVICE`. Options:
1. Run on 8080, put Nginx in front
2. Grant capability: `sudo setcap 'cap_net_bind_service=+ep' /srv/api/server`
3. Use systemd socket activation

### OOM killed (`exit code 137`)

Increase memory limit or profile with `pprof`:

```bash
lynxpm start "/srv/api/server -pprof :6060" --name api --memory-max 512M ...
```

## See also

- [lynxpm start](../reference/commands/start/) — full flag reference
- [Zero-downtime deployment on Linux](./zero-downtime-deployment-linux/)
- [systemd DynamicUser sandboxing](./systemd-dynamicuser/)
- [How to set environment variables for a Linux service](./linux-service-environment-variables/)
- [Auto-restart on crash](./auto-restart-on-crash/)
