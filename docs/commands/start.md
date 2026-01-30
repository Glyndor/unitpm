# start

Start a new process managed by Lynx.

## Synopsis

```bash
lynx start [flags] <command>
```

## Flags

| Flag | Type | Description |
|------|------|-------------|
| `--name` | string | Name of the process (default: auto-generated from command). |
| `--cwd` | string | Working directory for the process (default: current directory). |
| `--cron` | string | Cron schedule expression for scheduled execution. |
| `--runtime` | string | Runtime limit (e.g. "1h", "30s"). |
| `--shell` | boolean | Execute command inside a shell. |
| `--env-file` | string | Path to a file containing environment variables. |

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
