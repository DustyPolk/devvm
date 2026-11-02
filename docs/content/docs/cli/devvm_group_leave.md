---
title: devvm group leave
weight: 12
---

Take this folder's VM out of its group

## Synopsis

Take this folder's VM out of its group, and remove the links the group made, in this VM and in the group's other VMs, naming them. The links you made with devvm link add stay, and the group goes with its last VM.

```text
devvm group leave [flags]
```

## Examples

```text
  devvm group leave  # leave this folder's VM's group
```

## Options

```text
  -h, --help   help for leave
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm group leave](../using/links.md#leaving-a-group) explains it in full.

## See also

- [devvm group](devvm_group.md): Show the group this folder's VM is in
