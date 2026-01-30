# startup

Generate and install the system startup script for Lynx.

## Synopsis

```bash
lynx startup [flags]
```

## Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `-h`, `--help` | - | - | Show help message. | `lynx startup --help` |

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
