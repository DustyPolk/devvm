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

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
