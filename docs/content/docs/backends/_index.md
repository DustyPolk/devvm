---
title: Backends
weight: 40
---

A backend is the program that runs devvm's VMs and builds their images. devvm decides what goes into a VM, such as its image, its folders, its size and its network policy, and the backend runs it. [Lima](lima.md) is devvm's backend.

## What Lima gives you

| | Lima |
| --- | --- |
| Each project gets | a full VM with a kernel of its own |
| Hosts | macOS 13 or newer, or Linux, on arm64 or x86_64 |
| You need | Lima 2.0 or newer, and on Linux QEMU and access to `/dev/kvm` ([requirements](lima.md#requirements)) |
| Network policy | {{< yes >}} yes |
| The docker layer | {{< yes >}} yes |
| The nix layer | {{< yes >}} yes |
| Limits | paths with commas, and shrinking a disk ([limits](lima.md#limits)) |

`.git` protection uses your machine's file flags, so it doesn't depend on the backend.

An image's key includes its backend, so a backend never uses an image another backend built.

`devvm doctor` lists Lima's features, which are what Lima does itself, what devvm builds from other parts of it, and what it can't do. These are backend features, not the base image features you choose with `devvm setup`, even though `devvm doctor` calls both "features". [How it works › Backends](../internals/backends.md) explains them.

## A VM's backend

Every new VM runs on Lima, and no flag or setting chooses another backend. A VM's state records its backend, and the VM stays on that backend for as long as it exists. When a VM's state names a backend this devvm doesn't have, such as one a different build of devvm made, devvm stops and says so, and you use the devvm that made the VM.

The `--json` output of `devvm ls`, `devvm images` and `devvm space` names each VM's, image's and shared tool's backend in a `backend` field, and `devvm doctor --json` names the backend it checks.

## When a backend lacks something

When a VM needs something its backend can't do, devvm stops and says so in one line, naming the VM and what to do. It never falls back to something weaker, so a VM that asks for network policy never starts with an open network.

Network policy is the one exception, because it's on by default. When a backend can't give a new VM network policy, devvm makes the VM without it and warns you, unless you gave `--network-policy`, which devvm then refuses ([turning it on or off](../safety/network.md#turning-it-on-or-off)).
