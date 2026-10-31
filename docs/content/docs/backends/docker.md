---
title: Docker (experimental)
weight: 3
---

{{< experimental >}}

Docker is an experimental backend. Each project gets a container, made from an image that devvm builds by running its scripts in a container and committing it. You choose it for one VM with `--backend docker`, or for every new VM with `devvm setup --backend docker`.

## Isolation: a container is not a VM

> [!WARNING]
> Read this before you choose Docker. Lima and smolvm give each project a virtual machine with a kernel of its own, but Docker gives each project a container:
>
> - **On Linux, every container shares your machine's kernel.** A flaw in that kernel, or in Docker's engine, lets code in a VM reach your machine itself, and every other VM.
> - **On macOS, every container shares Docker Desktop's one VM.** A flaw there lets code in one VM reach every other VM, all your containers, and what Docker Desktop shares with them. Your Mac stays behind that VM's boundary.
> - **Root in a container is closer to root on the host** than root in a VM is. It has the same kernel's system calls, filtered by Docker's seccomp profile, and Docker's default capabilities.

Everything else devvm promises holds on Docker too, as far as the container boundary holds: only the project folder is mounted, agents' folders are read-only, and builds run without your files. The [security model](../internals/security.md) assumes a VM's boundary, and on Docker that boundary is a container's.

## What works

`--from` works with any container image Docker has or pulls. `devvm ssh`, `devvm code`, `--clone`, forwarding ports and SSH agent forwarding work as they do on Lima. Network policy works on Docker Desktop and on an engine on this Linux machine. Docker lacks some of these, so devvm builds them from other parts of Docker, and [How it works › Docker](../internals/docker.md) explains how.

## Requirements

{{< os-tabs >}}
{{< os-tab name="macOS" >}}

