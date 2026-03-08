# 🦁 `lynx install-tools`

> *Automatically symlink common development tools to `/usr/local/bin`.*

## 📖 Synopsis

```bash
sudo lynx install-tools [flags]
```

## Description

Automatically symlink common development tools (like `node`, `go`, `bun`, `python`) from the user's environment to `/usr/local/bin`.

This is crucial because the Lynx daemon (when running in system mode) has a restricted `PATH` and might not see tools installed in your user's home directory (e.g., via `nvm`, `brew`, or `go install`). This command bridges that gap safely.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-y`, `--yes` | boolean | false | Automatically confirm all prompts. |
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Examples

Scan and link tools interactively:
```bash
sudo lynx install-tools
```

Scan and link tools without confirmation:
```bash
sudo lynx install-tools --yes
```

## How it works

1.  **Scans for tools**: Checks for common tools (`bun`, `node`, `npm`, `pnpm`, `yarn`, `go`, `python`, `rustc`, `cargo`, `java`, `deno`, etc.).
2.  **Locates them**: Uses the `SUDO_USER` environment variable to find where these tools are installed for your specific user (even if they are in `~/.nvm` or `~/.cargo`).
3.  **Creates Symlinks**: Creates symbolic links in `/usr/local/bin/` pointing to the user's tools.
4.  **Verification**: Checks if the tool is already in `/usr/local/bin` to avoid overwriting or duplicating.

## Notes

- **Root Required**: This command must be run with `sudo` because it writes to `/usr/local/bin`.
- **Safe**: It will not overwrite existing system binaries in `/usr/local/bin` unless you manually remove them first.
