---
title: Access model
description: System-mode vs user-mode daemon, the unix-socket permissions, and the lynxadm group.
---

Lynx runs in one of two modes. Pick based on who should own the
supervised processes and how privileged the caller needs to be.

## System mode (default with the `.deb`)

The daemon runs as the `lynx` system user under `systemd`. It doesn't
inherit anything from the caller's environment.

- **Socket**: `/run/lynxd/lynx.sock`
- **Permissions**: `0660`, group `lynxadm`
- **Use for**: production, multi-user machines, CI runners.

Anyone in the `lynxadm` group can drive the daemon via `lynxpm`.
Everyone else gets `permission denied` on the socket — intentionally.

```bash
sudo usermod -aG lynxadm "$USER" && newgrp lynxadm
```

## User mode

The daemon runs under your own UID (`systemd --user` unit, or
`lynxd &` ad-hoc). It inherits your login environment.

- **Socket**: `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock`
- **Permissions**: `0600`
- **Use for**: dev machines, per-user isolation, CI jobs that don't
  want system-wide state.

```bash
lynxd &                   # foreground, dies on logout
sudo lynxpm startup       # installs the systemd --user unit properly
```

## Which mode is the CLI talking to?

`lynxpm` picks automatically:

1. If `LYNX_SOCKET` is set, it uses that.
2. Else, if `/run/lynxd/lynx.sock` is accessible, system mode.
3. Else, `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock`.

Override with `LYNX_SOCKET=/path/to/sock lynxpm list` when you need to
pin it explicitly.

## Privilege boundaries

- **CLI**: runs as the invoking user. Never needs root.
- **Daemon (system mode)**: runs as `lynx`, not `root`. Polkit rules
  grant it the few capabilities it needs (mostly start / stop units).
- **Managed processes**: default to the `lynx` user. With
  `--isolation dynamic`, each process gets its own ephemeral
  `DynamicUser=` allocation — a fresh UID that disappears when the
  process stops.

## Related

- [Install](/start/install/) — how the `.deb` wires this up.
- Security model — the [security reference](/reference/security/).
