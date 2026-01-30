# start

Start a new process managed by Lynx.

## Synopsis

```bash
lynx start [flags] <command>
```

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `--name` | string | auto | Name of the process. | `--name my-app` |
| `--cwd` | string | . | Working directory for the process. | `--cwd /var/www` |
| `--cron` | string | - | Cron schedule expression for scheduled execution. | `--cron "@hourly"` |
| `--runtime` | string | - | Runtime limit (e.g. "1h", "30s"). | `--runtime 30m` |
| `--shell` | boolean | false | Execute command inside a shell. | `--shell` |
| `--env-file` | string | - | Path to a file containing environment variables. | `--env-file .env` |

## Examples

Start a simple script:
```bash
lynx start ./script.sh
```

Start with a custom name:
```bash
lynx start --name my-app ./server
```

Start with environment variables from a file:
```bash
lynx start --env-file .env ./server
```

Start using a shell:
```bash
lynx start --shell "echo hello > out.txt"
```

Start with a cron schedule:
```bash
lynx start --cron "@hourly" ./backup.sh
```

## Example output

Success:
```
Spec saved to /home/user/.config/lynx/apps/my-app.json
Started my-app
  ID: e73a9f1b
  PID: 12345
  Status: online
```
