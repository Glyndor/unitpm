---
title: Quickstart
description: Spin up your first process with Lynx in under two minutes.
---

This page walks you from zero to a supervised, log-captured, auto-
restarting service in three commands.

Assumes [Lynx is already installed](/start/install/) and the daemon is
running (`systemctl is-active lynxd` or `pgrep lynxd`).

## 1. Start something

Pick any long-running command. This example uses Node, but it could
just as easily be `python`, `go run`, `bun dev`, or a compiled binary.

```bash
lynxpm start "node server.js" --name api --namespace prod --restart always
```

What the flags mean:

- `--name api` — the label you'll refer to it by.
- `--namespace prod` — groups this process with every other `prod:*`
  app for bulk operations.
- `--restart always` — restart on any exit. Other policies: `never`,
  `on-failure`.

After a successful start, Lynx prints the current process table with
the new row marked `▸`:

```
✓ Started api
  ID:     019dbd…
  PID:    2336607
  Status: running

┌──────────┬──────┬──────────┬────────┬─────────┐
│ id       │ name │ namespace│ status │ pid     │
├──────────┼──────┼──────────┼────────┼─────────┤
│ ▸ 019dbd │ api  │ prod     │ running│ 2336607 │
└──────────┴──────┴──────────┴────────┴─────────┘
```

## 2. Inspect

```bash
lynxpm list              # full table
lynxpm show api          # detail view for one process
lynxpm logs api --follow # live stdout/stderr
```

## 3. Operate on the whole tier

Every lifecycle command accepts a namespace selector, so you never
need `xargs` loops:

```bash
lynxpm restart --namespace prod   # roll every prod:* app
lynxpm stop    'prod:*'           # halt the tier (quote the glob)
lynxpm delete  --namespace old --purge
```

## From here

- **Pick your runtime**: [Runtimes guide](/guides/runtimes/) — Node /
  Bun / Python / Go / Rust / Ruby / JVM / PHP recipes.
- **Tutorials**: [Next.js, FastAPI, Django, production hardening](/guides/tutorials/).
- **Config-as-code**: `lynxpm export api > Lynxfile.yml` to capture
  the exact invocation, then commit it. `lynxpm apply Lynxfile.yml`
  re-applies on any box.
- **FAQ**: [Common questions and troubleshooting](/guides/faq/).
