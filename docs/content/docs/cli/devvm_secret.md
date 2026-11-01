---
title: devvm secret
weight: 31
---

List the secrets kept on this machine, for VMs to use

## Synopsis

List the secrets devvm keeps on this machine, whether each is saved, and the VMs using it: claude, the Claude Code token every VM whose image has claude uses, and github, the GitHub token of the VMs it's bound to. With network policy a VM gets a placeholder, and its proxy puts the secret into requests for the secret's hosts. Without network policy, the VM gets the secret itself.

```text
devvm secret [flags]
```

## Options

```text
  -h, --help   help for secret
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm secret rm](devvm_secret_rm.md): Delete a saved token
- [devvm secret set](devvm_secret_set.md): Save the Claude Code token or GitHub token on this machine
- [devvm secret unuse](devvm_secret_unuse.md): Stop this folder's VM using a token
- [devvm secret use](devvm_secret_use.md): Let this folder's VM use a token
