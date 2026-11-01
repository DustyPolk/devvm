---
title: devvm rm
weight: 30
---

Delete this folder's VM

## Synopsis

Delete this folder's VM, with what devvm keeps for it: its state, its own allowlist, its network log, its .git locks or the repo's remote for its clone, and its ssh config entry. It asks first in a terminal; --force deletes without asking, which it needs without a terminal. When its image has an agent, its agents' history is exported first, unless --no-export, and a failed export keeps the VM. A --clone VM's branches this repo hasn't fetched are offered for fetching, and its uncommitted changes deleted only after you confirm. A VM made for an earlier folder at this path is deleted only after you confirm, and in a folder without a VM of its own, it's the VM of the nearest folder with one that this folder is inside, which devvm names and deletes only after you confirm. These confirmations are asked only in a terminal, and --force never answers them.

```text
devvm rm [flags]
```

## Options

```text
  -f, --force       don't ask to confirm, which a command that deletes needs without a terminal
  -h, --help        help for rm
      --no-export   delete without exporting the agents' history first
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
