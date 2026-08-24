---
title: "unitpm version"
description: Show version numbers for the unitpm CLI (unitpm), daemon (unitpmd), and IPC protocol. Pass --json for machine-readable output suitable for scripts and CI.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm version","item":"https://jaro-c.github.io/unitpm/reference/commands/version/"}]}'
sidebar:
  label: version
---

## 📖 Synopsis

```bash
unitpm version [flags]
```

## Description

Show unitpm version information for the CLI, Daemon, and IPC Protocol.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | - | - | Output version info as JSON (CLI, daemon, protocol). |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Show version:
```bash
unitpm version
```

## 📋 Example Output

```
unitpm CLI
  Version : v0.1.0
  Commit  : a1b2c3d
  Built   : 2025-01-01T12:00:00Z

unitpm Daemon
  Version : v0.1.0
  Commit  : a1b2c3d
  Built   : 2025-01-01T12:00:00Z

Protocol
  CLI     : v1
  Daemon  : v1
```
