---
title: devvm
weight: 1
---

Open a shell in this folder's VM, creating it first

## Synopsis

Open a shell in this folder's VM, at the same path, creating the VM first. A new VM gets IMAGE (base, your layers and the shared tools joined with +: node+python+claude), else the toolchains the folder's files call for, with any agents IMAGE names on top, built if it isn't. Only the folder is mounted, read-write, with the shared tools read-only, and its .git hooks and config are locked against changes from the VM (macOS); with --clone, nothing is mounted but the shared tools, and the VM gets its own clone of the repo. A VM that exists is started if it's stopped and opened as it is; an IMAGE that names what it lacks adds that in place, after asking.

```text
devvm [IMAGE] [flags]
```

## Options

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --clone               create the VM with its own clone of this repo instead of mounting the folder: your branches are copied in, and the repo gets a devvm remote to fetch its work from
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
  -h, --help                help for devvm
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)
      --no-git-protect      create the VM without .git protection
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
      --ssh-agent           create the VM with your SSH agent forwarded: while you're connected, anything in it can use your keys (devvm ssh-agent changes it later)
      --unsafe-folder       mount a folder that is or holds your home folder or devvm's own, or is inside devvm's own, anyway
      --version             print devvm's version
```

## See also

- [devvm adopt](devvm_adopt.md): Use the VM made for an earlier folder at this path here
- [devvm auth](devvm_auth.md): Save the Claude token: devvm secret set claude
- [devvm build](devvm_build.md): Build an image and the images under it, unless they're built
- [devvm completion](devvm_completion.md): Print the shell completion script for bash, zsh or fish
- [devvm doctor](devvm_doctor.md): Check what devvm needs on this machine
- [devvm git-protect](devvm_git-protect.md): Show or change .git protection for this folder's VM
- [devvm log](devvm_log.md): Show what this folder's VM connected to and what its network policy blocked
- [devvm ls](devvm_ls.md): List the project VMs
- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
- [devvm policy](devvm_policy.md): Show the network policy allowlist of this folder's VM
- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
- [devvm rebuild](devvm_rebuild.md): Build an image again, and delete the old one
- [devvm remove](devvm_remove.md): Take shared tools out of this folder's VM, then open a shell in it
- [devvm restart](devvm_restart.md): Restart this folder's VM and open a shell in it
- [devvm rm](devvm_rm.md): Delete this folder's VM
- [devvm secret](devvm_secret.md): Tokens kept on this machine, for VMs to use
- [devvm size](devvm_size.md): Show or change this folder's VM's size
- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
- [devvm update](devvm_update.md): Move this folder's VM to the newest shared tools, then open a shell in it
