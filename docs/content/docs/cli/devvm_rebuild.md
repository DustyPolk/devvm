---
title: devvm rebuild
weight: 27
---

Build an image again, and delete the old one

## Synopsis

Build an image again, from its parent as it is, and delete the stale images no VM uses, the old one and those built on it among them. When images are built on it, it asks first in a terminal; --force goes ahead without asking, which it needs without a terminal. Shared tools aren't images: their versions come from the tools lock.

```text
devvm rebuild IMAGE [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
  -f, --force          don't ask to confirm, which a command that deletes needs without a terminal
      --from IMAGE     the container IMAGE the base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM)
  -h, --help           help for rebuild
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
