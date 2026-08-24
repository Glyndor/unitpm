# Running Apps with unitpm — by Runtime

unitpm is language-agnostic. It executes whatever command you give it as a child
process and supervises the PID. The `--runtime` flag and file-extension
auto-detection are convenience shortcuts — they never restrict what you can
actually run.

> **Verification status.** The following runtimes are exercised end-to-end
> through `unitpm start` in a clean systemd-nspawn container with the Debian
> package installed:
>
> Node 18, Bun 1.3, Deno 2.7, Python 3.12 (system / venv / uv / uvx),
> Go (source + compiled binary), Rust, C, C++, OCaml, Haskell, Nim,
> Java 21, Ruby 3.2, Perl 5.38, PHP 8.3, Lua 5.4, R 4.3, Erlang, Elixir 1.14,
> Tcl 8.6, Bash 5.2.
>
> Docker-as-managed-process and Kotlin/Scala are shape-correct per each
> tool's docs but not part of the automated matrix.

Two things matter:

1. **The daemon must see the binary you're asking for.** In system mode the
   daemon runs as the `glyndor-unitpm` user and searches its own `PATH`. If your
   interpreter lives under `~/.local/bin`, `~/.bun/bin`, or an fnm/nvm
   shell-managed directory, run `unitpm install-tools` (user mode) or
   `sudo unitpm install-tools --system` to symlink the important ones into a
   place unitpmd will find them.
2. **The cwd must be accessible to the daemon user.** In system mode the
   daemon runs as `glyndor-unitpm` and cannot read `/root` or other users' `$HOME`.
   Pass `--cwd` to a directory the daemon can enter (e.g. `/var/lib/unitpm`,
   `/srv/yourapp`, `/tmp`).

---

## Node.js

```bash
# Single file, auto-detected by extension
unitpm start server.js

# Explicit runtime
unitpm start app.mjs --runtime node

# With args
unitpm start "node --inspect server.js" --name api

# Package.json scripts
unitpm start "npm run start" --name api --cwd /srv/api --shell
unitpm start "pnpm start"     --name api --cwd /srv/api --shell
unitpm start "yarn start"     --name api --cwd /srv/api --shell

# Version-managed Node (fnm / nvm)
# Best: resolve the binary once and pass the full path.
unitpm start "$(fnm current-path)/node server.js" --shell

# Cluster / multi-instance
unitpm start server.js --name worker --scale 4
```

`--scale N` exposes `UNITPM_INSTANCE=0..N-1` to each child so your app can
bind to different ports (`const port = 3000 + Number(process.env.UNITPM_INSTANCE)`).

## Bun

```bash
unitpm start "bun run server.ts" --name api --cwd /srv/api
unitpm start "bun dev" --name dev-server
```

## Deno

```bash
unitpm start "deno run --allow-net server.ts" --name api --cwd /srv/api
```

## Python

### System interpreter

```bash
unitpm start app.py --runtime python3
# or explicit
unitpm start "python3 -u app.py" --name api --cwd /srv/api
```

The `-u` flag keeps stdout unbuffered so `unitpm logs` streams in real time.

### Virtualenv (venv)

```bash
# Option 1: point directly at the venv's python
unitpm start "/srv/api/.venv/bin/python app.py" --cwd /srv/api --name api

# Option 2: activate and run inside a shell
unitpm start "source .venv/bin/activate && python -u app.py" \
    --cwd /srv/api --shell --name api
```

### uv / uvx

