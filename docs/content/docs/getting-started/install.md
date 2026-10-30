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

It installs the release it comes with, or `--version`'s:

1. It downloads the archive for this machine (`devvm-<version>-<os>-<arch>.tar.gz`: `darwin` or `linux`, `arm64` or `amd64`; Apple silicon's under Rosetta too) and the release's `SHA256SUMS`, over HTTPS only.
2. It checks `SHA256SUMS`'s signature (`SHA256SUMS.minisig`, made with the maintainer's minisign key, whose public half is in the script and in `packaging/minisign.pub`), then the archive's checksum. A mismatch installs nothing. Without [minisign](https://jedisct1.github.io/minisign/) (`brew install minisign`), it says so, prints the command to check by hand, and checks only the checksum. A script that isn't from a release has no key, and checks only the checksum.
3. It puts devvm, one binary with its README, changelog and license, in `~/.local/lib/devvm/<version>`, links `~/.local/bin/devvm` to it, and says how to add `~/.local/bin` to `PATH` when it's missing.
4. It runs `devvm doctor --fixes` and offers each fix, asking first ([details](#checking-what-devvm-needs)): Lima with Homebrew when you have it, else Lima's latest release (checked against its `SHA256SUMS`) into `~/.local`; on Linux, QEMU with your package manager and access to `/dev/kvm` (joining the `kvm` group: log out and back in afterwards), which use sudo. Without a terminal or `--yes`, it installs none. Then it runs `devvm doctor`.

On macOS, the [Docker](../backends/docker.md#requirements) backend later fetches one more file the first time a Docker VM or build needs it: the same release's Linux archive, whose devvm it checks against the SHA-256 the installed devvm has of it before using it. Homebrew's devvm has its Linux build already, built from source.

The signature catches an archive or `SHA256SUMS` someone replaced, which the checksum alone can't. It can't catch a release replaced whole: `curl … | bash` runs the `install.sh` from that same release, key included, so the one-liner trusts GitHub's release files. Homebrew doesn't. You can also download `install.sh` and read it before you run it.

Options go after `bash -s --`:

- `--yes`: answer yes to every question.
- `--version 0.1.0`: install that release (also `DEVVM_VERSION=0.1.0`).
- `--no-deps`: install devvm only.
- `DEVVM_PREFIX=/some/dir`: install under that folder instead of `~/.local`.

Before you make your first VM, read [staying safe](../safety/attack-vectors.md#staying-safe), which lists the habits that close most of what devvm leaves open.

## Checking what devvm needs

devvm never installs anything on the host itself. Every command that needs the backend checks it first, and stops at the first thing missing.

`devvm doctor` checks the backend new VMs get (`--backend NAME` for another) and lists everything it lacks, each with what to do: the command that installs it, where there's one to run. It also lists each backend's [features](../backends/_index.md#features) and whether Docker is installed, then says devvm has everything it needs, or fails.

`devvm doctor --fixes` prints only what's missing, for scripts: one line each, `KEY<tab>FIX<tab>MESSAGE`. `FIX` is a command, or empty when there's none to run (virtualization turned off in the firmware, say). It succeeds either way. The keys:

| Key | Missing | Fix |
| --- | --- | --- |
| `os`, `arch` | a host the backend runs on | none |
| `macos` | macOS 13 or newer | none |
| `lima` | Lima 2.0 or newer, working | `brew install lima` (`upgrade`, `reinstall`) with Homebrew, else none |
| `qemu` | QEMU, on Linux | your package manager's (apt, dnf, pacman or zypper), else none |
| `kvm` | `/dev/kvm`, on Linux | none: turn on virtualization |
| `kvm-access` | access to `/dev/kvm` | `sudo usermod -aG kvm USER`, then log in again |
| `lima-config` | no Lima global configs in devvm's Lima home ([details](../backends/lima.md#a-lima-home-of-its-own)) | none: move them out |
| `docker` | Docker, for the Docker backend | none: install Docker Desktop, or Docker's engine on Linux |
| `docker-engine` | Docker's engine, running and reachable | none: start it |
| `docker-access` | access to Docker's socket, on Linux | `sudo usermod -aG docker USER`, then log in again |
| `docker-version`, `docker-os`, `docker-arch` | Docker 29.1 or newer, running Linux containers of this machine's processor | none |

devvm is one binary: its own files (`base.yaml`, base's scripts and policy, the features, the shared tools' config and build script, what it sends into VMs, `--from`'s conversion) are built into it, so a copy of it runs anywhere. What needs a path (Lima's `base.yaml`, the folder `--from`'s build VM mounts, the policy files VMs' proxies read) is written out to `~/.local/share/devvm/assets/<hash>`, a folder per version of those files.

## Upgrading

Run the install script again, or `brew upgrade devvm`. Images and VMs stay: they live in `~/.local/share/devvm`, and your settings, layers and features in `~/.config/devvm`, not with devvm. Images whose inputs changed rebuild on next use; existing VMs keep working (`devvm update` moves one to the new shared tools). `devvm gc` deletes the copies of devvm's files older versions wrote out, except those a VM's proxy was started with.

The script removes older versions in `~/.local/lib/devvm`, except one that a running VM's proxy (or QEMU, on Linux) still runs: a later install removes it once nothing does. A VM's proxy moves to the new version at its next launch (`devvm` in its folder), QEMU when the VM next starts.

## Uninstalling

1. Delete your VMs: `devvm ls`, then `devvm rm` in each folder. For a `--clone` VM, this also removes the `devvm` git remote from the repo.
2. Delete the images: `devvm gc`, then `devvm images rm` with every name `devvm images` lists. Images built `--from` a container image need the same `--from`: `devvm --from node:24 images rm base`.
3. Remove devvm: `rm -rf ~/.local/lib/devvm ~/.local/bin/devvm`, or `brew uninstall devvm`.
4. Remove its state: `rm -rf ~/.local/share/devvm ~/.config/devvm`. That includes your layers and features (`~/.config/devvm/layers`, `~/.config/devvm/features`): copy them somewhere first to keep them.
5. If you let devvm add it, remove the `Include ~/.config/devvm/ssh_config` line from `~/.ssh/config`.
6. Saved tokens are in the macOS Keychain under the service `devvm` (on Linux, in `~/.config/devvm/secrets/`, removed in step 4).
