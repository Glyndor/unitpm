# 🦁 `lynx list | ls | ps`

> *List all processes managed by Lynx.*

## 📖 Synopsis

```bash
lynx list|ls|ps [options]
```

## Description

List all processes managed by Lynx. Displays status, uptime, resource usage metrics, and Git information.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--long` | boolean | false | Show full process IDs. |
| `--namespace` | string | - | Filter by namespace. |
| `--sort` | string | - | Sort order (comma‑separated): fields `namespace`, `name`, `createdAt`, `id` with `asc|desc`. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

List all processes:
```bash
lynx list
```

List with full IDs:
```bash
lynx list --long
```

Filter by namespace:
```bash
lynx list --namespace default
```

Custom sort:
```bash
lynx list --sort "namespace:asc,name:asc,createdAt:desc"
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
e73a9f1b | test-app                       | default              | 1.0.0      | fork       | 12345    | 1h 2m      | 0     | online          | 0.1%     | 12.5 MB    | lynx            | main@a1b2c3*       | disabled
```

## Notes

- **Git Info**: The `git` column shows the branch and short commit hash (e.g., `main@a1b2c3`). An asterisk `*` indicates uncommitted changes (dirty state).
- **Metrics**: The `cpu` and `mem` columns display aggregated resource usage:
    - **Memory**: Resident Set Size (RSS) in bytes.
    - **CPU**: Percentage of CPU usage.
- **Aggregation**: Lynx automatically aggregates metrics for the entire process tree (including child processes). It prefers using Cgroup V2 when available, falling back to process tree scanning if necessary.
