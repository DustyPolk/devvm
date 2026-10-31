---
title: What devvm keeps on this machine
weight: 11
---

devvm, the tools it runs and its backends keep files on this machine outside your project folders. This page lists each of them: what it is, who makes it, where it is, whether devvm can make it again, and how it's cleaned up. Most of it is in devvm's data folder, `~/.local/share/devvm`, and devvm can make all of that again except your exports. The backends' own caches hold more, and they can also hold your own downloads.

## devvm space

`devvm space` shows what devvm keeps on this machine, on every backend, and how much disk each kind of thing takes. It changes nothing and starts no VM, and it gives each backend 30 seconds to answer. `devvm space --json` prints the same as JSON, for scripts. The first table has a row for each kind of thing devvm has, followed by a total:

| Column | |
| --- | --- |
| `WHAT` | `VMs`, `images`, `tools gnu` or `tools musl` for the shared tools' builds for each C library, `exports`, `network logs`, `run logs`, `mise`, `devvm's files` for the copies of devvm's own files, `leftover state` for what devvm keeps of VMs that their backend no longer has, and `state` for the rest of devvm's data folder |
| `BACKEND` | the backend that has them, for VMs, images and shared tools |
| `COUNT`, `SIZE` | how many there are and the disk they take, in powers of 1024 (`2.1G`) |
| `KEPT` | `remade` when devvm makes it again when it's needed, `only copy` for VMs and exports, which nothing else has, `log` for logs, `state` for what devvm knows of its VMs and images, and `leftover` |
| `GC` | what `devvm gc` frees of it now |
| `FREED BY` | what frees the rest, such as `devvm rm`, or `gc --backend lima`, which deletes all of a backend's images |

Sizes count each file's blocks once, as `du` does. On macOS a Lima VM's disk shares its image's blocks until the VM writes them, so those blocks count for both the VM and its image, and the total can be more than the disk they take. A shared tool's build shows `-` as its backend when devvm can't tell which backend it's for. When a backend that one of your VMs uses doesn't answer, devvm says so, and its VMs and images aren't counted.

### The backends' caches

The second table lists the caches that belong to the backends, with the backend, what each cache holds, how many entries it has, its size and where it is. These are Lima's download cache, smolvm's caches on macOS and the Docker volumes with devvm's label that aren't one VM's own, such as the shared tools' volume. devvm's builds fill them, but Lima's and smolvm's caches can also hold your own downloads, so they aren't in the total. The sections below say what each one holds.

## devvm's data folder

devvm keeps its state in `~/.local/share/devvm`, or in `$XDG_DATA_HOME/devvm` when `XDG_DATA_HOME` is set. Only you can read it. Everything in it is devvm's own, and devvm makes all of it again when it's needed, except the exports, which are the only copy of a deleted VM's agent history.

| Path | What it is | How it's cleaned up |
| --- | --- | --- |
| `vms/<vm>` | each VM's [state](settings.md#devvms-state): its image, folder, backend and settings | `devvm rm` deletes it with the VM |
| `images/` | each image's record, the [rebuild counters](images.md#rebuilding) and the digests of the container images that `--from` bases start from | `devvm gc` and `devvm images rm` delete an image's record with the image |
| `tools/` | the [shared tools](images.md#shared-tools), one read-only image per tool version, each with its checksum beside it in a `.sha256` file and what it's built for in a `.target` file | `devvm gc` deletes the versions that no VM has and that aren't the current ones, and `devvm gc --tools` also the current ones that no VM has |
| `mise.lock`, `mise.lock.toml`, `mise.lock.checked` | the tools lock, which holds the tools' versions and their checksums, the config it was resolved from, and when devvm last checked upstream for newer versions | devvm replaces them when it checks again |
| `mise/<version>` | the mise that devvm runs on this machine to resolve the tools lock, one folder per mise version | `devvm gc` deletes the folders of older versions, which an upgrade of devvm leaves |
| `exports/` | the agents' [history](coding-agents.md#history) that `devvm export` and `devvm rm` save, one `.tar.gz` per export | nothing deletes them, so you delete the ones you don't need |
| `logs/runs/` | the [run logs](troubleshooting.md#a-command-fails), one per devvm run that runs a command | devvm keeps the newest 20 |
| `logs/<vm>.jsonl`, `logs/<vm>.proxy.log` | a VM's network log, which holds its proxy's decisions, and its proxy's own output | `devvm rm` deletes them with the VM |
| `ca/` | the certificate authority that VMs trust for their proxy | devvm makes a new one when the old one has less than a week of its 30 days left |
| `assets/<hash>` | devvm's own files, written out where a program needs them at a path, one folder per version of devvm's files | `devvm gc` deletes the folders of older versions, except one that a running VM's proxy was started with |
| `run/` | files for running VMs and for builds, such as sandbox profiles, proxy pids and the ports lock | devvm deletes a VM's files when it deletes the VM |
| `agent.sock` | the socket of [devvm's own SSH agent](ssh.md#devvms-own-agent) | it takes no space |
| `oci-build.lock` | a folder that holds the pid of the devvm that builds a base from a container image on Lima | devvm deletes it when the build ends |
| `lima/` | devvm's own [Lima home](../backends/lima.md#devvms-lima-home), with its VMs and images, unless `DEVVM_LIMA_HOME` moves it | see [Lima](#lima) |
| `lima-vms/` | what the Lima backend sets on each VM: which ports it forwards, and its port forwarder's files | `devvm rm` deletes a VM's files with the VM |
| `smolvm/` | the smolvm backend's images, which are packs, and what it sets on each VM | see [smolvm](#smolvm) |
| `docker/` | what the Docker backend sets on each VM, the shared tools it unpacked, and devvm's Linux build | see [Docker](#docker) |

A build also makes a temporary folder in the data folder, `tools-build.<random>` for the shared tools, and devvm deletes it when the build ends, even after a failure or Ctrl+C.

### Shared tools

Each tool version is built once for each kind of VM that uses it, so `tools/` can hold more than one build of the same version. Lima's VMs mount compressed images, smolvm's need uncompressed ones, and Docker unpacks its own builds into a volume. A base on Alpine also needs its tools built for its C library, musl, rather than glibc. A node 24 that both a Lima VM and a smolvm VM use is therefore two builds, and `devvm images` lists both. [Cleaning up](images.md#cleaning-up) says which ones `devvm gc` deletes.

### Exports

`devvm rm` saves the history of a VM whose image has an agent before it deletes the VM, so each export in `exports/` can be the only copy of that VM's transcripts and sessions. devvm never deletes an export. To free the space, delete the files you don't need, which are named `<vm>-<date>-<time>.tar.gz`.

### Leftovers of a VM deleted outside devvm

When you delete a VM with its backend's own command instead of `devvm rm`, such as `limactl delete`, `smolvm machine delete` or `docker rm`, devvm still keeps the VM's state, its logs, its files in `run/` and its backend's folder, its ssh config entry and its host keys, and `docker rm` leaves the VM's volumes and networks in Docker. `devvm gc` also keeps the shared tools that the VM's state names. `devvm rm` deletes these only together with the VM, so once the VM is gone it says the folder has no VM.

## Your settings

`~/.config/devvm` holds your settings, your own layers and features, your company's CA certificates, the ssh config for running VMs and their host keys, and on Linux hosts your secrets. [Your settings](settings.md#your-settings) lists each file. devvm never deletes any of them.

## Lima

The Lima backend's VMs and images are in devvm's Lima home, `~/.local/share/devvm/lima`. Each image is a stopped VM named `devvm-img-<key>`, and each project VM is a clone of one. On macOS a clone's disk shares its image's blocks until the VM writes them, so `du` counts those blocks twice. On Linux hosts a VM's disk is an overlay of its image's. `devvm rm` deletes a VM, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend lima` deletes all of them.

