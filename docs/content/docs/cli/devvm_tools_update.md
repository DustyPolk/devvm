---
title: devvm tools update
weight: 46
---

Move this folder's VM to the newest shared tools, then open a shell in it

## Synopsis

Check for newer versions of the shared tools (node, python, rust, go, the agents), build them, and move this folder's VM to them, then open a shell in it. Nothing in the VM's disk changes: its tools are mounted from the share.

```text
devvm tools update [flags]
```

## Options

```text
  -h, --help   help for update
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
