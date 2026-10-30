---
title: Installing
weight: 1
---

## Requirements

devvm runs on macOS 13 or newer and on Linux, on arm64 or x86_64. Its VMs run on a [backend](../backends/_index.md): [Lima](../backends/lima.md#requirements), which needs Lima 2.0 or newer, and on Linux QEMU and access to `/dev/kvm`; or the experimental [smolvm](../backends/smolvm.md#requirements), or [Docker](../backends/docker.md#requirements), which needs Docker 29.1 or newer (Docker Desktop on macOS).

ssh and editors reach VMs through devvm's ssh config, which needs OpenSSH 8.7 or newer (`ssh -V`; macOS 13's and Ubuntu 22.04's have it): older versions refuse its `PermitRemoteOpen` ([details](../using/ssh.md#ssh-access-by-name)).

Docker is optional: only the Docker backend, and `--from` with an image only Docker has, need it ([details](../using/from.md#images-only-docker-has)). Everything else (toolchains, agents, Docker for your projects with the [docker layer](../using/images.md#built-in-layers)) is installed inside the VMs.

## Installing devvm

With Homebrew, `brew install w3cj/tap/devvm`, which also installs Lima. The formula's checksum lives in a repository of its own, so a replaced GitHub release wouldn't match it.

Or with the install script, which needs `curl` and `tar` and never needs sudo:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Options go after `bash -s --`:

- `--yes`: answer yes to every question.
- `--version 0.1.0`: install that release (also `DEVVM_VERSION=0.1.0`).
- `--no-deps`: install devvm only.
- `DEVVM_PREFIX=/some/dir`: install under that folder instead of `~/.local`.

Before you make your first VM, read [staying safe](../safety/_index.md#the-habits), which lists the habits that close most of what devvm leaves open.

## Checking what devvm needs

devvm never installs anything on the host itself. Every command that needs the backend checks it first, and stops at the first thing missing.

`devvm doctor` checks the backend new VMs get (`--backend NAME` for another) and lists everything it lacks, each with what to do: the command that installs it, where there's one to run. It also lists each backend's [features](../internals/backends.md#features) and whether Docker is installed, then says devvm has everything it needs, or fails.

## Upgrading

Run the install script again, or `brew upgrade devvm`. Images and VMs stay: they live in `~/.local/share/devvm`, and your settings, layers and features in `~/.config/devvm`, not with devvm. Images whose inputs changed rebuild on next use; existing VMs keep working (`devvm update` moves one to the new shared tools). `devvm gc` deletes the copies of devvm's files older versions wrote out, except those a VM's proxy was started with.

## Uninstalling

1. Delete your VMs: `devvm ls`, then `devvm rm` in each folder. For a `--clone` VM, this also removes the `devvm` git remote from the repo.
2. Delete the images: `devvm gc`, then `devvm images rm` with every name `devvm images` lists. Images built `--from` a container image need the same `--from`: `devvm --from node:24 images rm base`.
3. Remove devvm: `rm -rf ~/.local/lib/devvm ~/.local/bin/devvm`, or `brew uninstall devvm`.
4. Remove its state: `rm -rf ~/.local/share/devvm ~/.config/devvm`. That includes your layers and features (`~/.config/devvm/layers`, `~/.config/devvm/features`): copy them somewhere first to keep them.
5. If you let devvm add it, remove the `Include ~/.config/devvm/ssh_config` line from `~/.ssh/config`.
6. Saved tokens are in the macOS Keychain under the service `devvm` (on Linux, in `~/.config/devvm/secrets/`, removed in step 4).
