# 🦁 `unitpm version`

> *Show unitpm version information for the CLI, Daemon, and IPC Protocol.*

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
