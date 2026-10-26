---
title: Backends
weight: 30
---

A backend is what runs devvm's VMs and builds their images. devvm decides what goes into a VM (its image, its folders, its size, its network policy) and the backend runs it. [Lima](lima.md) is the default backend; [smolvm](smolvm.md) and [Docker](docker.md) are experimental. Docker runs containers, not VMs ([isolation](docker.md#isolation-a-container-is-not-a-vm)).

Each backend has its own images: an image's key includes its backend, so one backend never uses another's.

## Features

Each backend declares how it has each feature:

- **native**: the backend does it itself.
- **emulated**: devvm builds it from other parts of the backend. An emulation is used only once its review has passed: a [design note](design.md#emulated-features), a threat check and end-to-end checks proving the guarantee. Until then it's refused.
- **none**: the backend can't do it.

`devvm doctor` lists them for every backend devvm has, an emulation whose review hasn't passed as `unreviewed`. Docker's `build-confinement` depends on its engine, so the Docker column shows it once devvm has asked the engine: with `devvm doctor --backend docker`.

The smolvm column is tested with smolvm 1.32.2 on an arm64 Mac; Linux hosts are untested. Network policy, ssh for editors, every port forwarded and turning agent forwarding on or off are emulated and reviewed. An emulation is refused until it's reviewed.

The Docker column is tested with Docker Desktop 4.94 (Docker 29.8) on an arm64 Mac and Docker 29.1 on Ubuntu 26.04 (arm64). Docker Desktop and an engine on a Linux machine differ in what a container reaches on the host. Its native features are there. ssh for editors, ports (listed and every one) and agent forwarding (and turning it on or off) are emulated over one transport, `docker exec` into the container to an sshd of devvm's per connection, and reviewed; what it lacks is refused ([design](design.md#docker)).

| Feature | What it is | Lima | smolvm (experimental) | Docker (experimental) |
| --- | --- | --- | --- | --- |
| `build-image` | build images from the layer and feature scripts | native | native: a machine from Ubuntu's container image runs the scripts, then is packed (`pack create --from-vm`); each image is a full pack, about 450 MB for `base` ([details](smolvm.md#building-images)) | native: a container from Ubuntu's container image runs the scripts with no init system, then `docker commit`; each image shares its parent's layers ([details](docker.md#building-images)) |
| `build-confinement` | builds reach the internet but not the host's loopback, or nothing | native: Seatbelt (macOS); on Linux QEMU `restrict=on` for no network, while a build with the internet reaches the host's loopback ([details](lima.md#confinement)) | native: its default network refuses the host's loopback; no network for nothing | native on an engine of this Linux machine: its default network doesn't reach the host's loopback; no network for nothing. None on Docker Desktop, whose containers reach the host's loopback (`host.docker.internal`), so it builds nothing yet, nor on other engines ([details](docker.md#building-images)) |
| `from` | `base` from a container image (`--from`) | native: converted to a disk | native: boots registry images, by digest | native: any image Docker has or pulls |
| `clone` | a project VM from an image | native: APFS copy-on-write (macOS), qcow2 overlay (Linux) ([details](lima.md#clones-and-overlays)) | native: a machine from the image's pack ([details](smolvm.md#its-machines)) | native: a container from the image, its layers shared, its home a volume of its own ([details](docker.md#whats-devvms-in-docker)) |
| `project-mount` | the project folder, read-write | native: virtiofs (macOS), 9p (Linux) | native: virtiofs; files the VM makes are the user's on the host, mode 0600 | native: a bind mount; Docker Desktop mounts only folders in its file sharing list; on Linux, files root makes in the VM are root's on the host ([details](docker.md#what-devvm-sets-on-a-vm)) |
| `agent-folders` | agents' host folders, read-only | native: read-only mounts, enforced by the host | native: read-only mounts, enforced by the host | native: read-only bind mounts, which root in the container can't unmount (no `CAP_SYS_ADMIN`); added or removed, the container is made again from a commit of its own ([details](docker.md#changing-what-docker-fixes)) |
| `tools-share` | the shared tools, read-only | native: a read-only mount, enforced by the host | native: a read-only mount, enforced by the host; tool images uncompressed, as its kernel's EROFS has no compression | native: tools unpacked into a Docker volume, mounted read-only and linked; tools added later show up in running VMs. Not the EROFS images: containers get no loop devices without `--privileged` ([details](docker.md#shared-tools)) |
| `egress-proxy` | network traffic only through devvm's proxy | native: Seatbelt (macOS), QEMU `restrict=on` and a `guestfwd` to the proxy (Linux) ([details](lima.md#confinement)) | emulated (reviewed): an allow list of the host, DNS closed but for one name, and devvm's proxy deciding each connection ([details](smolvm.md#network-policy)) | none |
| `ssh` | ssh for editors | native: Lima's ssh forward and config | emulated (reviewed): an sshd of devvm's, started after each start with new host keys and the VM's own key alone, and entries that ask for no forwarding out, on a port forwarded from the host's loopback ([details](smolvm.md#ssh-for-editors)) | emulated (reviewed): no sshd listens; each connection is an sshd of its own in the container (`sshd -i`), run by `docker exec` through `devvm _docker-connect` as ssh's `ProxyCommand`, with new host keys each start and the VM's own key alone ([details](docker.md#ssh-for-editors)) |
| `ports` | the listed ports forwarded to the host's loopback | native: port forward rules ([details](lima.md#ports)) | native for servers listening on all addresses, at most 63 ports, as one of smolvm's 64 is ssh's ([details](smolvm.md#what-devvm-sets-on-a-vm)) | emulated (reviewed): devvm's forwarder over the VM's ssh, the listed ports alone; nothing published ([details](docker.md#ports)) |
| `ports-all` | every port forwarded, servers on the VM's loopback included | native | emulated (reviewed): a forwarder of devvm's on the host, over the VM's ssh, for what listens in the VM from port 1024 up, never a port the host has ([details](smolvm.md#every-port)) | emulated (reviewed): the same forwarder, over the VM's ssh through `docker exec` ([details](docker.md#ports)) |
| `agent-forwarding` | ssh-agent forwarding, chosen when the VM is made | native | native | emulated (reviewed): ssh sessions alone forward an agent (`ForwardAgent`), devvm's shell and its ssh config's; no socket mounted ([details](docker.md#ssh-agent-forwarding)) |
| `agent-toggle` | turning a VM's ssh-agent forwarding on or off later | native | emulated (reviewed): every VM made forwarding, and each start given your agent, devvm's, or none, a path no socket can be at; one started outside devvm is stopped ([details](smolvm.md#ssh-agent-forwarding)) | emulated (reviewed): the agent each session forwards, chosen at each start, as on Lima ([details](docker.md#ssh-agent-forwarding)) |
| `cpus` | a VM's CPUs | native | native | native: a quota; BusyBox's `nproc` still counts every CPU |
| `memory` | a VM's memory | native | native | native |
| `disk` | a VM's disk size | native: grows, never shrinks | native: grows, never shrinks | none: Docker Desktop refuses a size, and Docker's containerd image store ignores it; `devvm size` shows what a VM holds ([details](docker.md#what-devvm-sets-on-a-vm)) |
| `docker-layer` | the [docker layer](images.md#built-in-layers): Docker's engine running in the VM | native | native: its daemon started after each start | none: `dockerd` needs a privileged container |

Isolation differs too. Lima runs a full VM per project. smolvm runs a microVM per project, with the image as a container inside it and no init system. Docker runs containers, not VMs: on Linux every VM shares the host's kernel, and on macOS Docker Desktop runs all of them in one VM, so a kernel flaw reaches the host on Linux, and every other VM on both ([details](docker.md#isolation-a-container-is-not-a-vm)). Docker itself doesn't run inside them: it needs `--privileged`, which takes that isolation away.

`.git` protection uses the host's file flags, so it works the same on every backend.

## Missing and unreviewed features

When a VM needs a feature its backend lacks, devvm stops and names the VM, the feature and the backend. It never falls back to something weaker: a VM that asks for network policy never starts with an open network. The same goes for a feature the backend only emulates whose review hasn't passed.

To get the feature, use a backend that has it. A VM keeps its backend, so that means a new VM.

## VM names

A project VM's name is `devvm-<folder>-<hash>`: up to 20 characters of the folder's name, lowercase, with other characters as dashes, then 14 hex digits of a hash of its path. A backend may take shorter names, and then the folder's part is cut to fit. When fewer than 4 characters of it would fit, devvm refuses: with Lima, that's a Lima home path too long ([details](lima.md#limits)).

## Choosing a backend

A new VM gets the first of:

1. `--backend`.
2. The default. It's kept in `~/.config/devvm/backend`: one line, the backend's name.
3. `lima`.

To set the default:

```sh
devvm setup                  # asks, then for base's features
devvm setup --backend NAME   # without asking; the features stay as they are
```

In a terminal, `devvm setup` lists every backend, the experimental ones marked, starting from the current default: up and down (or `k` and `j`) move, Enter confirms. devvm saves and prints the choice, then asks for [base's features](customizing.md#choosing-features). With `--features` alone, or without a terminal, it doesn't ask for the backend.

A VM's state records its backend, and the VM stays on it: `--backend` on an existing VM is ignored. When the backend file, or a VM's state, names a backend devvm doesn't have, devvm stops and lists the ones it has.
