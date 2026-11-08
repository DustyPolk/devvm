---
title: devvm clipboard
weight: 8
explainer: "Show or change whether this folder's VM reads your clipboard's images"
---

Show or change whether this folder's VM reads your clipboard's images

## Synopsis

Show whether this folder's VM reads the image on your clipboard when you paste into a coding agent in it, as images or off. devvm clipboard images or off changes it at once, on a running VM too. With images on, anything in the VM can read the image on your clipboard while the VM runs, but never its text.

```text
devvm clipboard [images|off] [flags]
```

## Examples

```text
  devvm clipboard         # show whether the VM reads your clipboard's images
  devvm clipboard images  # let agents in the VM paste the image on your clipboard
  devvm clipboard off     # stop it
```

## Options

```text
  -h, --help     help for clipboard
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

[The docs page for devvm clipboard](../using/clipboard.md) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
