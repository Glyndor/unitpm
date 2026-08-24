---
title: "unitpm export"
description: Export running unitpm processes to a unitpm.yml YAML document. Capture the exact spec of all apps in a namespace for reproducible, version-controlled deploys.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm export","item":"https://jaro-c.github.io/unitpm/reference/commands/export/"}]}'
sidebar:
  label: export
---

## 📖 Synopsis

```bash
unitpm export --namespace <name>
```

## Description

Export all applications in a namespace to a unitpm.yml YAML document printed to stdout. Useful for migrating or backing up configurations.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n`, `--namespace` | string | default | Namespace to export. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Export the `default` namespace:
```bash
unitpm export --namespace default > unitpm.yml
```

## Notes

- Only applications whose specs belong to the selected namespace are exported.
- The resulting file matches the format accepted by `unitpm apply`.
