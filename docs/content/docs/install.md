---
title: Installing
weight: 10
---

## Requirements

devvm runs on macOS 13 or newer and on Linux, on arm64 or x86_64. Its VMs run on a [backend](backends.md): [Lima](lima.md#requirements), which needs Lima 2.0 or newer, and on Linux QEMU and access to `/dev/kvm`.

Docker is optional: only `--from` with an image only Docker has needs it ([details](from.md#images-only-docker-has)). Everything else (toolchains, agents, Docker for your projects) is installed inside the VMs.

## Checking what devvm needs

devvm never installs anything on the host itself. Every command that needs the backend checks it first, and stops at the first thing missing.

`devvm doctor` checks the backend new VMs get (`--backend NAME` for another) and lists everything it lacks, each with what to do: the command that installs it, where there's one to run. It also lists each backend's [features](backends.md#features) and whether Docker is installed, then says devvm has everything it needs, or fails.

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
