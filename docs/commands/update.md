# 🦁 `lynx update`

> *Documentation for the lynx command.*

Check for updates and apply them.

## 📖 Synopsis

```bash
lynx update [flags]
```

## Description

The `update` command checks if a new version of Lynx is available on GitHub.
It can also download and apply the update automatically for standalone installations.

**Note for Debian/Ubuntu Users:**
If you installed Lynx via a `.deb` package or APT repository, you should generally update using `sudo apt upgrade lynx`.
The `lynx update` command will detect this and warn you, unless you use `--force`.

## Options

| Flag | Description |
|------|-------------|
| `-a`, `--apply` | Download and apply the update if available. |
| `-c`, `--check` | Check for updates without applying (default). |
| `-f`, `--force` | Force update even if managed by system package manager. |
| `-h`, `--help` | Show help for command. |

## 🚀 Examples

Check for updates:
```bash
lynx update
```

Apply update:
```bash
lynx update --apply
```

Force update on a managed system (not recommended):
```bash
lynx update --apply --force
```
