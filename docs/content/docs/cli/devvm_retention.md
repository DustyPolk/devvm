---
title: devvm retention
weight: 29
---

Show or change the retention settings: what devvm deletes on its own

## Synopsis

Show each retention setting and its value, off when it isn't set, or change them, each KEY=VALUE, and KEY=off to turn one off: logs.runs (the run logs kept, 20 when off), history.age and history.count (the exports of agents' history older than an age, or beyond the newest count, deleted), tools.unused-age (current shared tool images no VM has had for that long deleted), orphans.age (what devvm keeps for VMs gone from their backend for that long, deleted by devvm gc --retention) and nix-cache.size (the shared Nix cache's cap, 10G when off). Ages are durations, such as 30m, 12h, 30d or 2w, and sizes 500M, 20G or 1T. Every setting is off until you set it. devvm refuses a bad key or value, or one with no value, and then writes nothing. devvm applies the settings once a day, and devvm gc --retention applies them at once. They never delete a VM, an image or a tool any VM has.

```text
devvm retention [KEY=VALUE...] [flags]
```

## Options

```text
  -h, --help   help for retention
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
