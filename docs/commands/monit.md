# 🦁 `lynx monit`

> *Display live statistics for all managed processes.*

## 📖 Synopsis

```bash
lynx monit
```

## Description

Display live statistics for all managed processes, refreshing periodically. Useful for quick monitoring without external tools.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Run live monitor:
```bash
lynx monit
```

Exit with Ctrl+C.

## Notes

- Shows namespace/name, PID, state, CPU%, and memory bytes per process.
- Updates every ~2 seconds.
