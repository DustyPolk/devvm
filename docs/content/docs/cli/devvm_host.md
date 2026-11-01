---
title: devvm host
weight: 12
---

Get this folder's VM ready for ssh and editors, and show its host name

## Synopsis

Get this folder's VM ready for ssh and editors as devvm does before opening a shell: checked, started if it's stopped, its network policy checked, set up and in devvm's ssh config. Then show the host name ssh and VS Code's Remote-SSH reach it by (devvm-website, for a folder named website). Without the Include line in ~/.ssh/config, devvm offers to add it.

```text
devvm host [flags]
```

## Options

```text
  -h, --help   help for host
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: on, asked in a terminal)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
