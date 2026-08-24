# 🦁 `unitpm logs`

> *View and follow process log files managed by unitpm.*

## 📖 Synopsis

```bash
unitpm logs <id|name|namespace:name> [--lines N] [--follow] [--stdout] [--stderr]
```

## Description

View and follow process log files managed by unitpm. Resolves per‑app stdout/stderr paths and tails their contents.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n`, `--lines` | int | 40 | Number of lines to show initially. |
| `-f`, `--follow` | boolean | false | Stream new log lines (tail -f). |
| `-o`, `--stdout` | boolean | auto | Show stdout only (if set). |
| `-e`, `--stderr` | boolean | auto | Show stderr only (if set). |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Show last 40 lines of both streams:
```bash
unitpm logs my-api
```

Follow stdout only:
```bash
unitpm logs default:my-api --stdout --follow
```

Increase initial lines:
```bash
unitpm logs e73a9f1b --lines 1000
```

## Notes

- Log files are located under a secure per‑app directory. System mode defaults to `/var/log/unitpm/<id>/`; user mode uses the XDG state directory.
- The command waits for log files to appear when `--follow` is enabled.
