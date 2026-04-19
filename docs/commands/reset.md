# 🦁 `lynxpm reset`

> *Zero the Restarts counter for a process without stopping or restarting it.*

## 📖 Synopsis

```bash
lynxpm reset <id|name>...
```

## Description

Useful after fixing a crash loop: reset the counter so you can observe
stability from a clean baseline. The process keeps running — only the
`Restarts` metric visible in `lynxpm list` and `lynxpm show` is zeroed. The
internal backoff bucket is also cleared.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

```bash
lynxpm reset api
lynxpm reset prod:worker
lynxpm reset api worker scheduler   # multiple at once
```
