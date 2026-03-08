# 🦁 `lynx update`

> *Check for updates and apply them.*

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

## ⚙️ Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `-a`, `--apply` | boolean | false | Download and apply the update if available. | `lynx update --apply` |
| `-c`, `--check` | boolean | true | Check for updates without applying. | `lynx update` |
| `-f`, `--force` | boolean | false | Force update even if managed by system package manager. | `lynx update --force` |
| `-h`, `--help` | - | - | Show help message. | — |

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

## 📋 Example Output

Update available:
```
! New version available: v1.2.0
  Release notes: https://github.com/Jaro-c/Lynx/releases/tag/v1.2.0

To update, run:
  lynx update --apply
```

Already up to date:
```
✓ You are using the latest version (v1.1.0)
```
