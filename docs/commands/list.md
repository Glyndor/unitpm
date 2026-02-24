# list | ls | ps

## Synopsis

```bash
lynx list|ls|ps [options]
```

## Usage

List all processes managed by Lynx. Displays status, uptime, and resource usage metrics.

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `--long` | boolean | false | Show full process IDs. | `lynx list --long` |
| `--namespace` | string | - | Filter by namespace. | `lynx list --namespace default` |
| `--sort` | string | - | Sort order (comma‑separated): fields `namespace`, `name`, `createdAt`, `id` with `asc|desc`. | `lynx list --sort "namespace:asc,name:asc,createdAt:desc"` |
| `-h`, `--help` | - | - | Show help message. | — |

## Examples

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

## Example output

Standard:
```
id       | name         | status  | uptime | cpu  | mem
e73a9f1b | test-app     | online  | 1h 2m  | 0.1% | 12 MB
```

Long:
```
id       | name                           | namespace            | version    | mode       | pid      | uptime     | ↺     | status          | cpu      | mem        | user            | watch
e73a9f1b | test-app                       | default              | 1.0.0      | fork       | 12345    | 1h 2m      | 0     | online          | 0.1%     | 12.5 MB    | lynx            | disabled
```

## Notes

*   **Metrics**: The `cpu` and `mem` columns display aggregated resource usage:
    *   **Memory**: Resident Set Size (RSS) in bytes.
    *   **CPU**: Percentage of CPU usage.
*   **Aggregation**: Lynx automatically aggregates metrics for the entire process tree (including child processes). It prefers using Cgroup V2 when available, falling back to process tree scanning if necessary.
