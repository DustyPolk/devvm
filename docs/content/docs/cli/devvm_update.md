---
title: devvm update
weight: 56
---

Make this folder's VM again from its image's new build, then open a shell in it

## Synopsis

Make this folder's VM again from its image's new build, with the same layers, when that image changed since devvm made the VM, then open a shell in it, after asking in a terminal. devvm keeps its home folder, the project folder and its settings, and everything else on its disk is lost. When its image has an agent, devvm first exports the agents' history, unless you give --no-export.

```text
devvm update [flags]
```

## Examples

```text
  devvm update               # make this folder's VM again from its image's new build, after asking
  devvm update --force       # the same without asking, as a script must
  devvm update --clean-home  # start from the image's home folder too
```

## Options

```text
      --clean-home   give the VM the image's home folder instead of keeping its own
  -f, --force        don't ask to confirm, which devvm needs without a terminal
  -h, --help         help for update
      --no-export    don't export the agents' history first
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm update](../using/images.md#outdated-vms) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
