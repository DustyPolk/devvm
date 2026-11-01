---
title: devvm gc
weight: 9
---

Delete stale images and unused tool images

## Synopsis

Delete what devvm images lists as stale or unfinished, on every backend, with their records, the shared tool images it lists as unused, the copies of devvm's own files older versions wrote out, the older versions of the mise devvm runs on this machine, and the shared Nix cache's least recently used files while it's over its cap (nix-cache.size). With --backend, only that backend's images, its current ones too, and its unused tool images; without it, every backend's, whatever the default backend is. With --tools, also the current tool images no VM has, on every backend and C library, or only those of the TOOLs named (node, claude), but not those built in the last hour, which it names and keeps because a VM being made may not list them yet, and none while a VM is being made or getting new shared tools, which it names too. With --tools --force, those built in the last hour too, without asking, unless a VM is being made or getting new shared tools, when it deletes nothing. With --exports, also the agents' history devvm export and devvm rm saved, the only copy of a deleted VM's, or only those older than --older-than (30d, 2w, 12h). With --nix-cache, also the whole shared Nix cache, which VMs download again when they need it. With --orphans, also what devvm keeps for VMs deleted outside it, as devvm rm does, for each VM whose backend answers that it no longer has it. With --caches, then what devvm's builds put in the backends' own caches that nothing of devvm's needs (Lima's Ubuntu downloads, smolvm's disks of devvm's packs, Docker volumes of devvm's no VM has), each backend listed and asked for apart, and only in a terminal. With --retention, then the retention settings at once. It lists them first, and in a terminal asks. VMs are never deleted; on Linux hosts, an image whose disk VMs use stays, and so does a smolvm or Docker image a VM was made from. A backend a VM uses that can't list its images is skipped with a warning, everything of its left alone, and gc goes on with the others.

```text
devvm gc [--tools [--force] [TOOL...]] [flags]
```

## Options

```text
      --caches           then free devvm's share of the backends' own caches, asking for each backend; only in a terminal
      --exports          also delete the saved exports of agents' history
      --force            with --tools, also delete the current shared tool images no VM has that were built in the last hour, and don't ask; refused while a VM is being made
  -h, --help             help for gc
      --nix-cache        also delete the whole shared Nix cache, which VMs download again when they need it
      --older-than AGE   with --exports, only those older than AGE: 30d, 2w or 12h
      --orphans          also delete what devvm keeps for VMs their backend no longer has, as devvm rm does
      --retention        then apply the retention settings now, which devvm otherwise does once a day
      --tools            also delete the current shared tool images no VM has; with TOOL names, only theirs
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
