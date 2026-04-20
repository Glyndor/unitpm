# 🦁 `lynxpm reload`

> *Reload a process configuration from its stored spec and restart it.*

## 📖 Synopsis

```bash
lynxpm reload [--namespace <ns>] [--json] <id|name|ns:*|*>...
```

## Description

Reload a process configuration from its stored spec and restart it. Useful after editing a spec file or changing environment.

Bulk selectors:

- `<ns>:*` — every process in that namespace. Quote the glob so the shell
  does not expand it: `lynxpm reload 'prod:*'`.
- `*` or `*:*` — every managed process.
- `--namespace <ns>` — same as `<ns>:*` but no shell quoting needed.
  Cannot be combined with positional targets.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--namespace <ns>` | string | - | Reload every process in this namespace. Mutually exclusive with positional targets. |
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Reload by name:
```bash
lynxpm reload my-api
```

Reload multiple:
```bash
lynxpm reload api-1 api-2
```

Reload every process in the `prod` namespace:
```bash
lynxpm reload 'prod:*'           # selector form (quote the glob)
lynxpm reload --namespace prod   # flag form (script-friendly)
```

Reload and inspect the summary:
```bash
lynxpm reload api worker --json | jq '.summary'
```

## Exit codes

- `0` — every target was reloaded.
- non-zero — at least one target failed; the per-target line (or
  `.results[].error` in `--json`) explains why.