Lima keeps a download cache of its own, `~/Library/Caches/lima` on macOS and `~/.cache/lima` on Linux. When devvm builds base, Lima downloads Ubuntu's cloud image into that cache, which is 600 to 900 MB for each release, and on macOS it also keeps a copy converted to a raw disk, which takes about 2.4 GB more. The cache is Lima's and it isn't in devvm's Lima home, so your own Lima VMs' downloads share it. devvm never deletes anything in it. `limactl prune` deletes the whole cache, your own downloads included, and Lima downloads what a VM needs again the next time it makes one.

## smolvm

The smolvm backend keeps each image as a pack in `~/.local/share/devvm/smolvm/images/<key>`, and base's is about 450 MB. A VM is a smolvm machine made from its image's pack. The machine reads the pack when its disk grows, so devvm doesn't delete an image that a VM was made from. `smolvm/vms/` holds what devvm sets on each VM. `devvm rm` deletes a VM, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend smolvm` deletes all of them but those a VM was made from.

On macOS, smolvm keeps every machine, devvm's and your own, in its own folders under `~/Library/Caches/smolvm`, and devvm marks its machines with a label. That folder also holds smolvm's caches, which grow with every pack that a machine is made from. smolvm unpacks a pack's disk once, which takes 1 to 3 GB, and keeps it for the next machine made from that pack. Those copies stay after devvm deletes the pack. Because the folder also holds the machines' own disks, never delete it by hand. [Cleaning smolvm's caches](../backends/smolvm.md#cleaning-smolvms-caches) says what smolvm's own commands clean. On Linux, smolvm's state and caches belong to devvm alone, in `~/.local/share/devvm/smolvm/state`.

## Docker

The Docker backend keeps its VMs and images in Docker's engine. Each VM is a container with a volume for its home, and a `--clone` VM has another for its clone. devvm's images are Docker images named `devvm-<hash>:<key>`. The shared tools are unpacked into a volume, `devvm-tools-<hash>`. Network policy and the docker layer add networks, and the docker layer adds volumes for the VM's own engine. Everything devvm makes there has the label `devvm.data=<devvm's data folder>` ([what you'll see in Docker](../backends/docker.md#what-youll-see-in-docker)). `devvm rm` deletes a VM's container, volumes and networks, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend docker` deletes all of them but those a VM was made from.

Docker also keeps the container image that base starts from, which it pulls as it would any image you pull. devvm doesn't delete it, because it isn't labelled and you may use it.

`~/.local/share/devvm/docker` holds what devvm sets on each VM, which tools it unpacked into the tools volume, the sockets that network policy's relays reach the proxy through, the files of a build's proxy, which a failed build keeps together with its proxy's logs, and on macOS the copy of devvm's Linux build that a release fetches for network policy's relays.

## Outside devvm's folders

devvm also changes a few things outside its own folders:

- **The Keychain:** on macOS, the [secrets](../safety/network.md#saving-and-binding-secrets) you save are Keychain items under the service `devvm`. `devvm secret rm` deletes one.
- **`~/.ssh/config`:** when you let it, devvm adds the line `Include ~/.config/devvm/ssh_config` so that ssh reaches VMs by name ([SSH and editors](ssh.md#ssh-access-by-name)).
- **Time Machine:** on macOS, devvm keeps `ca/` in its data folder out of Time Machine's backups, because a copy of its key elsewhere could impersonate any site to VMs.
- **devvm itself:** the install script puts each version in `~/.local/lib/devvm/<version>` and links `~/.local/bin/devvm` to it. A newer install removes older versions that nothing runs ([upgrading](../getting-started/install.md#upgrading)). Homebrew keeps devvm in its own folders.

[Uninstalling](../getting-started/install.md#uninstalling) removes all of it.
