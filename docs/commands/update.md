# `lynx update`

> *Check for updates and apply them.*

## Synopsis

```bash
lynx update [flags]
```

## Description

The `update` command checks if a new version of Lynx is available on GitHub.
It can also download and apply the update automatically for standalone installations.

**Signature verification (v0.5.0+):** downloaded binaries are verified against
an ed25519 signature (`.sig` asset) before installation. If a release does not
include a signature, or the embedded signing key is empty, `--apply` is refused
unless you pass `--insecure-skip-signature`.

**Note for Debian/Ubuntu Users:**
If you installed Lynx via a `.deb` package or APT repository, you should generally update using `sudo apt upgrade lynx-pm`.
The `lynx update` command will detect this and warn you, unless you use `--force`.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-a`, `--apply` | boolean | false | Download, verify, and apply the update if available. |
| `-c`, `--check` | boolean | true | Check for updates without applying. |
| `-f`, `--force` | boolean | false | Force update even if managed by system package manager. |
| `--insecure-skip-signature` | boolean | false | Accept unsigned releases. **Dangerous**: skips integrity and authenticity verification. |
| `-h`, `--help` | - | - | Show help message. |

## Examples

Check for updates:
```bash
lynx update
```

Apply update (requires signed release):
```bash
sudo lynx update --apply
```

Apply update when release is unsigned (not recommended):
```bash
sudo lynx update --apply --insecure-skip-signature
```

Force update on a managed system (not recommended):
```bash
sudo lynx update --apply --force
```

## Example Output

Update available:
```
! New version available: v0.5.0
  Release notes: https://github.com/Jaro-c/Lynx/releases/tag/v0.5.0

To update, run:
  lynx update --apply
```

Already up to date:
```
✓ You are using the latest version (v0.5.0)
```

Signature verification failed:
```
update failed: signature verification failed: ed25519 signature does not match downloaded binary
```
