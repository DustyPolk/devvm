---
title: devvm desktop
weight: 6
---

Open this folder's VM's desktop in a VNC viewer

## Synopsis

Get this folder's VM ready as devvm host does, start the desktop of its desktop feature, and serve it on this machine's 127.0.0.1 alone, with a password of the VM's own, until you press Ctrl+C or the VM stops. On macOS, Screen Sharing opens it and asks for the password, which devvm shows. The clipboard isn't shared unless you add --clipboard.

```text
devvm desktop [flags]
```

## Options

```text
      --clipboard   share the clipboard both ways: this machine's reaches the VM, and the VM's this machine
  -h, --help        help for desktop
      --no-open     show the address and password without opening Screen Sharing
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: devvm network -g's, else asked in a terminal, and on without one)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
