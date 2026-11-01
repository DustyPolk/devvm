---
title: devvm space
weight: 38
---

Show what devvm keeps on this machine and the space it takes

## Synopsis

Show each kind of thing devvm keeps on this machine, on every backend, with how many there are, the disk they take, whether devvm can make them again, what devvm gc frees of them now and what frees the rest, then a total. The backends' own caches come last and apart, because they can also hold your own downloads. With --backend, only that backend's VMs, images, shared tool images and caches. It changes nothing and starts no VM.

```text
devvm space [flags]
```

## Options

```text
      --backend NAME   the NAME of the only backend to act on (default: every backend)
  -h, --help           help for space
      --json           print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
