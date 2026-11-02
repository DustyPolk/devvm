---
title: devvm history
weight: 13
---

List the saved exports of agents' history

## Synopsis

List the exports of agents' history that devvm history export and devvm rm saved in ~/.local/share/devvm/exports, oldest first. It changes nothing and starts no VM.

```text
devvm history [flags]
```

## Examples

```text
  devvm history         # list the saved exports
  devvm history export  # save this folder's VM's agent history
```

## Options

```text
  -h, --help   help for history
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm history](../using/coding-agents.md#history) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm history export](devvm_history_export.md): Save this folder's VM's agent history to a .tar.gz
