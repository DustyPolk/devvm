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

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
