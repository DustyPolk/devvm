---
title: devvm secret unuse
weight: 44
---

Stop this folder's VM using a secret

## Synopsis

Stop this folder's VM using a secret, which its proxy stops putting in at once. A VM without network policy got the secret itself, which still works wherever it kept it. A VM whose image has claude always uses claude.

```text
devvm secret unuse NAME [flags]
```

## Examples

```text
  devvm secret unuse github  # stop the VM using your GitHub token
```

## Options

```text
  -h, --help   help for unuse
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm secret unuse](../safety/network.md#saving-and-binding-secrets) explains it in full.

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
