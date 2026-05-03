---
title: "lynxpm logs"
description: View and follow stdout and stderr log files for Lynx-managed processes. Supports live --follow mode, --lines limit, and --stdout / --stderr stream filtering.
sidebar:
  label: logs
---

## 📖 Synopsis

```bash
lynxpm logs <id|name|namespace:name> [--lines N] [--follow] [--stdout] [--stderr]
```

## Description

View and follow process log files managed by Lynx. Resolves per‑app stdout/stderr paths and tails their contents.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n`, `--lines` | int | 200 | Number of lines to show initially. |
| `-f`, `--follow` | boolean | false | Stream new log lines (tail -f). |
| `-o`, `--stdout` | boolean | auto | Show stdout only (if set). |
| `-e`, `--stderr` | boolean | auto | Show stderr only (if set). |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Show last 200 lines of both streams:
```bash
lynxpm logs my-api
```

Follow stdout only:
```bash
lynxpm logs default:my-api --stdout --follow
```

Increase initial lines:
```bash
lynxpm logs e73a9f1b --lines 1000
```

## Notes

- Log files are located under a secure per‑app directory. System mode defaults to `/var/log/lynx-pm/<id>/`; user mode uses the XDG state directory.
- The command waits for log files to appear when `--follow` is enabled.
