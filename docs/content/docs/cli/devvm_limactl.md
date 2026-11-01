---
title: devvm limactl
weight: 17
---

Run Lima's limactl on devvm's own Lima home

## Synopsis

Run limactl with ARGS on devvm's own Lima home, where the lima backend's VMs and images live, in devvm's place: its input, output and exit code are limactl's, and every argument is passed on, flags and --help included. It runs in devvm's data folder, so give it files by absolute path. A VM started this way runs unconfined, without its network policy (devvm ls shows it unenforced): start VMs with devvm.

```text
devvm limactl [ARGS...]
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
