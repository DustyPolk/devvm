---
title: devvm images
weight: 13
---

List the images and the shared tools

## Synopsis

List the images on every backend, by name: what each one's base started from and its digest, its key, whether it's current, stale (its inputs changed since) or unfinished (a build that didn't finish), the disk it takes and its backend. Then the shared tool images, by the name images have them by: each version, the backend and C library it's built for, the disk it takes, how many VMs use it, and whether it's the tools lock's version, in use, unused, or the lock's and not built yet. Then what they take in all, and what the shared Nix cache takes of its cap.

```text
devvm images [flags]
```

## Options

```text
  -h, --help   help for images
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
- [devvm images rm](devvm_images_rm.md): Delete images, by name or by key
