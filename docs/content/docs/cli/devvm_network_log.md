---
title: devvm network log
weight: 22
---

Show what this folder's VM connected to and what its network policy blocked

## Synopsis

Show the network log of this folder's VM, which it keeps only with network policy, with one line per connection or request its proxy allowed or blocked, oldest first. --blocked lists the blocks instead, one row per host and port, with the devvm network allow command that allows each.

```text
devvm network log [flags]
```

## Examples

```text
  devvm network log            # show the last 50 connections and requests
  devvm network log --follow   # keep showing new ones until Ctrl+C
  devvm network log --blocked  # list the blocks, with the command that allows each
```

## Options

```text
      --blocked   list the blocks by host and port, with the command that allows each
      --follow    keep showing new ones as they happen, until interrupted
  -h, --help      help for log
      --json      print it as one JSON object, for scripts
  -n, --lines N   show the last N connections and requests (default 50)
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network log](../safety/network.md#logs) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
