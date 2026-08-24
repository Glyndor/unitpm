# 🦁 `unitpm delete | remove | rm`

> *Delete one or more processes and their configurations.*

## 📖 Synopsis

```bash
unitpm delete|remove|rm [--purge] [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Stops and removes the specified processes from management. By default, it
removes the process from the list and deletes its spec file. Multiple
targets are processed in a single invocation; the command exits with a
non-zero status code when any target fails so scripts can tell the
difference between a clean run and a partial failure.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `unitpm delete 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets. `--purge` still applies to
  every expanded target.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--purge` | boolean | false | Also delete the log files and any runtime data associated with the process. |
| `--namespace <ns>` | string | - | Delete every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Delete a process (keep logs):
```bash
unitpm delete my-app
```

Delete a process and its logs:
```bash
unitpm delete --purge my-app
```

Delete every process in the `prod` namespace, including logs:
```bash
unitpm delete --namespace prod --purge
unitpm delete 'prod:*' --purge        # equivalent selector form
```

Delete many, read the outcome from JSON:
```bash
unitpm rm api worker-1 worker-2 --json | jq '.summary'
```

## Exit codes

- `0` — every target succeeded.
- non-zero — at least one target failed; per-target `✗ Failed to delete …`
  lines (or the `.results[].error` field in `--json`) explain why.
