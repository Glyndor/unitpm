# 🦁 `lynx flush`

> *Truncate the stdout/stderr log files for a process.*

## 📖 Synopsis

```bash
lynx flush <id|name>...
```

## Description

Truncate the stdout/stderr log files for a process. Resolves and validates log paths before truncation to avoid unsafe operations.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Flush logs for one process:
```bash
lynx flush my-api
```

Flush logs for multiple:
```bash
lynx flush api-1 api-2
```
