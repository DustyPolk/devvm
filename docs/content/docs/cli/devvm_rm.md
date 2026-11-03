---
title: devvm rm
weight: 38
---

Delete this folder's VM, or a VM whose folder is gone

## Synopsis

Delete this folder's VM and what devvm keeps for it, or the VM you name when its folder is gone, after asking in a terminal. When its image has an agent, devvm first exports the agents' history, unless you give --no-export. A --clone VM's work that this repo hasn't fetched is offered for fetching first, unless you give --no-clone-check.

```text
devvm rm [VM] [flags]
```

## Examples

```text
  devvm rm                           # delete this folder's VM, after asking
  devvm rm --force                   # delete it without asking, as a script must
  devvm rm --no-export               # delete it without exporting the agents' history
  devvm rm --no-clone-check          # delete a --clone VM without checking its clone, losing its unfetched work
  devvm rm devvm-web-71a99b1076d92c  # delete a VM whose folder is gone, by its name as devvm ls lists it
```

## Options

```text
  -f, --force            don't ask to confirm, which devvm needs without a terminal
  -h, --help             help for rm
      --no-clone-check   delete a --clone VM without checking its clone for unfetched commits and uncommitted changes, which are lost
      --no-export        delete without exporting the agents' history first
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
