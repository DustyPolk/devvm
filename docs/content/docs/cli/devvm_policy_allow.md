---
title: devvm policy allow
weight: 20
---

Allow hosts for this folder's VM

## Synopsis

Add RULEs to the allowlist of this folder's VM, or with -g of every VM. They apply at once, to the next connection. RULE is a host (example.com), any subdomain of one (*.example.com), any host (**), an address or CIDR (10.0.0.5, fd00::/8), each with an optional port (example.com:8443, [fd00::1]:22); without a port, 443 and 80.

```text
devvm policy allow RULE... [flags]
```

## Options

```text
  -g, --global   change your rules for every VM instead
  -h, --help     help for allow
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

- [devvm policy](devvm_policy.md): Show the network policy allowlist of this folder's VM
