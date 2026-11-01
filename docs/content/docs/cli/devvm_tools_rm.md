---
title: devvm tools rm
weight: 45
---

Take shared tools out of this folder's VM, then open a shell in it

## Synopsis

Take shared tools out of this folder's VM, each TOOL a separate word (devvm tools rm python claude), then open a shell in it. They're not mounted in it any more; what the VM installed itself stays. Your layers are in the VM's disk, so they can't be taken out: devvm rm, and make the VM again.

```text
devvm tools rm TOOL... [flags]
```

## Options

```text
  -h, --help   help for rm
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
