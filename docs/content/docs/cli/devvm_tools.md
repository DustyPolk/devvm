---
title: devvm tools
weight: 51
---

Show this folder's VM's shared tools

## Synopsis

Show the shared tools of this folder's VM, each with its version and any newer version in devvm's tools lock. It changes nothing, starts no VM and doesn't go online.

```text
devvm tools [flags]
```

## Examples

```text
  devvm tools                    # show the VM's shared tools
  devvm tools add python claude  # add python and Claude Code to the VM
  devvm tools update             # move the VM to the newest versions
```

## Options

```text
  -h, --help   help for tools
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm tools](../using/toolchains.md#adding-and-removing-tools) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm tools add](devvm_tools_add.md): Add shared tools to this folder's VM, then open a shell in it
- [devvm tools rm](devvm_tools_rm.md): Take shared tools out of this folder's VM, then open a shell in it
- [devvm tools update](devvm_tools_update.md): Move this folder's VM to the newest shared tools, then open a shell in it