- Docker Desktop with Docker 29.1 or newer, running, with `docker` on your `PATH`. Its containers run Linux, on this machine's processor.
- devvm's own Linux build, which a release of devvm fetches by itself, checked, the first time a Docker VM needs it. A build of devvm that isn't a release fetches nothing, so build devvm for Linux and put it beside `devvm` ([details](../internals/docker.md#devvms-linux-build)).

{{< /os-tab >}}
{{< os-tab name="Linux" >}}

- Docker's engine 29.1 or newer, running, with `docker` on your `PATH`. Its containers run Linux, on this machine's processor.
- Access to Docker's socket, which means being in the `docker` group (`sudo usermod -aG docker $USER`, then log in again). Being in that group is as good as being root on that machine.

{{< /os-tab >}}
{{< /os-tabs >}}

`devvm doctor --backend docker` checks these.

## Which engine devvm uses

devvm uses the engine that your `docker` command reaches, which is its current context, or `DOCKER_HOST`. `devvm doctor --backend docker` says which engine that is: Docker Desktop, an engine on this Linux machine, or another one, such as colima, OrbStack, a rootless engine or another machine's engine. On another engine, devvm can't confine builds, so it builds no images there, and VMs there get no network policy. On Docker Desktop, `devvm doctor` also lists the folders Desktop shares with containers.

## What you'll see in Docker

Docker's engine is yours, so devvm's containers, images, volumes and networks share it with your own, and `docker ps` and `docker images` show them next to yours. Everything devvm makes there carries the label `devvm.data=<devvm's data folder>`. devvm lists, starts, stops, changes and deletes only what has its label, and it never prunes.

Don't run an unfiltered `docker container prune` or `docker system prune` yourself either. A stopped VM is a stopped container, so a prune deletes it, and with it any unsaved work it holds.

## Docker Desktop's file sharing

Docker Desktop mounts only folders under the ones its settings list, in Settings > Resources > File sharing. By default those are `/Users`, `/Volumes`, `/private`, `/tmp` and `/var/folders`. devvm refuses a project or agent folder outside them, when it can read Desktop's settings, so add the folder there first.

A dev server or a file watcher in the VM sees the files you change or make on this machine, as Desktop's file sharing passes those changes on, but it doesn't see the files you delete here. A watcher that needs deletions can poll the project folder instead, with the settings on the [Lima page](lima.md#file-watchers). On a Linux engine, a watcher sees every change, deletions included.

## What Docker VMs can't do

A Docker VM is a container with Docker's default capabilities less `NET_RAW`, no devices added, and Docker's seccomp profile. So these don't work in one (one with the [docker layer](#the-docker-layer) has `NET_RAW` and `NET_ADMIN` in its own namespaces, and still none of the devices):

- **A disk size.** devvm refuses a VM that asks for one, and a default disk size you set (`devvm size -g --disk`) doesn't apply to Docker VMs, which devvm tells you. `devvm size` shows what a VM holds.
- **A free-space check on the engine's disk.** Before a build or a new VM, devvm doesn't check the engine's free space, but a tools build checks the disk of devvm's data folder, where the tool images come out ([free space](../using/ports-size.md#free-space)).
- **Paths with a comma or a quote.** Docker reads mounts as CSV, so devvm refuses a folder whose path has one.
- **Raw packet tools** (`tcpdump`, `nmap`'s raw scans, `arping`), because there's no `NET_RAW`. `ping` works.
- **Firewalls, routes and VPN clients** (`iptables`, `ip route`, WireGuard, OpenVPN), because there's no `NET_ADMIN` and no `/dev/net/tun`.
- **Mounting** with `mount`, loop devices or FUSE (`sshfs`, `rclone mount`), because there's no `CAP_SYS_ADMIN` and no `/dev/fuse`.
- **Nested virtualization** (QEMU with KVM, Android emulators, Firecracker), because there's no `/dev/kvm`.
- **Some profiling:** `perf` and eBPF tools, whose system calls the seccomp profile refuses. `strace` and `gdb` work.
- **Docker inside**, and with it Kubernetes in Docker (kind, k3d), except under Sysbox on a Linux engine ([below](#the-docker-layer)).
- **The [nix layer](../using/images.md#nix)**, because Nix builds packages in a sandbox made of kernel namespaces that the container can't make, and devvm's Nix builds nothing outside it. devvm refuses the layer before it builds or makes anything.

Use a VM backend (Lima or smolvm) for any of these.

## The docker layer

The [docker layer](../using/images.md#built-in-layers), as in `devvm node+docker`, runs Docker inside a Docker VM only under Sysbox, on a Linux engine on your machine with Sysbox's runtime (`sysbox-runc`). `devvm doctor --backend docker` reports whether the engine has it. Anywhere else, devvm refuses the layer before anything changes, so use a VM backend, Lima or smolvm, which run Docker inside with nothing more given up. Docker Desktop has no Sysbox.

Docker's engine needs more than devvm gives its other containers, and Sysbox gives it that without `--privileged`, in a user namespace of the VM's own. devvm never runs a container `--privileged`, because root in a privileged container has every capability and every device, without Docker's seccomp or AppArmor profile. It can load kernel modules and write the engine's disks, so on Linux it's, in effect, root on your machine. On Docker Desktop it's root of Desktop's VM, with every container in it and the folders Desktop shares, your home among them. Sysbox's own code is part of the boundary.

What a VM with the layer gets, and what it gives up:

- **You can add the layer to an existing VM.** devvm makes the VM's container again, keeping its disk and home ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)). A script that fails while the layer is added leaves the VM as it was.
- **It has a network of its own,** which reaches the internet but no other container, even as root. On Linux it also reaches your machine's servers on every address but its loopback, as any Docker VM without network policy does.
- **It has no network policy,** because root in it can change its own network. devvm doesn't ask about network policy for a new VM with the layer, whether you named the layer or a `devcontainer.json`'s docker-in-docker asked for it. It makes the VM without network policy and warns you. It refuses `--network-policy`, `devvm network on` and adding the layer to a VM that has network policy.
- **Root in it writes as your machine's root in the project folder,** as in any Docker VM. A set-user-id file it makes there runs as root on your machine for anyone who runs it, so devvm names such files at each launch ([details](../internals/security.md#what-it-doesnt-protect)).
- **Docker's userland proxy must stay on,** which is its default. With `"userland-proxy": false` in `daemon.json`, root in the VM could reach your machine's loopback, so devvm refuses the VM.
- `devvm ls` shows its backend as `docker (sysbox)`, and `devvm rm` also deletes its network and its engine's files.

[How it works › Docker](../internals/docker.md#the-docker-layer) has the rest.

### Installing Sysbox

Install the package for your distribution from Sysbox's release on GitHub (`nestybox/sysbox`). It adds `sysbox-runc` to Docker's runtimes and restarts Docker.

On Ubuntu 25.04 and later, fusermount3's AppArmor profile refuses Sysbox's FUSE mounts, so its containers don't start (`failed to pre-register with sysbox-fs`). Allow the mounts in `/etc/apparmor.d/local/fusermount3`:

```text
mount fstype=fuse options=(nosuid,nodev) options in (ro,rw) -> /var/lib/sysboxfs/**/,
umount /var/lib/sysboxfs/**/,
```

Then reload the profile with `sudo apparmor_parser -r /etc/apparmor.d/fusermount3`.

This is checked with Sysbox 0.7.1 on Ubuntu 26.04 (arm64, kernel 7.0, Docker 29.1).

## Looking inside

When a build fails, devvm keeps its container, stopped. To look inside, run `docker start devvm-img-<key>-<hash>`, then `docker exec -it devvm-img-<key>-<hash> bash`. `devvm gc` deletes the container.
