# 🦁 `unitpm startup`

> *Generate and install the system startup script for unitpm.*

## 📖 Synopsis

```bash
unitpm startup [flags]
```

## Description

Generate and install the system startup script for unitpm. This command configures `systemd` to start the unitpm daemon automatically on boot.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Generate and install systemd unit (requires sudo/root if installing to /etc):
```bash
unitpm startup
```

## 📋 Example Output

Success:
```
unitpm system daemon started. Autostart enabled.
```

Failure (not root):
```
Admin privileges required. Run:
  sudo unitpm startup
```

Failure (no systemd):
```
ERR_UNSUPPORTED: unitpm requires Linux with systemd
```

## Notes

- **Requirements**: This command requires a Linux system with `systemd` as the init system.
- **Permissions**: Root or sudo privileges are typically required to write to `/etc/systemd/system` and enable services.
