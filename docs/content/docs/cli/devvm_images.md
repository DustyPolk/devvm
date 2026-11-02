---
title: devvm images
weight: 16
---

List the images and the shared tools

## Synopsis

List the images on every backend, with whether each is current, stale or unfinished and the disk it takes, then the shared tools devvm has built, with how many VMs have each. It ends with what they take in all.

```text
devvm images [flags]
```

## Examples

```text
  devvm images                 # list the images and the shared tools
  devvm images --backend lima  # only the lima backend's
  devvm images --json          # the same lists as JSON, for scripts
```

## Options

```text
      --backend NAME   act only on the backend NAME (default: every backend)
  -h, --help           help for images
      --json           print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm images](../using/images.md#listing-images) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm images rm](devvm_images_rm.md): Delete images, by name or by key
