---
title: devvm ssh-agent rm
weight: 41
---

Remove a key from devvm's own SSH agent

## Synopsis

Remove a key added with devvm ssh-agent add from devvm's own SSH agent, and from ~/.config/devvm/agent-keys, so devvm doesn't load it again. The key file stays as it is, and needn't still exist. A key that wasn't added is refused: devvm ssh-agent lists those that were.

```text
devvm ssh-agent rm KEY_FILE [flags]
```

## Options

```text
  -h, --help   help for rm
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

- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
