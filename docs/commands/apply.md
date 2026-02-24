# apply

## Synopsis

```bash
lynx apply <Lynxfile.yml>
```

## Description

Apply a declarative Lynxfile to create and start one or more applications. Each app entry in the file is converted into an AppSpec, saved securely, and started via the daemon.

## Lynxfile format

```yaml
version: "1"
namespace: default
apps:
  - name: my-api
    command: "node server.js"
    cwd: "/srv/my-api"
    env:
      PORT: "3000"
    logs:
      dir: "/var/log/lynx"
      stdout: "stdout.log"
      stderr: "stderr.log"
    restart:
      policy: "on-failure"
      max_restarts: 10
      delay_ms: 2000
      backoff: "expo"
```

## Examples

Apply a Lynxfile:
```bash
lynx apply ./Lynxfile.yml
```

## Notes

- Specs are stored in `~/.config/lynx/apps` with `0600` permissions.
- If `namespace` is omitted per app, the file‑level namespace or `default` is used.
