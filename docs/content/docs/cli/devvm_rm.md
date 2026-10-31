---
title: devvm rm
weight: 29
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

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: on, asked in a terminal)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
