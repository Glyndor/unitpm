# 🦁 `unitpm start`

> *Start a new process managed by unitpm.*

## 📖 Synopsis

```bash
unitpm start <command|file> [flags] [-- <args...>]
```

## Description

Start a new process managed by unitpm. This command creates a new application specification and starts the process via the daemon.

## ⚙️ Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `--name` | string | auto | Assign a name to the process. | `--name my-api` |
| `--namespace` | string | default | Namespace for grouping and resolution. | `--namespace prod` |
| `--cwd` | string | CWD | Working directory for the process. | `--cwd /var/www` |
| `--shell` | boolean | false | Execute command inside a shell (`/bin/sh -c`). | `--shell` |
| `--schedule`, `--cron` | string | - | Cron schedule for restart (e.g. "@hourly"). | `--schedule "0 0 * * *"` |
| `--restart` | string | on-failure | Restart policy (`never`, `on-failure`, `always`). | `--restart always` |
| `--max-restarts` | int | 10 | Maximum number of restarts before giving up. | `--max-restarts 5` |
| `--restart-delay` | int | 2000 | Delay between restarts in milliseconds. | `--restart-delay 5000` |
| `--backoff` | string | expo | Backoff strategy (`none`, `linear`, `expo`). | `--backoff linear` |
| `--stop-on-exit` | list | 0 | Comma-separated exit codes that stop the process. | `--stop-on-exit 0,143` |
| `--log-dir` | string | auto | Directory for log files (default: system or user local). | `--log-dir /var/log/my-app` |
| `--stdout` | string | auto | Stdout log filename (relative to log-dir). | `--stdout stdout.log` |
| `--stderr` | string | auto | Stderr log filename (relative to log-dir). | `--stderr stderr.log` |
| `--log-format` | string | plain | Log format (`plain`, `json`). | `--log-format json` |
| `--log-timestamp` | string | rfc3339 | Log timestamp (`rfc3339`, `unix`, `none`). | `--log-timestamp unix` |
| `--runtime` | string | - | Runtime for entry file (e.g., node, python). | `--runtime python3` |
| `--env-file` | string | - | Path to a file containing environment variables. | `--env-file .env` |
| `--isolation` | string | self | Isolation mode (`self`, `dynamic`, `sandbox`). | `--isolation sandbox` |
| `--scale`, `--instances` | int | 1 | Number of instances to start. | `--scale 4` |
| `--stop-signal` | string | SIGTERM | Signal on stop (SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2). | `--stop-signal SIGINT` |
| `--stop-timeout` | int | 10000 | Grace period before SIGKILL, in ms (1000–300000). | `--stop-timeout 30000` |
| `--memory-max` | string | unlimited | Hard memory ceiling: `512M`, `2G`, or raw bytes. | `--memory-max 512M` |
| `--cpu-max` | int | unlimited | CPU cap as percent of one core (100=1 core, 200=2 cores). | `--cpu-max 100` |
| `--tasks-max` | int | unlimited | Maximum tasks (threads + subprocesses). | `--tasks-max 64` |
| `-n`, `--dry-run` | - | - | Print the resolved spec without starting (rendered as a `Spec` table; pair with `--json` for machine-readable output). | `--dry-run` |
| `--json` | boolean | false | Emit the start result as JSON on stdout (`{started, count}`). Works with `--dry-run` too (`{spec, scale}`). | `--json` |
| `-q`, `--quiet` | - | - | Suppress success messages; errors still printed. | `--quiet` |
| `--no-list` | boolean | false | Skip the process list printed after the action. The started instances are otherwise highlighted (▸) in the list for easy scanning. | `--no-list` |
| `-h`, `--help` | - | - | Show help message. | — |

## Supported Runtimes

Any Linux executable works. For language-specific recipes (Node/Bun/Deno,
Python with venv / uv / uvx, Go / Rust / Ruby / Java, shell scripts),
see [`docs/RUNTIMES.md`](../RUNTIMES.md).

## 🚀 Examples

Start a Node.js script:
```bash
unitpm start main.js
```

Start with DynamicUser isolation (secure):
```bash
unitpm start main.js --isolation dynamic
```

Start a scheduled task (runs every hour):
```bash
unitpm start cleanup.sh --schedule "@hourly" --restart never
```

## 📋 Example Output

Success:
```
Spec saved to /home/user/.config/unitpm/apps/my-api.json
Started my-api
  ID: e73a9f1b
  PID: 12345
  Status: online
```

Error (invalid path):
```
Error: ERR_BAD_REQUEST: invalid cwd: stat /invalid/path: no such file or directory (BAD_REQUEST)
```

## Mode Explanations

