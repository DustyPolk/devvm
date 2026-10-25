---
title: devvm log
weight: 10
---

Show what this folder's VM connected to and what its network policy blocked

## Synopsis

Show the network log of this folder's VM, which has network policy: one line per connection or request its proxy allowed or blocked, oldest first, with the time, the host and port, and the rule that allowed it, the request and the tokens put in, or why it was blocked. --blocked lists the blocks instead, one row per host and port, with the devvm policy allow command that allows each.

```text
devvm log [flags]
```

## Options

```text
      --blocked   list the blocks by host and port, with the command that allows each
  -f, --follow    then show each new one as it happens, until interrupted
  -h, --help      help for log
  -n, --lines N   show the last N connections and requests (default 50)
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
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
