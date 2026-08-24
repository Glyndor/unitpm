# 🦁 `unitpm export`

> *Export all applications in a namespace to a unitpm.yml YAML document.*

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
