---
title: devvm git-protect
weight: 10
---

Show or change .git protection for this folder's VM

## Synopsis

Show whether this folder's VM can change the parts of .git that run code on this machine: on (the default on macOS: hooks and config locked), strict (on, and host git status and diff stay out of submodules), off, or clone for a --clone VM. devvm git-protect on, strict or off changes it while the VM is stopped; on and strict lock .git as it is now. Locks need macOS.

```text
devvm git-protect [on|strict|off] [flags]
```

## Options

```text
  -h, --help   help for git-protect
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
