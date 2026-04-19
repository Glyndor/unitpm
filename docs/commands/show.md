# 🦁 `lynxpm show`

> *Show detailed information about a process.*

## 📖 Synopsis

```bash
lynxpm show <id|name|namespace:name>
```

## Description

Show detailed information about a process: ID, namespace, state, PID, uptime, CPU and memory usage, and user/mode metadata.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

By ID:
```bash
lynxpm show e73a9f1b
```

By name:
```bash
lynxpm show my-api
```

Namespaced:
```bash
lynxpm show default:my-api
```

## 📋 Example Output

```
Process my-api (e73a9f1b)
Namespace: default
State: running
PID: 12345
CPU: 1.2%  Memory: 33554432 bytes
Uptime: 60000 ms  Restarts: 0
User: lynx  Mode: fork  Version: 0.0.1
```
