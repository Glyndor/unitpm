# 🦁 `unitpm reset`

> *Zero the Restarts counter for a process without stopping or restarting it.*

## 📖 Synopsis

```bash
unitpm reset [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Useful after fixing a crash loop: reset the counter so you can observe
stability from a clean baseline. The process keeps running — only the
`Restarts` metric visible in `unitpm list` and `unitpm show` is zeroed. The
internal backoff bucket is also cleared.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `unitpm reset 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--namespace <ns>` | string | - | Reset every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

```bash
unitpm reset api
unitpm reset prod:worker
unitpm reset api worker scheduler   # multiple at once
unitpm reset 'prod:*'               # every process in namespace prod
unitpm reset --namespace prod       # equivalent flag form
unitpm reset api --json | jq '.summary'
```

## Exit codes

- `0` — every target was reset.
- non-zero — at least one target failed; the per-target line (or
  `.results[].error` in `--json`) explains why.
