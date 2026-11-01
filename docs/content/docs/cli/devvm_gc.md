---
title: devvm gc
weight: 8
---

Delete stale images and unused tool images

## Synopsis

Delete the stale and unfinished images on every backend, the shared tools devvm images lists as unused, the files devvm itself no longer needs, and the shared Nix cache's least recently used files while it's over its cap, after listing them and asking in a terminal. Its flags delete more, such as --tools for the current shared tools no VM has. gc never deletes a VM.

```text
devvm gc [--tools [--recent] [TOOL...]] [flags]
```

## Examples

```text
  devvm gc                             # delete stale images and unused tools, after asking
  devvm gc --tools                     # also the current shared tools no VM has
  devvm gc --history --older-than 30d  # also the history exports 30 days old or more
  devvm gc --backend smolvm            # delete every image and unused tool of the smolvm backend
  devvm gc --force                     # delete without asking, as a script must
```

## Options

```text
      --backend NAME          act only on the backend NAME (default: every backend)
      --caches                then free devvm's share of the backends' own caches, asking for each backend
  -f, --force                 don't ask to confirm, which devvm needs without a terminal
  -h, --help                  help for gc
      --history               also delete the saved exports of agents' history
      --nix-cache             also delete the whole shared Nix cache
      --older-than DURATION   with --history, only those older than DURATION, such as 30d
      --orphans               also delete what devvm keeps for VMs their backend no longer has
      --recent                with --tools, also delete those built in the last hour
      --retention             then apply the retention settings
      --tools                 also delete the current shared tools no VM has, or only those of the TOOLs named
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm gc](../using/images.md#cleaning-up) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
