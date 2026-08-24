---
title: Quickstart
description: Start a supervised, auto-restarting service with unitpm in three commands. Covers unitpm start, inspect with list and logs, and namespace bulk operations.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: |-
      {"@context":"https://schema.org","@type":"HowTo","name":"How to start a process with unitpm process manager","description":"Start a supervised, auto-restarting Linux service with unitpm in three commands.","totalTime":"PT2M","step":[{"@type":"HowToStep","position":1,"name":"Start a process","text":"Run unitpm start with your command, name, and restart policy: unitpm start 'node server.js' --name api --namespace prod --restart always","url":"https://jaro-c.github.io/unitpm/start/quickstart/#1-start-something"},{"@type":"HowToStep","position":2,"name":"Inspect the process","text":"Run unitpm list for the full table, unitpm show api for details, or unitpm logs api --follow for live output.","url":"https://jaro-c.github.io/unitpm/start/quickstart/#2-inspect"},{"@type":"HowToStep","position":3,"name":"Operate on a namespace","text":"Run unitpm restart --namespace prod to roll the entire tier, or unitpm stop 'prod:*' to halt all processes in the namespace.","url":"https://jaro-c.github.io/unitpm/start/quickstart/#3-operate-on-the-whole-tier"}]}
---

This page walks you from zero to a supervised, log-captured, auto-
restarting service in three commands.

Assumes [unitpm is already installed](./install/) and the daemon is
running (`systemctl is-active unitpmd` or `pgrep unitpmd`).

## 1. Start something

Pick any long-running command. This example uses Node, but it could
just as easily be `python`, `go run`, `bun dev`, or a compiled binary.

```bash
unitpm start "node server.js" --name api --namespace prod --restart always
```

What the flags mean:

- `--name api` — the label you'll refer to it by.
- `--namespace prod` — groups this process with every other `prod:*`
  app for bulk operations.
- `--restart always` — restart on any exit. Other policies: `never`,
  `on-failure`.

After a successful start, unitpm prints the current process table with
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
unitpm list              # full table
unitpm show api          # detail view for one process
unitpm logs api --follow # live stdout/stderr
```

## 3. Operate on the whole tier

Every lifecycle command accepts a namespace selector, so you never
need `xargs` loops:

```bash
unitpm restart --namespace prod   # roll every prod:* app
unitpm stop    'prod:*'           # halt the tier (quote the glob)
unitpm delete  --namespace old --purge
```

## From here

- **Pick your runtime**: [Runtimes guide](../guides/runtimes/) — Node /
  Bun / Python / Go / Rust / Ruby / JVM / PHP recipes.
- **Tutorials**: [Next.js, FastAPI, Django, production hardening](../guides/tutorials/).
- **Config-as-code**: `unitpm export api > unitpm.yml` to capture
  the exact invocation, then commit it. `unitpm apply unitpm.yml`
  re-applies on any box.
- **FAQ**: [Common questions and troubleshooting](../guides/faq/).
