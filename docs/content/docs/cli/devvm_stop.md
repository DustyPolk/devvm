---
title: devvm stop
weight: 42
---

Stop this folder's VM

## Synopsis

Stop this folder's VM and its network policy proxy. Once it's stopped, its .git is locked again and checked, as on a launch that finds it stopped, and its ssh config entry goes.

```text
devvm stop [flags]
```

## Options

```text
  -h, --help   help for stop
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
