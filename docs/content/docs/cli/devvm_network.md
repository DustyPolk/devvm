---
title: devvm network
weight: 19
---

Show or change the network policy of this folder's VM

## Synopsis

Show the network policy of this folder's VM, on, off or unenforced (on, but not confined: started outside devvm, or before it was turned on), then its allowlist: each policy file that has rules, in order (its image's, yours for every VM, the VM's own, its secrets' hosts), and the rules in it. devvm network on or off changes the VM's network policy from its next start: devvm restart. Secrets the VM got while it was off still work once it's on. With -g, show or change whether new VMs get network policy instead: ask (in a terminal, and on without one), on or off. devvm's --network-policy and --no-network-policy still choose for one new VM.

```text
devvm network [on|off] [flags]
```

## Options

```text
  -g, --global   show or change whether new VMs get network policy instead
  -h, --help     help for network
      --json     print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: devvm network -g's, else asked in a terminal, and on without one)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm network allow](devvm_network_allow.md): Allow hosts for this folder's VM
- [devvm network deny](devvm_network_deny.md): Deny hosts for this folder's VM
- [devvm network log](devvm_network_log.md): Show what this folder's VM connected to and what its network policy blocked
- [devvm network rm](devvm_network_rm.md): Remove rules from this folder's VM
