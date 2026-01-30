# start

## Synopsis

```bash
lynx start <command|file> [flags] [-- <args...>]
```

## Usage

Start a new process managed by Lynx. This command creates a new application specification and starts the process via the daemon.

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `--name` | string | auto | Assign a name to the process. | `--name my-api` |
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
| `--isolation` | string | self | Isolation mode (`self`, `dynamic`). | `--isolation dynamic` |
| `-h`, `--help` | - | - | Show help message. | — |

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
| `self` | Run as the current user (same as `lynxd`). Default. |
| `dynamic` | Run as a transient, isolated user via `systemd-run`. Uses `DynamicUser=yes` with hardening (`NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome=yes`). |

## Framework Examples

| Framework | Command |
|-----------|---------|
| **Next.js (dev)** | `lynx start --name next-dev -- npm run dev` |
| **Next.js (prod)** | `lynx start --name next-prod -- npm start` |
| **Next.js (pnpm)** | `lynx start --name next-pnpm -- pnpm dev` |
| **Next.js (bun)** | `lynx start --name next-bun -- bun dev` |
| **Astro (dev)** | `lynx start --name astro -- npm run dev` |
| **Node (script)** | `lynx start server.js` |
| **Node (cmd)** | `lynx start -- node server.js` |

## Examples

Start a Node.js script:
```bash
lynx start main.js
```

Start with DynamicUser isolation (secure):
```bash
lynx start main.js --isolation dynamic
```

Start a scheduled task (runs every hour):
```bash
lynx start cleanup.sh --schedule "@hourly" --restart never
```

## Example output

Success:
```
Spec saved to /home/user/.config/lynx/apps/my-api.json
Started my-api
  ID: e73a9f1b
  PID: 12345
  Status: online
```

Error (invalid path):
```
Error: ERR_BAD_REQUEST: invalid cwd: stat /invalid/path: no such file or directory (BAD_REQUEST)
```

## Security

*   **Environment Variables**: Environment variables provided via `--env-file` are loaded into the process environment but are **NOT** persisted in the application specification file (`~/.config/lynx/apps/<id>.json`). This ensures secrets are not stored in plain text on disk.
*   **Isolation**:
    *   **Self Mode** (`--isolation self`): Processes run as the same user as the daemon.
    *   **DynamicUser** (`--isolation dynamic`): Processes run as a transient system user with restricted filesystem access (`ProtectSystem=strict`, `ProtectHome=yes`) and no new privileges. Recommended for production.
*   **Shell Execution**: Shell execution is disabled by default. Use `--shell` only if necessary, as it introduces shell injection risks if inputs are not sanitized.

## Threat Model (DynamicUser)

When using `--isolation dynamic`, Lynx leverages `systemd-run` to create a transient, sandboxed execution environment.

### Security Guarantees
1.  **Ephemeral Identity**: A new, random UID/GID is allocated for the process lifetime and discarded afterwards. No persistent user is created on the system.
2.  **Filesystem Isolation**:
    - `ProtectSystem=strict`: The entire filesystem is mounted read-only.
    - `PrivateTmp=yes`: The process sees a private `/tmp` and `/var/tmp`.
    - `ProtectHome=yes`: `/home`, `/root`, and `/run/user` are inaccessible.
3.  **Credential Safety**: Environment variables are NOT passed via command line (which is visible in `ps`). Instead, they are written to a `0600` file in a secure directory and passed via systemd's `LoadCredential` logic, ensuring only the target process can read them.
4.  **No Privilege Escalation**: `NoNewPrivileges=yes` prevents the process from gaining new privileges (e.g., via setuid binaries).

### Usage Recommendation
Use `--isolation dynamic` for network-facing services (e.g., web servers, APIs) to minimize the blast radius if the service is compromised.
