---
title: devvm ssh-agent add
weight: 41
---

Load a key into devvm's own SSH agent

## Synopsis

Load a private key into devvm's own SSH agent, starting it first, and record it in ~/.config/devvm/agent-keys with its fingerprint and options, replacing what was recorded for it. devvm loads it again when a VM needs it (after a reboot, or once its lifetime ran out), only while the file holds the same key. A key in a folder a VM mounts is refused.

```text
devvm ssh-agent add KEY_FILE [flags]
```

## Options

```text
      --confirm             ask before each use of the key, through an ssh-askpass program
  -h, --help                help for add
      --lifetime DURATION   how long the agent keeps the key once loaded: a DURATION like 30m or 8h, or forever (default 12h)
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

- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
