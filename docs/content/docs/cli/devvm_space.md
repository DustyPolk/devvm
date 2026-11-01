---
title: devvm space
weight: 38
---

Show what devvm keeps on this machine and the space it takes

## Synopsis

Show each kind of thing devvm keeps on this machine, on every backend, with how many there are, the disk they take, whether devvm can make them again, what devvm gc frees of them now and what frees the rest, then a total. The backends' own caches come last and apart, because they can also hold your own downloads. It changes nothing and starts no VM.

```text
devvm space [flags]
```

## Options

```text
  -h, --help   help for space
      --json   print it as one JSON object, for scripts
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
