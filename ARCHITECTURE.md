# Architecture

High-level guide to how Lynx is put together. Intended for contributors.

## Top-Level Layout

```
cmd/
  lynx/           CLI entry point (client)
  lynxd/          Daemon entry point (server)
internal/
  cli/            All CLI command implementations (18 user-facing + 2 internal wrappers)
    commands/     One directory per command
      start/ list/ stop/ restart/ reload/ flush/ delete/
      show/ logs/ monit/ apply/ export/ startup/ version/
      update/ install-tools/ completion/
      execenv/      internal wrapper for --isolation dynamic (LoadCredential)
      execsandbox/  internal wrapper for --isolation sandbox (landlock + rlimit)
    root/         Command dispatch + global flag parsing (--quiet)
    registry/     Maps command names to Run() functions
    help/         Shared help rendering (Hidden flag, Examples slot)
    batch/        Aggregate result/summary shape for multi-target commands
    expand/       Namespace selector resolution (NS:* / *:* / --namespace)
    errs/         Usage error type
  daemon/         Daemon runtime
    manager/      Process lifecycle (spawn, monitor, restart, cron, scale)
    handlers/     IPC request handlers (start, stop, list, …)
    policy/       Authorization + restart policy + backoff calculators
    audit/        JSON-lines audit log for destructive actions (system mode)
    runtime/      Isolation glue; thin wrappers around:
      landlock/    Landlock ruleset (unprivileged filesystem sandbox)
      rlimit/      setrlimit for sandbox resource caps
  ipc/
    protocol/     Wire types: AppSpec, StartRequest, responses, errors
    transport/    Unix-socket client, server, framing, identity
  spec/           On-disk spec persistence (XDG_CONFIG_HOME/lynx/apps)
  env/            .env file parser (whitelist, escape handling)
  lynxfile/       Lynxfile.yml declarative format parser
  metrics/        Per-process /proc + cgroup collectors
  paths/          XDG-aware path resolution (log dirs, socket, config)
  git/            Git HEAD probe for list view
  updater/        GitHub release check + self-update
  version/        Compile-time version injection
  term/           Terminal colors + formatting helpers
  jsonx/          Fast JSON via bytedance/sonic wrapper
  types/          Shared types (ProcessInfo, State enum)
debian/           Debian packaging (service, polkit, postinst)
scripts/          Build scripts (build_cli.sh, build_deb.sh)
docs/             Per-command docs + release guide
```

## Process Model

Two binaries, one long-lived daemon:

```
┌──────────┐   Unix socket (JSON-RPC)   ┌──────────┐
│  lynx    │ ──────────────────────────▶│  lynxd   │
│  (CLI)   │ ◀──────────────────────────│  (daemon)│
└──────────┘                            └────┬─────┘
                                             │ fork+exec / systemd-run
                                             ▼
                                         Managed
                                         processes
```

The daemon survives CLI invocations. Managed processes survive daemon restarts
when supervised via systemd (`--isolation dynamic`). The CLI is stateless
beyond the short-lived socket connection.

## IPC Protocol

- **Transport**: Unix domain socket.
- **Framing**: length-prefixed (`uint32` big-endian) + JSON payload.
- **Identity**: `SO_PEERCRED` on every connection; UID/GID/PID captured.
- **Versioning**: every request carries `protocol_version`; mismatch yields
  a `PROTOCOL_MISMATCH` `RemoteError` that the CLI surfaces explicitly via
  `lynxpm version`.
- **Encoding**: JSON via `bytedance/sonic` (decoding-heavy workload benefits
  from sonic over `encoding/json`).

Socket location:

| Mode         | Path                                       | Perms  |
|--------------|--------------------------------------------|--------|
| System       | `/run/lynxd/lynx.sock`                     | `0660` |
| User         | `$XDG_RUNTIME_DIR/lynx-<uid>/lynx.sock`    | `0600` |

## Command Flow — `lynxpm start`

```
CLI                                     Daemon
───                                     ──────
ParseAppSpec(args)
  → flag parsing, tokenization
  → AppSpec

spec.GenerateID()           — UUID v7, time-ordered
spec.SaveSpec(id, appSpec)  — writes ~/.config/lynx/apps/<id>.json (0600)

client.Call("start", req) ────────────▶ handlers.Start(req)
                                          validate(spec)
                                            – name/namespace regex
                                            – cwd canonicalize + allowlist
                                            – env key sanitization
                                          manager.Spawn(spec)
                                            – exec.Cmd OR systemd-run
                                            – per-process cgroup (v2)
                                            – setupLogs, tee to file
                                          start monitor goroutine
                                          reply: {id, pid, status}
                   ◀───────────────────

If IPC error: spec.DeleteSpec(id)       — orphan cleanup
```

