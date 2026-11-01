---
title: devvm ssh-agent
weight: 39
---

Show or change SSH agent forwarding for this folder's VM

## Synopsis

Show whether this folder's VM forwards an SSH agent: off (the default), on (yours) or devvm (devvm's own, which holds only the keys added to it with devvm ssh-agent add). Then, once a key is added, it lists the keys devvm's own agent holds, as ssh-add -l does, loading those it lacks first, and each key added to it: its file, its fingerprint and how it's loaded. devvm ssh-agent on, devvm or off changes the VM's forwarding from its next start: devvm restart. While it's on and you're connected, anything in the VM can use the agent's keys.

```text
devvm ssh-agent [on|devvm|off] [flags]
```

## Options

```text
  -h, --help   help for ssh-agent
      --json   print it as one JSON object, for scripts
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
- [devvm ssh-agent add](devvm_ssh-agent_add.md): Load a key into devvm's own SSH agent
- [devvm ssh-agent rm](devvm_ssh-agent_rm.md): Remove a key from devvm's own SSH agent
