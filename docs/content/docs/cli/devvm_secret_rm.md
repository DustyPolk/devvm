---
title: devvm secret rm
weight: 32
---

Delete a saved token

## Synopsis

Delete a saved token, after asking in a terminal; --force deletes it without asking, which it needs without a terminal. Proxies running stop putting it in at once; what a VM did with it stays done.

```text
devvm secret rm NAME [flags]
```

## Options

```text
  -f, --force   don't ask to confirm, which a command that deletes needs without a terminal
  -h, --help    help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
