---
title: Lightweight process manager for Linux
description: unitpm is a lightweight Linux process manager — 7.2 MB binary, 14.7 MB idle RSS, 7.8 ms cold start. No Node.js or Python runtime required. Benchmarks vs PM2 and Supervisor.
---

When you choose a process manager, you're choosing what runs permanently on every server in your fleet. A process manager that consumes 60-70 MB idle — before your apps even start — is not a neutral choice. It affects container sizing, VM memory allocation, cold-start time in CI, and the blast radius of OOM events.

unitpm is a lightweight process manager for Linux. This page covers the benchmarks, explains where the weight difference comes from, and describes the scenarios where it matters most.

## Benchmark numbers

From [CI bench](https://github.com/Jaro-c/unitpm/actions/workflows/bench.yml) — Ubuntu 24.04, kernel 6.17, idle daemon supervising 10 noop processes:

| Metric | unitpm | PM2 | Supervisor |
|--------|------|-----|-----------|
| Cold start | **7.8 ms** | 366 ms | 252 ms |
| Idle RSS | **14.7 MB** | 66.7 MB | 27.1 MB |
| RSS with 10 processes | **22.8 MB** | 69.3 MB | 27.3 MB |
| Binary size | **7.2 MB** | Node.js + deps | Python + libs |

unitpm starts **47× faster than PM2** and **32× faster than Supervisor**. At idle it uses **4.5× less memory than PM2** and **1.8× less than Supervisor**.

## Why the weight difference

### PM2

PM2 is a Node.js application. To run PM2, you need a full Node.js runtime on the host — V8 engine, libuv event loop, and the entire npm dependency tree for PM2 itself. The daemon keeps V8 warm in memory. That's where the 66 MB idle baseline comes from.

Cold start is slow because Node.js needs to parse and JIT-compile the PM2 source before it can do anything. 366 ms is the V8 startup cost.

### Supervisor

Supervisor is a Python application. Python is lighter than Node.js but still requires the Python interpreter, its standard library, and Supervisor's own dependencies. The 27 MB idle footprint is the Python runtime plus Supervisor's in-memory process table.

Cold start at 252 ms reflects CPython startup and module import time.

### unitpm

unitpm is a compiled Go binary. There is no interpreter, no VM, no JIT. The binary is statically linked (`CGO_ENABLED=0`): copy it to any Linux host and it runs — no runtime installation required.

```bash
# Single binary, no dependencies
ls -lh unitpm_linux_amd64
# -rwxr-xr-x  1 user  group  7.2M unitpm_linux_amd64
```

The daemon's 14.7 MB idle RSS includes the Go runtime overhead plus unitpm's own process table, IPC server, and log rotation goroutines. There is nothing to trim further without removing features.

## Where lightweight matters most

### Containers and microVMs

Every megabyte of the process manager's footprint reduces the budget available to your application. In a 128 MB container, PM2 at 66 MB idle leaves 62 MB for your app. unitpm at 14.7 MB idle leaves 113 MB — nearly double.

```
Container memory: 128 MB
PM2 daemon:       - 66 MB
App budget:       = 62 MB

Container memory: 128 MB
unitpm daemon:      - 15 MB
App budget:       = 113 MB
```

### CI/CD pipelines

Process managers are sometimes used in CI to run background services (database, mock APIs, etc.) during test runs. At 366 ms, PM2 adds meaningful latency to every CI job that starts a background service. unitpm at 7.8 ms is effectively instantaneous.

### Low-memory VMs and edge nodes

$4-6/month VPS instances typically ship with 512 MB or 1 GB RAM. On a 512 MB VM running a Node.js app:

| Setup | Daemon RSS | Available for apps |
|-------|-----------|-------------------|
| PM2 + Node app | 66 + ~80 MB | ~366 MB |
| unitpm + Node app | 15 + ~80 MB | ~417 MB |

The delta is small in absolute terms but meaningful when you're trying to avoid OOM kills.

### Fleet-wide memory savings

If you run 50 servers each with a PM2 daemon, you're paying for 50 × 66 MB = 3.3 GB of RAM just for process managers. The same fleet with unitpm: 50 × 14.7 MB = 735 MB. The difference funds real application instances.

## No runtime installation required

PM2 requires Node.js on every managed host. Supervisor requires Python. If your app is a compiled binary (Go, Rust, C++), you still need to install a language runtime just to run the process manager.

unitpm has no runtime dependencies. The `.deb` package or static binary is self-contained:

```bash
# Debian/Ubuntu
sudo apt install ./unitpm_*_amd64.deb

# Any Linux (amd64)
install -m 0755 unitpm_linux_amd64 ~/.local/bin/unitpm

# Any Linux (arm64)
install -m 0755 unitpm_linux_arm64 ~/.local/bin/unitpm
```

This matters for:
- Minimal container base images (`FROM scratch`, `FROM alpine`)
- Security-hardened hosts where package installation is restricted
- Hosts where Node.js or Python versions conflict with your app's requirements
- Air-gapped environments where pulling npm packages is not possible

## Memory does not grow with more processes

One of the more surprising results from the benchmark: RSS with 10 supervised processes is only 22.8 MB — 8 MB over the idle baseline.

This is because unitpm delegates actual process supervision to systemd transient units. The apps run under systemd, not under `unitpmd`. unitpm's daemon memory is nearly constant regardless of how many processes you supervise — it holds metadata, not the process tree.

PM2 at 69.3 MB with 10 processes shows a similar pattern (it's mostly V8 overhead, not per-process cost), but starts from a much higher baseline.

## Getting started

```bash
# Install
sudo apt install ./unitpm_*_amd64.deb

# Start a process
unitpm start "node server.js" --name api --restart always

# Check memory usage of the daemon itself
ps -o pid,rss,comm -p $(pgrep unitpmd)
```

## See also

- [Install unitpm](../start/install/)
- [unitpm vs PM2](./vs-pm2/) — full feature and benchmark comparison
- [unitpm vs Supervisor](./vs-supervisor/) — full feature and benchmark comparison
- [systemd-native process manager](./systemd-process-manager/) — why systemd supervision matters
