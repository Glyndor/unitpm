---
title: Linux cron job management with a process manager
description: Manage scheduled cron jobs on Linux with unitpm process manager. Replace fragile cron with supervised, restartable scheduled tasks that have logging, failure alerts, and declarative config.
---

Traditional `cron` runs scheduled jobs without supervision — if a job fails silently, you won't know until you notice the side effect. **Linux cron job management** with a process manager adds logging, restart-on-failure, and unified visibility across all your scheduled tasks.

## The problem with plain cron

```
# crontab -e
0 3 * * * /srv/scripts/backup.sh
```

Plain cron:
- Runs the job with no supervision (fails silently unless you set `MAILTO`)
- Output goes to system mail by default (often unread)
- No restart on failure
- Not visible in `ps` until it runs
- No resource limits
- Cannot be managed alongside your long-running services

## unitpm cron scheduling

unitpm supports cron-syntax scheduling for one-shot and recurring jobs:

```bash
unitpm start "node /srv/scripts/backup.js" \
  --name backup \
  --cron "0 3 * * *" \
  --restart on-failure \
  --cwd /srv/scripts \
  --env-file /srv/scripts/.env
```

This registers the job with systemd as a transient timer. The job:
- Runs at 03:00 daily
- Automatically restarts if it exits non-zero
- Logs stdout/stderr via unitpm log management
- Appears in `unitpm list` with last run status

### Cron syntax

Standard 5-field cron expression:

```
┌──── minute (0-59)
│ ┌─── hour (0-23)
│ │ ┌── day of month (1-31)
│ │ │ ┌─ month (1-12)
│ │ │ │ ┌ day of week (0-7, 0=Sun)
│ │ │ │ │
* * * * *
```

Examples:

| Cron | Meaning |
|------|---------|
| `0 3 * * *` | Daily at 03:00 |
| `*/5 * * * *` | Every 5 minutes |
| `0 */6 * * *` | Every 6 hours |
| `0 9 * * 1` | Monday at 09:00 |
| `0 0 1 * *` | First of the month at midnight |

### View scheduled jobs

```bash
unitpm list
# ┌──────────┬────────┬──────────┬──────────────────┬──────────────┐
# │ id       │ name   │ type     │ schedule         │ last run     │
# ├──────────┼────────┼──────────┼──────────────────┼──────────────┤
# │ ▸ 019dbd │ backup │ cron     │ 0 3 * * *        │ 2h ago       │
# │ ▸ 019dbe │ report │ cron     │ 0 9 * * 1        │ 2 days ago   │
# └──────────┴────────┴──────────┴──────────────────┴──────────────┘
```

### Run a job manually

```bash
# Trigger immediately regardless of schedule
unitpm run backup
```

Useful for testing or running a job on demand without changing the schedule.

### View job output

```bash
# Last run output
unitpm logs backup --lines 100

# Follow the next scheduled run
unitpm logs backup --follow
```

## Common cron job patterns

### Database backup

```bash
unitpm start "pg_dump -Fc mydb > /backup/mydb-$(date +%Y%m%d).dump" \
  --name db-backup \
  --cron "0 2 * * *" \
  --restart on-failure \
  --env-file /srv/.env.production
```

### Log rotation and cleanup

```bash
unitpm start "find /var/log/app -name '*.log' -mtime +30 -delete" \
  --name log-cleanup \
  --cron "0 4 * * 0" \
  --restart never
```

### Sending a weekly report

```bash
unitpm start "node /srv/reports/weekly.js" \
  --name weekly-report \
  --cron "0 9 * * 1" \
  --restart on-failure \
  --cwd /srv/reports \
  --env-file /srv/reports/.env
```

### Cache warming

```bash
unitpm start "python3 /srv/scripts/warm-cache.py" \
  --name cache-warm \
  --cron "*/15 * * * *" \
  --restart never \
  --env-file /srv/.env
```

`--restart never` is appropriate for cache warming — if it fails, skip this cycle and try again in 15 minutes.

## Declarative cron jobs in unitpm.yml

```yaml
# unitpm.yml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    env_file: .env.production

  db-backup:
    command: /usr/local/bin/backup.sh
    cron: "0 2 * * *"
    restart: on-failure
    env_file: .env.production

  weekly-report:
    command: node reports/weekly.js
    cwd: /srv/reports
    cron: "0 9 * * 1"
    restart: on-failure
    env_file: .env.production

  log-cleanup:
    command: find /var/log/app -name '*.log' -mtime +30 -delete
    cron: "0 4 * * 0"
    restart: never
```

Long-running services and cron jobs coexist in the same file, managed by the same CLI.

## Prevent overlapping runs

By default, if a cron job is still running when the next scheduled time arrives, a new instance starts (same behavior as system cron). To prevent overlap (run at most one instance at a time):

```bash
unitpm start "node /srv/scripts/sync.js" \
  --name data-sync \
  --cron "*/10 * * * *" \
  --no-overlap
```

With `--no-overlap`, if the job from the previous cycle is still running, unitpm skips the new run and logs the skip.

## System cron vs unitpm cron: comparison

| | System cron | unitpm cron |
|--|------------|-----------|
| Visibility | Invisible until it runs | Always in `unitpm list` |
| Logging | System mail or syslog | `unitpm logs <job>` |
| Restart on failure | No | Configurable |
| Resource limits | No | Memory + CPU caps |
| Declarative config | Per-user crontab | unitpm.yml |
| Manual trigger | `run-parts` or direct | `unitpm run <job>` |
| Overlap prevention | Needs `flock` wrapper | `--no-overlap` |

## Migrating from crontab

Export current crontab:

```bash
crontab -l
```

Convert each line to a `unitpm start --cron` command. Example:

```
# Old crontab
0 3 * * * /srv/scripts/backup.sh
*/5 * * * * /srv/scripts/healthcheck.sh
0 9 * * 1 /srv/scripts/report.py
```

```yaml
# unitpm.yml
version: 1
processes:
  backup:
    command: /srv/scripts/backup.sh
    cron: "0 3 * * *"
    restart: on-failure

  healthcheck:
    command: /srv/scripts/healthcheck.sh
    cron: "*/5 * * * *"
    restart: never

  weekly-report:
    command: python3 /srv/scripts/report.py
    cron: "0 9 * * 1"
    restart: on-failure
```

```bash
unitpm apply unitpm.yml
# Remove old crontab entries after verifying
crontab -r
```

## See also

- [unitpm start](../reference/commands/start/) — full flag reference including `--cron`
- [unitpm run](../reference/commands/run/) — manual trigger
- [Auto-restart on crash](./auto-restart-on-crash/)
- [Monitor process memory and CPU on Linux](./monitor-process-memory-cpu-linux/)
