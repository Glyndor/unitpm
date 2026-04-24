---
title: "lynxpm restart"
description: "Restart one or more processes"
sidebar:
  label: restart
---

## 📖 Synopsis

```bash
lynxpm restart [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Restarts the specified processes. This sends a stop signal followed by
starting the process again with the same configuration.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `lynxpm restart 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--namespace <ns>` | string | - | Restart every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `--no-list` | boolean | false | Skip the process list printed after the action. The restarted instances are otherwise highlighted (▸) in the list for easy scanning. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Restart a process:
```bash
lynxpm restart my-app
```

Restart every process in the `prod` namespace:
```bash
lynxpm restart 'prod:*'           # selector form (quote the glob)
lynxpm restart --namespace prod   # flag form (script-friendly)
```

Restart many, capture outcomes as JSON:
```bash
lynxpm restart api worker-1 worker-2 --json | jq '.results[] | {id, status}'
```

## Exit codes

- `0` — every target was restarted.
- non-zero — at least one target failed; the per-target line (or
  `.results[].error` in `--json`) explains why.
