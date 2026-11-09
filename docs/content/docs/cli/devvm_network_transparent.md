---
title: devvm network transparent
explainer: "Show or change transparent mode for this folder's VM"
---

Show or change transparent mode for this folder's VM

## Synopsis

Show whether this folder's VM sends tools that ignore the proxy settings to its proxy by the names they look up, on or off, and where that comes from: the VM's own setting, config.toml's transparent-proxy, or devvm's default, on. devvm network transparent on or off gives the VM its own setting, which applies from its next launch, while it's on a confined tier.

```text
devvm network transparent [on|off] [flags]
```

## Examples

```text
  devvm network transparent      # show the VM's transparent mode and where it comes from
  devvm network transparent off  # turn it off for this VM, from its next launch
  devvm network transparent on   # turn it on again, whatever config.toml says
```

## Options

```text
  -h, --help     help for transparent
      --here     use this folder's own VM, never that of a folder it's inside
      --json     print it as one JSON object, for scripts
      --parent   use the VM of the nearest folder this one is inside, even across a git repo of its own
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network transparent](../safety/network.md#tools-that-ignore-the-proxy) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network tier of this folder's VM
