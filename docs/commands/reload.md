# 🦁 `lynxpm reload`

> *Reload a process configuration from its stored spec and restart it.*

## 📖 Synopsis

```bash
lynxpm reload <id|name>...
```

## Description

Reload a process configuration from its stored spec and restart it. Useful after editing a spec file or changing environment.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Reload by name:
```bash
lynxpm reload my-api
```

Reload multiple:
```bash
lynxpm reload api-1 api-2
```
