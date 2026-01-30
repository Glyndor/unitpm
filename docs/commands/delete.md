# delete

Delete one or more processes and their configurations.

## Synopsis

```bash
lynx delete [--purge] <id|name>...
```

## Description

Stops and removes the specified processes from management. By default, it removes the process from the list and deletes its spec file.

## Flags

- `--purge`: Also delete the log files and any runtime data associated with the process.

## Examples

Delete a process (keep logs):
```bash
lynx delete my-app
```

Delete a process and its logs:
```bash
lynx delete --purge my-app
```
