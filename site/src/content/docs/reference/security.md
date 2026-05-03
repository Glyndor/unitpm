---
title: Security
description: Lynx security model — DynamicUser + landlock isolation, systemd credentials for secrets, release signing, SLSA provenance, and vulnerability disclosure.
---


Lynx is designed around the principle that a **process manager should not add attack surface**. Every managed process runs with the minimum privilege required. Secrets never appear in environment variable lists. The daemon itself runs as an unprivileged system user.

The security model rests on three Linux kernel primitives: **systemd DynamicUser** for per-process user isolation, **landlock LSM** for filesystem access restrictions, and **systemd credentials** for secret injection without exposing values to `ps` or `/proc/<pid>/environ`.

## Supported Versions

Only the latest minor release receives security updates. Older versions are
considered end-of-life.

| Version | Supported |
| ------- | --------- |
| 0.4.x   | ✅        |
| < 0.4   | ❌        |

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security reports.

Send a private report via GitHub's Private Vulnerability Reporting:

  https://github.com/Jaro-c/Lynx/security/advisories/new

Include:

- Affected version (`lynxpm version --json`)
- Reproduction steps
- Impact assessment
- Any proposed mitigation

You will receive an acknowledgement within 72 hours. Coordinated disclosure
windows are typically 30–90 days depending on severity.

## Threat Model

### In scope
- Daemon IPC surface (`lynx.sock`)
- Spec validation (process name, namespace, cwd, env, exec command)
- Process spawn flow (`exec.Cmd`, systemd integration, `DynamicUser`)
- Credential handling (`LoadCredential`, env file injection)
- Log file permissions and rotation

### Out of scope
- Compromise of the host kernel, systemd, or the `lynx` / user account itself
- Physical access
- Denial of service via legitimate resource exhaustion (e.g. user intentionally spawning 10k processes under their own uid)
- Vulnerabilities in managed applications themselves

## Design Guarantees

### Identity & Access Control

| Mode          | Socket                                     | Perms  | Who can connect                |
|---------------|--------------------------------------------|--------|--------------------------------|
| System mode   | `/run/lynxd/lynx.sock`                     | `0660` | `root` + `lynxadm` group       |
| User mode     | `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock`    | `0600` | Only the owner                 |

Peer identity verified via `SO_PEERCRED` on every connection. UID/GID/PID
of the caller are logged with every destructive action.

### Spec Validation (Server-Side)

All specs are validated in the daemon *after* IPC, never trusting the CLI:

- **Name**: `^[a-zA-Z0-9][a-zA-Z0-9 ._-]{0,63}$` — colon removed to prevent
  ambiguity with `namespace:name` resolution.
- **Namespace**: same regex as name.
- **Cwd**: canonicalized via `filepath.EvalSymlinks`; rejected if it resolves
  under `/etc`, `/proc`, `/sys`, `/boot`, `/dev`, `/run`.
- **Spec size**: bounded (see `internal/ipc/transport/limits.go`).
- **Env keys**: shell-safe characters only; no control chars.

### Process Isolation

**System mode with `--isolation dynamic`**:
- `systemd-run` with `DynamicUser=yes` creates a transient synthetic UID/GID.
- `ProtectSystem=strict`, `ProtectHome=read-only`, `PrivateTmp=yes`,
  `NoNewPrivileges=yes` applied to the transient unit.
- Secrets injected via `LoadCredential=` — never appear in `/proc/<pid>/environ`.
- Polkit rule restricts the `lynx` user to units whose names start with `lynx-`;
  it cannot stop or start `sshd`, `docker`, etc.

**System mode with `--isolation self`** (default):
- Process inherits the `lynx` system user's privileges.
- No synthetic user; suitable for apps that must read files owned by `lynx`.

**User mode**:
- `--isolation dynamic` is **not available**: the user's systemd instance
  cannot create synthetic UIDs.
- All processes run as the current user.

### Credential Handling

- `--env-file` is read by the daemon, written to a systemd `LoadCredential`
  slot (mode `0600`), then injected into the process at exec time via
  `CREDENTIALS_DIRECTORY`.
- If the write fails, the credentials directory is removed immediately to
  avoid leaving secrets on disk.
- The `_exec-env` internal wrapper re-parses the credential file into
  `environ` before `execve`, so the child sees real env vars but no other
  process can read them via `/proc/<pid>/environ` (inaccessible to non-root).

### Daemon Hardening

`lynxd.service` applies (see `debian/lynxpm.lynxd.service`):

- `NoNewPrivileges=yes`
- `ProtectSystem=strict`
- `ProtectHome=read-only`
- `PrivateTmp=yes`
- `ReadWritePaths=/var/lib/lynx-pm /var/log/lynx-pm /run/lynxd`
- `User=lynx`, `Group=lynx` (no root)
- Restart on failure

### Build Integrity

- Binaries are built with `-trimpath` to strip build-machine paths.
- Version, commit, and build date are injected via `-ldflags` — verifiable
  with `lynxpm version --json`.
- Releases are built via `scripts/build_deb.sh` from a clean checkout.

## Mitigations Shipped

1. **IPC rate limiting (v0.4.11).** Per-UID token-bucket (200 burst,
   100 req/s by default). Requests over limit receive `ERR_RATE_LIMIT`.
   Configurable via `LYNX_IPC_RATE_BURST` / `LYNX_IPC_RATE_PER_SEC`.
2. **Audit log (v0.4.11).** Every destructive action (start/stop/delete/
   reload/restart/reset/flush/scale) writes a JSON-line to
   `/var/log/lynx-pm/audit.log` (system mode, 0600). Includes caller
   UID/GID/PID, target ID+name+namespace, success/error, UTC timestamp.

## Known Limitations

Contributions welcome.

1. **No seccomp filter on managed processes.** Only `NoNewPrivileges` is
   applied. Per-app seccomp profiles are a planned feature.
2. **PID namespace visibility in `--isolation sandbox`.** The sandbox creates
   a new PID namespace, but remounting `/proc` inside is blocked by locked
   mounts and AppArmor policies on modern Ubuntu. `ps`, `top`, etc. still
   read the host `/proc` and see host processes. Filesystem access and UID
   isolation are unaffected (landlock + user namespace remain fully enforced).
3. **No signature verification** of the `lynxd` binary on startup.

## Security Contacts

- GitHub Private Vulnerability Reporting:
  <https://github.com/Jaro-c/Lynx/security/advisories/new>
