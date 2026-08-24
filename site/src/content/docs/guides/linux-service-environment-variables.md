---
title: How to set environment variables for a Linux service
description: Pass environment variables to a Linux service using unitpm process manager, plain systemd, or PM2. Covers env files, inline vars, secrets management, and per-environment configuration.
---

Passing configuration to a Linux service securely — without leaking secrets into shell history or process listings — requires a deliberate strategy. This guide covers how to set environment variables for a Linux service using unitpm process manager, plain systemd, and PM2.

## The problem with inline environment variables

Avoid this:

```bash
DATABASE_URL=postgres://user:password@host/db node server.js
```

Environment variables passed inline:
- Appear in `ps aux` output visible to all users
- Land in your shell history (`~/.bash_history`, `~/.zsh_history`)
- Get exposed in `/proc/[pid]/environ` (readable by any process running as the same user)

Use env files or systemd `EnvironmentFile` instead.

## unitpm: environment variable options

### Option 1: env file (recommended)

```bash
unitpm start "node server.js" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file /srv/api/.env.production
```

unitpm passes the file to systemd's `EnvironmentFile=` directive. Variables are loaded into the process environment but never stored in process listings.

**`.env.production` format**:

```bash
DATABASE_URL=postgres://user:secret@localhost/app
REDIS_URL=redis://localhost:6379
NODE_ENV=production
PORT=3000
LOG_LEVEL=info
```

Lines starting with `#` are comments. Quotes are optional (values are not shell-interpreted). Blank lines are ignored.

### Option 2: inline env vars

```bash
unitpm start "node server.js" \
  --name api \
  --restart always \
  --env NODE_ENV=production \
  --env PORT=3000
```

Use this only for non-sensitive configuration. Multiple `--env` flags are supported.

### Option 3: unitpm.yml

Declare variables in your declarative config:

```yaml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production
    env:
      NODE_ENV: production
      PORT: "3000"
```

`env_file` and `env` can coexist. `env` values override values from `env_file` when keys conflict.

### Inspect loaded variables

```bash
# Show all env vars for a running process
unitpm show api --env

# Or read from /proc directly
cat /proc/$(unitpm show api --pid)/environ | tr '\0' '\n'
```

## Plain systemd: EnvironmentFile

In a systemd unit file, use `EnvironmentFile`:

```ini
# /etc/systemd/system/api.service
[Unit]
Description=Node.js API

[Service]
Type=simple
User=www-data
WorkingDirectory=/srv/api
EnvironmentFile=/srv/api/.env.production
ExecStart=/usr/bin/node server.js
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart api
```

For multiple env files, add multiple `EnvironmentFile=` lines. If a file is optional, prefix with a minus: `EnvironmentFile=-/srv/api/.env.local`.

### Inline in unit file

```ini
[Service]
Environment=NODE_ENV=production
Environment=PORT=3000
```

Avoid this for secrets — the unit file is world-readable in `/etc/systemd/system/`.

## PM2

```bash
pm2 start server.js --name api --env production
```

PM2 uses `env_production` blocks in `ecosystem.config.js`:

```js
module.exports = {
  apps: [{
    name: 'api',
    script: 'server.js',
    env_production: {
      NODE_ENV: 'production',
      PORT: 3000,
    }
  }]
};
```

PM2 does not natively support `.env` files — you need a library like `dotenv` loaded in your application, or an npm package like `pm2-dotenv`.

## Per-environment configuration

A common pattern is multiple env files with an override layer:

```
/srv/api/
├── .env.base          # shared across all environments
├── .env.production    # production overrides
└── .env.staging       # staging overrides
```

With unitpm, pass the environment-specific file:

```bash
# Production server
unitpm start "node server.js" --name api --env-file /srv/api/.env.production

# Staging server
unitpm start "node server.js" --name api --env-file /srv/api/.env.staging
```

## Secrets management

For production secrets, avoid committing env files to git. Options:

### 1. Deploy env file separately

Keep `.env.production` out of version control. Provision it via your deploy pipeline (Ansible, Terraform, GitHub Actions secrets, etc.):

```bash
# GitHub Actions example
- name: Deploy env file
  run: echo "${{ secrets.ENV_PRODUCTION }}" > /srv/api/.env.production
```

### 2. Runtime secrets injection

Mount secrets from a secrets manager (Vault, AWS Secrets Manager, Doppler) at deploy time:

```bash
# Doppler example
doppler run -- unitpm start "node server.js" --name api --restart always
```

### 3. systemd credentials

For systemd 250+, use `LoadCredential`:

```ini
[Service]
LoadCredential=db-password:/etc/credentials/db-password
ExecStart=/usr/bin/node server.js
```

The credential is available at `$CREDENTIALS_DIRECTORY/db-password`. More secure than env files because credentials are never exposed in the environment.

## File permissions

Env files should be readable only by the service user:

```bash
sudo chown www-data:www-data /srv/api/.env.production
sudo chmod 600 /srv/api/.env.production
```

With unitpm's `DynamicUser=true` (default), the service runs as a generated user. Pass the env file path; unitpm configures systemd to read it with the right permissions.

## Common mistakes

| Mistake | Fix |
|---------|-----|
| Inline secrets in start command | Use `--env-file` |
| Committing `.env.production` to git | Add to `.gitignore`, provision via pipeline |
| World-readable env file (`chmod 644`) | `chmod 600`, owned by service user |
| Hardcoded paths in env file | Use relative paths + `--cwd`, or absolute with provisioning |
| Missing `NODE_ENV=production` | Always set — enables production optimizations in most frameworks |

## See also

- [unitpm start](../reference/commands/start/) — full flag reference
- [How to run a Node.js app as a Linux service](./nodejs-linux-service/)
- [Run a Python worker as a Linux service](./python-worker-linux/)
- [systemd DynamicUser sandboxing](./systemd-dynamicuser/)
