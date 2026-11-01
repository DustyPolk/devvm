---
title: devvm ports
weight: 23
---

Show or change which ports this folder's VM forwards

## Synopsis

Show which ports this folder's VM forwards to the same port on this machine's localhost: all, none, or a LIST of ports and ranges like 5173,8000-8010. devvm ports all, none or LIST changes it at once: a stopped VM forwards the new ports from its next start, and a running one's forwarder starts again with them, closing the connections open through it, while the VM runs on. With -g, show or change what new VMs forward instead.

```text
devvm ports [all|none|LIST] [flags]
```

## Options

```text
  -g, --global   show or change what new VMs forward instead
  -h, --help     help for ports
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
- [devvm ports add](devvm_ports_add.md): Forward more ports from this folder's VM
- [devvm ports rm](devvm_ports_rm.md): Stop forwarding some ports from this folder's VM
