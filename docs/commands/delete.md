# 🦁 `lynxpm delete | remove | rm`

> *Delete one or more processes and their configurations.*

## 📖 Synopsis

```bash
lynxpm delete|remove|rm [--purge] <id|name>...
```

## Description

Stops and removes the specified processes from management. By default, it removes the process from the list and deletes its spec file.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--purge` | boolean | false | Also delete the log files and any runtime data associated with the process. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Delete a process (keep logs):
```bash
lynxpm delete my-app
```

Delete a process and its logs:
```bash
lynxpm delete --purge my-app
```
