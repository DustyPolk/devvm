---
title: devvm setup
weight: 41
---

Choose the default backend, base's features, and new VMs' size and network policy

## Synopsis

Choose the backend new VMs get by default, the features base gets, and new VMs' CPUs, memory and network policy. In a terminal devvm asks for each, starting from the current choice, and --backend and --features choose without asking. A new choice of features rebuilds base and the images on it when they're next used.

```text
devvm setup [flags]
```

## Examples

```text
  devvm setup                     # answer every question
  devvm setup --backend smolvm    # make smolvm the default backend, without asking
  devvm setup --features omb,fzf  # give base oh-my-bash and fzf, without asking
  devvm setup --features none     # give base no features
```

## Options

```text
      --backend NAME    the backend NAME new VMs get by default
      --features LIST   the LIST of features base gets, such as fzf,omb, or none
  -h, --help            help for setup
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm setup](../using/settings.md#devvm-setup) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
