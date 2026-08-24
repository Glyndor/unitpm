---
title: Access model
description: unitpm system-mode daemon runs as the glyndor-unitpm user under systemd. User-mode runs per-UID. Learn socket paths, unitpm group permissions, and privilege boundaries.
---

unitpm runs in one of two modes. Pick based on who should own the
supervised processes and how privileged the caller needs to be.

## System mode (default with the `.deb`)

The daemon runs as the `glyndor-unitpm` system user under `systemd`. It doesn't
inherit anything from the caller's environment.

- **Socket**: `/run/unitpmd/unitpm.sock`
- **Permissions**: `0660`, group `unitpm`
- **Use for**: production, multi-user machines, CI runners.

Anyone in the `unitpm` group can drive the daemon via `unitpm`.
Everyone else gets `permission denied` on the socket — intentionally.

```bash
sudo usermod -aG unitpm "$USER" && newgrp unitpm
```

## User mode

The daemon runs under your own UID (`systemd --user` unit, or
`unitpmd &` ad-hoc). It inherits your login environment.

- **Socket**: `$XDG_RUNTIME_DIR/unitpm-<uid>/unitpm.sock`
- **Permissions**: `0600`
- **Use for**: dev machines, per-user isolation, CI jobs that don't
  want system-wide state.

```bash
unitpmd &                   # foreground, dies on logout
sudo unitpm startup       # installs the systemd --user unit properly
```

## Which mode is the CLI talking to?

`unitpm` picks automatically:

1. If `UNITPM_SOCKET` is set, it uses that.
2. Else, if `/run/unitpmd/unitpm.sock` is accessible, system mode.
3. Else, `$XDG_RUNTIME_DIR/unitpm-<uid>/unitpm.sock`.

Override with `UNITPM_SOCKET=/path/to/sock unitpm list` when you need to
pin it explicitly.

## Privilege boundaries

- **CLI**: runs as the invoking user. Never needs root.
- **Daemon (system mode)**: runs as `glyndor-unitpm`, not `root`. Polkit rules
  grant it the few capabilities it needs (mostly start / stop units).
- **Managed processes**: default to the `glyndor-unitpm` user. With
  `--isolation dynamic`, each process gets its own ephemeral
  `DynamicUser=` allocation — a fresh UID that disappears when the
  process stops.

## Related

- [Install](./install/) — how the `.deb` wires this up.
- Security model — the [security reference](../reference/security/).
