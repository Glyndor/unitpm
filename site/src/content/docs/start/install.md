---
title: Install
description: Install Lynx process manager on Debian, Ubuntu, or any systemd Linux. Prebuilt .deb for amd64 and arm64, static binary download, or build from Go source.
---

Pick the path that matches your target machine.

## Debian / Ubuntu — `.deb` (recommended)

The `.deb` is built, signed, and tested in CI against Debian bookworm,
Debian trixie, Ubuntu 22.04, and Ubuntu 24.04. It installs the
`lynxpm` CLI, the `lynxd` daemon, a system-mode `systemd` unit, and
polkit rules for the `lynxadm` group.

```bash
# Grab the latest .deb from https://github.com/Jaro-c/Lynx/releases
sudo apt install ./lynxpm_*_amd64.deb
sudo usermod -aG lynxadm "$USER" && newgrp lynxadm
sudo systemctl enable --now lynxd
sudo lynxpm install-tools   # optional: expose bun/node/go/… to the daemon
```

You're done. `lynxpm --version` should print `0.12.0` or newer.

## Prebuilt binary (any Linux)

Use this when you're not on Debian/Ubuntu, or when you want to pin a
specific version without the package manager in the loop. The binary
is statically linked (`CGO_ENABLED=0`) and ships with a signature +
SBOM + SLSA provenance attestation.

```bash
# amd64
gh release download --repo Jaro-c/Lynx --pattern 'lynxpm_linux_amd64'
install -m 0755 lynxpm_linux_amd64 ~/.local/bin/lynxpm

# arm64
gh release download --repo Jaro-c/Lynx --pattern 'lynxpm_linux_arm64'
install -m 0755 lynxpm_linux_arm64 ~/.local/bin/lynxpm
```

Then start a user-mode daemon:

```bash
lynxd &
```

Or wire it as a `systemd --user` unit:

```bash
sudo lynxpm startup   # installs the unit, enables + starts it
```

## Build from source

Requires Go 1.26+.

```bash
git clone https://github.com/Jaro-c/Lynx
cd Lynx
go build -o lynxpm ./cmd/lynxpm
go build -o lynxd  ./cmd/lynxd
```

## Verify the release signature (optional)

Every release ships with a detached signature over the binary. The
public key lives in `SECURITY.md` on the repo.

```bash
gh release download --repo Jaro-c/Lynx --pattern 'lynxpm_linux_amd64*'
# verify signature with the key in SECURITY.md
```

## Next

- [Quickstart](./quickstart/) — run your first process.
- [Access model](./access-model/) — system-mode vs user-mode daemon.
