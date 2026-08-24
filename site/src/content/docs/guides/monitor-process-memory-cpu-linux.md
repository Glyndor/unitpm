---
title: How to monitor process memory and CPU usage on Linux
description: Monitor memory and CPU usage of Linux processes with unitpm monit dashboard, unitpm show, systemd-cgtop, and standard Linux tools. Set alerts and resource limits to prevent runaway processes.
---

A process that consumes all available memory or pins the CPU to 100% will degrade or crash other services on the same host. This guide covers how to **monitor process memory and CPU usage on Linux** using unitpm's built-in tools and standard Linux utilities.

## unitpm: built-in monitoring

### Live dashboard

```bash
unitpm monit
```

Renders a terminal dashboard with real-time CPU%, RSS memory, uptime, restart count, and PID for every managed process. Updates every second. Press `q` to exit.

```
┌─────────────────────────────────────────────────────────────┐
│  unitpm — Process Monitor              2026-05-01 14:32:01    │
├──────────┬────────┬──────────┬───────┬───────┬─────────────┤
│ id       │ name   │ status   │ cpu % │ rss   │ restarts    │
├──────────┼────────┼──────────┼───────┼───────┼─────────────┤
│ ▸ 019dbd │ api    │ running  │  2.1% │ 87 MB │ 0           │
│ ▸ 019dbe │ worker │ running  │  8.4% │ 45 MB │ 2           │
│ ▸ 019dbf │ cron   │ running  │  0.0% │ 12 MB │ 0           │
└──────────┴────────┴──────────┴───────┴───────┴─────────────┘
```

### Per-process stats

```bash
unitpm show api
```

Output:

```
Name:      api
Status:    running
PID:       2336612
Uptime:    2h 14m
Restarts:  0
CPU:       2.1%
RSS:       87.3 MB
Namespace: default
```

### JSON output for scripting

```bash
unitpm show api --json
# {
#   "name": "api",
#   "status": "running",
#   "pid": 2336612,
#   "cpu_pct": 2.1,
#   "rss_bytes": 91594752,
#   "restarts": 0
# }

# Parse with jq
unitpm list --json | jq '.[] | select(.rss_bytes > 500000000)'
```

## Set resource limits (prevent runaway processes)

### Memory limit

```bash
unitpm start "node server.js" \
  --name api \
  --restart always \
  --memory-max 512M
```

When the process exceeds 512 MB RSS, systemd sends SIGKILL. unitpm then restarts it according to the restart policy. This prevents one service from OOM-killing the entire host.

### CPU limit

```bash
unitpm start "python worker.py" \
  --name worker \
  --restart on-failure \
  --cpu-max 200
```

`--cpu-max 200` caps the process at 200% CPU (2 full cores on a multi-core system). Uses systemd's `CPUQuota` cgroup directive.

### Both limits together

```yaml
# unitpm.yml
version: 1
processes:
  api:
    command: node server.js
    cwd: /srv/api
    restart: always
    memory_max: 512M
    cpu_max: 200
```

## Standard Linux tools

### ps — point-in-time snapshot

```bash
# Show all processes sorted by memory (RSS)
ps aux --sort=-%mem | head -20

# Show specific process
ps -p 2336612 -o pid,ppid,%cpu,%mem,rss,vsz,comm
```

RSS = resident set size (physical RAM in use). VSZ = virtual memory size (includes mmap'd files, not useful for comparison).

### top / htop

```bash
# top: sort by memory (press M), CPU (press P)
top

# htop: tree view, color-coded, interactive
htop
```

In htop, press `t` for tree view — useful for seeing which parent process owns which children.

### /proc filesystem

Each process exposes live stats in `/proc/[pid]/`:

```bash
# Memory breakdown (in kB)
cat /proc/2336612/status | grep -E 'VmRSS|VmPeak|VmSwap'
# VmPeak:  102400 kB
# VmRSS:    91548 kB
# VmSwap:       0 kB

# CPU time (utime + stime in clock ticks)
cat /proc/2336612/stat | awk '{print "user:", $14, "sys:", $15}'
```

### systemd-cgtop

Monitor cgroup resource usage — works perfectly with unitpm since each managed process is a systemd transient unit:

```bash
systemd-cgtop
```

Shows CPU%, memory, and I/O per cgroup in real time. Press `m` to sort by memory, `c` for CPU.

### systemctl status

For a unitpm-managed process `api`, the underlying unit is `my-api.service`:

```bash
systemctl status my-api.service
# Shows: memory usage, CPU time, cgroup limits
```

## Watch for memory leaks

A process with a memory leak shows steadily increasing RSS over hours. Script a periodic check:

```bash
#!/bin/bash
# /usr/local/bin/mem-watch
while true; do
  rss=$(unitpm show api --json | jq .rss_bytes)
  echo "$(date +%s) $rss" >> /var/log/api-rss.log
  sleep 60
done
```

Or use `watch` for a live view:

```bash
watch -n5 'unitpm show api --json | jq .rss_bytes'
```

## Alerting on high memory or CPU

With unitpm's JSON output, integrate into any monitoring system:

```bash
# Simple bash alert
rss=$(unitpm show api --json | jq .rss_bytes)
limit=$((400 * 1024 * 1024))  # 400 MB
if [ "$rss" -gt "$limit" ]; then
  echo "ALERT: api using ${rss} bytes RSS" | mail -s "High memory" ops@example.com
fi
```

For production monitoring, use Prometheus + node_exporter or the systemd collector, which exports cgroup metrics directly:

```bash
# Node exporter exposes systemd unit metrics
# systemd_unit_process_resident_memory_bytes{name="my-api.service"}
```

## Diagnose high memory

If a process grows unexpectedly:

```bash
# 1. Check restart history
unitpm show api
# Restarts: 0 → steady growth, likely leak
# Restarts: 47 → crash loop, different problem

# 2. Check logs around the time memory spiked
unitpm logs api --lines 200

# 3. Profile (Node.js example)
# Start with --inspect and connect Chrome DevTools
unitpm start "node --inspect=0.0.0.0:9229 server.js" --name api ...

# 4. Force a controlled restart if memory is critical
unitpm restart api
```

## See also

- [unitpm monit](../reference/commands/monit/) — live dashboard reference
- [unitpm show](../reference/commands/show/) — per-process stats
- [Auto-restart on crash](./auto-restart-on-crash/)
- [systemd DynamicUser sandboxing](./systemd-dynamicuser/)
- [How to run a Node.js app as a Linux service](./nodejs-linux-service/)
