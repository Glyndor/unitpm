---
title: How to run a Python worker as a Linux service
description: Run a Python worker, script, or Celery process as a persistent Linux service with auto-restart, log management, and boot persistence using unitpm process manager and systemd.
---

Running a Python script or worker as a **Linux service** means it starts on boot, restarts on crash, and keeps running when you close your SSH session. This guide covers how to daemonize a Python process using unitpm process manager (recommended), plain systemd unit files, and Supervisor.

## Prerequisites

- Linux with systemd (Ubuntu, Debian, RHEL, Arch)
- Python 3 and your app's dependencies installed
- App path known (e.g., `/srv/worker/worker.py`)

## Option 1: unitpm (recommended)

unitpm registers your Python process as a systemd transient unit, so it survives unitpm daemon restarts or updates.

### Install unitpm

```bash
sudo apt install ./unitpm_*_amd64.deb
sudo usermod -aG unitpm "$USER" && newgrp unitpm
sudo systemctl enable --now unitpmd
```

### Start the worker

```bash
unitpm start "python3 /srv/worker/worker.py" \
  --name worker \
  --restart on-failure \
  --cwd /srv/worker
```

### Using a virtual environment

Always specify the full path to the venv interpreter:

```bash
unitpm start "/srv/worker/.venv/bin/python worker.py" \
  --name worker \
  --restart on-failure \
  --cwd /srv/worker
```

Or activate in a wrapper script:

```bash
# /srv/worker/start.sh
#!/bin/bash
source /srv/worker/.venv/bin/activate
exec python worker.py
```

```bash
unitpm start "/srv/worker/start.sh" \
  --name worker \
  --restart on-failure \
  --cwd /srv/worker
```

### Pass environment variables

```bash
unitpm start "/srv/worker/.venv/bin/python worker.py" \
  --name worker \
  --restart on-failure \
  --cwd /srv/worker \
  --env-file /srv/worker/.env.production
```

`.env.production`:
```bash
REDIS_URL=redis://localhost:6379
CELERY_CONCURRENCY=4
LOG_LEVEL=info
DATABASE_URL=postgres://user:pass@localhost/app
```

### Set resource limits

```bash
unitpm start "/srv/worker/.venv/bin/python worker.py" \
  --name worker \
  --restart on-failure \
  --cwd /srv/worker \
  --memory-max 512M \
  --cpu-max 200
```

`--cpu-max 200` means 200% of one core — useful on multi-core servers where the worker can use up to 2 threads.

### Enable on boot

```bash
sudo unitpm startup
```

### Verify

```bash
unitpm list
# ┌──────────┬────────┬──────────┬─────────┬─────────┐
# │ id       │ name   │ namespace│ status  │ pid     │
# ├──────────┼────────┼──────────┼─────────┼─────────┤
# │ ▸ 019dbe │ worker │ default  │ running │ 2336800 │
# └──────────┴────────┴──────────┴─────────┴─────────┘

unitpm logs worker --follow
```

## Running Celery workers

Celery is a common Python task queue. Run each worker pool as a separate unitpm process:

```bash
# Default worker pool
unitpm start "/srv/app/.venv/bin/celery -A app worker --loglevel=info --concurrency=4" \
  --name celery-worker \
  --restart on-failure \
  --cwd /srv/app \
  --env-file /srv/app/.env.production

# Beat scheduler (only one instance)
unitpm start "/srv/app/.venv/bin/celery -A app beat --loglevel=info" \
  --name celery-beat \
  --restart on-failure \
  --cwd /srv/app \
  --env-file /srv/app/.env.production

# Flower monitoring (optional)
unitpm start "/srv/app/.venv/bin/celery -A app flower" \
  --name celery-flower \
  --restart on-failure \
  --cwd /srv/app
```

Group them in a namespace for bulk operations:

