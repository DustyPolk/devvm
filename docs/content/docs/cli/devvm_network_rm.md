---
title: devvm network rm
weight: 23
---

Remove rules from this folder's VM

## Synopsis

Remove RULEs, as written (a deny with its !), from this folder's VM's own rules, or with -g yours for every VM. They apply at once.

```text
devvm network rm RULE... [flags]
```

## Options

```text
  -g, --global   change your rules for every VM instead
  -h, --help     help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
