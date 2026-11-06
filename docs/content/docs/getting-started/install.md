---
title: Installing
weight: 1
---

This page gets devvm installed and working on your machine, then covers upgrading and uninstalling it.

## Installing devvm

To install devvm, run the command for your system:

{{< install >}}

The install script needs `curl`, `tar`, and `sha256sum` or `shasum` to check what it downloads, and it never needs sudo for devvm itself. It takes options after `bash -s --`, such as `curl -fsSL … | bash -s -- --yes`:

- `--yes`: answer yes to every question.
- `--version 0.1.0`: install that release. `DEVVM_VERSION=0.1.0` does the same.
- `--no-deps`: install devvm only.
- `--force`: install even when Homebrew installed devvm, which leaves you with two copies ([One devvm](#one-devvm)).
- `DEVVM_PREFIX=/some/dir`: install under that folder instead of `~/.local`.

The script checks the download's checksum and signature before it installs anything, as [Installing and updating](../internals/install.md) explains.

### One devvm

Install devvm one way, with Homebrew or with the install script, but not both. With two copies, the one that comes first on your `PATH` is the one that runs, and `brew upgrade devvm` may update the one you don't run. So when Homebrew installed devvm, the install script stops before it downloads anything and says to run `brew upgrade devvm` instead, or `brew uninstall devvm` first if you want to use the script. It stops whether or not it runs in a terminal, and `--force` installs a second copy anyway.

To switch from one way to the other, remove devvm with the command for the way you installed it, as step 4 of [Uninstalling](#uninstalling) shows, and then install it the other way. Your images, VMs and settings stay, because they don't live with devvm.

After it installs devvm, the script warns when another devvm comes before `~/.local/bin` on your `PATH`. The warning names that copy's path and says to remove it, or to put `~/.local/bin` before its folder in `PATH`. `devvm doctor` warns the same way when the devvm first on your `PATH` isn't the one you ran, naming both, and the warning doesn't make it fail.

## Requirements

devvm runs its VMs on [Lima](../backends/lima.md#requirements), its default [backend](../backends/_index.md). It's tested on Apple silicon Macs and on Linux on both arm64 and x86_64, but not yet on Intel Macs ([Known limits](../using/limits.md#intel-macs)). For your system, the table lists what devvm needs, how to check each requirement and how to get it, and whether the install script offers to install it for you.

{{< os-tabs >}}
{{< os-tab name="macOS" >}}

| You need | To check | To get it | The install script |
| --- | --- | --- | --- |
| macOS at [Lima's minimum](../backends/lima.md#requirements), on arm64 or x86_64 | `sw_vers` and `uname -m` | | {{< no >}} doesn't |
| 8 GiB of memory and 4 CPUs, recommended | `sysctl hw.memsize hw.ncpu` | | {{< no >}} doesn't |
| Lima at [its minimum](../backends/lima.md#requirements) | `limactl --version` | `brew install lima`, or [Lima's install guide](https://lima-vm.io/docs/installation/) | {{< yes >}} offers it: Homebrew's when you have Homebrew, otherwise Lima's latest release into `~/.local` |
| OpenSSH 8.7 or newer, for ssh and editors by name | `ssh -V` | macOS 13's has it | {{< no >}} doesn't |
| Docker Desktop, only for `--from` with an image only Docker has | `docker version` | Docker Desktop | {{< no >}} doesn't |

{{< /os-tab >}}
{{< os-tab name="Linux" >}}

| You need | To check | To get it | The install script |
| --- | --- | --- | --- |
| Linux on arm64 or x86_64 | `uname -m` | | {{< no >}} doesn't |
| 8 GiB of memory and 4 CPUs, recommended | `free -g` and `nproc` | | {{< no >}} doesn't |
| Lima at [its minimum](../backends/lima.md#requirements) | `limactl --version` | `brew install lima`, or [Lima's install guide](https://lima-vm.io/docs/installation/) | {{< yes >}} offers it: Homebrew's when you have Homebrew, otherwise Lima's latest release into `~/.local` |
| QEMU | `qemu-img --version` | your package manager: apt, dnf, pacman or zypper | {{< yes >}} offers it, with sudo |
| Access to `/dev/kvm` | `ls -l /dev/kvm` | `sudo usermod -aG kvm $USER`, then log out and back in | {{< yes >}} offers it, with sudo |
| OpenSSH 8.7 or newer, for ssh and editors by name | `ssh -V` | Ubuntu 22.04's has it | {{< no >}} doesn't |
| Docker's engine, only for `--from` with an image only Docker has | `docker version` | Docker's engine | {{< no >}} doesn't |

{{< /os-tab >}}
{{< /os-tabs >}}

The Homebrew formula installs Lima along with devvm. ssh and editors reach VMs through devvm's ssh config, and OpenSSH versions older than 8.7 refuse its `PermitRemoteOpen` ([SSH access by name](../using/ssh.md#ssh-access-by-name)). Docker on this machine is optional, because only `--from` with an [image only Docker has](../using/from.md#images-only-docker-has) needs it.

devvm also runs on a smaller machine, because it gives base fewer CPUs and less memory there, and the VMs made from base get the same unless you give them a size, as [size and ports](../using/ports-size.md#size-and-ports) explains. `devvm doctor` warns when this machine has less than 8 GiB of memory or fewer than 4 CPUs.

Your first VM needs at least 8 GiB of free disk space, because devvm builds the base image first and then the VM's own image and shared tools. Each VM then grows its own disk as you work in it, and the disk never shrinks. devvm checks the free space before each build and each new VM, and refuses when it's low ([free space](../using/ports-size.md#free-space)).

Everything else is installed inside the VMs, including the toolchains, the agents, and Docker for your projects with the [docker layer](../using/images.md#built-in-layers). You don't install Claude Code, Codex or opencode on this machine, because the VMs get their settings and logins from devvm's own [agent store](../using/coding-agents.md#devvms-agent-store).

On Linux hosts, devvm has a few limits, such as no `.git` protection, that [Known limits](../using/limits.md#linux-hosts) lists.

## Checking what devvm needs

devvm never installs anything on this machine itself. Every command that needs the backend checks it first, and stops at the first thing that's missing.

`devvm doctor` checks what [Lima](../backends/lima.md#requirements), devvm's backend, needs. It lists everything that's missing, each with what to do, which is the command that installs it when there's one to run. It then lists what Lima can do, which it calls Lima's [features](../internals/backends.md#features), each as `native`, `emulated`, `unreviewed` for an emulation that devvm refuses until it's reviewed, or `none`. It also says whether Docker is installed, which only `--from` with [an image only Docker has](../using/from.md#images-only-docker-has) needs. When `HTTPS_PROXY` is set and `~/.config/devvm/ca` has no CA certificates, it says that a proxy that inspects TLS needs its CA there ([a company's CA](../using/settings.md#a-companys-ca)). It warns when another devvm comes first on your `PATH` ([One devvm](#one-devvm)), when the disk that devvm's images and VMs go on has less free space than building base needs, when this machine has less than the 8 GiB of memory or the 4 CPUs that devvm recommends ([Requirements](#requirements)), and for each line of your policy files that isn't a rule, which VMs' proxies skip ([rules](../safety/network.md#rules)). Then it says devvm has everything it needs, or it fails. The warnings never make it fail, so a small machine with everything installed passes.

When `devvm doctor` finds nothing missing, you're ready for [your first VM](first-vm.md). Before you make it, read [staying safe](../safety/_index.md#the-habits), which lists the habits that close most of what devvm leaves open.

## Upgrading

To upgrade devvm, run `devvm upgrade`, which installs the newest release the way you installed devvm:

```sh
devvm upgrade
```

It says which release it will install in place of the devvm you have, and asks first. `devvm upgrade --force` installs it without asking, which devvm needs without a terminal or with `--no-input`, and `devvm upgrade --check` says whether a newer release is out and installs nothing.

You can also upgrade devvm without devvm upgrade, with the command for the way you installed it:

{{< os-tabs >}}
{{< os-tab name="macOS" >}}

```sh
brew upgrade devvm
```

If you installed devvm with the install script instead, run the script again, as on Linux.

{{< /os-tab >}}
{{< os-tab name="Linux" >}}

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Running the install script again installs the newest release.

{{< /os-tab >}}
{{< /os-tabs >}}

Your images and VMs stay, because they live in `~/.local/share/devvm`, and your settings and layers stay in `~/.config/devvm`, apart from devvm itself.

Images whose inputs changed are built again the next time you use them. Existing VMs keep working, and `devvm tools update` moves a VM to the new shared tools. `devvm gc` deletes the copies of devvm's files that older versions wrote out, except those that a VM's proxy was started with.

### How devvm upgrade upgrades

devvm upgrade tells how devvm was installed from where the devvm you ran is, and it never runs anything to find out:

- **With the install script**, devvm is `~/.local/lib/devvm/<version>/devvm`, and `~/.local/bin/devvm` links to it, or the same folders under `DEVVM_PREFIX`. devvm upgrade downloads the new release and checks it, as [What devvm upgrade checks](#what-devvm-upgrade-checks) describes. It unpacks the release into a folder of its own beside the devvm you have, runs the new devvm to see that it works, and then switches `~/.local/bin/devvm` to it. When anything fails before the new devvm runs through `~/.local/bin/devvm`, the link goes back, so the devvm you had stays the one that runs. The version you upgraded from stays in `~/.local/lib/devvm`, and so does any older version that a running VM still uses, until a later upgrade finds nothing using it.
- **With Homebrew**, devvm is in Homebrew's Cellar, and devvm upgrade runs `brew upgrade devvm` after asking. Homebrew may upgrade Lima at the same time, because the formula needs it. Homebrew installs the release its formula names, which can come out a little after the release itself, so until then `brew upgrade devvm` finds nothing to upgrade.
- **Any other way**, such as `go install`, a build from devvm's source, or a copy of devvm somewhere else, devvm upgrade can't upgrade. It says how that devvm was installed and how to upgrade it, such as with `go install github.com/w3cj/devvm/cmd/devvm@latest`, and fails.

A devvm built without devvm's release key can't check a release's signature, so devvm upgrade refuses to install a release with it and says to run the install script again.

### What devvm upgrade checks

devvm upgrade downloads over HTTPS only, and checks each file before it uses the next:

1. It downloads the release's `SHA256SUMS` and its signature, `SHA256SUMS.minisig`, and checks that devvm's release key signed it for that release's version. The key is built into the devvm you have, so a replaced release can't bring its own.
2. It downloads the archive for this machine and checks it against its checksum in `SHA256SUMS`.
3. It unpacks the archive, which must hold only devvm's files in one folder.
4. It runs the new devvm with `--version`, which must say the release's version.

When a check fails, devvm upgrade installs nothing, and the devvm you have stays the one that runs. It checks the signature itself, so it doesn't need minisign on this machine, as the install script does. It never runs a script that it downloaded, only the new devvm's `--version`.

### An older release

When the newest release is older than the devvm you have, devvm upgrade refuses to install it, even with `--force`. A server that offers an old release in place of the newest could bring back problems that a newer release fixed, so devvm upgrade never goes back a version unless you name it. `devvm upgrade --to VERSION` installs the release you name, newer or older, and it asks first, saying when the release is older than the devvm you have.

Homebrew installs only its formula's release, so with Homebrew devvm upgrade doesn't take `--to`.

### devvm upgrade --json

`devvm upgrade --json` prints what `devvm upgrade --check` says as one JSON object, for scripts, as [output for scripts](commands.md#output-for-scripts) describes, and installs nothing:

- `current` is the version of the devvm you ran.
- `latest` is the newest release's version.
- `newer` says whether that's newer than `current`.
- `install` is how devvm was installed, `install.sh`, `homebrew` or `other`.
- `path` is where the devvm you ran is, its links followed.

```json
{
  "version": 1,
  "current": "0.1.0",
  "latest": "0.2.0",
  "newer": true,
  "install": "install.sh",
  "path": "/Users/you/.local/lib/devvm/0.1.0/devvm"
}
```

## Uninstalling

1. Delete your VMs: `devvm ls`, then `devvm rm` in each folder. For a `--clone` VM, this also removes the `devvm` git remote from the repo.
2. Delete the images: `devvm gc`, then `devvm images rm` with every name `devvm images` lists. Images built `--from` a container image need the same `--from`: `devvm images rm --from node:24 base`.
3. Free Lima's cache. In a terminal, `devvm gc --caches` frees devvm's share of it, after asking. If you don't use Lima yourself, you can also empty its cache: Lima keeps Ubuntu's cloud images in `~/Library/Caches/lima` on macOS and `~/.cache/lima` on Linux, and `limactl prune` deletes that whole cache.
4. Delete your saved secrets: `devvm secret rm NAME` for each one `devvm secret` lists. On macOS they're in the Keychain under the service `devvm`, and on Linux in `~/.local/share/devvm/secrets/`.
5. Remove devvm itself, with the command for the way you installed it:

   {{< os-tabs >}}
   {{< os-tab name="macOS" >}}

   ```sh
   brew uninstall devvm
   ```

   If you installed devvm with the install script instead, remove it as on Linux.

   {{< /os-tab >}}
   {{< os-tab name="Linux" >}}

   ```sh
   rm -rf ~/.local/lib/devvm ~/.local/bin/devvm
   ```

   {{< /os-tab >}}
   {{< /os-tabs >}}

6. Remove its state: `rm -rf ~/.local/share/devvm ~/.config/devvm`. That includes your layers, in `~/.config/devvm/layers`, so copy them somewhere first to keep them. It also includes your exports, the agents' history that `devvm rm` saved, and devvm's agent store, the agents' settings in `~/.config/devvm/agents` and their logins in `~/.local/share/devvm/logins`. On macOS, the Time Machine exclusions that devvm set on its CA folders and logins folders go with the folders.
7. If you let devvm add it, remove the `Include ~/.local/share/devvm/ssh_config` line from `~/.ssh/config`.