Key invariants:
- **Spec saved before daemon call** so a mid-flight daemon restart doesn't
  lose the app.
- **Spec deleted if daemon rejects** so bad specs don't resurrect on restart.

## Process Lifecycle (daemon side)

`internal/daemon/manager/process.go` is the heart of the daemon:

```
           spawn()
              │
              ▼
        [StateStarting] ──exec─▶ [StateRunning]
                                       │
        ┌──────────────────────────────┤
        │                              │
   Wait() returns               user calls stop
        │                              │
        ▼                              ▼
  [StateExited]            SIGTERM → SIGKILL
        │                              │
        ▼                              ▼
  policy.ShouldRestart?          [StateStopped]
        │
     yes│no
        ▼
  backoff(restartCount)
        │
        ▼
   [StateRestarting] → spawn() again
```

Concurrency rules:
- `Process.mu` protects `info`, `cmd`, `logFiles`, `exitError`, `restartCount`.
- The monitor goroutine acquires the lock before closing log files and setting
  `logFiles = nil` (the race fixed in commit 702b82a).
- Restart count is bucketed: resets if >60s since last restart.

## Isolation Modes

Set via `--isolation`:

| Mode      | Implementation                          | Privilege model                       |
|-----------|-----------------------------------------|---------------------------------------|
| `self`    | Plain `exec.Cmd`                        | Runs as daemon user (`lynx` or user)  |
| `dynamic` | `systemd-run DynamicUser=yes` transient | Synthetic UID/GID per process         |
| `sandbox` | user ns + landlock + rlimit + NO_NEW_PRIVS | Unprivileged, no sudo required        |

`--isolation dynamic` only works in system mode (the user's systemd instance
cannot create synthetic users). `--isolation sandbox` will fill the gap for
user-mode deployments — see `SECURITY.md` "Known Limitations".

## Spec Persistence

Specs live in `$XDG_CONFIG_HOME/lynx/apps/<id>.json` (default
`~/.config/lynx/apps/`). File mode `0600`, directory mode `0700`.

- Written by `lynxpm start` before the daemon call.
- Written by `lynxpm apply` (one file per app in the Lynxfile).
- Loaded by the daemon on startup to restore managed processes.
- Deleted by `lynxpm delete` or when the daemon rejects a spec on start.

## Metrics Collection

`internal/metrics/factory_linux.go` picks the collector at spawn time:

1. **`ProcTreeCollector`** — reads `/proc/<pid>/stat` for per-process RSS
   and CPU ticks. **Preferred.**
2. **`CgroupCollector`** — reads `memory.current` from the process's cgroup.
   Fallback only; accurate only for `--isolation dynamic` (dedicated cgroup).

The priority was swapped (commit 8b8905e) because the session cgroup
(`ptyxis-spawn-*.scope`) holds the entire terminal session (~1 GB) not the
single process (~7 MB).

## Cron Scheduling

`github.com/robfig/cron/v3` drives `--cron` / `--schedule`. Each scheduled
tick calls `handleRestart()` on the process. Missed ticks are dropped
(no catch-up queue).

## Error Taxonomy

All daemon errors use a common shape:

```go
RemoteError{
  Code: "ERR_BAD_REQUEST",  // machine-readable code
  Message: "...",            // human text
  Data: any,                 // structured payload (e.g. MismatchData)
}
```

Codes used: `ERR_BAD_REQUEST`, `ERR_NOT_FOUND`, `ERR_CONFLICT`,
`ERR_LIMITS`, `ERR_UNSUPPORTED`, `ERR_RATE_LIMIT`, `ERR_TIMEOUT`,
`PROTOCOL_MISMATCH`, `INTERNAL_ERROR`.

The CLI maps these to exit codes in `internal/cli/errs`.

## Testing Strategy

- **Pure helpers**: direct unit tests (`env`, `lynxfile`, `protocol`,
  `version`, `paths`, `metrics formatters`).
- **IPC-bound commands**: an inline `mockClient` that implements
  `transport.IPCClient` — round-trips JSON through a captured response.
- **Daemon manager**: spawns real `echo`/`sleep` processes against a temp
  state directory.
- **Filesystem-bound commands** (`export`, `logs`): `t.Setenv` +
  `t.TempDir()` to isolate spec dirs.

Current coverage: ~69% across the CLI surface. Gaps are documented in
test-file comments.

## Release Flow

```
debian/changelog bump → git tag vX.Y.Z → git push --tags
                     → .github/workflows/release.yml builds,
                       signs (ed25519), attests (SLSA), and
                       publishes: lynxpm_linux_{amd64,arm64},
                       lynxpm_<ver>_amd64.deb, SBOM, sigs.
```

The `updater` package checks GitHub releases on demand (`lynxpm update`) and
prefers guiding users to `apt install ./file.deb` when `IsManagedByPackageSystem()`
returns true.
