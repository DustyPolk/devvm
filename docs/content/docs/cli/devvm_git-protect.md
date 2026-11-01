---
title: devvm git-protect
weight: 9
---

Show or change .git protection for this folder's VM

## Synopsis

Show whether this folder's VM can change the parts of .git that run code on this machine: on (the default on macOS: hooks and config locked), strict (on, and host git status and diff stay out of submodules), off, or clone for a --clone VM. devvm git-protect on, strict or off changes it while the VM is stopped; on and strict lock .git as it is now. Locks need macOS.

```text
devvm git-protect [on|strict|off] [flags]
```

## Options

```text
  -h, --help   help for git-protect
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
