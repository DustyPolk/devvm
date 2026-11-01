---
title: devvm secret use
weight: 35
---

Let this folder's VM use a token

## Synopsis

Let this folder's VM use a token: its proxy puts it in at once, and the VM gets its placeholder (the token, without network policy) from its next launch.

```text
devvm secret use NAME [flags]
```

## Options

```text
  -h, --help   help for use
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
