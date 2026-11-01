---
title: devvm images rm
weight: 14
---

Delete images, by name or by key

## Synopsis

Delete images, with their records: each by its name (base, foo+bar), or by the key devvm images shows, for a stale or unfinished one; with --from, a name is its image on a base from that container image. Every one must be built, or nothing is deleted. VMs made from an image keep working, except on Linux hosts, where an image whose disk VMs use is refused. Shared tools aren't images.

```text
devvm images rm IMAGE|KEY... [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
      --from IMAGE     the container IMAGE the base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM)
  -h, --help           help for rm
```

## See also

- [devvm images](devvm_images.md): List the images and the shared tools
