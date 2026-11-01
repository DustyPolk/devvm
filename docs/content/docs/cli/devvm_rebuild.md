---
title: devvm rebuild
weight: 27
---

Build an image again, and delete the old one

## Synopsis

Build an image again, from its parent as it is, and delete the stale images no VM uses, the old one and those built on it among them. Shared tools aren't images: their versions come from the tools lock.

```text
devvm rebuild IMAGE [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
      --from IMAGE     the container IMAGE the base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM)
  -h, --help           help for rebuild
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
