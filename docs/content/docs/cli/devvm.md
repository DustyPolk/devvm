---
title: devvm
weight: 1
---

Open a shell in this folder's VM, creating it first

## Synopsis

Open a shell in this folder's VM, at the same path, and create the VM first when the folder has none. A new VM gets IMAGE, such as node+claude, or else the toolchains the folder's files call for with any agents IMAGE names, and devvm builds the image first if it isn't built. The VM mounts only this folder read-write, and with --clone it gets its own clone of the repo instead.

```text
devvm [IMAGE] [flags]
```

## Examples

```text
  devvm              # open this folder's VM, creating it with the toolchains the folder needs
  devvm node+claude  # create the VM with node and Claude Code
  devvm --clone      # create the VM with its own clone of this repo
  devvm ls           # list every VM
```

## Options

```text
      --backend NAME        the backend NAME to use (default: devvm setup's, else lima)
      --clone               create the VM with its own clone of this repo instead of mounting the folder
      --cpus N              the N CPUs the VM gets (default: devvm size -g, else the image's)
      --disk GiB            the VM's disk in GiB, at least the image's (default: devvm size -g, else the image's)
      --from IMAGE          the container IMAGE base starts from, such as node:24, instead of Ubuntu's cloud image (default: $DEVVM_FROM)
  -h, --help                help for devvm
      --here                create a VM for this folder even when a folder it's inside has one
      --memory GiB          the VM's memory in GiB, such as 16 or 16G (default: devvm size -g, else the image's)
      --network-policy      create the VM with network policy (default: devvm network -g, else asked in a terminal, and on without one)
      --no-devcontainer     create the VM without reading the folder's devcontainer.json
      --no-git-protect      create the VM without .git protection
      --no-input            never ask, and fail where devvm would ask, naming the flag that answers it
      --no-network-policy   create the VM without network policy
      --ports PORTS         the PORTS the VM forwards: all, none or a list like 5173,8000-8010 (default: devvm ports -g, else all)
      --ssh-agent           create the VM with your SSH agent forwarded, so anything in it can use your keys while you're connected
      --ssh-config          add devvm's Include line to ~/.ssh/config without asking
      --unsafe-folder       mount a folder devvm refuses, such as your home folder
  -v, --verbose             show the output of the commands devvm runs instead of a status line
      --version             print devvm's version
```

## Docs

[The docs page for devvm](../getting-started/commands.md) explains it in full.

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
- [devvm layer](devvm_layer.md): Make image layers of your own
- [devvm limactl](devvm_limactl.md): Run Lima's limactl for debugging, which starts VMs without network policy
- [devvm link](devvm_link.md): List this folder's VM's links to your other VMs
- [devvm ls](devvm_ls.md): List the project VMs
- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
- [devvm ports](devvm_ports.md): Show or change which ports this folder's VM forwards
- [devvm rebuild](devvm_rebuild.md): Build an image again, and delete the old one
- [devvm restart](devvm_restart.md): Restart this folder's VM and open a shell in it
- [devvm retention](devvm_retention.md): Show or change what devvm deletes on its own
- [devvm rm](devvm_rm.md): Delete this folder's VM
- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
- [devvm setup](devvm_setup.md): Choose the default backend, base's features, and new VMs' size and network policy
- [devvm size](devvm_size.md): Show or change this folder's VM's size
- [devvm space](devvm_space.md): Show what devvm keeps on this machine and the space it takes
- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
- [devvm stop](devvm_stop.md): Stop this folder's VM
- [devvm tools](devvm_tools.md): Show this folder's VM's shared tools
