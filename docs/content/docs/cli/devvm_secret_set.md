---
title: devvm secret set
weight: 34
---

Save a token on this machine

## Synopsis

Save a token on this machine, claude or github: in the macOS Keychain, else in ~/.config/devvm/secrets/NAME, a file only you can read. It asks for the token without showing it; --stdin reads a line instead. A claude token comes from claude setup-token. Proxies running put the new token in at once.

```text
devvm secret set NAME [flags]
```

## Options

```text
  -h, --help    help for set
      --stdin   read the token from a line of input, without asking
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

- [devvm secret](devvm_secret.md): Tokens kept on this machine, for VMs to use
