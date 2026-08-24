---
title: Install
description: Install unitpm process manager on Debian, Ubuntu, or any systemd Linux. Prebuilt .deb for amd64 and arm64, static binary download, or build from Go source.
---

Pick the path that matches your target machine.

## Debian / Ubuntu — `.deb` (recommended)

The `.deb` is built, signed, and tested in CI against Debian bookworm,
Debian trixie, Ubuntu 22.04, and Ubuntu 24.04. It installs the
`unitpm` CLI, the `unitpmd` daemon, a system-mode `systemd` unit, and
polkit rules for the `unitpm` group.

```bash
# Grab the latest .deb from https://github.com/Jaro-c/unitpm/releases
sudo apt install ./unitpm_*_amd64.deb
sudo usermod -aG unitpm "$USER" && newgrp unitpm
sudo systemctl enable --now unitpmd
sudo unitpm install-tools   # optional: expose bun/node/go/… to the daemon
```

You're done. `unitpm --version` should print `0.13.0` or newer.

## Prebuilt binary (any Linux)

Use this when you're not on Debian/Ubuntu, or when you want to pin a
specific version without the package manager in the loop. The binary
is statically linked (`CGO_ENABLED=0`) and ships with a signature +
SBOM + SLSA provenance attestation.

```bash
# amd64
gh release download --repo Jaro-c/unitpm --pattern 'unitpm_linux_amd64'
install -m 0755 unitpm_linux_amd64 ~/.local/bin/unitpm

# arm64
gh release download --repo Jaro-c/unitpm --pattern 'unitpm_linux_arm64'
install -m 0755 unitpm_linux_arm64 ~/.local/bin/unitpm
```

Then start a user-mode daemon:

```bash
unitpmd &
```

Or wire it as a `systemd --user` unit:

```bash
sudo unitpm startup   # installs the unit, enables + starts it
```

## Build from source

Requires Go 1.26+.

```bash
git clone https://github.com/Jaro-c/unitpm
cd unitpm
go build -o unitpm ./cmd/unitpm
go build -o unitpmd  ./cmd/unitpmd
```

## Verify the release signature (optional)

Every release ships with a detached signature over the binary. The
public key lives in `SECURITY.md` on the repo.

```bash
gh release download --repo Jaro-c/unitpm --pattern 'unitpm_linux_amd64*'
# verify signature with the key in SECURITY.md
```

## Next

- [Quickstart](./quickstart/) — run your first process.
- [Access model](./access-model/) — system-mode vs user-mode daemon.
