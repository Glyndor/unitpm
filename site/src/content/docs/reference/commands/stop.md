---
title: "lynxpm stop"
description: "Stop one or more running processes"
sidebar:
  label: stop
---

## 📖 Synopsis

```bash
lynxpm stop [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Stops the specified processes. You can provide either the full ID, a short
ID prefix (if unique), or the process name (if unique). A target that was
already stopped renders as `! Already stopped: …` and is recorded as
`status: "noop"` in `--json` output — distinct from an actual stop.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `lynxpm stop 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--namespace <ns>` | string | - | Stop every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `--no-list` | boolean | false | Skip the process list printed after the action. The stopped instances are otherwise highlighted (▸) in the list for easy scanning. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Stop a process by name:
```bash
lynxpm stop my-app
```

Stop multiple processes by ID:
```bash
lynxpm stop 1234 5678
```

Stop every process in the `prod` namespace:
```bash
lynxpm stop 'prod:*'           # selector form (quote the glob)
lynxpm stop --namespace prod   # flag form (script-friendly)
```

Stop many and capture per-target status:
```bash
lynxpm stop api worker-1 worker-2 --json | jq '.results[] | {id, status}'
```

## Exit codes

- `0` — every target succeeded or was already stopped.
- non-zero — at least one target failed; the reason is in the per-target
  line or in `.results[].error` when running with `--json`.
