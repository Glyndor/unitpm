---
title: "lynxpm scale"
description: Grow or shrink a Lynx-managed app to a target number of parallel instances. Running instances are preserved; only the delta is started or stopped in-place.
sidebar:
  label: scale
---

## 📖 Synopsis

```bash
lynxpm scale <name> <N> [--json]
```

## Description

Brings the number of processes whose name matches `<name>` (including
`<name>-1`, `<name>-2`, … siblings in the same namespace) to exactly N.

**Scale up:** clones the spec of the first existing member as a template;
new instances get auto-assigned names `<base>-<nextFreeIndex>` and a fresh
`LYNX_INSTANCE` env var.

**Scale down:** stops and deletes the highest-indexed members first so
lower indices stay stable.

Requires at least one existing instance for scale-up (the template source).
Target must be in [0, 1024].

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | boolean | false | Emit the `ScaleResponse` as JSON on stdout (`{before, after, created, deleted}`). |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

```bash
lynxpm scale worker 5          # set 'worker' to exactly 5 instances
lynxpm scale prod:api 10       # namespace-qualified
lynxpm scale worker 0          # stop and delete all instances
lynxpm scale worker 5 --json | jq '.created, .deleted'
```

## Notes

- Use `namespace:name` to target a specific namespace.
- Each scaled instance inherits the original's restart policy, isolation
  mode, resource limits, and env-file.
- `LYNX_INSTANCE` is set to the instance's ordinal (0-based from the
  original count).
