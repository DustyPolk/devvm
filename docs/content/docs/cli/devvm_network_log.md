---
title: devvm network log
weight: 22
---

Show what this folder's VM connected to and what its network policy blocked

## Synopsis

Show the network log of this folder's VM, which has network policy: one line per connection or request its proxy allowed or blocked, oldest first, with the time, the host and port, and the rule that allowed it, the request and the secrets put in, or why it was blocked. --blocked lists the blocks instead, one row per host and port, with the devvm network allow command that allows each.

```text
devvm network log [flags]
```

## Options

```text
      --blocked   list the blocks by host and port, with the command that allows each
      --follow    keep showing each new one as it happens, until interrupted
  -h, --help      help for log
      --json      print it as one JSON object, for scripts
  -n, --lines N   show the last N connections and requests (default 50)
```

## See also

- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
