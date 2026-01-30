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
| `-h`, `--help` | - | - | Show help message. | — |

## Examples

Start a Node.js script:
```bash
lynx start main.js
```

Start a Python script with explicit runtime:
```bash
lynx start app.py --runtime python3
```

Start a command with arguments:
```bash
lynx start --name server -- "python3 -m http.server 8080"
```

Start with production restart policy:
```bash
lynx start app.js --restart always --backoff expo --max-restarts 50
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
    *   **User Mode**: Processes run as the current user. They cannot create new OS users.
    *   **System Mode**: Processes run as the `lynx` user (or configured user). `DynamicUser` support is planned for future releases.
*   **Shell Execution**: Shell execution is disabled by default. Use `--shell` only if necessary, as it introduces shell injection risks if inputs are not sanitized.
