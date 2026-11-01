---
title: devvm host
weight: 12
---

Get this folder's VM ready for ssh and editors, and show its host name

## Synopsis

Get this folder's VM ready for ssh and editors as devvm does before opening a shell: checked, started if it's stopped, its network policy checked, set up and in devvm's ssh config. Then show the host name ssh and VS Code's Remote-SSH reach it by (devvm-website, for a folder named website). Without the Include line in ~/.ssh/config, devvm offers to add it.

```text
devvm host [flags]
```

## Options

```text
  -h, --help   help for host
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
