# 🦁 FAQ — Puedo / No puedo

Respuestas directas, sin vueltas. Agrupado por tema.

---

## 📇 Naming & organización

| ¿Puedo…? | Sí/No | Ejemplo / Nota |
|----------|-------|----------------|
| Espacios en `--name` | ✅ | `--name "my api"` |
| Colon `:` en `--name` | ✅ | `--name "TEST: Release 1"` — address con `ns:name` |
| Símbolos `# @ ! , ( ) + = &` en `--name` | ✅ | `--name "api (v2) #blue"` |
| Acentos / emoji en `--name` | ❌ | solo ASCII. Usa `lynx-espanol` no `lynx-español` |
| `;` `"` `$` backtick `|` `<>` en `--name` | ❌ | shell-peligrosos, rechazados con `ERR_BAD_REQUEST` |
| Nombre > 128 chars | ❌ | límite 128 |
| Espacios en `--namespace` | ❌ | strict `[a-zA-Z0-9._-]`, 64 chars |
| Dos procesos con mismo `ns:name` | ❌ | `ERR_CONFLICT` |
| Sin dar `--name` | ✅ | auto: `<basename>-<shortid>` |
| Renombrar un proceso vivo | ❌ | borra+recrea con nuevo nombre |

---

## 🎬 Ciclo de vida

| ¿Puedo…? | Sí/No | Ejemplo |
|----------|-------|---------|
| Arrancar y olvidarme | ✅ | `lynx start app.js --restart always` |
| Detener todos de un namespace | ⚠️ | no hay `stop --all`; usa: `lynx list --json \| jq -r '.[] \| .name' \| xargs lynx stop` |
| Reiniciar varios a la vez | ✅ | `lynx restart a b c` |
| Recargar spec sin tumbar proceso | ❌ | `lynx reload` hace stop+start; no hay hot-reload de spec |
| Enviar señal custom | ❌ | solo `--stop-signal` para stop; usa `kill -USR1 $(pidof app)` |
| Escalar sin reiniciar | ✅ | `lynx scale app 5` (respeta running instances) |
| Bajar escala a 0 | ✅ | `lynx scale app 0` = equivalent delete all |
| Resetear contador `Restarts` | ✅ | `lynx reset app` |

---

## 🔁 Reinicio & resiliencia

| ¿Puedo…? | Sí/No | Ejemplo |
|----------|-------|---------|
| Reinicio infinito | ✅ | `--restart always --max-restarts 0` (0 = sin límite vía env) |
| Reinicio exponencial | ✅ | `--backoff expo` (default) |
| Reiniciar solo si crash | ✅ | `--restart on-failure` (default) |
| No reiniciar jamás | ✅ | `--restart never` |
| Parar si exit code X | ✅ | `--stop-on-exit 0,143,15` |
| Timeout personalizado al stop | ✅ | `--stop-timeout 30000` (30s) |
| Health check HTTP probe | ❌ | eliminado por SSRF — usa sidecar: `lynx start "curl -sSf http://localhost/h \|\| exit 1" --cron '@every 10s' --shell` |
| Cron estilo unix | ✅ | `--cron "0 */6 * * *"` |
| Cron intervalo | ✅ | `--cron "@every 5s"` (mín 5s) |

---

## 🌱 Variables de entorno

| ¿Puedo…? | Sí/No | Alternativa |
|----------|-------|-------------|
| `--env KEY=VAL` inline | ❌ | **no existe**. Usa `--env-file` |
| Pasar `.env` file | ✅ | `--env-file .env.production` |
| Rutas relativas en `--env-file` | ✅ | relativas al `--cwd` |
| `..` en `--env-file` | ❌ | rechazado `ERR_BAD_REQUEST` |
| Ver env de un proceso vivo | ⚠️ | `lynx show <name>` muestra spec; env real en `/proc/<pid>/environ` |
| Secrets sin leak en `ps` | ✅ | `--isolation dynamic` usa `LoadCredential` (systemd) |

---

## 💾 Recursos & límites

| ¿Puedo…? | Sí/No | Ejemplo |
|----------|-------|---------|
| Cap memoria | ✅ | `--memory-max 512M` (acepta `k`/`m`/`M`/`G` o bytes) |
| Cap CPU % | ✅ | `--cpu-max 100` (100=1 core, 200=2 cores) |
| Cap nro de threads/procs | ✅ | `--tasks-max 64` |
| Cap file descriptors | ⚠️ | indirecto — runtime default RLIMIT_NOFILE |
| Cap disk I/O | ❌ | no expuesto (systemd `IOWeight` no wired) |
| Memoria < 1 MiB | ❌ | floor mínimo |

---

## 🔒 Isolation & seguridad

| ¿Puedo…? | Sí/No | Modo |
|----------|-------|------|
| Correr sin isolation extra | ✅ | `--isolation self` (default) |
| User sintético per-proceso | ✅ | `--isolation dynamic` (solo system mode, systemd) |
| Sandbox sin sudo | ✅ | `--isolation sandbox` (user+PID namespace + landlock) |
| Bloquear escrituras a `/home`, `/etc` | ✅ | `--isolation sandbox` (landlock allowlist) |
| `--cwd` a `/etc` | ❌ | bloqueado: `/etc /proc /sys /boot /dev /run` |
| Path traversal `../../etc` | ❌ | canonicalized + rechazado |
| `--shell` en system mode | ❌ | bloqueado (hardening); user mode sí |
| Ver socket perms | `srw-rw---- lynx:lynxadm` (system) / `0600` (user) | |

---

## 📊 Logs & debugging

