# flush

## Synopsis

```bash
lynx flush <id|name>...
```

## Description

Truncate the stdout/stderr log files for a process. Resolves and validates log paths before truncation to avoid unsafe operations.

## Examples

Flush logs for one process:
```bash
lynx flush my-api
```

Flush logs for multiple:
```bash
lynx flush api-1 api-2
```
