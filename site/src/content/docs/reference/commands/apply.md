---
title: "lynxpm apply"
description: Declaratively create and start processes from a Lynxfile.yml under Lynx process manager. Reads YAML specs and starts each app with its stored config.
sidebar:
  label: apply
---

## 📖 Synopsis

```bash
lynxpm apply [--json] <Lynxfile.yml>
```

## Description

Apply a declarative Lynxfile to create and start one or more applications.
Each app entry in the file is converted into an AppSpec, saved securely,
and started via the daemon. Apply aborts on the first failure — any
successfully-started apps remain running. When `--json` is used and an
abort happens mid-file, the partial report is still emitted on stdout with
`partial: true` so callers can see exactly which apps started.

## Lynxfile format

```yaml
version: "1"
namespace: default
apps:
  - name: my-api
    command: "node server.js"
    cwd: "/srv/my-api"
    env:
      PORT: "3000"
    logs:
      dir: "/var/log/lynx-pm"
      stdout: "stdout.log"
      stderr: "stderr.log"
    restart:
      policy: "on-failure"
      max_restarts: 10
      delay_ms: 2000
      backoff: "expo"
```

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | boolean | false | Emit a machine-readable `{results, summary}` batch report on stdout. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Apply a Lynxfile:
```bash
lynxpm apply ./Lynxfile.yml
```

Apply and collect outcomes:
```bash
lynxpm apply ./Lynxfile.yml --json | jq '.results[] | {id, status, extra}'
```

## Notes

- Specs are stored in `~/.config/lynx/apps` with `0600` permissions.
- If `namespace` is omitted per app, the file‑level namespace or `default` is used.
