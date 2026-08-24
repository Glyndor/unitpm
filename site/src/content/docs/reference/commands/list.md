---
title: "unitpm list"
description: List all processes managed by unitpm with status, PID, namespace, restart count, and uptime. Supports --json for scripting and --namespace for filtering.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm list","item":"https://jaro-c.github.io/unitpm/reference/commands/list/"}]}'
sidebar:
  label: list
---

## 📖 Synopsis

```bash
unitpm list|ls|ps [options]
```

## Description

List all processes managed by unitpm. Displays status, uptime, resource usage metrics, and Git information.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--long` | boolean | false | Show full process IDs. |
| `--namespace` | string | - | Filter by namespace. |
| `--sort` | string | - | Sort order (comma‑separated): fields `namespace`, `name`, `createdAt`, `id` with `asc|desc`. |
| `--json` | boolean | false | Emit the process list as a JSON array on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

List all processes:
```bash
unitpm list
```

List with full IDs:
```bash
unitpm list --long
```

Filter by namespace:
```bash
unitpm list --namespace default
```

Custom sort:
```bash
unitpm list --sort "namespace:asc,name:asc,createdAt:desc"
```

JSON output (for scripting):
```bash
unitpm list --json | jq '.[] | {name, state, pid}'
```

## 📋 Example Output

Standard:
```
id       | name         | status  | uptime | cpu  | mem   | user | git
e73a9f1b | test-app     | online  | 1h 2m  | 0.1% | 12 MB | jaro | main@a1b2c3
```

Long:
```
id       | name                           | namespace            | version    | mode       | pid      | uptime     | ↺     | status          | cpu      | mem        | user            | git                | watch
e73a9f1b | test-app                       | default              | 1.0.0      | fork       | 12345    | 1h 2m      | 0     | online          | 0.1%     | 12.5 MB    | glyndor-unitpm  | main@a1b2c3*       | disabled
```

## Notes

- **Git Info**: The `git` column shows the branch and short commit hash (e.g., `main@a1b2c3`). An asterisk `*` indicates uncommitted changes (dirty state).
- **Metrics**: The `cpu` and `mem` columns display aggregated resource usage:
    - **Memory**: Resident Set Size (RSS) in bytes.
    - **CPU**: Percentage of CPU usage.
- **Aggregation**: unitpm automatically aggregates metrics for the entire process tree (including child processes). It prefers using Cgroup V2 when available, falling back to process tree scanning if necessary.
- **Update notice**: after the table, `unitpm list` prints a one-line banner on stderr when a newer release is available (`! New version available: vX.Y.Z — run 'unitpm update --apply'`). The check is cached for 6 hours at `$XDG_CACHE_HOME/unitpm/update-check.json` and suppressed under `--json`.
