---
title: "unitpm help"
description: Show usage, flags, and examples for any unitpm CLI command. Run unitpm help followed by a command name for detailed documentation on flags and options.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm help","item":"https://jaro-c.github.io/unitpm/reference/commands/help/"}]}'
sidebar:
  label: help
---

## 📖 Synopsis

```bash
unitpm help [command]
```

## Description

Display the help message for the specified command, or the general help message if no command is specified.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `command` | string | - | The command to get help for. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Show general help:
```bash
unitpm help
```

Show help for the `start` command:
```bash
unitpm help start
```

## 📋 Example Output

```
Usage:
  unitpm <command> [flags]

Commands:
  start       Start a new process
  list, ls    List all processes
  startup     Setup system startup script
  version     Show version info
  help        Help about any command

Get Help:
  unitpm --help
  unitpm <command> --help
```
