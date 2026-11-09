---
title: devvm network rm
explainer: "Remove rules from this folder's VM"
---

Remove rules from this folder's VM

## Synopsis

Remove RULEs, as written with a deny's !, from this folder's VM's own rules, or with -g yours for every VM, with --method and --path for a rule that narrows. The change applies at once.

```text
devvm network rm RULE... [flags]
```

## Examples

```text
  devvm network rm example.com                        # remove an allow
  devvm network rm '!example.com'                     # remove a deny
  devvm network rm api.example.com --method GET,HEAD  # remove an allow that narrows
```

## Options

```text
  -g, --global           change your rules for every VM instead
  -h, --help             help for rm
      --here             use this folder's own VM, never that of a folder it's inside
      --method METHODS   narrow each rule to these METHODS of a host's requests, such as GET,HEAD
      --parent           use the VM of the nearest folder this one is inside, even across a git repo of its own
      --path PATH        narrow each rule to the requests whose path matches PATH, such as /repos/ for everything below /repos/
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network rm](../safety/network.md#changing-rules) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network tier of this folder's VM
