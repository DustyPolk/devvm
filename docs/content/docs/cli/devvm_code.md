---
title: devvm code
weight: 4
---

Open this folder's VM in VS Code over Remote-SSH

## Synopsis

Get this folder's VM ready as devvm host does, then open its folder in VS Code over Remote-SSH, with VS Code's code command. The VM's VS Code settings turn off forwarding its ports to this machine and answering git's sign-in prompts with your GitHub sign-in.

```text
devvm code [flags]
```

## Options

```text
  -h, --help   help for code
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
