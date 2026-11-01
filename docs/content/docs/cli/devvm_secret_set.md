---
title: devvm secret set
weight: 33
---

Save the Claude Code token or GitHub token on this machine

## Synopsis

Save a secret on this machine, claude (the Claude Code token that claude setup-token prints) or github (a GitHub token), in the macOS Keychain, else in ~/.config/devvm/secrets/NAME, a file only you can read. It asks for the token without showing it, and --stdin reads a line instead. Proxies that are running put the new token in at once.

```text
devvm secret set NAME [flags]
```

## Options

```text
  -h, --help    help for set
      --stdin   read the token from a line of input, without asking
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
