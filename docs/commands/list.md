# list

List all processes managed by Lynx.

## Synopsis

```bash
lynx list
```

## Flags

No flags.

## Metrics

The `cpu` and `mem` columns display aggregated resource usage:
- **Memory**: Resident Set Size (RSS) in bytes.
- **CPU**: Percentage of CPU usage.

Lynx automatically aggregates metrics for the entire process tree (including child processes). It prefers using Cgroup V2 when available, falling back to process tree scanning if necessary.

## Common Examples

List all processes:
```bash
lynx list
```

List with full IDs:
```bash
lynx list --long
```

Example output:
```
id       | name                           | namespace            | version    | mode       | pid      | uptime     | ↺     | status          | cpu      | mem        | user            | watch
-------- | ------------------------------ | -------------------- | ---------- | ---------- | -------- | ---------- | ----- | --------------- | -------- | ---------- | --------------- | ----------
e73a9f1b | test-app                       | default              | 1.0.0      | fork       | 12345    | 1h 2m      | 0     | online          | 0.1%     | 12.5 MB    | lynx            | disabled
```
