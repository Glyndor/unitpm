---
title: Zero-downtime deployment on Linux
description: Deploy application updates on Linux without dropping connections using Lynx process manager. Covers graceful restart, rolling deploys, signal handling, health checks, and blue-green deployment.
---

A **zero-downtime deployment** updates a running application without dropping active HTTP connections or interrupting in-progress work. This guide explains how to achieve zero-downtime deploys on Linux using Lynx process manager, graceful shutdown patterns, and Nginx.

## The problem with naive restarts

A naive `kill && start` sequence:

1. Kill old process (all in-flight requests fail with 502/connection reset)
2. Start new process (startup latency — 0 to several seconds)
3. Traffic flows again

During step 1-2, users see errors. Unacceptable for production.

## What makes a restart "graceful"

A graceful restart requires two things:

1. **The process handles SIGTERM**: finishes in-flight requests, stops accepting new ones, then exits
2. **The process manager waits**: gives the process time to drain before sending SIGKILL

### Example: graceful shutdown in Node.js

```js
const server = app.listen(3000);

process.on('SIGTERM', () => {
  server.close(() => {
    // All connections drained
    process.exit(0);
  });

  // Force exit if drain takes too long
  setTimeout(() => process.exit(1), 25000);
});
```

### Example: graceful shutdown in Go

```go
srv := &http.Server{Addr: ":8080", Handler: mux}

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

### Example: graceful shutdown in Python (FastAPI)

```python
from contextlib import asynccontextmanager
from fastapi import FastAPI

@asynccontextmanager
async def lifespan(app: FastAPI):
    yield
    # Cleanup on shutdown: drain connections, close DB pool
    await db.close()

app = FastAPI(lifespan=lifespan)
```

FastAPI + Uvicorn handle SIGTERM gracefully by default.

## Graceful restart with Lynx

Lynx's `restart` command sends SIGTERM, waits for the stop timeout, then starts the new process:

```bash
lynxpm restart api
```

Configure how long Lynx waits before sending SIGKILL:

```bash
lynxpm start "node server.js" \
  --name api \
  --restart always \
  --stop-timeout 30000
```

`--stop-timeout 30000` = wait up to 30 seconds for graceful drain before SIGKILL.

For a standard deploy workflow:

```bash
# 1. Pull and build
git pull origin main
npm ci --production

# 2. Graceful restart (SIGTERM → wait → start new)
lynxpm restart api

# 3. Verify
lynxpm show api
# Status: running
# Restarts: 1
# Uptime: 0m 12s
```

## Rolling deploy for multiple processes

When running multiple worker processes, restart one at a time to keep capacity online:

```bash
# Start 3 workers with explicit names
lynxpm start "node worker.js" --name worker-1 --namespace prod --restart always
lynxpm start "node worker.js" --name worker-2 --namespace prod --restart always
lynxpm start "node worker.js" --name worker-3 --namespace prod --restart always

# Update: restart one at a time
lynxpm restart worker-1 && sleep 5
lynxpm restart worker-2 && sleep 5
lynxpm restart worker-3
```

For Nginx-proxied HTTP services, this keeps at least 2/3 workers accepting requests during the deploy.

## Nginx + upstream health checks

Configure Nginx to detect unhealthy upstreams:

```nginx
upstream api {
    server 127.0.0.1:4001;
    server 127.0.0.1:4002;
    server 127.0.0.1:4003;
}

server {
    location /api/ {
        proxy_pass http://api/;
        proxy_next_upstream error timeout http_502 http_503;
        proxy_next_upstream_tries 2;
    }
}
```

`proxy_next_upstream` retries on error to the next healthy upstream. Combined with Lynx's graceful restart, a deploy causes zero dropped requests from the user's perspective.

## Blue-green deployment

Blue-green runs two environments: one live, one idle. Deploy to idle, then switch:

```
Blue (live):  port 4000  ← Nginx proxies here
Green (idle): port 5000  ← deploy new version here
```

### Setup with Lynx

```bash
# Initial: blue is live
lynxpm start "node server.js" --name api-blue --restart always \
  --cwd /srv/api \
  --env-file .env \
  --env PORT=4000

# Start green with new version (doesn't affect live traffic yet)
lynxpm start "node server.js" --name api-green --restart always \
  --cwd /srv/api-new \
  --env-file .env \
  --env PORT=5000
```

### Verify green is healthy

```bash
curl -s http://127.0.0.1:5000/health
# {"status":"ok","version":"2.1.0"}
```

### Switch Nginx to green

```nginx
upstream api {
    server 127.0.0.1:5000;  # was 4000
}
```

```bash
sudo nginx -t && sudo nginx -s reload
# Zero-downtime: Nginx reloads config without dropping connections
```

### Remove blue

```bash
lynxpm stop api-blue
lynxpm delete api-blue
```

## Declarative deploys with Lynxfile

Use `lynxpm apply` for idempotent updates:

```yaml
# Lynxfile.yml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production
    stop_timeout: 30000
    memory_max: 512M
```

```bash
# Deploy: pull code, apply config
git pull
lynxpm apply Lynxfile.yml
```

`apply` compares current state to the file. Only changed processes restart. Unchanged processes keep running with zero disruption.

## Smoke test after deploy

Automate verification:

```bash
#!/bin/bash
# deploy.sh
set -e

git pull origin main
npm ci --production
lynxpm restart api

# Wait for process to start
sleep 3

# Health check
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:3000/health)
if [ "$HTTP_CODE" != "200" ]; then
  echo "Deploy failed: health check returned $HTTP_CODE"
  lynxpm logs api --lines 50
  exit 1
fi

echo "Deploy OK"
```

## Common mistakes

| Mistake | Result | Fix |
|---------|--------|-----|
| No SIGTERM handler | SIGKILL after stop-timeout, connections dropped | Handle SIGTERM in app |
| Stop timeout too short | In-flight requests cut off | Match timeout to max request duration |
| Restart whole namespace at once | All processes restart simultaneously, 100% downtime | Restart one-by-one or use rolling |
| Health check not implemented | Can't verify deploy success | Add `/health` endpoint |
| Config change without restart | New env vars not loaded | `lynxpm restart` after env file changes |

## See also

- [lynxpm restart](../reference/commands/restart/) — command reference
- [lynxpm apply](../reference/commands/apply/) — declarative apply
- [Auto-restart on crash](./auto-restart-on-crash/)
- [Manage multiple Node.js apps on a VPS](./manage-multiple-nodejs-apps-vps/)
- [How to run a Node.js app as a Linux service](./nodejs-linux-service/)
