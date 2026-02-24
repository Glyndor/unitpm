# export

## Synopsis

```bash
lynx export --namespace <name>
```

## Description

Export all applications in a namespace to a Lynxfile YAML document printed to stdout. Useful for migrating or backing up configurations.

## Flags

| Flag | Type | Description |
|------|------|-------------|
| `--namespace`, `-n` | string | Namespace to export |

## Examples

Export the `default` namespace:
```bash
lynx export --namespace default > Lynxfile.yml
```

## Notes

- Only applications whose specs belong to the selected namespace are exported.
- The resulting file matches the format accepted by `lynx apply`.
