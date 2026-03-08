# 🦁 `lynx restart`

> *Restart one or more processes.*

## 📖 Synopsis

```bash
lynx restart <id|name>...
```

## Description

Restarts the specified processes. This sends a stop signal followed by starting the process again with the same configuration.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Restart a process:
```bash
lynx restart my-app
```
