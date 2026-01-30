# help

Show help for Lynx commands.

## Synopsis

```bash
lynx help [command]
```

## Description

Display the help message for the specified command, or the general help message if no command is specified.

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `command` | string | - | The command to get help for. | `lynx help start` |

## Examples

Show general help:
```bash
lynx help
```

Show help for the `start` command:
```bash
lynx help start
```

## Example output

General help:
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
