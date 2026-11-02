---
title: devvm secret set
weight: 41
---

Save the Claude Code token or GitHub token on this machine

## Synopsis

Save a secret on this machine, claude for the Claude Code token that claude setup-token prints or github for a GitHub token. devvm asks for it without showing it, and --stdin reads a line instead. Proxies that are running put the new value in at once.

```text
devvm secret set NAME [flags]
```

## Examples

```text
  devvm secret set claude                          # save the Claude Code token, asking for it
  gh auth token | devvm secret set github --stdin  # save GitHub CLI's token without asking
```

## Options

```text
  -h, --help    help for set
      --stdin   read the secret from a line of input, without asking
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm secret set](../safety/network.md#saving-and-binding-secrets) explains it in full.

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
