# 🦁 `lynx reset`

> *Zero the Restarts counter for a process without stopping or restarting it.*

## 📖 Synopsis

```bash
lynx reset <id|name>...
```

## Description

Useful after fixing a crash loop: reset the counter so you can observe
stability from a clean baseline. The process keeps running — only the
`Restarts` metric visible in `lynx list` and `lynx show` is zeroed. The
internal backoff bucket is also cleared.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

```bash
lynx reset api
lynx reset prod:worker
lynx reset api worker scheduler   # multiple at once
```
