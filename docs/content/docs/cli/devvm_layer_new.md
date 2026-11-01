---
title: devvm layer new
weight: 16
---

Start a layer of your own

## Synopsis

Start a layer: ~/.config/devvm/layers/NAME.sh, a script to edit, and NAME.policy next to it, the hosts VMs with the layer can reach with network policy (kept if it exists). NAME is lowercase letters, digits and dashes, and not base, a built-in layer's (docker, nix) or a shared tool's. Then devvm build NAME builds its image, and devvm node+NAME makes a VM with it.

```text
devvm layer new NAME [flags]
```

## Options

```text
  -h, --help   help for new
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm layer](devvm_layer.md): Your own image layers
