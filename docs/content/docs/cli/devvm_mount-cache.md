---
title: devvm mount-cache
explainer: "Show or change how this folder's VM caches its project folder"
---

Show or change how this folder's VM caches its project folder

## Synopsis

Show how this folder's VM caches its project folder on a Linux host: none, which new VMs get, or mmap for a VM made before devvm chose none, which can lose writes and breaks SQLite and Gradle there. devvm mount-cache none switches it from the VM's next start, so in a terminal devvm offers to restart a running VM now.

```text
devvm mount-cache [none] [flags]
```

## Examples

```text
  devvm mount-cache       # show the VM's cache mode
  devvm mount-cache none  # stop the VM losing writes, from its next start
```

## Options

```text
  -h, --help     help for mount-cache
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

[The docs page for devvm mount-cache](../backends/lima.md#the-project-folders-cache) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
