---
title: systemd DynamicUser — zero-privilege process sandboxing
description: Use systemd DynamicUser to run Linux services as unprivileged, isolated users with no persistent identity. How Lynx integrates DynamicUser sandboxing for Node.js, Python, and Go services.
---

`DynamicUser=true` is a systemd directive that runs a service as a **randomly generated, unprivileged user** that is created at service start and destroyed at service stop. Combined with filesystem isolation, it provides strong sandboxing with zero configuration complexity.

This is one of the key security features Lynx exposes — PM2 and Supervisor have no equivalent.

## What DynamicUser does

When `DynamicUser=true` is set on a systemd unit:

1. systemd allocates a UID/GID from the range 61184-65519 (or configured range)
2. The UID has no persistent entry in `/etc/passwd`
3. The process runs as that UID
4. On service stop, the UID is reclaimed — it never exists between runs

This means:
- **No persistent user account to compromise**: if an attacker escapes the process, there's no persistent identity to abuse
- **No home directory**: the dynamic user has no `~` with shell history, SSH keys, or `.bashrc`
- **Automatic privilege drop**: the service can never run as root, even if misconfigured

## DynamicUser in combination with other hardening

DynamicUser pairs naturally with these systemd directives (all managed by Lynx with `--sandbox`):

| Directive | Effect |
|-----------|--------|
| `DynamicUser=true` | Unprivileged random UID, no persistent account |
| `PrivateTmp=true` | Private `/tmp` — other services can't read temp files |
| `ProtectSystem=strict` | `/usr`, `/boot`, `/etc` are read-only |
| `ProtectHome=true` | `/home`, `/root`, `/run/user` are inaccessible |
| `NoNewPrivileges=true` | Process cannot gain privileges via setuid/setgid |
| `CapabilityBoundingSet=` | Drops all Linux capabilities |
| `RestrictNamespaces=true` | Cannot create namespaces (prevents container escape) |
| `LockPersonality=true` | Cannot change ABI personality |

## Enable DynamicUser with Lynx

### Command line

```bash
lynxpm start "node server.js" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file .env \
  --sandbox
```

`--sandbox` enables `DynamicUser=true` plus a hardened set of systemd security directives.

### Lynxfile.yml

```yaml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production
    dynamic_user: true
    memory_max: 512M
```

### Verify sandboxing is active

```bash
lynxpm show api --security
# DynamicUser:      enabled
# PrivateTmp:       enabled
# ProtectSystem:    strict
# NoNewPrivileges:  enabled
# CapabilityBoundingSet: (empty)

# Or inspect the underlying systemd unit
systemctl show lynx-api.service | grep -E 'DynamicUser|PrivateTmp|ProtectSystem'
```

## State directories with DynamicUser

Since `DynamicUser` uses a rotating UID, file ownership across restarts is handled by systemd's state directory mechanism:

```bash
# Directories are created with the correct UID and persist across restarts
lynxpm start "node server.js" \
  --name api \
  --sandbox \
  --state-dir /var/lib/lynx-api \
  --cache-dir /var/cache/lynx-api
```

Or in systemd terms, `StateDirectory=lynx-api` creates `/var/lib/lynx-api` owned by the dynamic UID, and systemd reassigns ownership on each start. Your app can write there safely.

In Lynxfile.yml:

```yaml
processes:
  api:
    command: node server.js
    dynamic_user: true
    state_directory: lynx-api
    cache_directory: lynx-api
```

Accessible at runtime as `/var/lib/lynx-api` and `/var/cache/lynx-api`.

## What DynamicUser restricts

With `--sandbox` / `DynamicUser=true`:

| Action | Result |
|--------|--------|
| Write to `/tmp` | Allowed (private `/tmp`) |
| Write to `/var/lib/lynx-api` (state dir) | Allowed |
| Write to `/etc` | Blocked (read-only) |
| Read `/home/otheruser` | Blocked |
| Bind port < 1024 | Blocked (no `CAP_NET_BIND_SERVICE`) |
| Create setuid binary | Blocked (`NoNewPrivileges`) |
| `fork()` + exec new process | Allowed |
| Network access | Allowed (no restriction by default) |

## Network sandboxing (additional)

To restrict network access to localhost only:

```bash
lynxpm start "node worker.js" \
  --name worker \
  --sandbox \
  --restrict-network private
```

Or restrict to specific address families:

```bash
# IPv4 only
lynxpm start "node server.js" --name api --sandbox --address-families inet
```

## Landlock filesystem restriction

Lynx also supports Linux 5.13+ Landlock for fine-grained filesystem access control:

```bash
lynxpm start "node server.js" \
  --name api \
  --sandbox \
  --landlock-allow /srv/api:rx \
  --landlock-allow /var/lib/lynx-api:rwx \
  --landlock-allow /etc/ssl:rx
```

This restricts the process to only the listed paths with the specified permissions. Any access to unlisted paths returns `EACCES`.

## Security analysis

Check your service's systemd security score:

```bash
systemd-analyze security lynx-api.service
```

Output:

```
  NAME                                                        DESCRIPTION                       EXPOSURE
✓ PrivateNetwork=                                             Service has no private network       0.5
✓ User=/DynamicUser=                                         Service runs under a static non-...  0
✓ NoNewPrivileges=                                           Service processes cannot acquire...  0
✓ PrivateTmp=                                                Service has a private /tmp/         0.5
…
→ Overall exposure level for lynx-api.service: 1.6 OK 🙂
```

A Lynx-managed service with `--sandbox` typically scores below 2.0 (excellent). A plain PM2 or Supervisor setup with no hardening scores 9.0+ (dangerous).

## Comparison with PM2 and Supervisor

| | Lynx (--sandbox) | PM2 | Supervisor |
|--|---------|-----|-----------|
| DynamicUser | ✓ | ✗ | ✗ |
| PrivateTmp | ✓ | ✗ | ✗ |
| NoNewPrivileges | ✓ | ✗ | ✗ |
| ProtectSystem | ✓ | ✗ | ✗ |
| Landlock | ✓ | ✗ | ✗ |
| Configurable via CLI | ✓ | ✗ | ✗ |

PM2 and Supervisor run your process as whatever user you invoke them with. If that user is `root` (common in many tutorials), your application has full root access. Lynx's `--sandbox` enforces privilege drop at the systemd level — it cannot be bypassed by the application.

## Getting started

```bash
# Start with sandboxing enabled
lynxpm start "node server.js" \
  --name api \
  --restart always \
  --cwd /srv/api \
  --env-file .env.production \
  --sandbox \
  --state-dir /var/lib/my-api

# Verify
systemd-analyze security lynx-api.service
```

## See also

- [lynxpm start](../reference/commands/start/) — `--sandbox`, `--dynamic-user`, `--landlock-allow` flags
- [systemd-native process manager](./systemd-process-manager/) — architecture overview
- [How to run a Node.js app as a Linux service](./nodejs-linux-service/)
- [Lynx vs PM2](./vs-pm2/) — security comparison