```bash
unitpm start "/srv/app/.venv/bin/celery -A app worker -c 4" \
  --name celery-worker \
  --namespace celery \
  --restart on-failure \
  --cwd /srv/app \
  --env-file /srv/app/.env.production

unitpm start "/srv/app/.venv/bin/celery -A app beat" \
  --name celery-beat \
  --namespace celery \
  --restart on-failure \
  --cwd /srv/app \
  --env-file /srv/app/.env.production

# Restart all celery processes at once
unitpm restart --namespace celery
```

## FastAPI / Uvicorn service

```bash
unitpm start "/srv/api/.venv/bin/uvicorn app.main:app --host 0.0.0.0 --port 8000 --workers 2" \
  --name fastapi \
  --restart always \
  --cwd /srv/api \
  --env-file /srv/api/.env.production \
  --memory-max 512M
```

For Gunicorn with Uvicorn workers:

```bash
unitpm start "/srv/api/.venv/bin/gunicorn app.main:app \
  -k uvicorn.workers.UvicornWorker \
  --workers 4 \
  --bind 0.0.0.0:8000 \
  --access-logfile -" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file /srv/api/.env.production
```

## Option 2: Plain systemd unit file

```ini
# /etc/systemd/system/worker.service
[Unit]
Description=Python Worker
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/srv/worker
EnvironmentFile=/srv/worker/.env.production
ExecStart=/srv/worker/.venv/bin/python worker.py
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=worker

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now worker
sudo journalctl -u worker -f
```

## Option 3: Supervisor

Supervisor (supervisord) is Python-native and historically popular for Python services:

```ini
[program:worker]
command=/srv/worker/.venv/bin/python worker.py
directory=/srv/worker
user=www-data
autostart=true
autorestart=true
stdout_logfile=/var/log/supervisor/worker.log
stderr_logfile=/var/log/supervisor/worker-err.log
environment=LOG_LEVEL="info"
```

**Drawback**: If `supervisord` crashes, the worker dies with it. unitpm delegates to systemd so the worker survives unitpm restarts.

## Multiple workers with unitpm.yml

Declare the full stack as code:

```yaml
# unitpm.yml
version: 1
processes:
  api:
    command: .venv/bin/uvicorn app.main:app --host 0.0.0.0 --port 8000
    cwd: /srv/app
    restart: always
    env_file: .env.production
    memory_max: 512M
    namespace: app

  celery-worker:
    command: .venv/bin/celery -A app worker --loglevel=info
    cwd: /srv/app
    restart: on-failure
    env_file: .env.production
    namespace: app

  celery-beat:
    command: .venv/bin/celery -A app beat --loglevel=info
    cwd: /srv/app
    restart: on-failure
    env_file: .env.production
    namespace: app
```

```bash
unitpm apply unitpm.yml
```

Deploy everywhere identically:

```bash
git pull
unitpm apply unitpm.yml  # only changed processes restart
```

## Common issues

### ModuleNotFoundError on start

The system Python doesn't have your dependencies. Use the venv interpreter:

```bash
# Wrong
unitpm start "python3 worker.py" ...

# Right
unitpm start "/srv/worker/.venv/bin/python worker.py" ...
```

### Buffered stdout (logs not appearing)

Python buffers stdout by default. Force unbuffered output:

```bash
unitpm start "python3 -u worker.py" --name worker ...
# Or set env var
unitpm start "python3 worker.py" --name worker --env PYTHONUNBUFFERED=1 ...
```

### Worker exits with 0 (not restarting on clean exit)

Use `--restart always` if the worker should never stop:

```bash
unitpm start "python3 worker.py" --name worker --restart always ...
```

## See also

- [unitpm start](../reference/commands/start/) — full flag reference
- [How to set environment variables for a Linux service](./linux-service-environment-variables/)
- [Auto-restart on crash](./auto-restart-on-crash/)
- [Monitor process memory and CPU on Linux](./monitor-process-memory-cpu-linux/)
- [unitpm vs Supervisor](./vs-supervisor/) — detailed comparison
