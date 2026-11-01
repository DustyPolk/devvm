---
title: devvm ports rm
weight: 26
---

Stop forwarding some ports from this folder's VM

## Synopsis

Remove LIST's ports and ranges, each as listed, from those this folder's VM forwards, or with -g from those new VMs forward. Removing the last leaves none.

```text
devvm ports rm LIST [flags]
```

## Options

```text
  -g, --global   change what new VMs forward instead
  -h, --help     help for rm
```

## See also

- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
