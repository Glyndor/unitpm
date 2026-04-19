# 🦁 `lynxpm stop`

> *Stop one or more running processes.*

## 📖 Synopsis

```bash
lynxpm stop <id|name>...
```

## Description

Stops the specified processes. You can provide either the full ID, a short ID prefix (if unique), or the process name (if unique).

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Stop a process by name:
```bash
lynxpm stop my-app
```

Stop multiple processes by ID:
```bash
lynxpm stop 1234 5678
```
