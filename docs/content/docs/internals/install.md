---
title: Installing and updating
weight: 2
---

This page explains what the install script and devvm doctor do, and what devvm keeps on this machine.

## The install script

It installs the release it comes with, or `--version`'s:

1. It downloads the archive for this machine (`devvm-<version>-<os>-<arch>.tar.gz`: `darwin` or `linux`, `arm64` or `amd64`; Apple silicon's under Rosetta too) and the release's `SHA256SUMS`, over HTTPS only.
2. It checks `SHA256SUMS`'s signature (`SHA256SUMS.minisig`, made with the maintainer's minisign key, whose public half is in the script and in `packaging/minisign.pub`), then the archive's checksum. A mismatch installs nothing. Without [minisign](https://jedisct1.github.io/minisign/) (`brew install minisign`), it says so, prints the command to check by hand, and checks only the checksum. A script that isn't from a release has no key, and checks only the checksum.
3. It puts devvm, one binary with its README, changelog and license, in `~/.local/lib/devvm/<version>`, links `~/.local/bin/devvm` to it, and says how to add `~/.local/bin` to `PATH` when it's missing.
4. It runs `devvm doctor --fixes` and offers each fix, asking first ([details](../getting-started/install.md#checking-what-devvm-needs)): Lima with Homebrew when you have it, else Lima's latest release (checked against its `SHA256SUMS`) into `~/.local`; on Linux, QEMU with your package manager and access to `/dev/kvm` (joining the `kvm` group: log out and back in afterwards), which use sudo. Without a terminal or `--yes`, it installs none. Then it runs `devvm doctor`.

On macOS, the [Docker](../backends/docker.md#requirements) backend later fetches one more file the first time a Docker VM or build needs it: the same release's Linux archive, whose devvm it checks against the SHA-256 the installed devvm has of it before using it. Homebrew's devvm has its Linux build already, built from source.

The signature catches an archive or `SHA256SUMS` someone replaced, which the checksum alone can't. It can't catch a release replaced whole: `curl … | bash` runs the `install.sh` from that same release, key included, so the one-liner trusts GitHub's release files. Homebrew doesn't: the formula's checksum lives in a repository of its own, so a replaced GitHub release wouldn't match it. You can also download `install.sh` and read it before you run it.

## devvm doctor --fixes

`devvm doctor --fixes` prints only what's missing, for scripts: one line each, `KEY<tab>FIX<tab>MESSAGE`. `FIX` is a command, or empty when there's none to run (virtualization turned off in the firmware, say). It succeeds either way. The keys:

| Key | Missing | Fix |
| --- | --- | --- |
| `os`, `arch` | a host the backend runs on | none |
| `macos` | macOS 13 or newer | none |
| `lima` | Lima 2.0 or newer, working | `brew install lima` (`upgrade`, `reinstall`) with Homebrew, else none |
| `qemu` | QEMU, on Linux | your package manager's (apt, dnf, pacman or zypper), else none |
| `kvm` | `/dev/kvm`, on Linux | none: turn on virtualization |
| `kvm-access` | access to `/dev/kvm` | `sudo usermod -aG kvm USER`, then log in again |
| `lima-config` | no Lima global configs in devvm's Lima home ([details](lima.md#a-lima-home-of-its-own)) | none: move them out |
| `docker` | Docker, for the Docker backend | none: install Docker Desktop, or Docker's engine on Linux |
| `docker-engine` | Docker's engine, running and reachable | none: start it |
| `docker-access` | access to Docker's socket, on Linux | `sudo usermod -aG docker USER`, then log in again |
| `docker-version`, `docker-os`, `docker-arch` | Docker 29.1 or newer, running Linux containers of this machine's processor | none |

## devvm's own files

devvm is one binary: its own files (`base.yaml`, base's scripts and policy, the features, the shared tools' config and build script, what it sends into VMs, `--from`'s conversion) are built into it, so a copy of it runs anywhere. What needs a path (Lima's `base.yaml`, the folder `--from`'s build VM mounts, the policy files VMs' proxies read) is written out to `~/.local/share/devvm/assets/<hash>`, a folder per version of those files.

## Upgrading

The script removes older versions in `~/.local/lib/devvm`, except one that a running VM's proxy (or QEMU, on Linux) still runs: a later install removes it once nothing does. A VM's proxy moves to the new version at its next launch (`devvm` in its folder), QEMU when the VM next starts.
