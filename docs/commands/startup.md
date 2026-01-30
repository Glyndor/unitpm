# startup

## Synopsis

```bash
lynx startup [flags]
```

## Usage

Generate and install the system startup script for Lynx. This command configures `systemd` to start the Lynx daemon automatically on boot.

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `-h`, `--help` | - | - | Show help message. | — |

## Examples

Generate and install systemd unit (requires sudo/root if installing to /etc):
```bash
lynx startup
```

## Example output

Success:
```
Lynx system daemon started. Autostart enabled.
```

Failure (not root):
```
Admin privileges required. Run:
  sudo lynx startup
```

Failure (no systemd):
```
ERR_UNSUPPORTED: Lynx requires Linux with systemd
```

## Notes

*   **Requirements**: This command requires a Linux system with `systemd` as the init system.
*   **Permissions**: Root or sudo privileges are typically required to write to `/etc/systemd/system` and enable services.
