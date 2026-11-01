---
title: devvm ports add
weight: 25
---

Forward more ports from this folder's VM

## Synopsis

Add LIST's ports and ranges (5173,8000-8010) to those this folder's VM forwards, or with -g to those new VMs forward. A VM that forwards every port keeps doing so.

```text
devvm ports add LIST [flags]
```

## Options

```text
  -g, --global   change what new VMs forward instead
  -h, --help     help for add
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
