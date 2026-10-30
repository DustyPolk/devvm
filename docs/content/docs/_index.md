---
title: Docs
---

The docs are in the order you'll need them: getting devvm running, using it day to day, staying safe, then how it works.

## [Getting started](getting-started/_index.md)

- [Installing](getting-started/install.md): what devvm needs on your machine, Homebrew and the install script, `devvm doctor`, upgrading and uninstalling.
- [Your first VM](getting-started/first-vm.md): making a VM for a project folder, what's in it, and coming back to it later.
- [Everyday commands](getting-started/commands.md): devvm in a project folder, its options, and the image commands.

## [Using devvm](using/_index.md)

- [Images and VMs](using/images.md): base, devvm's docker layer and your own layers, which image a VM gets, how images are built and cached, and where devvm keeps its state.
- [Customizing images](using/customizing.md): your own layers, and the optional extras in every VM, such as oh-my-bash, fzf and CLI tools.
- [Starting from a container image](using/from.md): `--from`, a base from `node:24`, `alpine:3` or your own image.
- [devcontainer.json](using/devcontainer.md): what a new VM takes from a project's `devcontainer.json`, and what it refuses.
- [Coding agents](using/coding-agents.md): the config Claude Code, Codex and opencode get in a VM, and their history.
- [SSH and editors](using/ssh.md): reaching VMs by name from ssh and editors, their host keys, and agent forwarding.
- [Known limits](using/limits.md): what devvm can't do, in one list.

## [Staying safe](safety/_index.md)

- [Attack vectors](safety/attack-vectors.md): how to stay safe, and what devvm does about supply chain attacks, rogue agents, git hooks and data theft.
- [Network policy](safety/network.md): the proxy that limits a VM to an allowlist and keeps tokens on the host.
- [Working on a clone](safety/clone.md): `--clone`, a VM with its own clone of the repo instead of the folder mounted.
- [.git protection](safety/git-protection.md): how devvm keeps a VM from changing what git runs on the host.
- [Security model](safety/security.md): what a VM can and can't reach on your machine.

## [Backends](backends/_index.md)

- [Backends](backends/_index.md): what runs the VMs, what each one can do, and choosing one.
- [Lima](backends/lima.md): what devvm sets up in Lima, how it confines VMs, and its limits.
- [smolvm](backends/smolvm.md): the experimental microVM backend, and what devvm emulates on it.
- [Docker](backends/docker.md): the experimental container backend, which runs containers, not VMs, and what that means.

## [How it works](internals/_index.md)

- [Design notes](internals/design.md): why devvm is built this way, what failed, and what's still open.

## [CLI reference](cli/_index.md)

Every command, with its arguments and flags.

## [Contributing](contributing/_index.md)

- [Contributing](contributing/_index.md): building, testing and writing docs.
- [Testing on other hosts](contributing/testing.md): Linux and x86_64.
- [Maintaining](contributing/maintaining.md): version bumps and releases.
