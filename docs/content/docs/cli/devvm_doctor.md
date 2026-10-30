---
title: devvm doctor
weight: 7
---

Check what devvm needs on this machine

## Synopsis

Check what the backend new VMs get (--backend for another) needs on this machine, and say how to get what's missing: devvm never installs it. The docker backend also says which engine it uses, and on Docker Desktop which folders it shares. Then list each backend's features (native, emulated, unreviewed: emulated but refused until reviewed, or none) and whether Docker is installed, which only --from with an image only Docker has needs. Fails when something is missing.

```text
devvm doctor [flags]
```

## Options

```text
      --fixes   print only what's missing, for scripts: a KEY<tab>FIX<tab>MESSAGE line each, FIX being a command or empty; succeeds either way
  -h, --help    help for doctor
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
