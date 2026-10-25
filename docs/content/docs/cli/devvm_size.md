---
title: devvm size
weight: 34
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
      --memory GiB   the VM's memory in GiB: 16, 16G or 16GiB
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --network-policy      create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
