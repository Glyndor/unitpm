# Lynx

Lynx is a cross-platform process manager written in Go. It allows you to manage and supervise processes on Windows and Linux through a lightweight daemon and a command-line interface.

## Components

*   **lynxd**: The background daemon that supervises processes. It must be running for the CLI to work.
*   **lynx**: The command-line client used to interact with the daemon (start, stop, and list processes).

## Quick Start

1.  **Start the daemon**
    Open a terminal and run the daemon. It will listen for IPC connections.
    ```bash
    ./lynxd
    ```

2.  **Manage processes**
    In a separate terminal, use the `lynx` command to interact with the daemon.
    ```bash
    # Check status
    ./lynx list
    ```

## CLI Reference

> **Note**: The CLI reference is copied from `--help` output to stay accurate.

### Main Help

```text
$ lynx --help
(Placeholder: Output of lynx --help)
```

### Start a Process

Starts a new managed process.

```text
$ lynx start --help
(Placeholder: Output of lynx start --help)
```

### Stop a Process

Stops a running process by its ID.

```text
$ lynx stop --help
(Placeholder: Output of lynx stop --help)
```

### List Processes

Displays a table of all managed processes and their current state.

```text
$ lynx list --help
(Placeholder: Output of lynx list --help)
```

## Examples

### Start a new process
```bash
lynx start --name "my-app" --command "node server.js"
```

### List all processes
```bash
lynx list
```
*Output:*
```text
ID  NAME     PID    STATUS   UPTIME
0   my-app   1234   running  5s
```

### Stop a process
```bash
lynx stop --id 0
```

## Development

### Build

To build the daemon and client binaries:

```bash
go build ./cmd/lynxd
go build ./cmd/lynx
```

### Lint

```bash
golangci-lint run
```
