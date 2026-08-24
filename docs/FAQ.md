# 🦁 FAQ — Can / Can't

Direct answers, no detours. Grouped by topic.

---

## 📇 Naming & organization

| Can I…? | Yes/No | Example / Note |
|---------|--------|----------------|
| Spaces in `--name` | ✅ | `--name "my api"` |
| Colon `:` in `--name` | ✅ | `--name "TEST: Release 1"` — address with `ns:name` |
| Symbols `# @ ! , ( ) + = &` in `--name` | ✅ | `--name "api (v2) #blue"` |
| Accents / emoji in `--name` | ❌ | ASCII only. Use `app-espanol` not `app-español` |
| `;` `"` `$` backtick `|` `<>` in `--name` | ❌ | shell-dangerous, rejected with `ERR_BAD_REQUEST` |
| Name > 128 chars | ❌ | 128 limit |
| Spaces in `--namespace` | ❌ | strict `[a-zA-Z0-9._-]`, 64 chars |
| Two processes with same `ns:name` | ❌ | `ERR_CONFLICT` |
| Omit `--name` | ✅ | auto: `<basename>-<shortid>` |
| Rename a live process | ❌ | delete+recreate with new name |

---

## 🎬 Lifecycle

| Can I…? | Yes/No | Example |
|---------|--------|---------|
| Start and forget | ✅ | `unitpm start app.js --restart always` |
| Stop all in a namespace | ✅ | `unitpm stop --namespace prod` or `unitpm stop 'prod:*'` |
| Stop / restart / delete every managed process | ✅ | `unitpm stop '*'` (quote the glob) |
| Restart several at once | ✅ | `unitpm restart a b c` |
| Reload spec without stopping process | ❌ | `unitpm reload` does stop+start; no hot-reload of spec |
| Send custom signal | ❌ | only `--stop-signal` for stop; use `kill -USR1 $(pidof app)` |
| Scale without restarting | ✅ | `unitpm scale app 5` (respects running instances) |
| Scale down to 0 | ✅ | `unitpm scale app 0` = equivalent delete all |
| Reset `Restarts` counter | ✅ | `unitpm reset app` |

---

## 🔁 Restart & resilience

| Can I…? | Yes/No | Example |
|---------|--------|---------|
| Infinite restart | ✅ | `--restart always --max-restarts 0` (0 = no limit via env) |
| Exponential backoff | ✅ | `--backoff expo` (default) |
| Restart only on crash | ✅ | `--restart on-failure` (default) |
| Never restart | ✅ | `--restart never` |
| Stop on exit code X | ✅ | `--stop-on-exit 0,143,15` |
| Custom stop timeout | ✅ | `--stop-timeout 30000` (30s) |
| HTTP health check probe | ❌ | removed due to SSRF — use sidecar: `unitpm start "curl -sSf http://localhost/h \|\| exit 1" --cron '@every 10s' --shell` |
| Unix-style cron | ✅ | `--cron "0 */6 * * *"` |
| Interval cron | ✅ | `--cron "@every 5s"` (min 5s) |

---

## 🌱 Environment variables

| Can I…? | Yes/No | Alternative |
|---------|--------|-------------|
| `--env KEY=VAL` inline | ❌ | **does not exist**. Use `--env-file` |
| Pass `.env` file | ✅ | `--env-file .env.production` |
| Relative paths in `--env-file` | ✅ | relative to `--cwd` |
| `..` in `--env-file` | ❌ | rejected `ERR_BAD_REQUEST` |
| View env of a live process | ⚠️ | `unitpm show <name>` shows spec; real env in `/proc/<pid>/environ` |
| Secrets without leaking in `ps` | ✅ | `--isolation dynamic` uses `LoadCredential` (systemd) |

---

## 💾 Resources & limits

| Can I…? | Yes/No | Example |
|---------|--------|---------|
| Cap memory | ✅ | `--memory-max 512M` (accepts `k`/`m`/`M`/`G` or bytes) |
| Cap CPU % | ✅ | `--cpu-max 100` (100=1 core, 200=2 cores) |
| Cap number of threads/procs | ✅ | `--tasks-max 64` |
| Cap file descriptors | ⚠️ | indirect — runtime default RLIMIT_NOFILE |
| Cap disk I/O | ❌ | not exposed (systemd `IOWeight` not wired) |
| Memory < 1 MiB | ❌ | minimum floor |

---

## 🔒 Isolation & security

| Can I…? | Yes/No | Mode |
|---------|--------|------|
| Run without extra isolation | ✅ | `--isolation self` (default) |
| Synthetic per-process user | ✅ | `--isolation dynamic` (system mode only, systemd) |
| Sandbox without sudo | ✅ | `--isolation sandbox` (user+PID namespace + landlock) |
| Block writes to `/home`, `/etc` | ✅ | `--isolation sandbox` (landlock allowlist) |
| `--cwd` to `/etc` | ❌ | blocked: `/etc /proc /sys /boot /dev /run` |
| Path traversal `../../etc` | ❌ | canonicalized + rejected |
| `--shell` in system mode | ❌ | blocked (hardening); user mode yes |
| View socket perms | `srw-rw---- glyndor-unitpm:unitpm` (system) / `0600` (user) | |

---

## 📊 Logs & debugging

