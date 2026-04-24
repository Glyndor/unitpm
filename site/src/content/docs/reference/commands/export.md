---
title: "lynxpm export"
description: "Export all applications in a namespace to a Lynxfile YAML document"
sidebar:
  label: export
---

## 📖 Synopsis

```bash
lynxpm export --namespace <name>
```

## Description

Export all applications in a namespace to a Lynxfile YAML document printed to stdout. Useful for migrating or backing up configurations.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n`, `--namespace` | string | default | Namespace to export. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Export the `default` namespace:
```bash
lynxpm export --namespace default > Lynxfile.yml
```

## Notes

- Only applications whose specs belong to the selected namespace are exported.
- The resulting file matches the format accepted by `lynxpm apply`.
