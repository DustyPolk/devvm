---
title: devvm tools add
weight: 44
---

Add shared tools to this folder's VM, then open a shell in it

## Synopsis

Add shared tools to this folder's VM, each TOOL a separate word (devvm tools add python claude), then open a shell in it: node, python, rust, go, claude, codex or opencode. devvm builds the ones the share lacks, and nothing in the VM's disk changes, because its tools are mounted. A new agent's config folders are mounted too, which needs the VM to start again: devvm asks before restarting a running VM. Layers can't be added: only a new VM gets one.

```text
devvm tools add TOOL... [flags]
```

## Options

```text
  -h, --help   help for add
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