[`uv`](https://github.com/astral-sh/uv) manages its own envs. Use `uv run` to
execute within the project's lockfile-pinned env without pre-activation:

```bash
# Run a script via uv
unitpm start "uv run app.py" --cwd /srv/api --name api

# Run a tool ad-hoc via uvx
unitpm start "uvx --from 'httpie' http :8080/health" --name probe --shell

# Pin a Python version
unitpm start "uv run --python 3.12 app.py" --cwd /srv/api --name api
```

### pyenv

```bash
unitpm start "$(pyenv which python) app.py" --cwd /srv/api --shell --name api
```

### FastAPI / uvicorn / gunicorn

```bash
unitpm start "uv run uvicorn main:app --host 0.0.0.0 --port 8080" \
    --cwd /srv/api --name api --restart always
```

## Go

```bash
# Source file, auto-detected (uses `go run`)
unitpm start main.go

# Compiled binary (preferred for production)
go build -o /srv/api/bin/api ./cmd/api
unitpm start /srv/api/bin/api --cwd /srv/api --name api --restart always

# `go run` with args
unitpm start "go run ./cmd/api --config /srv/api/config.yml" --cwd /srv/api
```

In production you almost always want the compiled binary — `go run`
re-compiles every restart.

## Rust

```bash
# Compiled (release)
cargo build --release
unitpm start ./target/release/api --cwd /srv/api --name api

# cargo run (dev only)
unitpm start "cargo run --release" --cwd /srv/api --shell --name api
```

## Ruby / Rails

```bash
# System ruby
unitpm start "bundle exec rails server -e production" --cwd /srv/api --shell

# rbenv
unitpm start "$(rbenv which bundle) exec rails s" --cwd /srv/api --shell
```

## Java / JVM (Spring, Kotlin, Scala)

```bash
unitpm start "java -Xmx512m -jar app.jar" --cwd /srv/api --name api

# With JAVA_HOME from env-file
echo "JAVA_HOME=/opt/jdk-21" > /srv/api/.env
unitpm start "/opt/jdk-21/bin/java -jar app.jar" \
    --cwd /srv/api --env-file /srv/api/.env --name api
```

## Shell scripts

```bash
unitpm start /srv/api/start.sh --name api

# Inline command
unitpm start "bash -c 'while true; do date; sleep 5; done'" --shell --name clock
```

The `--shell` flag wraps your command in `sh -c '…'`, which you need for
glob expansion, pipes, `&&`, variable interpolation. **Security note**:
`--shell` is rejected in system-mode daemons for hardening reasons. In
user mode it works as expected.

## Docker container as a managed process

You can make unitpm babysit a specific `docker run`:

```bash
unitpm start "docker run --rm --name myapp nginx" --name myapp --restart always
```

…but for most workloads a native binary + `--isolation sandbox` gives you
stronger isolation without the daemon overhead.

## Environment files

Every language flow accepts `--env-file` to inject variables:

```bash
cat > /srv/api/.env <<EOF
DATABASE_URL=postgres://…
PORT=8080
EOF
unitpm start "uv run app.py" --cwd /srv/api --env-file /srv/api/.env
```

In `--isolation dynamic` the env-file is delivered via systemd
`LoadCredential=` — secrets never appear in `/proc/<pid>/environ`.

## Isolation mode quick picker

| Goal | Mode | Works in |
|------|------|----------|
| Default, fast, lean | `--isolation self` (default) | user + system |
| Strongest isolation with DynamicUser | `--isolation dynamic` | **system only** |
| Unprivileged sandbox (landlock + user ns) | `--isolation sandbox` | user + system |

See `SECURITY.md` for the threat-model behind each mode.

## Startup

Make unitpm and your apps survive reboots:

```bash
sudo systemctl enable --now unitpmd   # system mode
# or
unitpm startup                              # user mode — wires the user systemd unit
```

When `unitpmd` starts it calls `manager.Restore()` which re-reads the specs in
`~/.config/unitpm/apps` and re-spawns any app that was running before.

## What `unitpm install-tools` does

Scans for common dev tools (bun, node, npm, pnpm, yarn, go, python3, pip,
ruby, cargo, java, deno) and symlinks them into `~/.local/bin` (default) or
`/usr/local/bin` (with `--system`). This is a convenience for getting the
daemon's `PATH` lookups to succeed when your interpreter lives under
`~/.bun/bin` or an fnm shim directory.
