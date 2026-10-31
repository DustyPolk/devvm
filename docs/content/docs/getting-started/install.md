---
title: Installing
weight: 1
---

This page gets devvm installed and working on your machine, then covers upgrading and uninstalling it.

## Installing devvm

To install devvm, run the command for your system:

{{< install >}}

The install script needs `curl` and `tar`, and it never needs sudo for devvm itself. It takes options after `bash -s --`, such as `curl -fsSL … | bash -s -- --yes`:

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

devvm runs its VMs on [Lima](../backends/lima.md#requirements), its default [backend](../backends/_index.md). For your system, the table lists what devvm needs, how to check each requirement and how to get it, and whether the install script offers to install it for you.

{{< os-tabs >}}
{{< os-tab name="macOS" >}}

| You need | To check | To get it | The install script |
| --- | --- | --- | --- |
| macOS 13 or newer, on arm64 or x86_64 | `sw_vers` and `uname -m` | | {{< no >}} doesn't |
| 8 GiB of memory and 4 CPUs, recommended | `sysctl hw.memsize hw.ncpu` | | {{< no >}} doesn't |
| Lima 2.0 or newer | `limactl --version` | `brew install lima`, or [Lima's install guide](https://lima-vm.io/docs/installation/) | {{< yes >}} offers it: Homebrew's when you have Homebrew, otherwise Lima's latest release into `~/.local` |
| OpenSSH 8.7 or newer, for ssh and editors by name | `ssh -V` | macOS 13's has it | {{< no >}} doesn't |
| Docker Desktop with Docker 29.1 or newer, only for the Docker backend or `--from` with an image only Docker has | `docker version` | Docker Desktop | {{< no >}} doesn't |

{{< /os-tab >}}
{{< os-tab name="Linux" >}}

| You need | To check | To get it | The install script |
| --- | --- | --- | --- |
| Linux on arm64 or x86_64 | `uname -m` | | {{< no >}} doesn't |
| 8 GiB of memory and 4 CPUs, recommended | `free -g` and `nproc` | | {{< no >}} doesn't |
| Lima 2.0 or newer | `limactl --version` | `brew install lima`, or [Lima's install guide](https://lima-vm.io/docs/installation/) | {{< yes >}} offers it: Homebrew's when you have Homebrew, otherwise Lima's latest release into `~/.local` |
| QEMU | `qemu-img --version` | your package manager: apt, dnf, pacman or zypper | {{< yes >}} offers it, with sudo |
| Access to `/dev/kvm` | `ls -l /dev/kvm` | `sudo usermod -aG kvm $USER`, then log out and back in | {{< yes >}} offers it, with sudo |
| OpenSSH 8.7 or newer, for ssh and editors by name | `ssh -V` | Ubuntu 22.04's has it | {{< no >}} doesn't |
| Docker's engine 29.1 or newer, only for the Docker backend or `--from` with an image only Docker has | `docker version` | Docker's engine | {{< no >}} doesn't |

{{< /os-tab >}}
{{< /os-tabs >}}

The Homebrew formula installs Lima along with devvm. ssh and editors reach VMs through devvm's ssh config, and OpenSSH versions older than 8.7 refuse its `PermitRemoteOpen` ([SSH access by name](../using/ssh.md#ssh-access-by-name)). Docker on this machine is optional, because only the [Docker backend](../backends/docker.md#requirements) and `--from` with an [image only Docker has](../using/from.md#images-only-docker-has) need it. The experimental [smolvm](../backends/smolvm.md#requirements) backend has requirements of its own.

devvm also runs on a smaller machine, because it fits base to the machine. Base asks for 4 CPUs and 8 GiB of memory. On a machine with fewer than 4 CPUs, base gets as many CPUs as the machine has, and when 8 GiB would leave the machine less than 2 GiB of its own, base gets half of the machine's memory, which is 4 GiB on an 8 GiB machine. The VMs made from base get the same unless you give them a size, as [size and ports](../using/ports-size.md#size-and-ports) explains. When devvm builds base on such a machine, it says how many CPUs and how much memory base gets. `devvm size -g` sets the size that new VMs get instead, and `devvm doctor` warns when this machine has less than 8 GiB of memory or fewer than 4 CPUs.

Your first VM needs at least 8 GiB of free disk space, because devvm builds the base image first and then the VM's own image and shared tools. Each VM then grows its own disk as you work in it, and the disk never shrinks. devvm checks the free space before each build and each new VM, and refuses when it's low ([free space](../using/ports-size.md#free-space)).

Everything else is installed inside the VMs, including the toolchains, the agents, and Docker for your projects with the [docker layer](../using/images.md#built-in-layers).

On Linux hosts, devvm has a few limits, such as no `.git` protection, that [Known limits](../using/limits.md#linux-hosts) lists.

## Checking what devvm needs

devvm never installs anything on this machine itself. Every command that needs the backend checks it first, and stops at the first thing that's missing.

`devvm doctor` checks the backend new VMs get, or another one with `--backend NAME`. It lists everything that backend lacks, each with what to do, which is the command that installs it when there's one to run. It also lists what each backend can do, which it calls the backend's [features](../internals/backends.md#features), and whether Docker is installed. It warns when another devvm comes first on your `PATH` ([One devvm](#one-devvm)), when the disk that devvm's images and VMs go on has less free space than building base needs, and when this machine has less than the 8 GiB of memory or the 4 CPUs that devvm recommends ([Requirements](#requirements)). Then it says devvm has everything it needs, or it fails. The warnings never make it fail, so a small machine with everything installed passes.

When `devvm doctor` finds nothing missing, you're ready for [your first VM](first-vm.md). Before you make it, read [staying safe](../safety/_index.md#the-habits), which lists the habits that close most of what devvm leaves open.

## Upgrading

To upgrade devvm, run the command for the way you installed it:

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

Your images and VMs stay, because they live in `~/.local/share/devvm`, and your settings, layers and features stay in `~/.config/devvm`, apart from devvm itself.

Images whose inputs changed are built again the next time you use them. Existing VMs keep working, and `devvm update` moves a VM to the new shared tools. `devvm gc` deletes the copies of devvm's files that older versions wrote out, except those that a VM's proxy was started with.

## Uninstalling

1. Delete your VMs: `devvm ls`, then `devvm rm` in each folder. For a `--clone` VM, this also removes the `devvm` git remote from the repo.
2. Delete the images: `devvm gc`, then `devvm images rm` with every name `devvm images` lists. Images built `--from` a container image need the same `--from`: `devvm --from node:24 images rm base`.
3. Delete your saved secrets: `devvm secret rm NAME` for each one `devvm secret ls` lists. On macOS they're in the Keychain under the service `devvm`, and on Linux in `~/.config/devvm/secrets/`.
4. Remove devvm itself, with the command for the way you installed it:

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

5. Remove its state: `rm -rf ~/.local/share/devvm ~/.config/devvm`. That includes your layers and features, in `~/.config/devvm/layers` and `~/.config/devvm/features`, so copy them somewhere first to keep them.
6. If you let devvm add it, remove the `Include ~/.config/devvm/ssh_config` line from `~/.ssh/config`.
