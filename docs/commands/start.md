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

## Scaling and Load Balancing

Lynx supports scaling via `--scale N` (or `--instances N`). This starts N independent instances of your application.

**Important**: Lynx does **not** provide a built-in load balancer.
- Each instance runs as a separate process with a unique ID and Name.
- The `LYNX_INSTANCE` environment variable (0, 1, 2...) is injected into each instance.
- If your application binds to a port, you must ensure each instance uses a different port (e.g. `PORT=3000 + LYNX_INSTANCE`) or use `SO_REUSEPORT` if supported.
- For web applications, it is recommended to run a reverse proxy (Nginx, Caddy, HAProxy) in front of the Lynx instances.

**Examples**:

**1. Next.js with offset ports**:
Next.js does not natively support `SO_REUSEPORT`. Use the `LYNX_INSTANCE` variable to offset the port.
```bash
# In your package.json: "start": "PORT=$((3000 + LYNX_INSTANCE)) next start"
lynx start --name next-app --scale 3 --shell -- npm start
```
*Note: Using `--shell` allows variable expansion in the command.*

**2. Generic Node.js Server**:
```javascript
// server.js
const port = 3000 + parseInt(process.env.LYNX_INSTANCE || 0);
server.listen(port);
```
```bash
lynx start server.js --scale 4 --env-file .env
```

## Clarifications

### Auto-Naming
If `--name` is omitted, Lynx generates a deterministic name:
- Format: `basename-shortID` (e.g., `server-a1b2c3d4`).
- If scaling: `basename-index-shortID` (e.g., `server-1-a1b2c3d4`).

### Max Restarts
The `--max-restarts` limit applies only to **automatic** restarts triggered by crashes or failures.
- Manual restarts (`lynx restart <id>`) **reset** the restart counter and backoff timer.
- You can manually restart a process as many times as needed without hitting the limit.

### Isolation Visibility
- **System Mode** (`sudo lynx`): Applications are managed by the system-wide daemon. They are visible to and manageable by any user in the `lynxadm` group (or root).
- **User Mode** (`lynx`): Applications are managed by a per-user daemon. They are private to your user account and cannot be seen by other users.

## Environment Variables

### Mode Behavior
- **User Mode**: The process inherits the full environment of the user running `lynx start`.
- **System Mode**: The process does **not** inherit the system environment (to prevent leaking sensitive variables like `AWS_KEYS`). Instead, a whitelist is applied:
  - `PATH`, `LANG`, `TERM`, `TZ`, `TMPDIR`
  - `USER`, `LOGNAME`, `SHELL`, `PWD`
  - `XDG_*`, `LC_*`
  - Any variables defined in `--env-file` or `AppSpec.Env`.

## DynamicUser Env-File

When using `--isolation dynamic` combined with `--env-file`, Lynx bridges the environment variables securely to the isolated process.

**How it works:**
1. Lynx reads the env file.
2. Writes it to a secure, daemon-owned file (`/var/lib/lynx/creds/<id>/env`) with `0600` permissions.
3. Uses `systemd-run --property=LoadCredential=...` to expose it to the process.
4. An internal wrapper (`_exec-env`) reads the credential and exports variables before executing your application.

**Example:**

```bash
# .env file
PORT=8080
API_KEY=secret_123
```

```bash
# Start with isolation and env file
lynx start server.js --isolation dynamic --env-file .env
```

**Note on Security:**
This mechanism ensures secrets are never visible in `ps` output or persisted in the global AppSpec. However, once the process starts, the secrets exist in its memory.

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
    - **Note**: `HOME` environment variable is NOT inherited or injected in this mode to ensure compliance with `ProtectHome`.
3.  **Credential Safety**: Environment variables are NOT passed via command line (which is visible in `ps`). Instead, they are written to a `0600` file in a secure directory and passed via systemd's `LoadCredential` logic, ensuring only the target process can read them.
4.  **No Privilege Escalation**: `NoNewPrivileges=yes` prevents the process from gaining new privileges (e.g., via setuid binaries).

### Usage Recommendation
Use `--isolation dynamic` for network-facing services (e.g., web servers, APIs) to minimize the blast radius if the service is compromised.
