---
title: devvm setup
weight: 36
---

Choose the default backend, base's features, and new VMs' size and network policy

## Synopsis

Choose the backend new VMs get by default (lima, or docker and smolvm, both experimental), then the optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, desktop, and your own in ~/.config/devvm/features. In a terminal it asks for both, starting from the current choices, then for the CPUs and memory new VMs get, as devvm size -g sets them, and for the network policy new VMs get: ask (each new VM asks, and gets it without a terminal), on or off. --backend and --features each choose without asking anything, and what they leave out stays as it is. A new choice of features makes base and the images built on it rebuild when next used. The size and network policy change only the VMs made after them.

```text
devvm setup [flags]
```

## Options

```text
      --backend NAME    the NAME of the backend new VMs get by default
      --features LIST   the LIST of features base gets, without asking: fzf,omb, or none
  -h, --help            help for setup
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
