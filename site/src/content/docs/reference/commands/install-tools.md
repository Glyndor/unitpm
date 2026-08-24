---
title: "unitpm install-tools"
description: Symlink Node.js, Bun, Go, Python and other runtimes to /usr/local/bin so the unitpm daemon can find them. Required when binaries are absent from the system PATH.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"unitpm","item":"https://jaro-c.github.io/unitpm/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/unitpm/reference/architecture/"},{"@type":"ListItem","position":3,"name":"unitpm install-tools","item":"https://jaro-c.github.io/unitpm/reference/commands/install-tools/"}]}'
sidebar:
  label: install-tools
---

## 📖 Synopsis

```bash
sudo unitpm install-tools [flags]
```

## Description

Automatically symlink common development tools (like `node`, `go`, `bun`, `python`) from the user's environment to `/usr/local/bin`.

This is crucial because the unitpm daemon (when running in system mode) has a restricted `PATH` and might not see tools installed in your user's home directory (e.g., via `nvm`, `brew`, or `go install`). This command bridges that gap safely.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-y`, `--yes` | boolean | false | Automatically confirm all prompts. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Scan and link tools interactively:
```bash
sudo unitpm install-tools
```

Scan and link tools without confirmation:
```bash
sudo unitpm install-tools --yes
```

## How it works

1.  **Scans for tools**: Checks for common tools (`bun`, `node`, `npm`, `pnpm`, `yarn`, `go`, `python`, `rustc`, `cargo`, `java`, `deno`, etc.).
2.  **Locates them**: Uses the `SUDO_USER` environment variable to find where these tools are installed for your specific user (even if they are in `~/.nvm` or `~/.cargo`).
3.  **Creates Symlinks**: Creates symbolic links in `/usr/local/bin/` pointing to the user's tools.
4.  **Verification**: Checks if the tool is already in `/usr/local/bin` to avoid overwriting or duplicating.

## Notes

- **Root Required**: This command must be run with `sudo` because it writes to `/usr/local/bin`.
- **Safe**: It will not overwrite existing system binaries in `/usr/local/bin` unless you manually remove them first.
