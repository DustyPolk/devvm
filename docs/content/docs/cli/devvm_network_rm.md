---
title: devvm network rm
weight: 34
---

Remove rules from this folder's VM

## Synopsis

Remove RULEs, as written with a deny's !, from this folder's VM's own rules, or with -g yours for every VM. The change applies at once.

```text
devvm network rm RULE... [flags]
```

## Examples

```text
  devvm network rm example.com     # remove an allow
  devvm network rm '!example.com'  # remove a deny
```

## Options

```text
  -g, --global   change your rules for every VM instead
  -h, --help     help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network rm](../safety/network.md#changing-rules) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
