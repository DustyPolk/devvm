---
title: devvm secret
weight: 31
---

List the secrets kept on this machine, for VMs to use

## Synopsis

List the secrets devvm keeps on this machine, whether each is saved, and the VMs using it: claude, the Claude Code token every VM whose image has claude uses, and github, the GitHub token of the VMs it's bound to. With network policy a VM gets a placeholder, and its proxy puts the secret into requests for the secret's hosts. Without network policy, the VM gets the secret itself.

```text
devvm secret [flags]
```

## Options

```text
  -h, --help   help for secret
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
- [devvm secret rm](devvm_secret_rm.md): Delete a saved token
- [devvm secret set](devvm_secret_set.md): Save the Claude Code token or GitHub token on this machine
- [devvm secret unuse](devvm_secret_unuse.md): Stop this folder's VM using a token
- [devvm secret use](devvm_secret_use.md): Let this folder's VM use a token
