---
title: "unitpm scale"
description: Grow or shrink a unitpm-managed app to a target number of parallel instances. Running instances are preserved; only the delta is started or stopped in-place.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm scale","item":"https://jaro-c.github.io/unitpm/reference/commands/scale/"}]}'
sidebar:
  label: scale
---

## 📖 Synopsis

```bash
unitpm scale <name> <N> [--json]
```

## Description

Brings the number of processes whose name matches `<name>` (including
`<name>-1`, `<name>-2`, … siblings in the same namespace) to exactly N.

**Scale up:** clones the spec of the first existing member as a template;
new instances get auto-assigned names `<base>-<nextFreeIndex>` and a fresh
`UNITPM_INSTANCE` env var.

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
unitpm scale worker 5          # set 'worker' to exactly 5 instances
unitpm scale prod:api 10       # namespace-qualified
unitpm scale worker 0          # stop and delete all instances
unitpm scale worker 5 --json | jq '.created, .deleted'
```

## Notes

- Use `namespace:name` to target a specific namespace.
- Each scaled instance inherits the original's restart policy, isolation
  mode, resource limits, and env-file.
- `UNITPM_INSTANCE` is set to the instance's ordinal (0-based from the
  original count).
