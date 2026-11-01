---
title: devvm
weight: 1
---

Open a shell in this folder's VM, creating it first

## Synopsis

Open a shell in this folder's VM, at the same path, creating the VM first. A new VM gets IMAGE (base, layers such as docker or your own, and the shared tools joined with +: node+python+docker+claude), else the toolchains (and the nix layer) the folder's files call for, with any agents IMAGE names on top, built if it isn't. Only the folder is mounted, read-write, with the shared tools read-only, and its .git hooks and config are locked against changes from the VM (macOS); with --clone, nothing is mounted but the shared tools, and the VM gets its own clone of the repo. A new VM also gets what the folder's devcontainer.json says it can, unless --no-devcontainer. A VM that exists is started if it's stopped and opened as it is; an IMAGE that names what it lacks is refused, naming the devvm tools add command that adds its shared tools, as only a new VM gets a layer. In a folder without a VM of its own, inside one that has a VM, devvm asks whether to open that VM in this folder instead of creating one, and opens it without asking when there's no terminal. --here and --clone create the new VM without asking.

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
      --here                create a VM for this folder when it has none, even when a folder it's inside has one, without asking whether to use that VM
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: on, asked in a terminal)
      --no-devcontainer     create the VM without reading the folder's .devcontainer/devcontainer.json or .devcontainer.json
      --no-git-protect      create the VM without .git protection
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
      --ssh-agent           create the VM with your SSH agent forwarded: while you're connected, anything in it can use your keys (devvm ssh-agent changes it later)
      --unsafe-folder       mount a folder that is or holds your home folder or devvm's own, or is inside devvm's own, anyway
      --version             print devvm's version
```

## See also

- [devvm adopt](devvm_adopt.md): Use the VM made for an earlier folder at this path here
- [devvm build](devvm_build.md): Build an image and the images under it, unless they're built
- [devvm code](devvm_code.md): Open this folder's VM in VS Code over Remote-SSH
- [devvm completion](devvm_completion.md): Print the shell completion script for bash, zsh or fish
- [devvm desktop](devvm_desktop.md): Open this folder's VM's desktop in a VNC viewer
- [devvm doctor](devvm_doctor.md): Check what devvm needs on this machine
- [devvm gc](devvm_gc.md): Delete stale images and unused tool images
- [devvm git-protect](devvm_git-protect.md): Show or change .git protection for this folder's VM
- [devvm history](devvm_history.md): List the saved exports of agents' history
- [devvm host](devvm_host.md): Get this folder's VM ready for ssh and editors, and show its host name
- [devvm images](devvm_images.md): List the images and the shared tools
- [devvm layer](devvm_layer.md): Your own image layers
- [devvm limactl](devvm_limactl.md): Run Lima's limactl on devvm's own Lima home
- [devvm log](devvm_log.md): Show what this folder's VM connected to and what its network policy blocked
- [devvm ls](devvm_ls.md): List the project VMs
- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
- [devvm policy](devvm_policy.md): Show the network policy allowlist of this folder's VM
- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
- [devvm rebuild](devvm_rebuild.md): Build an image again, and delete the old one
- [devvm restart](devvm_restart.md): Restart this folder's VM and open a shell in it
- [devvm retention](devvm_retention.md): Show or change the retention settings: what devvm deletes on its own
- [devvm rm](devvm_rm.md): Delete this folder's VM
- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
- [devvm setup](devvm_setup.md): Choose the default backend, base's features, and new VMs' size and network policy
- [devvm size](devvm_size.md): Show or change this folder's VM's size
- [devvm space](devvm_space.md): Show what devvm keeps on this machine and the space it takes
- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
- [devvm stop](devvm_stop.md): Stop this folder's VM
- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
