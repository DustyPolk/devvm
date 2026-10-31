---
title: devvm setup
weight: 36
---

Choose the default backend, base's features and new VMs' size

## Synopsis

Choose the backend new VMs get by default (lima, or smolvm, experimental), then the optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, and your own in ~/.config/devvm/features, then the CPUs and memory new VMs get. In a terminal it asks for all three, starting from the current choices, and for the size that's what devvm size -g saved, else base's size fitted to this machine. --backend, --features, --cpus and --memory each choose without asking, and what they leave out stays as it is. A new choice of features makes base and the images built on it rebuild when next used. A new size changes only the VMs made after it.

```text
devvm setup [flags]
```

## Options

```text
      --backend NAME    the NAME of the backend new VMs get by default
      --cpus N          the N CPUs new VMs get, without asking
      --features LIST   the LIST of features base gets, without asking: fzf,omb, or '' for none
  -h, --help            help for setup
      --memory GiB      the memory new VMs get in GiB, without asking: 16, 16G or 16GiB
```

## Options inherited from parent commands

```text
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: on, asked in a terminal)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
