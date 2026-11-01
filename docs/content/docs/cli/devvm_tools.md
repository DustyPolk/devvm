---
title: devvm tools
weight: 43
---

Show this folder's VM's shared tools

## Synopsis

Show the shared tools of this folder's VM: each tool's version, and the newer version devvm's tools lock has, if any, with whether it's built. devvm tools update moves the VM to the newer ones. It changes nothing, starts no VM and doesn't go online.

```text
devvm tools [flags]
```

## Options

```text
  -h, --help   help for tools
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm tools add](devvm_tools_add.md): Add shared tools to this folder's VM, then open a shell in it
- [devvm tools rm](devvm_tools_rm.md): Take shared tools out of this folder's VM, then open a shell in it
- [devvm tools update](devvm_tools_update.md): Move this folder's VM to the newest shared tools, then open a shell in it
