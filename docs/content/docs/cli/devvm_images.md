---
title: devvm images
weight: 13
---

List the images and the shared tools

## Synopsis

List the images on every backend, by name: what each one's base started from and its digest, its key, whether it's current, stale (its inputs changed since) or unfinished (a build that didn't finish), the disk it takes and its backend. Then the shared tool images, by the name images have them by: each version, the backend and C library it's built for, the disk it takes, how many VMs use it, and whether it's the tools lock's version, in use, unused, or the lock's and not built yet. Then what they take in all, and what the shared Nix cache takes of its cap. With --backend, only that backend's images and shared tool images.

```text
devvm images [flags]
```

## Options

```text
      --backend NAME   the NAME of the only backend to act on (default: every backend)
  -h, --help           help for images
      --json           print it as one JSON object, for scripts
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm images rm](devvm_images_rm.md): Delete images, by name or by key
