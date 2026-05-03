---
title: "lynxpm version"
description: Show version numbers for the Lynx CLI (lynxpm), daemon (lynxd), and IPC protocol. Pass --json for machine-readable output suitable for scripts and CI.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm version","item":"https://jaro-c.github.io/Lynx/reference/commands/version/"}]}'
sidebar:
  label: version
---

## 📖 Synopsis

```bash
lynxpm version [flags]
```

## Description

Show Lynx version information for the CLI, Daemon, and IPC Protocol.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | - | - | Output version info as JSON (CLI, daemon, protocol). |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Show version:
```bash
lynxpm version
```

## 📋 Example Output

```
Lynx CLI
  Version : v0.1.0
  Commit  : a1b2c3d
  Built   : 2025-01-01T12:00:00Z

Lynx Daemon
  Version : v0.1.0
  Commit  : a1b2c3d
  Built   : 2025-01-01T12:00:00Z

Protocol
  CLI     : v1
  Daemon  : v1
```
