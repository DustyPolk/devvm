---
title: Docker (experimental)
weight: 3
---

Docker is an experimental backend: each project gets a container, made from an image devvm builds by running its scripts in a container and committing it. Choose it per VM with `--backend docker`, or for every new VM with `devvm setup --backend docker`.

## Isolation: a container is not a VM

Read this before you choose Docker. Lima and smolvm give each project a virtual machine with a kernel of its own. Docker gives each project a container:

- **On Linux, every container shares your machine's kernel.** A flaw in that kernel, or in Docker's engine, lets code in a VM reach your machine itself, and every other VM.
- **On macOS, every container shares Docker Desktop's one VM.** A flaw there lets code in one VM reach every other VM, and all your containers, and what Docker Desktop shares with them; your Mac stays behind that VM's boundary.
- **Root in a container is closer to root on the host** than in a VM: it has the same kernel's system calls (filtered by Docker's seccomp profile) and Docker's default capabilities.

Everything else devvm promises (only the project folder mounted, agents' folders read-only, builds without your files) holds on Docker too, as far as the container boundary holds. The [security model](../internals/security.md) assumes a VM's boundary; on Docker it's a container's.

## What it has

Docker's container features are native: building images, `--from` any image Docker has or pulls, the project folder, agents' folders, the shared tools, and CPUs and memory ([features](../internals/backends.md#features)). On Linux, builds are confined natively; on Docker Desktop, through a proxy of devvm's ([below](../internals/docker.md#building-images)).

ssh for editors (and so `devvm ssh`, `devvm code` and `--clone`), forwarding ports (listed, and every one) and SSH agent forwarding (and turning it on or off later) are emulated, and reviewed, all over one transport, `docker exec` into the container ([below](../internals/docker.md#ssh-for-editors)).

[Network policy](../internals/docker.md#network-policy), and build confinement on Docker Desktop ([below](../internals/docker.md#building-images)), are emulated, and reviewed: an internal network of the VM's (or build's) own and a relay to devvm's proxy on your machine.

The [docker layer](#the-docker-layer) is emulated, and reviewed: under Sysbox, on a Linux engine that has it, and nowhere else. Some tools need more than a container gets ([what Docker VMs can't do](#what-docker-vms-cant-do)).

A disk size isn't there, so devvm refuses a VM that asks for one, in one line naming the VM (`backend_lacks`). A default disk size you set (`devvm size -g --disk`) doesn't apply to Docker VMs, and devvm says so (`size_disk_default_unused`).

## Requirements

- Docker 29.1 or newer, `docker` on `PATH`, and its engine running: Docker Desktop on macOS, or Docker's engine on Linux. Its containers run Linux, on this machine's processor.
- On Linux, access to Docker's socket: being in the `docker` group (`sudo usermod -aG docker $USER`, then log in again), which is as good as root on that machine. devvm can't tell builds on a rootless engine are confined, so it builds nothing there.
- On macOS, devvm's Linux build for your Mac's processor, of the same version: [network policy](../internals/docker.md#network-policy)'s relays and Docker Desktop's [builds](../internals/docker.md#building-images) run it in containers. On Linux, devvm runs itself. devvm looks for it as `devvm-linux-arm64` or `devvm-linux-amd64` beside `devvm` or in `../libexec` (the Homebrew formula builds it there from source). A release installed with `install.sh` fetches it instead, the first time a Docker VM or build needs it (`docker_linux_build_fetching`, a status step, no question asked): from that release's own Linux archive on GitHub, checked against the SHA-256 the release has of it before it's used, and kept in `~/.local/share/devvm/docker/bin`. One that doesn't match is refused, and nothing kept (`docker_linux_build_mismatch`); one that can't be fetched is `docker_linux_build_fetch_failed`. A build of devvm that isn't a release fetches nothing: build devvm for Linux beside it (`docker_linux_build_missing`). One of another build is `docker_linux_build_other`.

`devvm doctor --backend docker` checks them, and says which engine devvm uses: Docker Desktop, an engine on this Linux machine, or another one (colima, OrbStack, a rootless engine, another machine's), where builds aren't confined and network policy isn't there. On Docker Desktop it lists the folders Desktop shares with containers.

devvm uses the engine your `docker` reaches: its current context, or `DOCKER_HOST`.

## The docker layer

Docker inside a Docker VM (the [docker layer](../using/images.md#built-in-layers), `devvm node+docker`) needs Sysbox on a Linux engine. Anywhere else, use a VM backend, Lima or smolvm, which run Docker inside with nothing more given up. Emulated, and reviewed ([design](../internals/design.md#the-docker-layer-on-docker)).

A container runs `dockerd` only with more than devvm gives its other containers. Sysbox gives it that without `--privileged`: a user namespace of its own, so root in it is an unprivileged user on your machine, but on its mounts, ID-mapped, so your files are yours in it and what you write there is yours. What root in it writes in the project folder is your machine's root's, as in any Docker VM: a set-user-id file it makes there runs as root on your machine for anyone who runs it (devvm names such files at each launch, `setid_root_files`; [details](../internals/security.md#what-it-doesnt-protect)). Sysbox's own code is part of the boundary.

devvm never runs a container `--privileged`. Root in a privileged container has every capability and every device, without Docker's seccomp or AppArmor profile, so it can load kernel modules and write the engine's disks: on Linux that's, in effect, root on your machine; on Docker Desktop, root of Desktop's VM, with every container in it and the folders Desktop shares (your home among them).

- **Where it runs:** on a Linux engine on your machine with Sysbox's runtime (`sysbox-runc`), which `devvm doctor --backend docker` reports (`docker_sysbox`). A VM with the layer runs under it, said (`docker_layer_runtime`), whether it's made with the layer or gets it in place ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)): then its container is [made again](../internals/docker.md#changing-what-docker-fixes) from a commit, its disk and home kept. On a Linux engine without Sysbox the layer is refused in one line, before anything changes (`docker_layer_no_sysbox`); on Docker Desktop and other engines (another machine's), too (`docker_layer_needs_vm`).
- **Its own network:** a network of its own, `<vm>-docker-net`, labelled, not internal, with nothing else on it, instead of Docker's default one: it reaches the internet, and no other container, on Docker's default network or any other, even as root. On Linux it reaches your machine's servers on every address but its loopback, as any Docker VM without network policy does (with IPv6 turned on, those on IPv6 too). One with another container on it is refused (`docker_network_shared`).
- **Its engine's files** are in volumes of its own, `<vm>-docker` at `/var/lib/docker` and `<vm>-containerd` at `/var/lib/containerd`: Docker's storage doesn't stack on the container's own, and they're kept when its container is made again. `devvm rm` deletes them and its network.
- **Docker's daemon** starts after each start, from base's services; the VM's user is in the `docker` group.
- **No network policy:** root in it can change its own network: in its own network namespace it can turn IPv6 on and reach your machine's servers on the bridge's link-local address. devvm doesn't ask about network policy for a new VM with the layer, whether you named the layer or a `devcontainer.json`'s docker-in-docker asked for it. It makes the VM without network policy and warns you (`net_off_docker_layer`). It refuses `--network-policy`, `devvm network on` and adding the layer to a VM with network policy (`net_docker_layer`).
- **`NET_RAW` and `NET_ADMIN`:** it has them, in its own namespaces: Sysbox gives root every capability there, and Docker's engine needs `NET_ADMIN`. Its own network keeps what it sends from other containers, and your machine drops what it routes to its loopback.
- **Docker's userland proxy stays on** (its default): `"userland-proxy": false` in `daemon.json` turns `route_localnet` on for Docker's bridges, and then root in the VM could reach your machine's loopback. devvm reads its network's bridge setting at each create and start and refuses the VM in one line if it's on (`docker_route_localnet`).
- **A script that fails** when the layer's added in place leaves the VM as it was: its container made again as any VM's, its network and engine's volumes deleted.
- `devvm ls` shows its backend as `docker (sysbox)`; its state has `docker_layer=sysbox`.

**Installing Sysbox:** its release's package for your distribution, from GitHub (`nestybox/sysbox`), which adds `sysbox-runc` to Docker's runtimes and restarts it. On Ubuntu 25.04 and later, fusermount3's AppArmor profile refuses Sysbox's FUSE mounts, and its containers don't start (`failed to pre-register with sysbox-fs`): allow them in `/etc/apparmor.d/local/fusermount3`, then reload the profile (`sudo apparmor_parser -r /etc/apparmor.d/fusermount3`):

```text
mount fstype=fuse options=(nosuid,nodev) options in (ro,rw) -> /var/lib/sysboxfs/**/,
umount /var/lib/sysboxfs/**/,
```

Checked with Sysbox 0.7.1 on Ubuntu 26.04 (arm64, kernel 7.0, Docker 29.1). Docker Desktop has no Sysbox.

## What Docker VMs can't do

A Docker VM is a container with Docker's default capabilities less `NET_RAW`, no devices added, and Docker's seccomp profile. So these don't work in one (one with the [docker layer](#the-docker-layer) has `NET_RAW` and `NET_ADMIN` in its own namespaces, and still none of the devices):

- **Raw packet tools** (`tcpdump`, `nmap`'s raw scans, `arping`): no `NET_RAW`. `ping` works.
- **Firewalls, routes and VPN clients** (`iptables`, `ip route`, WireGuard, OpenVPN): no `NET_ADMIN`, and no `/dev/net/tun`.
- **Mounting:** `mount`, loop devices, FUSE (`sshfs`, `rclone mount`): no `CAP_SYS_ADMIN`, no `/dev/fuse`.
- **Nested virtualization** (QEMU with KVM, Android emulators, Firecracker): no `/dev/kvm`.
- **Some profiling:** `perf` and eBPF tools, whose system calls the seccomp profile refuses. `strace` and `gdb` work.
- **Docker inside**, and with it Kubernetes in Docker (kind, k3d): only under Sysbox on a Linux engine ([above](#the-docker-layer)).

Use a VM backend (Lima or smolvm) for any of these.
