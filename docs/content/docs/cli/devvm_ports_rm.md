---
title: devvm ports rm
weight: 37
---

Stop forwarding some ports from this folder's VM

## Synopsis

Remove LIST's ports and ranges, each as listed, from those this folder's VM forwards, or with -g from those new VMs forward. Removing the last leaves none.

```text
devvm ports rm LIST [flags]
```

## Examples

```text
  devvm ports rm 5173     # stop forwarding one port
  devvm ports rm -g 5173  # stop new VMs forwarding it
```

## Options

```text
  -g, --global   change what new VMs forward instead
  -h, --help     help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm ports rm](../using/ports-size.md#ports) explains it in full.

## See also

- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
