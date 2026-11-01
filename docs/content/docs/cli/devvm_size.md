---
title: devvm size
weight: 37
---

Show or change this folder's VM's size

## Synopsis

Show this folder's VM's CPUs, memory and disk, and what changes when it next starts. --cpus, --memory and --disk change them, at once for a stopped VM, from its next start for a running one: devvm restart. A disk only grows. With -g, show or change the size new VMs get instead; what it leaves unset is the image's.

```text
devvm size [flags]
```

## Options

```text
      --cpus N       the N CPUs the VM gets
      --disk GiB     the VM's disk in GiB, at least what it has
  -g, --global       show or change the size new VMs get instead
  -h, --help         help for size
      --json         print it as one JSON object, for scripts
      --memory GiB   the VM's memory in GiB: 16, 16G or 16GiB
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
