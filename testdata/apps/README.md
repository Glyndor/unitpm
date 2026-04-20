# Test apps

Sample applications used by the Debian package tests and local
end-to-end validation. Each subdirectory is a **minimal** standalone
app meant to exercise one specific supervisor behaviour.

Runtime toolchains required:

| App                  | Needs         | Purpose                                                  |
|----------------------|---------------|----------------------------------------------------------|
| `node-http/`         | `node`        | HTTP listener with graceful SIGTERM shutdown             |
| `node-ignores-term/` | `node`        | Listener that masks SIGTERM → forces SIGKILL timeout     |
| `python-worker/`     | `python3`     | Long-running worker; verifies plain start/stop/list      |
| `python-crashloop/`  | `python3`     | Exits 1 after 1s → regresses `--max-restarts` cap        |
| `php-worker/`        | `php` (CLI)   | PHP worker with pcntl SIGTERM handling                   |
| `ruby-worker/`       | `ruby`        | Ruby worker with Signal.trap SIGTERM handling            |
| `go-compiled/`       | `go` (build)  | Compiled binary with ctx-based graceful shutdown         |
| `shell-forkstorm/`   | `bash`        | Forks 10 workers → regresses the `/proc` descendant walk |

## Invariants every app honours

- No external dependencies at runtime (node/python stdlib only; Go
  compiled ahead of time by the Makefile).
- No side effects outside its own `cwd` + `--log-dir`.
- Prints its own PID on startup so the test harness can correlate
  lifecycle events without grepping `ps`.

## Running one app by hand

```bash
# Build the Go app (others run directly).
make -C testdata/apps/go-compiled

# Start it.
lynxpm start "node server.js" --name node-smoke --cwd testdata/apps/node-http
lynxpm logs node-smoke --follow
lynxpm stop  node-smoke
lynxpm delete node-smoke
```

## Used by

- `.github/workflows/debian-tests.yml` — smoke step installs each
  runtime only where required, then walks every lifecycle command
  against the corresponding app.
