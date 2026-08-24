# 🦁 `unitpm help`

> *Display help information about unitpm commands.*

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
