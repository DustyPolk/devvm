---
title: devvm rm
weight: 30
---

Delete this folder's VM

## Synopsis

Delete this folder's VM and what devvm keeps for it, after asking in a terminal. When its image has an agent, devvm first exports the agents' history, unless you give --no-export. A --clone VM's work that this repo hasn't fetched is offered for fetching first.

```text
devvm rm [flags]
```

## Examples

```text
  devvm rm              # delete this folder's VM, after asking
  devvm rm --force      # delete it without asking, as a script must
  devvm rm --no-export  # delete it without exporting the agents' history
```

## Options

```text
  -f, --force       don't ask to confirm, which devvm needs without a terminal
  -h, --help        help for rm
      --no-export   delete without exporting the agents' history first
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm rm](../getting-started/commands.md#deleting-a-vm) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
