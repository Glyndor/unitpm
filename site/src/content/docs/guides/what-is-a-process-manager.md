---
title: What is a Linux process manager?
description: A Linux process manager supervises long-running applications — restarting them on crash, capturing logs, and enforcing resource limits. Learn how they work and which one to choose.
---

A **Linux process manager** is a tool that keeps long-running applications alive. You hand it a command — `node server.js`, `python worker.py`, a compiled binary — and it becomes responsible for starting the process, restarting it if it crashes, capturing its output, and optionally enforcing memory and CPU limits.

Without a process manager, your application stops the moment the shell session ends, or stays dead after a crash with nobody to revive it.

## What problems does a process manager solve?

### 1. Crash recovery

Applications crash. A bug, an out-of-memory event, a transient dependency failure — the process exits with a non-zero code. A process manager detects the exit and restarts the process according to a policy you configure: always restart, restart only on failure, or never restart.

Without a process manager you need a custom shell loop, a systemd unit file authored from scratch, or manual intervention.

### 2. Startup on boot

When a server reboots, your application does not restart automatically unless something is responsible for starting it. A process manager integrates with the system init (systemd) to launch the daemon on boot, which then restores all registered processes.

### 3. Log management

A process manager captures `stdout` and `stderr` from each process and writes them to files (or the systemd journal). It typically handles log rotation — capping file size, keeping N backups — so your disk does not fill up.

### 4. Resource limits

Process managers can enforce memory caps (`--memory-max 512M`) and CPU quotas so a runaway process does not take down the entire server.

### 5. Bulk operations

On a server running multiple services, a process manager lets you stop, restart, or reload a group of processes with a single command rather than hunting down each PID.

## How a process manager works

At its core, a process manager is a daemon that:

1. **Maintains a registry** of processes it should supervise — command, restart policy, namespace, resource limits
2. **Forks the processes** (directly or via the OS init system)
3. **Monitors exit events** and restarts based on policy
4. **Exposes a control interface** — usually a CLI or socket — so you can query status, read logs, and issue commands

The key architectural choice is: **who actually holds the processes?**

- **Self-supervising model** (PM2, Supervisor): the process manager daemon is the direct parent. If the daemon dies, the children die with it.
- **Systemd-delegating model** (unitpm): the daemon registers processes as systemd transient units. The OS init system holds them. The process manager daemon is just a control plane — kill it and the apps keep running.

## Linux process manager comparison

| | unitpm | PM2 | Supervisor |
|--|------|-----|-----------|
| Runtime | Go binary (no deps) | Node.js | Python |
| Cold start | 7.8 ms | 366 ms | 252 ms |
| Idle RSS | 14.7 MB | 66.7 MB | 27.1 MB |
| Supervision | systemd (kernel) | Custom daemon | Custom daemon |
| Apps survive daemon restart | ✓ | ✗ | ✗ |
| Sandboxing | DynamicUser + landlock | None | None |
| Linux only | ✓ | ✗ | ✗ |

## Do I need a process manager or plain systemd?

Plain systemd unit files are the right tool for permanent, infrastructure-level services that change infrequently and belong in version-controlled `/etc/systemd/system/`. If you are a sysadmin managing a handful of stable services, writing unit files by hand is the correct answer.

A process manager is better when:

- You deploy **application-level services** that change often
- You want a **CLI** instead of editing files and running `systemctl daemon-reload`
- You run **many processes** and want namespace-level bulk operations
- You need **declarative YAML** you can commit alongside your code
- You want the ergonomics of `pm2 start` but without the Node.js runtime overhead

## Getting started with unitpm

```bash
# Install
sudo apt install ./unitpm_*_amd64.deb

# Start a process
unitpm start "node server.js" --name api --restart always

# List all processes
unitpm list

# Auto-start on boot
sudo unitpm startup
```

## See also

- [Install unitpm](../start/install/)
- [Quickstart](../start/quickstart/)
- [unitpm vs PM2](./vs-pm2/) — detailed benchmark comparison
- [unitpm vs Supervisor](./vs-supervisor/) — detailed benchmark comparison
- [PM2 vs Supervisor vs unitpm](./pm2-vs-supervisor-vs-unitpm/) — three-way comparison
- [systemd-native process manager](./systemd-process-manager/) — why systemd delegation matters