| ¿Puedo…? | Sí/No | Ejemplo |
|----------|-------|---------|
| Follow logs | ✅ | `lynx logs api --follow` |
| Solo stdout | ✅ | `lynx logs api --stdout` |
| Solo stderr | ✅ | `lynx logs api --stderr` |
| Últimas N líneas | ✅ | `lynx logs api --lines 50` |
| Logs en formato JSON | ✅ | `--log-format json` al `start` |
| Rotación automática | ✅ | 50 MiB default, 3 backups (tunable env) |
| Truncar logs | ✅ | `lynx flush api` |
| Redirigir a custom dir | ✅ | `--log-dir /var/log/my-app` |
| Redirigir stdout a stderr | ❌ | ambos van a files separados |

---

## 🏗️ Declarative (Lynxfile.yml)

| ¿Puedo…? | Sí/No | Nota |
|----------|-------|------|
| Múltiples apps en un YAML | ✅ | todas en el namespace del file |
| Aplicar incrementalmente | ⚠️ | `apply` siempre crea nuevos; hay que `delete` antes para re-aplicar |
| Export state running → YAML | ✅ | `lynx export --namespace prod > apps.yml` |
| Dependencias entre apps | ❌ | no implementado; arranca independientes |
| Env-file por app | ✅ | `env_file: .env` en cada entry |
| Lint antes de apply | ❌ | no expuesto (aunque `apply` valida) |

---

## 🔌 IPC & CLI

| ¿Puedo…? | Sí/No | Ejemplo |
|----------|-------|---------|
| Preview sin ejecutar | ✅ | `--dry-run` / `-n` |
| Silenciar output | ✅ | `--quiet` / `-q` |
| Salida JSON parseable | ✅ | `lynx list --json` / `lynx version --json` |
| Shell completion | ✅ | `lynx completion bash\|zsh\|fish` |
| Namespace:name syntax | ✅ | `lynx show prod:api` |
| Resolver por prefix de ID | ✅ | `lynx show 019d9` (si único) |
| Multiple lifecycle commands con 1 cmd | ✅ | `lynx stop a b c d` |
| HTTP API | ❌ | solo Unix socket IPC |
| Remote daemon vía TCP | ❌ | socket es local-only por diseño |

---

## 🌐 Runtimes

| ¿Puedo correr…? | Sí/No |
|-----------------|-------|
| Node / Bun / Deno | ✅ |
| Python sistema / venv / uv / uvx | ✅ |
| Go source / binary | ✅ |
| Rust / C / C++ / Nim / OCaml / Haskell | ✅ |
| Ruby / Perl / PHP / Lua / R / Tcl | ✅ |
| Java / JVM (Kotlin, Scala) | ✅ |
| Erlang / Elixir | ✅ |
| Bash scripts | ✅ |
| Docker container | ⚠️ | sí via `docker run`, pero sandbox de lynx redundante |
| Windows .exe | ❌ | Linux-only |
| Apps GUI (X11/Wayland) | ⚠️ | técnicamente sí, pero sandbox bloquea acceso |

Ver [`RUNTIMES.md`](RUNTIMES.md) para recipes por runtime.

---

## ⚙️ Persistencia & arranque

| ¿Puedo…? | Sí/No | Cómo |
|----------|-------|------|
| Auto-arrancar en boot | ✅ | `sudo lynx startup` (systemd) |
| Restore de specs tras reboot | ✅ | automático al iniciar daemon |
| Backup del state | ✅ | copiar `~/.config/lynx/apps/*.json` |
| Migrar entre hosts | ✅ | `lynx export` → copia YAML → `lynx apply` |
| Matar daemon sin matar apps | ✅ (dynamic) / ❌ (self) | en `dynamic` apps sobreviven (systemd-managed); en `self` mueren |

---

## ❌ Explícitamente **no** soportado (por diseño)

| Feature | Alternativa |
|---------|-------------|
| HTTP health check (`--health-url`) | Sidecar cron con `curl` |
| `lynx attach` / stdin interactivo | `docker exec`-style no hay |
| Prometheus metrics endpoint | Parse `lynx list --json` desde tu scraper |
| Watch file mode (`--watch`) | Usa nodemon/cargo-watch como sidecar |
| Deploy vía SSH | Usa Ansible / Terraform / rsync + `lynx apply` |
| Módulos/plugins | No plugin system |
| Hot-reload spec vivo | Hacer `delete` + `apply` |
| Mac / Windows | Linux-only (kernel features requeridos) |

---

## 🆘 "Me salió un error raro…"

| Error | Qué significa | Fix |
|-------|---------------|-----|
| `cannot reach the Lynx daemon` | daemon off | `lynxd &` (user) o `sudo systemctl start lynx.lynxd` (system) |
| `ERR_RATE_LIMIT` | pasaste 100 req/s | Espera. Bajás a burst normal. |
| `ERR_CONFLICT: ... already exists` | duplicate `ns:name` | distinto nombre o namespace |
| `invalid name format` | nombre con char prohibido | sólo `a-zA-Z0-9 ._-:#@!,()+=&` |
| `cwd is a restricted system directory` | `--cwd /etc` etc | usa `/srv`, `/var/lib/lynx-pm`, `/tmp` |
| `cwd is not accessible to the daemon user` | user mismatch system mode | `--cwd /srv/algo` que `lynx` user pueda leer |
| `ERR_UNSUPPORTED: run_as=dynamic requires system daemon` | dynamic en user mode | usa `sandbox` o corre daemon system-mode |
| `fork/exec: executable not found` | binary no en PATH daemon | `lynx install-tools` |
| `ambiguous argument 'X'` | multiple matches | usa `ns:name` completo o ID |
