---
title: devvm secret use
weight: 44
---

Let this folder's VM use a secret

## Synopsis

Let this folder's VM use a secret. Its proxy puts the secret in at once, and the VM gets its placeholder from its next launch, or the secret itself without network policy.

```text
devvm secret use NAME [flags]
```

## Examples

```text
  devvm secret use github  # let the VM use your GitHub token
```

## Options

```text
  -h, --help   help for use
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm secret use](../safety/network.md#saving-and-binding-secrets) explains it in full.

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
