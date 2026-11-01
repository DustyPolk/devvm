---
title: devvm secret unuse
weight: 34
---

Stop this folder's VM using a token

## Synopsis

Stop this folder's VM using a token: its proxy stops putting it in at once. Without network policy the VM got the token itself, which still works wherever it kept it. A VM whose image has claude always uses claude.

```text
devvm secret unuse NAME [flags]
```

## Options

```text
  -h, --help   help for unuse
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
