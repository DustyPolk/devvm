---
title: Backends
weight: 30
---

A backend is what runs devvm's VMs and builds their images. devvm decides what goes into a VM (its image, its folders, its size, its network policy) and the backend runs it. [Lima](lima.md) is the default backend; [smolvm](smolvm.md) is experimental, and Docker is planned.

Each backend has its own images: an image's key includes its backend, so one backend never uses another's.

## Features

Each backend declares how it has each feature:

- **native**: the backend does it itself.
- **emulated**: devvm builds it from other parts of the backend. An emulation is used only once its review has passed: a [design note](design.md#emulated-features), a threat check and end-to-end checks proving the guarantee. Until then it's refused.
- **none**: the backend can't do it.

`devvm doctor` lists them for every backend devvm has, an emulation whose review hasn't passed as `unreviewed`.

The smolvm column is tested with smolvm 1.32.2 on an arm64 Mac; Linux hosts are untested. Network policy and ssh for editors are emulated and reviewed; its other emulations are planned. Until each is reviewed, devvm refuses it.

| Feature | What it is | Lima | smolvm (experimental) | Docker (planned) |
| --- | --- | --- | --- | --- |
| `build-image` | build images from the layer and feature scripts | native | native: a machine from Ubuntu's container image runs the scripts, then is packed (`pack create --from-vm`); each image is a full pack, about 450 MB for `base` ([details](smolvm.md#building-images)) | native |
| `build-confinement` | builds reach the internet but not the host's loopback, or nothing | native: Seatbelt (macOS); on Linux QEMU `restrict=on` for no network, while a build with the internet reaches the host's loopback ([details](lima.md#confinement)) | native: its default network refuses the host's loopback; no network for nothing | to be decided |
| `from` | `base` from a container image (`--from`) | native: converted to a disk | native: boots registry images, by digest | native |
| `clone` | a project VM from an image | native: APFS copy-on-write (macOS), qcow2 overlay (Linux) ([details](lima.md#clones-and-overlays)) | native: a machine from the image's pack ([details](smolvm.md#its-machines)) | native |
| `project-mount` | the project folder, read-write | native: virtiofs (macOS), 9p (Linux) | native: virtiofs; files the VM makes are the user's on the host, mode 0600 | native |
| `agent-folders` | agents' host folders, read-only | native: read-only mounts, enforced by the host | native: read-only mounts, enforced by the host | native |
| `tools-share` | the shared tools, read-only | native: a read-only mount, enforced by the host | native: a read-only mount, enforced by the host; tool images uncompressed, as its kernel's EROFS has no compression | native |
| `egress-proxy` | network traffic only through devvm's proxy | native: Seatbelt (macOS), QEMU `restrict=on` and a `guestfwd` to the proxy (Linux) ([details](lima.md#confinement)) | emulated (reviewed): an allow list of the host, DNS closed but for one name, and devvm's proxy deciding each connection ([details](smolvm.md#network-policy)) | emulated |
| `ssh` | ssh for editors | native: Lima's ssh forward and config | emulated (reviewed): an sshd of devvm's, started after each start with new host keys and the VM's own key alone, and entries that ask for no forwarding out, on a port forwarded from the host's loopback ([details](smolvm.md#ssh-for-editors)) | emulated |
| `ports` | the listed ports forwarded to the host's loopback | native: port forward rules ([details](lima.md#ports)) | native for servers listening on all addresses, at most 63 ports, as one of smolvm's 64 is ssh's ([details](smolvm.md#what-devvm-sets-on-a-vm)) | native (recreating the container) |
| `ports-all` | every port forwarded, servers on the VM's loopback included | native | none; planned emulation | emulated |
| `agent-forwarding` | ssh-agent forwarding, chosen when the VM is made | native | native | native |
| `agent-toggle` | turning a VM's ssh-agent forwarding on or off later | native | none; planned emulation | native |
| `cpus` | a VM's CPUs | native | native | native |
| `memory` | a VM's memory | native | native | native |
| `disk` | a VM's disk size | native: grows, never shrinks | native: grows, never shrinks | none |

Isolation differs too. Lima runs a full VM per project. smolvm runs a microVM per project, with the image as a container inside it and no init system. Docker runs containers, which share a kernel: on macOS, one VM runs all of them.

`.git` protection uses the host's file flags, so it works the same on every backend.

## Missing and unreviewed features

When a VM needs a feature its backend lacks, devvm stops and names the VM, the feature and the backend. It never falls back to something weaker: a VM that asks for network policy never starts with an open network. The same goes for a feature the backend only emulates whose review hasn't passed.

To get the feature, use a backend that has it. A VM keeps its backend, so that means a new VM.

## VM names

A project VM's name is `devvm-<folder>-<hash>`: up to 20 characters of the folder's name, lowercase, with other characters as dashes, then 14 hex digits of a hash of its path. A backend may take shorter names, and then the folder's part is cut to fit. When fewer than 4 characters of it would fit, devvm refuses: with Lima, that's a Lima home path too long ([details](lima.md#limits)).

## Choosing a backend

A new VM gets the first of:

1. `--backend`.
2. The default, which `devvm setup --backend NAME` sets. It's kept in `~/.config/devvm/backend`: one line, the backend's name.
3. `lima`.

A VM's state records its backend, and the VM stays on it: `--backend` on an existing VM is ignored. When the backend file, or a VM's state, names a backend devvm doesn't have, devvm stops and lists the ones it has.
