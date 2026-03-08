# 🦁 `lynx reload`

> *Reload a process configuration from its stored spec and restart it.*

## 📖 Synopsis

```bash
lynx reload <id|name>...
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
lynx reload my-api
```

Reload multiple:
```bash
lynx reload api-1 api-2
```
