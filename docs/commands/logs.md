# 🦁 `lynx logs`

> *View and follow process log files managed by Lynx. Resolves per‑app stdout/stderr paths and tails their contents.*

## 📖 Synopsis

```bash
lynx logs <id|name|namespace:name> [--lines N] [--follow] [--stdout] [--stderr]
```

## 💡 Usage

View and follow process log files managed by Lynx. Resolves per‑app stdout/stderr paths and tails their contents.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--lines`, `-n` | int | 200 | Number of lines to show initially |
| `--follow`, `-f` | boolean | false | Stream new log lines (tail -f) |
| `--stdout`, `-o` | boolean | auto | Show stdout only (if set) |
| `--stderr`, `-e` | boolean | auto | Show stderr only (if set) |

If neither `--stdout` nor `--stderr` is provided, both streams are shown.

## 🚀 Examples

Show last 200 lines of both streams:
```bash
lynx logs my-api
```

Follow stdout only:
```bash
lynx logs default:my-api --stdout --follow
```

Increase initial lines:
```bash
lynx logs e73a9f1b --lines 1000
```

## Notes

- Log files are located under a secure per‑app directory. System mode defaults to `/var/log/lynx/<id>/`; user mode uses the XDG state directory.
- The command waits for log files to appear when `--follow` is enabled.