| Can I…? | Yes/No | Example |
|---------|--------|---------|
| Follow logs | ✅ | `unitpm logs api --follow` |
| stdout only | ✅ | `unitpm logs api --stdout` |
| stderr only | ✅ | `unitpm logs api --stderr` |
| Last N lines | ✅ | `unitpm logs api --lines 50` |
| JSON-formatted logs | ✅ | `--log-format json` at `start` |
| Automatic rotation | ✅ | 50 MiB default, 3 backups (tunable env) |
| Truncate logs | ✅ | `unitpm flush api` |
| Redirect to custom dir | ✅ | `--log-dir /var/log/my-app` |
| Redirect stdout to stderr | ❌ | both go to separate files |

---

## 🏗️ Declarative (unitpm.yml)

| Can I…? | Yes/No | Note |
|---------|--------|------|
| Multiple apps in one YAML | ✅ | all in the file's namespace |
| Apply incrementally | ⚠️ | `apply` always creates new; must `delete` before re-applying |
| Export running state → YAML | ✅ | `unitpm export --namespace prod > apps.yml` |
| Dependencies between apps | ❌ | not implemented; starts independently |
| Per-app env-file | ✅ | `env_file: .env` in each entry |
| Lint before apply | ❌ | not exposed (though `apply` validates) |

---

## 🔌 IPC & CLI

| Can I…? | Yes/No | Example |
|---------|--------|---------|
| Preview without executing | ✅ | `--dry-run` / `-n` |
| Silence output | ✅ | `--quiet` / `-q` |
| Parseable JSON output | ✅ | `unitpm list --json` / `unitpm version --json` |
| Shell completion | ✅ | `unitpm completion bash\|zsh\|fish` |
| Namespace:name syntax | ✅ | `unitpm show prod:api` |
| Resolve by ID prefix | ✅ | `unitpm show 019d9` (if unique) |
| Multiple lifecycle commands in 1 cmd | ✅ | `unitpm stop a b c d` |
| Bulk by namespace (stop/restart/reload/reset/delete/flush) | ✅ | `unitpm restart --namespace prod` or `unitpm restart 'prod:*'` |
| HTTP API | ❌ | Unix socket IPC only |
| Remote daemon via TCP | ❌ | socket is local-only by design |

---

## 🌐 Runtimes

| Can I run…? | Yes/No |
|-------------|--------|
| Node / Bun / Deno | ✅ |
| System Python / venv / uv / uvx | ✅ |
| Go source / binary | ✅ |
| Rust / C / C++ / Nim / OCaml / Haskell | ✅ |
| Ruby / Perl / PHP / Lua / R / Tcl | ✅ |
| Java / JVM (Kotlin, Scala) | ✅ |
| Erlang / Elixir | ✅ |
| Bash scripts | ✅ |
| Docker container | ⚠️ | yes via `docker run`, but unitpm sandbox redundant |
| Windows .exe | ❌ | Linux-only |
| GUI apps (X11/Wayland) | ⚠️ | technically yes, but sandbox blocks access |

See [`RUNTIMES.md`](RUNTIMES.md) for per-runtime recipes.

---

## ⚙️ Persistence & startup

| Can I…? | Yes/No | How |
|---------|--------|-----|
| Auto-start on boot | ✅ | `sudo unitpm startup` (systemd) |
| Restore specs after reboot | ✅ | automatic on daemon start |
| Backup state | ✅ | copy `~/.config/unitpm/apps/*.json` |
| Migrate between hosts | ✅ | `unitpm export` → copy YAML → `unitpm apply` |
| Kill daemon without killing apps | ✅ (dynamic) / ❌ (self) | in `dynamic` apps survive (systemd-managed); in `self` they die |

---

## ❌ Explicitly **not** supported (by design)

| Feature | Alternative |
|---------|-------------|
| HTTP health check (`--health-url`) | Sidecar cron with `curl` |
| `unitpm attach` / interactive stdin | no `docker exec`-style |
| Prometheus metrics endpoint | Parse `unitpm list --json` from your scraper |
| Watch file mode (`--watch`) | Use nodemon/cargo-watch as sidecar |
| Deploy via SSH | Use Ansible / Terraform / rsync + `unitpm apply` |
| Modules/plugins | No plugin system |
| Hot-reload live spec | Do `delete` + `apply` |
| Mac / Windows | Linux-only (kernel features required) |

---

## 🆘 "I got a weird error…"

| Error | What it means | Fix |
|-------|---------------|-----|
| `cannot reach the unitpm daemon` | daemon off | `unitpmd &` (user) or `sudo systemctl start unitpmd` (system) |
| `ERR_RATE_LIMIT` | exceeded 100 req/s | Wait. Drop to normal burst. |
| `ERR_CONFLICT: ... already exists` | duplicate `ns:name` | different name or namespace |
| `invalid name format` | name with forbidden char | only `a-zA-Z0-9 ._-:#@!,()+=&` |
| `cwd is a restricted system directory` | `--cwd /etc` etc | use `/srv`, `/var/lib/unitpm`, `/tmp` |
| `cwd is not accessible to the daemon user` | user mismatch system mode | `--cwd /srv/something` that `glyndor-unitpm` user can read |
| `ERR_UNSUPPORTED: run_as=dynamic requires system daemon` | dynamic in user mode | use `sandbox` or run daemon in system-mode |
| `fork/exec: executable not found` | binary not in daemon PATH | `unitpm install-tools` |
| `ambiguous argument 'X'` | multiple matches | use full `ns:name` or ID |
