---
title: Backends
weight: 30
---

A backend is what runs devvm's VMs and builds their images. devvm decides what goes into a VM (its image, its folders, its size, its network policy) and the backend runs it. Lima is the backend today. smolvm and Docker are planned.

Each backend has its own images: an image's key includes its backend, so one backend never uses another's.

## Features

Each backend declares how it has each feature:

- **native**: the backend does it itself.
- **emulated**: devvm builds it from other parts of the backend. An emulation is used only once its review has passed: a design note, a threat check and end-to-end checks proving the guarantee. Until then it's refused.
- **none**: the backend can't do it.

| Feature | What it is | Lima | smolvm (planned) | Docker (planned) |
| --- | --- | --- | --- | --- |
| `build-image` | build images from the layer and feature scripts | native | native | native |
| `build-confinement` | builds reach the internet but not the host's loopback, or nothing | native: Seatbelt (macOS), QEMU `restrict=on` (Linux) | to be decided | to be decided |
| `from` | `base` from a container image (`--from`) | native: converted to a disk | native | native |
| `clone` | a project VM from an image | native: APFS copy-on-write (macOS), qcow2 overlay (Linux) | native | native |
| `project-mount` | the project folder, read-write | native: virtiofs (macOS), 9p (Linux) | native | native |
| `agent-folders` | agents' host folders, read-only | native | native | native |
| `tools-share` | the shared tools, read-only | native | native | native |
| `egress-proxy` | network traffic only through devvm's proxy | native: Seatbelt (macOS), QEMU `restrict=on` (Linux) | emulated | emulated |
| `ssh` | ssh for editors | native | emulated | emulated |
| `ports` | the VM's ports forwarded to the host's loopback | native | native (its config's ports) or emulated | native (recreating the container) or emulated |
| `agent-forwarding` | ssh-agent forwarding | native | emulated | native |
| `cpus` | a VM's CPUs | native | native | native |
| `memory` | a VM's memory | native | native | native |
| `disk` | a VM's disk size | native | none | none |

Isolation differs too. Lima runs a full VM per project. smolvm runs a microVM. Docker runs containers, which share a kernel: on macOS, one VM runs all of them.

`.git` protection uses the host's file flags, so it works the same on every backend.

## Missing and unreviewed features

When a VM needs a feature its backend lacks, devvm stops and names the VM, the feature and the backend. It never falls back to something weaker: a VM that asks for network policy never starts with an open network. The same goes for a feature the backend only emulates whose review hasn't passed.

To get the feature, use a backend that has it. A VM keeps its backend, so that means a new VM.

## Choosing a backend

A new VM gets the first of:

1. `--backend`.
2. The default in `~/.config/devvm/backend`: one line, the backend's name.
3. `lima`.

A VM's state records its backend, and the VM stays on it: `--backend` on an existing VM is ignored. When the backend file, or a VM's state, names a backend devvm doesn't have, devvm stops and lists the ones it has.
