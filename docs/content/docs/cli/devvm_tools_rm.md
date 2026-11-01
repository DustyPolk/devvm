---
title: devvm tools rm
weight: 46
---

Take shared tools out of this folder's VM, then open a shell in it

## Synopsis

Take shared tools out of this folder's VM, each TOOL a separate word (devvm tools rm python claude), then open a shell in it. They're not mounted in it any more; what the VM installed itself stays. Your layers are in the VM's disk, so they can't be taken out: devvm rm, and make the VM again.

```text
devvm tools rm TOOL... [flags]
```

## Options

```text
  -h, --help   help for rm
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

- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
