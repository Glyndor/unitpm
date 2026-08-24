# 🦁 `unitpm update`

> *Check for updates and apply them.*

**Aliases:** `upgrade`

## 📖 Synopsis

```bash
unitpm update|upgrade [flags]
```

## Description

Checks GitHub Releases for a newer version of unitpm. With `--apply`, it
downloads and swaps the binary in place — signature-verified first.

**Signature verification**: downloaded binaries are checked against an
ed25519 signature (`.sig` asset) before installation. Releases without a
signature — or builds where the embedded signing key is empty — refuse
`--apply` unless you pass `--insecure-skip-signature`.

**Debian/Ubuntu note**: if unitpm was installed from the `.deb`, prefer
`sudo apt install ./unitpm_*_amd64.deb` (or `apt upgrade` once the
project ships an APT repo). `unitpm update` detects the package origin
and refuses `--apply` unless you pass `--force`.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-a`, `--apply` | boolean | false | Download, verify, and apply the update if available. |
| `-c`, `--check` | boolean | true | Check for updates without applying. |
| `-f`, `--force` | boolean | false | Force update even if managed by the system package manager. |
| `--insecure-skip-signature` | boolean | false | Accept unsigned releases. **Dangerous**: skips integrity and authenticity verification. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Check for updates:
```bash
unitpm update
```

Apply update (requires signed release):
```bash
sudo unitpm update --apply
```

Apply update when release is unsigned (not recommended):
```bash
sudo unitpm update --apply --insecure-skip-signature
```

Force update on a managed system (not recommended):
```bash
sudo unitpm update --apply --force
```

## 📋 Example Output

Update available:
```
! New version available: v0.7.1
  Release notes: https://github.com/Jaro-c/unitpm/releases/tag/v0.7.1

To update, run:
  unitpm update --apply
```

Already up to date:
```
✓ You are using the latest version (v0.7.1)
```

Signature verification failed:
```
update failed: signature verification failed: ed25519 signature does not match downloaded binary
```

## Notes

- `unitpm list` also surfaces a banner when a newer release is available,
  backed by a 6-hour cache at `$XDG_CACHE_HOME/unitpm/update-check.json`.
  So users learn about releases from day-to-day commands without running
  `update` explicitly.
