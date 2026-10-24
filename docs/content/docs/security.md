---
title: Security model
weight: 25
---

devvm treats everything in a VM as untrusted, a coding agent with root included: the VM's user has passwordless sudo. This is what that code can and can't reach on your machine.

| | |
| --- | --- |
| The project folder | read and write |
| `.git` hooks and config | locked (macOS); what a lock can't stop is reported, not prevented ([details](git-protection.md)) |
| The rest of your home folder, other projects | not mounted |
| Your SSH keys | not reachable: VMs have no SSH agent |
| The network | everything this machine reaches, its own services on `localhost` included: network policy isn't available yet |

## What devvm protects, and how

### Only the project folder

A VM mounts its project folder, read-write, at the same path, and nothing else from the host: not your home folder, which Lima's own templates mount.

devvm refuses a project folder that would hand the VM more than the project:

- one that is or holds your home folder, devvm's settings (`~/.config/devvm`), its state (`~/.local/share/devvm`, with its binaries and other VMs' disks) or its Lima home;
- one inside devvm's own folders: a VM in `~/.config/devvm/layers` could rewrite the scripts every image build runs, one in its data folder the files devvm runs on the host.

`--unsafe-folder` mounts such a folder anyway, saying so.

### One VM per folder

A VM belongs to the folder it was made for: its path, and the folder itself (its device and inode, and on Linux its birth time). A folder deleted and made again at the same path, another repo cloned where an old one was, never quietly gets the old folder's VM and what's in its disk: devvm refuses it.

### Read-only and confined builds

Images are built with nothing from the host mounted, and their builds are confined: they reach the internet but not your machine's own services ([details](lima.md#confinement)). devvm's VMs live in a Lima home of their own, so Lima settings you made for other VMs never reach them ([details](lima.md#a-lima-home-of-its-own)).

### `.git` protection

The VM can't change the parts of `.git` that run code on the host: devvm locks hooks and config with a flag the VM can't clear ([details](git-protection.md)).

## What it doesn't protect

- **Files the VM writes into the project folder.** Review changes before you run project code on the host (`npm install`, `make`, editor tasks, hooks in the working tree).
- **The network.** Network policy isn't available yet, so a VM reaches anything this machine can, its services on `localhost` (other VMs' ports, dev servers, databases) and your local network included. devvm creates a VM only with `--no-network-policy`, so none runs without it by mistake.
- **Ports the VM listens on**, which are forwarded to this machine's `localhost`: all of them, unless you list them with [`--ports`](commands.md#size-and-ports). A VM could take a port a program on the host trusts.
- **Set-id bits on Linux hosts.** Root in a VM can set the set-user-id and set-group-id bits on files in the project folder. On macOS they don't reach the host (virtiofs drops them). On Linux they do, and the files are yours, so a set-user-id one runs as you for anyone else on the machine who runs it. devvm names such files when it creates a VM for the folder (`chmod u-s,g-s FILE` clears them). Don't run programs from the project folder as another user.
- **Escapes from the VM itself**: bugs in Lima, Apple's Virtualization framework or QEMU.
