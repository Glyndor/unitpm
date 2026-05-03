---
title: "lynxpm completion"
description: Generate shell completion scripts for the Lynx CLI (lynxpm) for bash, zsh, or fish. Enables tab-completion for all commands, flags, and process names.
head:
  - tag: script
    attrs:
      type: application/ld+json
    content: '{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Lynx","item":"https://jaro-c.github.io/Lynx/"},{"@type":"ListItem","position":2,"name":"Reference","item":"https://jaro-c.github.io/Lynx/reference/architecture/"},{"@type":"ListItem","position":3,"name":"lynxpm completion","item":"https://jaro-c.github.io/Lynx/reference/commands/completion/"}]}'
sidebar:
  label: completion
---

## 📖 Synopsis

```bash
lynxpm completion <bash|zsh|fish>
```

## Description

Generates a ready-to-source completion script. The script completes the
top-level command names (including aliases like `ls`, `ps`, `rm`) and, for
commands that target running processes (`stop`, `restart`, `reload`,
`flush`, `delete`, `show`, `logs`), the names of the currently managed
processes via a call to `lynxpm list`.

Internal wrapper commands (`_exec-env`, `_exec-sandbox`) are excluded from
the completion table.

## ⚙️ Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-h`, `--help` | - | - | Show help message. |

## 🚀 Install

### Bash

```bash
lynxpm completion bash > ~/.local/share/bash-completion/completions/lynxpm
```

Re-open your shell or `source` the file.

### Zsh

```bash
lynxpm completion zsh > "${fpath[1]}/_lynxpm"
```

Make sure `compinit` is called from your `.zshrc`.

### Fish

```bash
lynxpm completion fish > ~/.config/fish/completions/lynxpm.fish
```

Fish picks it up on the next shell start.

## Notes

- Dynamic process-name completion shells out to `lynxpm list` at completion
  time. If the daemon is down you get only command-name completion.
- The scripts are regenerated each time you run `lynxpm completion` — rerun
  after upgrades so new aliases show up.
