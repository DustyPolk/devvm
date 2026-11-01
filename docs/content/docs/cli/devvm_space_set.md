---
title: devvm space set
weight: 40
---

Change the retention settings: what devvm deletes on its own

## Synopsis

Change the retention settings, each KEY=VALUE, and KEY= to turn one off: logs.runs (the run logs kept, 20 when off), exports.age and exports.count (the exports older than an age, or beyond the newest count, deleted), tools.unused-age (current shared tool images no VM has had for that long deleted), orphans.age (what devvm keeps for VMs gone from their backend for that long, deleted by devvm gc --retention) and nix-cache.size (the shared Nix cache's cap, 10G when off). Ages are 30d, 2w or 12h, and sizes 500M, 20G or 1T. Every setting is off unless set. A bad key or value is refused, and nothing is written. devvm applies them once a day, and devvm gc --retention at once; they never delete a VM, an image or a tool any VM has.

```text
devvm space set KEY=VALUE... [flags]
```

## Options

```text
  -h, --help   help for set
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

- [devvm space](devvm_space.md): Show what devvm keeps on this machine and the space it takes
