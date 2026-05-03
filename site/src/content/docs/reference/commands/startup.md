---
title: "lynxpm startup"
description: Install the Lynx daemon as a systemd service that starts on boot and restores all managed processes after a reboot. Supports system and user mode.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm startup","item":"https://jaro-c.github.io/Lynx/reference/commands/startup/"}]}'
sidebar:
  label: startup
---

## 📖 Synopsis

```bash
lynxpm startup [flags]
```

## Description

Generate and install the system startup script for Lynx. This command configures `systemd` to start the Lynx daemon automatically on boot.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Generate and install systemd unit (requires sudo/root if installing to /etc):
```bash
lynxpm startup
```

## 📋 Example Output

Success:
```
Lynx system daemon started. Autostart enabled.
```

Failure (not root):
```
Admin privileges required. Run:
  sudo lynxpm startup
```

Failure (no systemd):
```
ERR_UNSUPPORTED: Lynx requires Linux with systemd
```

## Notes

- **Requirements**: This command requires a Linux system with `systemd` as the init system.
- **Permissions**: Root or sudo privileges are typically required to write to `/etc/systemd/system` and enable services.
