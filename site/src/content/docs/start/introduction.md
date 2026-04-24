---
title: Introduction
description: What Lynx is, why it exists, and how it fits into the Linux process-manager landscape.
---

Lynx is a **process manager for Linux** — spawn, supervise, restart, and
contain long-running apps. Think PM2 or Supervisor, but compiled,
secure, and built directly on top of `systemd` instead of reinventing
the wheel.

## What you get

- **A CLI (`lynxpm`) + daemon (`lynxd`)** that talk over a local unix
  socket. `lynxpm` stays out of the supervision path — the daemon is
  the one holding the apps up, so quitting the CLI never kills your
  services.
- **`systemd`-native supervision**: the daemon delegates restart,
  resource limits, sandboxing, and journal capture to `systemd` units
  generated per process. No duplicate watchdog logic.
- **Namespace-aware operations**: group apps by `namespace`, then
  `stop`, `restart`, `reload`, or `delete` an entire tier with a
  single flag or selector.
- **Secure-by-default isolation** through `DynamicUser`, landlock,
  cgroup resource caps, and systemd credentials.

## Who it's for

- Teams deploying Node / Bun / Deno / Python / Go / Rust services on
  Linux VMs or bare metal.
- Operators who want a process manager that doesn't add itself as a
  new crash surface.
- Developers who want `pm2 start`-style ergonomics without the 100 MB
  memory footprint.

## What it is not

- **Not a container runtime.** Lynx isolates via systemd + landlock,
  not namespaces + OCI images. Use it alongside containers, not
  instead.
- **Not cross-platform.** Linux only. macOS and Windows are explicit
  non-goals.
- **Not a replacement for `systemd` itself.** Lynx generates units —
  if you already hand-author unit files, keep doing that.

## Next

- [Install](/start/install/)
- [Quickstart](/start/quickstart/)
- [Access model](/start/access-model/) — system vs. user mode
