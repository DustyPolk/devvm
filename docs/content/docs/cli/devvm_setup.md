---
title: devvm setup
weight: 36
---

Choose the default backend and base's features

## Synopsis

Choose the backend new VMs get by default (lima, or smolvm, experimental), then the optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, and your own in ~/.config/devvm/features. In a terminal it asks for both, starting from the current choices. --backend chooses the backend without asking, and with only it the features stay as they are; --features chooses the features without asking, and with only it the backend stays as it is. A new choice of features makes base and the images built on it rebuild when next used.

```text
devvm setup [flags]
```

## Options

```text
      --backend NAME    the NAME of the backend new VMs get by default
      --features LIST   the LIST of features base gets, without asking: fzf,omb, or '' for none
  -h, --help            help for setup
```

## Options inherited from parent commands

```text
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