### Restart Policies
| Policy | Description |
|--------|-------------|
| `never` | Never restart the process, regardless of exit code. |
| `on-failure` | Restart only if the process exits with a non-zero code (or code not in `--stop-on-exit`). |
| `always` | Always restart the process, even if it exits successfully (code 0). |

### Backoff Strategies
| Strategy | Description |
|----------|-------------|
| `none` | No delay between restarts (immediate). |
| `linear` | Delay increases linearly: `delay * restart_count`. |
| `expo` | Delay increases exponentially: `delay * 2^(restart_count-1)`. Capped at 5 minutes. |

### Logging
| Option | Values | Description |
|--------|--------|-------------|
| `format` | `plain` | Raw output as received from the process. |
| | `json` | Wrap output in JSON structure with metadata. |
| `timestamp` | `rfc3339` | ISO 8601 format (e.g., `2024-01-01T12:00:00Z`). |
| | `unix` | Unix timestamp (seconds). |
| | `none` | No timestamp added. |

### Isolation
| Mode | Description |
|------|-------------|
| `self` | Run as the current user (same as `unitpmd`). Default. |
| `dynamic` | Run as a transient, isolated user via `systemd-run`. Uses `DynamicUser=yes` with hardening (`NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome=yes`). |

## Framework recipes

Per-framework patterns (Next.js, FastAPI, Django, Rails, Spring, static
sites, cron jobs) live in [`docs/TUTORIALS.md`](../TUTORIALS.md) —
runtime-specific invocations (Bun, Deno, venv/uv, compiled Go/Rust,
`fnm`/`pyenv`/`rbenv`) live in [`docs/RUNTIMES.md`](../RUNTIMES.md).

## Scaling

`--scale N` (alias `--instances N`) spawns N independent processes. Each
one gets a unique ID and name, plus `UNITPM_INSTANCE=0..N-1` in its env.
unitpm **does not** load-balance — put a reverse proxy (nginx/Caddy/HAProxy)
in front of the instances, or use `SO_REUSEPORT` if your runtime supports
it.

```js
// server.js — give each instance its own port
const port = 3000 + Number(process.env.UNITPM_INSTANCE ?? 0);
```

Worked examples (Nginx upstream, `--scale` with Next.js standalone,
live scale up/down) in [`TUTORIALS.md`](../TUTORIALS.md).

## Notes

- **Auto-naming**: omit `--name` and unitpm derives `<basename>-<shortid>`,
  or `<basename>-<index>-<shortid>` when `--scale > 1`.
- **Manual restarts reset the counter**: `unitpm restart <id>` clears the
  restart count and backoff timer. `--max-restarts` only caps the crash
  loop, not manual operator actions.
- **Visibility**: in system mode, processes are visible to anyone in the
  `unitpm` group. In user mode (`unitpmd &`), each user has a private
  daemon — no cross-user visibility.

## Environment variables

- **User mode** — the process inherits the full environment of the user
  running `unitpm start`.
- **System mode** — the daemon is run by the `glyndor-unitpm` system user and
  does **not** forward its caller's env (prevents leaking `AWS_*` /
  `DATABASE_URL` / etc.). Whitelisted: `PATH`, `HOME`, `USER`,
  `LOGNAME`, `SHELL`, `PWD`, `LANG`, `LC_*`, `TERM`, `TZ`, `TMPDIR`,
  `XDG_*`. Anything else must come from `--env-file` or `AppSpec.Env`.

## Security

- **Secrets stay off disk**: values loaded via `--env-file` are injected
  into the process env but **not** written into the AppSpec JSON in
  `~/.config/unitpm/apps/`. No plaintext credentials on-disk in the spec.
- **`--shell` is gated**: accepted in user mode only. System mode
  refuses it — shell evaluation of an attacker-controlled string against
  the daemon's privileges is the exact footgun the hardening model
  rules out.
- **Isolation picker**:

  | Mode | Works in | Trade-off |
  |------|----------|-----------|
  | `self` *(default)* | user + system | Zero overhead. No containment — the process inherits the daemon user. |
  | `dynamic` | system only | Strongest. `systemd-run` wraps the process: transient UID/GID, `ProtectSystem=strict`, `PrivateTmp`, `ProtectHome=yes`, `NoNewPrivileges`. Secrets pass via `LoadCredential` — `/proc/<pid>/environ` shows nothing. Recommended for network-facing prod services. |
  | `sandbox` | user + system | User-namespace + landlock. Blocks writes outside `cwd` + `/tmp`. No root or polkit needed. |

  In `--isolation dynamic --env-file …` unitpm stages the file at
  `/var/lib/unitpm/creds/<id>/env` (`0600`, daemon-owned) and exposes
  it via systemd credentials — a small internal wrapper reads the
  credential and `exec`s your command.
