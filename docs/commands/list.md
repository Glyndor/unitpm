# list

List all processes managed by Lynx.

## Synopsis

```bash
lynx list
```

## Flags

No flags.

## Examples

List all processes:
```bash
lynx list
```

Example output:
```
id       | name                           | namespace            | version    | mode       | pid      | uptime     | ↺     | status          | cpu      | mem        | user            | watch
-------- | ------------------------------ | -------------------- | ---------- | ---------- | -------- | ---------- | ----- | --------------- | -------- | ---------- | --------------- | ----------
e73a9f1b | test-app                       | default              | 1.0.0      | fork       | 12345    | 1h 2m      | 0     | online          | 0.1%     | 12.5 MB    | lynx            | disabled
```
