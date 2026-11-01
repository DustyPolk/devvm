---
title: devvm tools add
weight: 44
---

Add shared tools to this folder's VM, then open a shell in it

## Synopsis

Add shared tools to this folder's VM, then open a shell in it. TOOL is node, python, rust, go, claude, codex or opencode, each a separate word. devvm builds any of them that aren't built, and for a new agent in a running VM it asks before restarting the VM.

```text
devvm tools add TOOL... [flags]
```

## Examples

```text
  devvm tools add claude        # add Claude Code to the VM
  devvm tools add python codex  # add python and Codex
```

## Options

```text
  -h, --help   help for add
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm tools add](../using/toolchains.md#adding-and-removing-tools) explains it in full.

## See also

- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
