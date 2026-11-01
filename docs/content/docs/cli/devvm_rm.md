---
title: devvm rm
weight: 30
---

Delete this folder's VM

## Synopsis

Delete this folder's VM, with what devvm keeps for it: its state, its own allowlist, its network log, its .git locks or the repo's remote for its clone, and its ssh config entry. When its image has an agent, its agents' history is exported first, and a failed export keeps the VM. A --clone VM's branches this repo hasn't fetched are offered for fetching, and its uncommitted changes deleted only after you confirm. A VM made for an earlier folder at this path is deleted only after you confirm, even with -f. In a folder without a VM of its own, it's the VM of the nearest folder with one that this folder is inside: devvm names it and deletes it only after you confirm in a terminal, even with -f.

```text
devvm rm [flags]
```

## Options

```text
  -f, --force   delete without exporting the agents' history or checking a clone for unfetched commits and uncommitted changes
  -h, --help    help for rm
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
