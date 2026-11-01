---
title: devvm build
weight: 3
---

Build an image and the images under it, unless they're built

## Synopsis

Build an image and the images under it, unless they're built, and the shared tool images it names that aren't. IMAGE is base, or layers (devvm's docker and nix, your own) joined with +, bottom first (foo+bar is bar on foo on base), with any shared tools (node+foo+claude).

```text
devvm build IMAGE [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
      --from IMAGE     the container IMAGE the base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM)
  -h, --help           help for build
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
