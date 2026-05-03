---
title: "lynxpm update"
description: Check for and apply updates to Lynx process manager. Downloads the latest release from GitHub and replaces the installed binary. Process state is preserved.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm update","item":"https://jaro-c.github.io/Lynx/reference/commands/update/"}]}'
sidebar:
  label: update
---

**Aliases:** `upgrade`

## 📖 Synopsis

```bash
lynxpm update|upgrade [flags]
```

## Description

Checks GitHub Releases for a newer version of Lynx. With `--apply`, it
downloads and swaps the binary in place — signature-verified first.

**Signature verification**: downloaded binaries are checked against an
ed25519 signature (`.sig` asset) before installation. Releases without a
signature — or builds where the embedded signing key is empty — refuse
`--apply` unless you pass `--insecure-skip-signature`.

**Debian/Ubuntu note**: if Lynx was installed from the `.deb`, prefer
`sudo apt install ./lynxpm_*_amd64.deb` (or `apt upgrade` once the
project ships an APT repo). `lynxpm update` detects the package origin
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
lynxpm update
```

Apply update (requires signed release):
```bash
sudo lynxpm update --apply
```

Apply update when release is unsigned (not recommended):
```bash
sudo lynxpm update --apply --insecure-skip-signature
```

Force update on a managed system (not recommended):
```bash
sudo lynxpm update --apply --force
```

## 📋 Example Output

Update available:
```
! New version available: v0.7.1
  Release notes: https://github.com/Jaro-c/Lynx/releases/tag/v0.7.1

To update, run:
  lynxpm update --apply
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

- `lynxpm list` also surfaces a banner when a newer release is available,
  backed by a 6-hour cache at `$XDG_CACHE_HOME/lynx-pm/update-check.json`.
  So users learn about releases from day-to-day commands without running
  `update` explicitly.
