---
title: devvm images rm
weight: 14
---

Delete images, by name or by key

## Synopsis

Delete images, with their records: each by its name (base, foo+bar), or by the key devvm images shows, for a stale or unfinished one; with --from, a name is its image on a base from that container image. Every one must be built, or nothing is deleted. VMs made from an image keep working, except on Linux hosts, where an image whose disk VMs use is refused. Shared tools aren't images. It asks first in a terminal; --force deletes without asking, which it needs without a terminal.

```text
devvm images rm IMAGE|KEY... [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
  -f, --force          don't ask to confirm, which a command that deletes needs without a terminal
      --from IMAGE     the container IMAGE the base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM)
  -h, --help           help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm images](devvm_images.md): List the images and the shared tools
