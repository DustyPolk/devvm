---
title: devvm adopt
weight: 2
---

Use the VM made for an earlier folder at this path here

## Synopsis

Give this folder the VM made for an earlier folder at the same path, with its files, secrets and allowlist. devvm refuses that VM in a folder made again, until you adopt it or delete it with devvm rm. devvm asks first in a terminal.

```text
devvm adopt [flags]
```

## Examples

```text
  devvm adopt          # give this folder the VM, after asking
  devvm adopt --force  # give it without asking, as a script must
```

## Options

```text
  -f, --force   don't ask to confirm, which devvm needs without a terminal
  -h, --help    help for adopt
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm adopt](../getting-started/commands.md#a-folder-made-again) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
