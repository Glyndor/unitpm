---
title: "lynxpm flush"
description: "Truncate the stdout/stderr log files for a process"
sidebar:
  label: flush
---

## 📖 Synopsis

```bash
lynxpm flush [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Truncate the stdout/stderr log files for a process. Resolves and validates
log paths before truncation to avoid unsafe operations. The human-readable
output reports how many bytes were freed per target; `--json` surfaces the
same number at `.results[].extra.bytes_freed`.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `lynxpm flush 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--namespace <ns>` | string | - | Flush every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Flush logs for one process:
```bash
lynxpm flush my-api
```

Flush logs for multiple:
```bash
lynxpm flush api-1 api-2
```

Flush every process in the `prod` namespace:
```bash
lynxpm flush 'prod:*'           # selector form (quote the glob)
lynxpm flush --namespace prod   # flag form (script-friendly)
```

Total bytes reclaimed across a batch:
```bash
lynxpm flush api-1 api-2 --json | jq '[.results[].extra.bytes_freed] | add'
```

## Exit codes

- `0` — every target was flushed.
- non-zero — at least one target failed; per-target lines (or
  `.results[].error` in `--json`) explain why.
