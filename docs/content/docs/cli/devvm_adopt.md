---
title: devvm adopt
weight: 2
---

Use the VM made for an earlier folder at this path here

## Synopsis

A VM belongs to the folder it was made for: its path, and the folder itself. When that folder was deleted and a new one made at the same path, devvm won't use the VM for it; adopt gives it to this folder, with its files, tokens and allowlist, after asking in a terminal. --force gives it without asking, which it needs without a terminal.

```text
devvm adopt [flags]
```

## Options

```text
  -f, --force   don't ask to confirm, which a command that deletes needs without a terminal
  -h, --help    help for adopt
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
