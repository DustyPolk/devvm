---
title: devvm restart
weight: 28
---

Restart this folder's VM and open a shell in it

## Synopsis

Stop this folder's VM, then start it and open a shell in it as devvm does: what applies only when it starts (agent folder mounts, size, ports) applies.

```text
devvm restart [flags]
```

## Options

```text
  -h, --help   help for restart
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
