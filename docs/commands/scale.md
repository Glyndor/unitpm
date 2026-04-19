# 🦁 `lynxpm scale`

> *Grow or shrink an app to the target number of running instances.*

## 📖 Synopsis

```bash
lynxpm scale <name> <N>
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
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

```bash
lynxpm scale worker 5          # set 'worker' to exactly 5 instances
lynxpm scale prod:api 10       # namespace-qualified
lynxpm scale worker 0          # stop and delete all instances
```

## Notes

- Use `namespace:name` to target a specific namespace.
- Each scaled instance inherits the original's restart policy, isolation
  mode, resource limits, and env-file.
- `LYNX_INSTANCE` is set to the instance's ordinal (0-based from the
  original count).
