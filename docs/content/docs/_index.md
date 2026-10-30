---
title: Docs
---

The docs are in the order you'll need them: getting devvm running, using it day to day, staying safe and choosing a backend, then every command, how devvm works and how to change it.

## [Getting started](getting-started/_index.md)

- [Installing](getting-started/install.md): what devvm needs on your machine, Homebrew and the install script, `devvm doctor`, upgrading and uninstalling.
- [Your first VM](getting-started/first-vm.md): making a VM for a project folder, what's in it, and coming back to it later.
- [Everyday commands](getting-started/commands.md): devvm in a project folder, its options, and the commands you use most.

## [Using devvm](using/_index.md)

- [Images and tools](using/images.md): base, devvm's docker layer and your own layers, the shared tools, which image a new VM gets, and listing and deleting images.
- [Working on a clone](using/clone.md): `--clone`, a VM with its own clone of the repo instead of the folder mounted.
- [Customizing images](using/customizing.md): your own layers, and the optional extras in every VM, such as oh-my-bash, fzf and CLI tools.
- [Ports and size](using/ports-size.md): the ports a VM forwards to your machine, and its CPUs, memory and disk.
- [Coding agents](using/coding-agents.md): the config Claude Code, Codex and opencode get in a VM, and their history.
- [SSH and editors](using/ssh.md): reaching VMs by name from ssh and editors, their host keys, and agent forwarding.
- [devcontainer.json](using/devcontainer.md): what a new VM takes from a project's `devcontainer.json`, and what it refuses.
- [Starting from a container image](using/from.md): `--from`, a base from `node:24`, `alpine:3` or your own image.
- [Settings and files](using/settings.md): where devvm keeps its state and your settings, and the environment variables it reads.
- [Troubleshooting](using/troubleshooting.md): devvm's run logs, interrupted runs and failed builds.
- [Known limits](using/limits.md): what devvm can't do, in one list.

## [Staying safe](safety/_index.md)

- [Staying safe](safety/_index.md): the habits that keep you safe, and what a VM can reach on your machine.
- [Network policy](safety/network.md): limiting a VM to the hosts you allow, and keeping your secrets on your machine.
- [.git protection](safety/git-protection.md): how devvm keeps a VM from changing what git runs on your machine.
- [Attack vectors](safety/attack-vectors.md): what devvm does about supply chain attacks, rogue agents, git hooks and data theft.

## [Backends](backends/_index.md)

- [Backends](backends/_index.md): what runs the VMs, and choosing one.
- [Lima](backends/lima.md): the default backend, what it needs, and its limits.
- [smolvm (experimental)](backends/smolvm.md): the microVM backend, and what it needs.
- [Docker (experimental)](backends/docker.md): the container backend, which runs containers, not VMs, and what that means.

## [CLI reference](cli/_index.md)

Every command, with its arguments and flags.

## [How it works](internals/_index.md)

- [Overview](internals/overview.md): what devvm does, step by step, when it opens, stops and deletes a VM.
- [Installing and updating](internals/install.md): what the install script checks and installs, `devvm doctor --fixes`, and devvm's own files.
- [Images and caching](internals/images.md): how images and shared tools are built, keyed and cached, and how devvm checks what they start from.
- [Port forwarding](internals/ports.md): how devvm's forwarder forwards a VM's ports to your machine.
- [Network policy](internals/network.md): the proxy, the self-check, and how secrets get into requests.
- [.git protection](internals/git-protection.md): what devvm locks, when it checks the locks, and what they can't stop.
- [Security model](internals/security.md): what a VM can and can't reach on your machine, and how devvm enforces it.
- [Backends](internals/backends.md): each backend's features, native or emulated, and what happens when one is missing.
- [Lima](internals/lima.md): devvm's Lima home, confinement, clones and port forwarding on Lima.
- [smolvm](internals/smolvm.md): how devvm builds and runs VMs on smolvm, and what it emulates there.
- [Docker](internals/docker.md): what devvm makes in Docker's engine, and what it emulates there.
- [Design notes](internals/design.md): why devvm is built this way, what failed, and what's still open.

## [Contributing](contributing/_index.md)

- [Contributing](contributing/_index.md): building, testing and writing docs.
- [Testing on other hosts](contributing/testing.md): Linux and x86_64.
- [Maintaining](contributing/maintaining.md): version bumps and releases.
