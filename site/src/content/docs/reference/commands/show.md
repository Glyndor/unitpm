---
title: "lynxpm show"
description: Show detailed runtime and spec for a single Lynx process — PID, uptime, restart history, resource limits, environment variables, and isolation mode.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm show","item":"https://jaro-c.github.io/Lynx/reference/commands/show/"}]}'
sidebar:
  label: show
---

**Aliases:** `info`, `describe`

## 📖 Synopsis

```bash
lynxpm show <id|name|namespace:name> [--json]
```

## Description

Prints everything Lynx knows about a single process as a set of box-drawing
tables grouped by topic (Process, Exec, Environment, Logs, Restart, Stop,
Resources, Isolation, Schedule, Watch). Values carry dual representations
where useful — memory is rendered as both a human string and exact bytes,
uptime as both a short form and milliseconds, timestamps as absolute and
relative. Pipe `--json` into `jq` for programmatic use.

## ⚙️ Flags

| Flag | Type | Default | Description | Example |
|------|------|---------|-------------|---------|
| `--json` | boolean | false | Emit the raw daemon response as JSON on stdout. | `--json` |
| `-h`, `--help` | - | - | Show help message. | — |

## 🚀 Examples

Show by name:

```bash
lynxpm show my-api
```

Show by namespace-qualified name:

```bash
lynxpm info prod:my-api
```

Show by short ID:

```bash
lynxpm describe 019d9a04
```

Pipe JSON through `jq`:

```bash
lynxpm show my-api --json | jq '.spec.env'
lynxpm show my-api --json | jq '.info.memory_bytes'
```

## 📋 Example Output

```
Process App-Web (019d9a04-84fc-76a0-a48a-78f328e3ab2f)

Process
┌────────────┬──────────────────────────────┐
│ field      │ value                        │
├────────────┼──────────────────────────────┤
│ state      │ running                      │
│ pid        │ 261230                       │
│ namespace  │ PNUDxSENA                    │
│ version    │ 1.1.38                       │
│ mode       │ fork                         │
│ uptime     │ 22m 29s (1349941 ms)         │
│ restarts   │ 1                            │
│ cpu        │ 0.2%                         │
│ memory     │ 232.6 MB (243867648 bytes)   │
│ user       │ md3uu52l80m7                 │
│ created at │ 2026-04-19 09:00:00 (6h ago) │
│ git        │ main@0b6f1167                │
│ watch      │ disabled                     │
│ disabled   │ false                        │
└────────────┴──────────────────────────────┘

Exec
┌─────────┬───────────────────────────┐
│ field   │ value                     │
├─────────┼───────────────────────────┤
│ type    │ command                   │
│ runtime │ bun                       │
│ command │ bun                       │
│ args    │ run server.ts --port 3000 │
│ shell   │ false                     │
│ cwd     │ /srv/app-web              │
└─────────┴───────────────────────────┘

Environment
┌──────────────┬───────────────────┐
│ field        │ value             │
├──────────────┼───────────────────┤
│ env-file     │ /srv/app-web/.env │
│ API_TOKEN    │ ********          │
│ DATABASE_URL │ postgres://…      │
│ NODE_ENV     │ production        │
│ PORT         │ 3000              │
└──────────────┴───────────────────┘

Logs
┌───────────┬──────────────────────────────────┐
│ field     │ value                            │
├───────────┼──────────────────────────────────┤
│ mode      │ file                             │
│ dir       │ /var/log/lynx-pm/App-Web            │
│ stdout    │ /var/log/lynx-pm/App-Web/stdout.log │
│ stderr    │ /var/log/lynx-pm/App-Web/stderr.log │
│ format    │ plain                            │
│ timestamp │ rfc3339                          │
└───────────┴──────────────────────────────────┘

Restart
┌────────────┬───────────┐
│ field      │ value     │
├────────────┼───────────┤
│ policy     │ always    │
│ maxRetries │ 10        │
│ backoff    │ expo (2s) │
│ stopOnExit │ 0, 143    │
└────────────┴───────────┘

Stop
┌─────────┬────────────────┐
│ field   │ value          │
├─────────┼────────────────┤
│ signal  │ SIGTERM        │
│ timeout │ 30s (30000 ms) │
└─────────┴────────────────┘

Resources
┌────────────┬────────────────────────────┐
│ field      │ value                      │
├────────────┼────────────────────────────┤
│ memory max │ 512.0 MB (536870912 bytes) │
│ cpu max    │ 200% (2.00 cores)          │
│ tasks max  │ 64                         │
└────────────┴────────────────────────────┘
```

Sections that hold no data are skipped — a process without `--schedule`
won't render an empty Schedule table, and a spec without resource limits
omits the Resources table entirely.

## Notes

- **Value transformations**: memory shows both human (`232.6 MB`) and exact
  bytes, uptime shows both human (`22m 9s`) and raw milliseconds, timestamps
  show both absolute local time and a relative age (`6h ago`), CPU caps
  show both percent-of-core and fractional cores.
- **Secret masking**: env values whose key contains `TOKEN`, `SECRET`,
  `PASSWORD`, `PASSWD`, `KEY`, `CREDENTIAL`, or `PRIVATE` render as
  `********`. Use `--json` to emit the raw values for programmatic use.
- **Color coding**: `running`/`online` green; `stopped`/`failed` red;
  `restarting` yellow. Unavailable fields show a dimmed `-`.
- **JSON schema**: `{ info: ProcessInfo, spec: AppSpec }` — see
  `internal/types/process.go` and `internal/ipc/protocol/types.go`.
