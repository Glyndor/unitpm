# 🦁 Tutorials

Real-world recipes. Copy-paste and adapt.

## 🎯 Pick your stack

| Stack | Jump to | Time |
|-------|---------|------|
| ▲ Next.js | [Next.js](#-nextjs) | 3 min |
| 🟢 Express / Fastify | [Express / Fastify (Node.js)](#-express--fastify-nodejs) | 2 min |
| 🥟 Bun | [Bun](#-bun) | 1 min |
| 🐍 FastAPI + Uvicorn | [Python — FastAPI + Uvicorn](#-python--fastapi--uvicorn) | 2 min |
| 🦄 Django + Gunicorn | [Python — Django + Gunicorn](#-python--django--gunicorn) | 2 min |
| 🐹 Go web server | [Go web server](#-go-web-server) | 2 min |
| 🦀 Rust (Actix/Axum) | [Rust (Actix / Axum)](#-rust-actix--axum) | 2 min |
| 📄 Static site | [Static site server (Caddy / Nginx)](#-static-site-server-caddy--nginx) | 1 min |
| ⏰ Cron / scheduled | [Cron / scheduled tasks](#-cron--scheduled-tasks) | 1 min |
| 🔒 Production hardening | [Secure isolation (production)](#-secure-isolation-production) | 3 min |
| 🚀 Full deploy walkthrough | [Full production deploy (step by step)](#-full-production-deploy-step-by-step) | 10 min |
| 📜 unitpm.yml (declarative) | [unitpm.yml — declarative multi-app deploy](#-unitpmyml--declarative-multi-app-deploy) | 5 min |
| 📊 Monitor & debug | [Monitoring and debugging](#-monitoring-and-debugging) | 1 min |
| 💡 Daily-use tips | [Tips](#-tips) | - |

> 💡 **Tip**: all examples work identically in user mode (`unitpmd &`) and
> system mode (`sudo systemctl start unitpmd`). The only difference in prod:
> swap `--isolation self` for `--isolation dynamic`.

---

## ▲ Next.js

### Development

```bash
# Inside your Next.js project directory
unitpm start "npm run dev" --name nextjs-dev --cwd /srv/myapp --shell
unitpm logs nextjs-dev --follow
```

**What you see:**
```
Started nextjs-dev
  ID: 019d93ab-...  PID: 12345  Status: running
[STDOUT]   ▲ Next.js 15.0.0
[STDOUT]   - Local:        http://localhost:3000
[STDOUT]   ✓ Ready in 2.1s
```

### Production (standalone build)

```bash
# 1. Build first
cd /srv/myapp && npm run build

# 2. Start the standalone server
unitpm start "node .next/standalone/server.js" \
    --name nextjs-prod \
    --cwd /srv/myapp \
    --restart always \
    --env-file .env.production \
    --memory-max 512M

# 3. Verify
unitpm show nextjs-prod
```

### Production + multiple instances (cluster-like)

Next.js standalone doesn't support Node cluster natively. Use `--scale`
instead — each instance listens on a different port:

```bash
# Start 3 instances; each reads UNITPM_INSTANCE to pick a port
unitpm start "node .next/standalone/server.js" \
    --name nextjs \
    --cwd /srv/myapp \
    --scale 3 \
    --restart always \
    --env-file .env.production

# In your server.js or next.config.js:
#   const port = 3000 + Number(process.env.UNITPM_INSTANCE || 0);
```

Then put Nginx or Caddy in front:

```nginx
upstream nextjs {
    server 127.0.0.1:3000;
    server 127.0.0.1:3001;
    server 127.0.0.1:3002;
}
server {
    listen 80;
    location / { proxy_pass http://nextjs; }
}
```

### Scale up / down on the fly

```bash
unitpm scale nextjs 5    # add 2 more instances
unitpm scale nextjs 2    # drop back to 2
```

**Output:**
```
Scaled nextjs: 3 → 5
  + nextjs-4
  + nextjs-5
```

> ⚠️ **Warning**: Each instance must bind a unique port. Read `UNITPM_INSTANCE`
> (0-based) and compute `port = 3000 + UNITPM_INSTANCE`.

---

## 🟢 Express / Fastify (Node.js)

```bash
# Simple
unitpm start "node server.js" --name api --cwd /srv/api --restart always

# With env file
unitpm start "node server.js" \
    --name api \
    --cwd /srv/api \
    --env-file .env \
    --restart always \
    --memory-max 256M

# Cluster (4 workers)
unitpm start "node server.js" --name api --scale 4 --cwd /srv/api
# Your app reads process.env.UNITPM_INSTANCE to bind to port 3000+N
```

### Graceful shutdown (Express)

Express needs SIGINT to close connections cleanly:

```bash
unitpm start "node server.js" \
    --name api \
    --stop-signal SIGINT \
    --stop-timeout 30000 \
    --restart always
```

In your Express app:

```js
process.on('SIGINT', () => {
    server.close(() => process.exit(0));
});
```

---

## 🥟 Bun

```bash
# Dev
unitpm start "bun run dev" --name bun-dev --cwd /srv/app

# Production
unitpm start "bun run src/index.ts" \
    --name bun-prod \
    --cwd /srv/app \
    --restart always \
    --memory-max 256M

# Hot reload: Bun already watches files by default in dev
```

---

## 🐍 Python — FastAPI + Uvicorn

```bash
# Development (with reload)
unitpm start "uvicorn main:app --reload --host 0.0.0.0 --port 8000" \
    --name fastapi-dev \
    --cwd /srv/api \
    --shell

# Production (with uv)
unitpm start "uv run uvicorn main:app --host 0.0.0.0 --port 8000 --workers 4" \
    --name fastapi-prod \
    --cwd /srv/api \
    --restart always \
    --memory-max 1G \
    --env-file .env

# Production with venv (direct path)
unitpm start "/srv/api/.venv/bin/uvicorn main:app --host 0.0.0.0 --port 8000" \
    --name fastapi-prod \
    --cwd /srv/api \
    --restart always
```

---

## 🦄 Python — Django + Gunicorn

```bash
# Via uv
unitpm start "uv run gunicorn myproject.wsgi:application --bind 0.0.0.0:8000 --workers 4" \
    --name django \
    --cwd /srv/django \
    --restart always \
    --env-file .env \
    --stop-signal SIGINT \
    --stop-timeout 30000

# Via venv
unitpm start "/srv/django/.venv/bin/gunicorn myproject.wsgi:application -b 0.0.0.0:8000" \
    --name django \
    --cwd /srv/django \
    --restart always
```

---

## 🐹 Go web server

```bash
# Compiled binary (recommended for production)
cd /srv/api && go build -o bin/api ./cmd/api
unitpm start ./bin/api \
    --name go-api \
    --cwd /srv/api \
    --restart always \
    --memory-max 128M \
    --stop-signal SIGINT \
    --stop-timeout 15000

# Development (go run)
unitpm start "go run ./cmd/api" --name go-dev --cwd /srv/api
```

Go servers typically handle SIGINT for graceful shutdown:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()
srv.Shutdown(ctx)
```

---

## 🦀 Rust (Actix / Axum)

```bash
# Build and run
cd /srv/api && cargo build --release
unitpm start ./target/release/api \
    --name rust-api \
    --cwd /srv/api \
    --restart always \
    --memory-max 64M
```

---

## 📄 Static site server (Caddy / Nginx)

```bash
# Caddy (auto-HTTPS)
unitpm start "caddy run --config /srv/site/Caddyfile" \
    --name caddy \
    --restart always \
    --stop-signal SIGINT

# Python simple server (quick sharing)
unitpm start "python3 -m http.server 8080" \
    --name static \
    --cwd /srv/site
```

---

## ⏰ Cron / scheduled tasks

```bash
# Run a backup script every 6 hours
unitpm start "/srv/scripts/backup.sh" \
    --name backup \
    --schedule "0 */6 * * *" \
    --restart never

# Run a health probe every 10 seconds (sidecar pattern)
unitpm start "curl -sSf http://localhost:3000/healthz || exit 1" \
    --name probe \
    --schedule "@every 10s" \
    --restart on-failure \
    --shell
```

---

## 🔒 Secure isolation (production)

### DynamicUser (system mode, strongest)

Each process runs as a unique synthetic user. Secrets never appear in
`/proc/<pid>/environ`.

```bash
unitpm start "node server.js" \
    --name api \
    --cwd /srv/api \
    --isolation dynamic \
    --env-file .env.production \
    --restart always \
    --memory-max 512M \
    --stop-signal SIGINT \
    --stop-timeout 15000
```

### Sandbox (user mode, no sudo)

Runs inside user namespace + landlock. Can't write to `/home`, `/etc`,
`/usr`. Can write to cwd + `/tmp`.

```bash
unitpm start "node server.js" \
    --name api \
    --cwd /srv/api \
    --isolation sandbox \
    --restart always
```

---

## 🚀 Full production deploy (step by step)

A complete workflow for deploying a Node.js API:

```bash
# 1. Install unitpm
sudo apt install ./unitpm_*_amd64.deb
sudo usermod -aG unitpm $USER && newgrp unitpm

# 2. Make dev tools visible to the daemon
unitpm install-tools

# 3. Prepare app directory
sudo mkdir -p /srv/api && sudo chown $USER:$USER /srv/api
cd /srv/api && git clone https://github.com/you/api.git .
npm install && npm run build

# 4. Create env file (secrets stay on disk, not in ps)
cat > .env.production <<EOF
DATABASE_URL=postgres://user:pass@db:5432/app
PORT=3000
NODE_ENV=production
EOF

# 5. Start with all hardening
unitpm start "node dist/server.js" \
    --name api \
    --namespace prod \
    --cwd /srv/api \
    --env-file .env.production \
    --isolation dynamic \
    --restart always \
    --memory-max 512M \
    --stop-signal SIGINT \
    --stop-timeout 30000

# 6. Scale to 3 workers
unitpm scale prod:api 3

# 7. Verify
unitpm list --namespace prod
unitpm logs prod:api --follow

# 8. Enable boot persistence
sudo unitpm startup
```

**What `unitpm list --namespace prod` shows after step 6:**
```
┌──────────┬───────┬───────────┬─────────┬────────┬───────┬─────────┬─────┬────────┐
│ id       │ name  │ namespace │ version │ mode   │ pid   │ status  │ cpu │ mem    │
├──────────┼───────┼───────────┼─────────┼────────┼───────┼─────────┼─────┼────────┤
│ 019d9... │ api-1 │ prod      │ 0.0.1   │ forked │ 12340 │ running │ 0%  │ 52 MB  │
│ 019d9... │ api-2 │ prod      │ 0.0.1   │ forked │ 12341 │ running │ 0%  │ 48 MB  │
│ 019d9... │ api-3 │ prod      │ 0.0.1   │ forked │ 12342 │ running │ 0%  │ 50 MB  │
└──────────┴───────┴───────────┴─────────┴────────┴───────┴─────────┴─────┴────────┘
```

> 💡 **Tip**: `sudo unitpm startup` wires the `unitpmd.service` into
> systemd so apps restart after reboot. All specs in `~/.config/unitpm/apps/`
> are restored automatically at boot.

---

## 📜 unitpm.yml — declarative multi-app deploy

Instead of individual `start` commands, declare everything in a file:

```yaml
# unitpm.yml
version: "1"
namespace: prod
apps:
  - name: api
    command: node dist/server.js
    cwd: /srv/api
    env_file: .env.production
    restart:
      policy: always
      max_restarts: 10
      backoff: expo

  - name: worker
    command: node dist/worker.js
    cwd: /srv/api
    env_file: .env.production
    restart:
      policy: always

  - name: scheduler
    command: node dist/scheduler.js
    cwd: /srv/api
    restart:
      policy: always
```

```bash
unitpm apply unitpm.yml
unitpm list --namespace prod
```

Update later:

```bash
# Edit unitpm.yml, then:
unitpm delete --namespace prod   # wipe the whole namespace in one shot
unitpm apply unitpm.yml
```

---

## 📊 Monitoring and debugging

```bash
# Live dashboard (refreshes every 2s, Ctrl+C to exit)
unitpm monit

# JSON output for scripting
unitpm list --json | jq '.[] | select(.state == "running") | {name, pid, memory}'

# Check restart history
unitpm show api

# Reset counter after fixing a bug
unitpm reset api

# View logs
unitpm logs api --follow           # both stdout+stderr
unitpm logs api --stdout --lines 50  # only stdout, last 50 lines

# Flush old logs
unitpm flush api
```

---

## 💡 Tips

1. **Name your processes.** `--name api` is easier to type than a UUID.
2. **Use namespaces.** `--namespace prod` + `--namespace staging` keeps
   things clean. Filter with `unitpm list --namespace prod`.
3. **Use `namespace:name` syntax.** `unitpm show prod:api`, `unitpm stop
   staging:worker`.
4. **Bulk lifecycle ops by namespace.** Every lifecycle command (`stop`,
   `restart`, `reload`, `reset`, `delete`, `flush`) accepts `--namespace
   <ns>` or the `<ns>:*` selector to target a whole namespace at once.
   Use `'*'` (quoted) to hit every managed process. Examples:
   ```bash
   unitpm restart --namespace prod    # roll the prod tier
   unitpm flush 'staging:*'           # truncate logs across staging
   unitpm delete --namespace prod --purge   # wipe + drop logs
   ```
5. **Always set `--restart always` in production.** Default `on-failure`
   doesn't restart on clean exit.
6. **Set `--memory-max` in production.** Prevents a single leak from
   killing the host. The daemon auto-restarts when the OOM kills the
   process.
7. **Use `--stop-signal SIGINT` for Node.js/Python.** These runtimes
   handle SIGINT more gracefully than SIGTERM by default.
8. **Use `--dry-run` when unsure.** `unitpm start "complex command" --dry-run`
   prints the resolved spec without touching the daemon.
9. **Use `--quiet` in scripts.** `unitpm start ... -q && echo ok` keeps
   CI output clean.
10. **Export + apply for backups.** `unitpm export --namespace prod > backup.yml`
    saves your running config. Restore with `unitpm apply backup.yml`.
11. **Shell completion saves keystrokes.**
    `unitpm completion bash > ~/.local/share/bash-completion/completions/unitpm`
