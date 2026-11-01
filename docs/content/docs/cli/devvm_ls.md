---
title: devvm ls
weight: 18
---

List the project VMs

## Synopsis

List the project VMs on every backend, by name: each one's status, image, .git protection (on, off, or clone for a --clone VM), network policy (on, off, or unenforced: not confined by devvm), backend and folder. A docker VM made for the docker layer shows its runtime beside its backend: docker (sysbox). What a VM's state doesn't have (its creation didn't finish) is -. A VM whose backend sizes no disks (docker) and that holds more than VMs get elsewhere is warned about.

```text
devvm ls [flags]
```

## Options

```text
  -h, --help   help for ls
      --json   print it as one JSON object, for scripts
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
