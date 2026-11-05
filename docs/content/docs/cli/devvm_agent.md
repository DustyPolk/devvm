---
title: devvm agent
weight: 3
---

Fill devvm's store of the agents' settings and logins that every VM gets

## Synopsis

devvm keeps the coding agents' settings in ~/.config/devvm/agents and their logins in ~/.local/share/devvm/logins, and every VM with an agent gets them from there, never from the agents' own files in your home folder. devvm agent import copies those files into the store once.

```text
devvm agent [flags]
```

## Examples

```text
  devvm agent import        # copy every agent's settings and logins from this machine
  devvm agent import codex  # copy only Codex's
```

## Options

```text
  -h, --help   help for agent
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm agent](../using/coding-agents.md#devvms-agent-store) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm agent import](devvm_agent_import.md): Copy the agents' settings and logins from this machine into devvm's store
