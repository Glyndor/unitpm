---
title: "lynxpm export"
description: Export running Lynx processes to a Lynxfile YAML document. Capture the exact spec of all apps in a namespace for reproducible, version-controlled deploys.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm export","item":"https://jaro-c.github.io/Lynx/reference/commands/export/"}]}'
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
