# 🦁 `unitpm completion`

> *Emit a shell completion script for bash, zsh, or fish.*

## 📖 Synopsis

```bash
unitpm completion <bash|zsh|fish>
```

## Description

Generates a ready-to-source completion script. The script completes the
top-level command names (including aliases like `ls`, `ps`, `rm`) and, for
commands that target running processes (`stop`, `restart`, `reload`,
`flush`, `delete`, `show`, `logs`), the names of the currently managed
processes via a call to `unitpm list`.

Internal wrapper commands (`_exec-env`, `_exec-sandbox`) are excluded from
the completion table.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Install

### Bash

```bash
unitpm completion bash > ~/.local/share/bash-completion/completions/unitpm
```

Re-open your shell or `source` the file.

### Zsh

```bash
unitpm completion zsh > "${fpath[1]}/_unitpm"
```

Make sure `compinit` is called from your `.zshrc`.

### Fish

```bash
unitpm completion fish > ~/.config/fish/completions/unitpm.fish
```

Fish picks it up on the next shell start.

## Notes

- Dynamic process-name completion shells out to `unitpm list` at completion
  time. If the daemon is down you get only command-name completion.
- The scripts are regenerated each time you run `unitpm completion` — rerun
  after upgrades so new aliases show up.
