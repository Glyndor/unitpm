---
title: "lynxpm monit"
description: Live CPU, memory, and uptime dashboard for all Lynx-managed processes. Refreshes in-place in the terminal. Use --json for machine-readable metric output.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm monit","item":"https://jaro-c.github.io/Lynx/reference/commands/monit/"}]}'
sidebar:
  label: monit
---

**Aliases:** `top`, `monitor`

## 📖 Synopsis

```bash
lynxpm monit
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
lynxpm monit
```

Exit with Ctrl+C.

## Notes

- Shows namespace/name, PID, state, CPU%, and memory bytes per process.
- Updates every ~2 seconds.
