---
title: "unitpm logs"
description: View and follow stdout and stderr log files for unitpm-managed processes. Supports live --follow mode, --lines limit, and --stdout / --stderr stream filtering.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm logs","item":"https://jaro-c.github.io/unitpm/reference/commands/logs/"}]}'
sidebar:
  label: logs
---

## 📖 Synopsis

```bash
unitpm logs <id|name|namespace:name> [--lines N] [--follow] [--stdout] [--stderr]
```

## Description

View and follow process log files managed by unitpm. Resolves per‑app stdout/stderr paths and tails their contents.

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
