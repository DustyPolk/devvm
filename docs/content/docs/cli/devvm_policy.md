---
title: devvm policy
weight: 10
---

Show the network policy allowlist of this folder's VM

## Synopsis

Show the allowlist of this folder's VM, which its network policy proxy reads: each policy file that has rules, in order (its image's, yours for every VM, the VM's own, its tokens' hosts), and the rules in it. devvm policy allow, deny and rm change the VM's own rules, or with -g yours for every VM; a change applies at once, to the next connection.

```text
devvm policy [flags]
```

## Options

```text
  -h, --help   help for policy
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
- [devvm policy allow](devvm_policy_allow.md): Allow hosts for this folder's VM
- [devvm policy deny](devvm_policy_deny.md): Deny hosts for this folder's VM
- [devvm policy rm](devvm_policy_rm.md): Remove rules from this folder's VM
