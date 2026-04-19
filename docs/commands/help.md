# 🦁 `lynxpm help`

> *Display help information about Lynx commands.*

## 📖 Synopsis

```bash
lynxpm help [command]
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
lynxpm help
```

Show help for the `start` command:
```bash
lynxpm help start
```

## 📋 Example Output

```
Lynx - Process Manager for Linux

Usage:
  lynx [command]

Available Commands:
  start       Start a new process
  list        List all processes
  startup     Setup system startup script
  version     Show version info
  help        Help about any command

Flags:
  -h, --help   help for lynx

Use "lynx [command] --help" for more information about a command.
```
