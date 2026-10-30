---
title: Installing
weight: 1
---

This page gets devvm installed and working on your machine, then covers upgrading and uninstalling it.

## Requirements

devvm runs its VMs on a [backend](../backends/_index.md), and its default backend is [Lima](../backends/lima.md#requirements). This table lists what devvm needs for Lima, how to check each requirement and how to get it, and whether devvm's install script offers to install it for you:

| You need | To check | To get it | The install script |
| --- | --- | --- | --- |
| macOS 13 or newer, or Linux, on arm64 or x86_64 | `sw_vers` on macOS, `uname -m` | | doesn't |
| Lima 2.0 or newer | `limactl --version` | `brew install lima`, or [Lima's install guide](https://lima-vm.io/docs/installation/) | offers it: Homebrew's when you have Homebrew, otherwise Lima's latest release into `~/.local` |
| On Linux, QEMU | `qemu-img --version` | your package manager: apt, dnf, pacman or zypper | offers it, with sudo |
| On Linux, access to `/dev/kvm` | `ls -l /dev/kvm` | `sudo usermod -aG kvm $USER`, then log out and back in | offers it, with sudo |
| OpenSSH 8.7 or newer, for ssh and editors by name | `ssh -V` | macOS 13's and Ubuntu 22.04's have it | doesn't |
| Docker 29.1 or newer, only for the Docker backend or `--from` with an image only Docker has | `docker version` | Docker Desktop on macOS, Docker's engine on Linux | doesn't |

The Homebrew formula installs Lima along with devvm. ssh and editors reach VMs through devvm's ssh config, and OpenSSH versions older than 8.7 refuse its `PermitRemoteOpen` ([SSH access by name](../using/ssh.md#ssh-access-by-name)). Docker on this machine is optional, because only the [Docker backend](../backends/docker.md#requirements) and `--from` with an [image only Docker has](../using/from.md#images-only-docker-has) need it. The experimental [smolvm](../backends/smolvm.md#requirements) backend has requirements of its own.

Everything else is installed inside the VMs, including the toolchains, the agents, and Docker for your projects with the [docker layer](../using/images.md#built-in-layers).

On Linux hosts, devvm has a few limits, such as no `.git` protection, that [Known limits](../using/limits.md#linux-hosts) lists.

## Installing devvm

To install devvm with Homebrew, which also installs Lima, run:

```sh
brew install w3cj/tap/devvm
```

To install it with the install script instead, run the line below. The script needs `curl` and `tar`, and it never needs sudo for devvm itself.

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

The script takes options after `bash -s --`, such as `curl -fsSL … | bash -s -- --yes`:

- `--yes`: answer yes to every question.
- `--version 0.1.0`: install that release. `DEVVM_VERSION=0.1.0` does the same.
- `--no-deps`: install devvm only.
- `DEVVM_PREFIX=/some/dir`: install under that folder instead of `~/.local`.

The script checks the download's checksum and signature before it installs anything, as [Installing and updating](../internals/install.md) explains.

## Checking what devvm needs

devvm never installs anything on this machine itself. Every command that needs the backend checks it first, and stops at the first thing that's missing.

`devvm doctor` checks the backend new VMs get, or another one with `--backend NAME`. It lists everything that backend lacks, each with what to do, which is the command that installs it when there's one to run. It also lists what each backend can do, which it calls the backend's [features](../internals/backends.md#features), and whether Docker is installed. Then it says devvm has everything it needs, or it fails.

When `devvm doctor` finds nothing missing, you're ready for [your first VM](first-vm.md). Before you make it, read [staying safe](../safety/_index.md#the-habits), which lists the habits that close most of what devvm leaves open.

## Upgrading

To upgrade devvm, run the install script again, or run `brew upgrade devvm`. Your images and VMs stay, because they live in `~/.local/share/devvm`, and your settings, layers and features stay in `~/.config/devvm`, apart from devvm itself.

Images whose inputs changed are built again the next time you use them. Existing VMs keep working, and `devvm update` moves a VM to the new shared tools. `devvm gc` deletes the copies of devvm's files that older versions wrote out, except those that a VM's proxy was started with.

## Uninstalling

1. Delete your VMs: `devvm ls`, then `devvm rm` in each folder. For a `--clone` VM, this also removes the `devvm` git remote from the repo.
2. Delete the images: `devvm gc`, then `devvm images rm` with every name `devvm images` lists. Images built `--from` a container image need the same `--from`: `devvm --from node:24 images rm base`.
3. Remove devvm: `rm -rf ~/.local/lib/devvm ~/.local/bin/devvm`, or `brew uninstall devvm`.
4. Remove its state: `rm -rf ~/.local/share/devvm ~/.config/devvm`. That includes your layers and features, in `~/.config/devvm/layers` and `~/.config/devvm/features`, so copy them somewhere first to keep them.
5. If you let devvm add it, remove the `Include ~/.config/devvm/ssh_config` line from `~/.ssh/config`.
6. Saved secrets are in the macOS Keychain under the service `devvm`. On Linux, they're in `~/.config/devvm/secrets/`, which step 4 removes.
