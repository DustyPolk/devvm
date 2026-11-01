---
title: devvm gc
weight: 8
---

Delete stale images and unused tool images

## Synopsis

Delete what devvm images lists as stale or unfinished, on every backend, with their records, the shared tool images it lists as unused, the copies of devvm's own files older versions wrote out, the older versions of the mise devvm runs on this machine, and the shared Nix cache's least recently used files while it's over its cap (nix-cache.size). With --backend, only that backend's images, its current ones too, and its unused tool images; without it, every backend's, whatever the default backend is. With --tools, also the current tool images no VM has, on every backend and C library, or only those of the TOOLs named (node, claude), but not those built in the last hour, which it names and keeps because a VM being made may not list them yet, and none while a VM is being made or getting new shared tools, which it names too. With --tools --force, those built in the last hour too, without asking, unless a VM is being made or getting new shared tools, when it deletes nothing. With --history, also the agents' history devvm history export and devvm rm saved, the only copy of a deleted VM's, or only those older than --older-than (a duration: 12h, 30d, 2w). With --nix-cache, also the whole shared Nix cache, which VMs download again when they need it. With --orphans, also what devvm keeps for VMs deleted outside it, as devvm rm does, for each VM whose backend answers that it no longer has it. With --caches, then what devvm's builds put in the backends' own caches that nothing of devvm's needs (Lima's Ubuntu downloads, smolvm's disks of devvm's packs, Docker volumes of devvm's no VM has), each backend listed and asked for apart, and only in a terminal. With --retention, then the retention settings at once. It lists them first, and in a terminal asks. VMs are never deleted; on Linux hosts, an image whose disk VMs use stays, and so does a smolvm or Docker image a VM was made from. A backend a VM uses that can't list its images is skipped with a warning, everything of its left alone, and gc goes on with the others.

```text
devvm gc [--tools [--force] [TOOL...]] [flags]
```

## Options

```text
      --backend NAME          the NAME of the only backend to act on (default: every backend)
      --caches                free devvm's share of the backends' own caches afterwards, asking for each backend; only in a terminal
  -f, --force                 with --tools, delete the current shared tool images no VM has that were built in the last hour too, without asking; refused while a VM is being made
  -h, --help                  help for gc
      --history               delete the saved exports of agents' history too
      --nix-cache             delete the whole shared Nix cache too, which VMs download again when they need it
      --older-than DURATION   with --history, only those older than DURATION: 12h, 30d or 2w
      --orphans               delete what devvm keeps for VMs their backend no longer has too, as devvm rm does
      --retention             apply the retention settings afterwards, which devvm otherwise does once a day
      --tools                 delete the current shared tool images no VM has too; with TOOL names, only theirs
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
