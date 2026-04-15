# 🦁 `lynx completion`

> *Emit a shell completion script for bash, zsh, or fish.*

## 📖 Synopsis

```bash
lynx completion <bash|zsh|fish>
```

## Description

Generates a ready-to-source completion script. The script completes the
top-level command names (including aliases like `ls`, `ps`, `rm`) and, for
commands that target running processes (`stop`, `restart`, `reload`,
`flush`, `delete`, `show`, `logs`), the names of the currently managed
processes via a call to `lynx list`.

Internal wrapper commands (`_exec-env`, `_exec-sandbox`) are excluded from
the completion table.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Install

### Bash

```bash
lynx completion bash > ~/.local/share/bash-completion/completions/lynx
```

Re-open your shell or `source` the file.

### Zsh

```bash
lynx completion zsh > "${fpath[1]}/_lynx"
```

Make sure `compinit` is called from your `.zshrc`.

### Fish

```bash
lynx completion fish > ~/.config/fish/completions/lynx.fish
```

Fish picks it up on the next shell start.

## Notes

- Dynamic process-name completion shells out to `lynx list` at completion
  time. If the daemon is down you get only command-name completion.
- The scripts are regenerated each time you run `lynx completion` — rerun
  after upgrades so new aliases show up.
