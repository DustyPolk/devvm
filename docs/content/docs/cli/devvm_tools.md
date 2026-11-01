---
title: devvm tools
weight: 44
---

Show this folder's VM's shared tools

## Synopsis

Show the shared tools of this folder's VM: each tool's version, and the newer version devvm's tools lock has, if any, with whether it's built. devvm tools update moves the VM to the newer ones. It changes nothing, starts no VM and doesn't go online.

```text
devvm tools [flags]
```

## Options

```text
  -h, --help   help for tools
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
- [devvm tools add](devvm_tools_add.md): Add shared tools to this folder's VM, then open a shell in it
- [devvm tools rm](devvm_tools_rm.md): Take shared tools out of this folder's VM, then open a shell in it
- [devvm tools update](devvm_tools_update.md): Move this folder's VM to the newest shared tools, then open a shell in it
