---
title: Docs
---

- [Installing](install.md): what devvm needs on your machine, Homebrew and the install script, `devvm doctor`, upgrading and uninstalling.
- [Commands](commands.md): devvm in a project folder, its options, and the image commands.
- [Images and VMs](images.md): base and your layers, which image a VM gets, how images are built and cached, and where devvm keeps its state.
- [Starting from a container image](from.md): `--from`, a base from `node:24`, `alpine:3` or your own image.
- [Customizing images](customizing.md): your own layers, and the optional extras in every VM (oh-my-bash, fzf, CLI tools, your own).
- [Security model](security.md): what a VM can and can't reach on your machine.
- [Network policy](network.md): the proxy that limits a VM to an allowlist and keeps tokens on the host.
- [.git protection](git-protection.md): how a VM is kept from changing what git runs on the host.
- [SSH](ssh.md): reaching VMs by name from ssh and editors, their host keys, and agent forwarding.
- [Working on a clone](clone.md): `--clone`, a VM with its own clone of the repo instead of the folder mounted.
- [Backends](backends.md): what runs the VMs, what each one can do, and choosing one.
- [Lima](lima.md): what devvm sets up in Lima, how it confines VMs, and its limits.
- [Known limits](limits.md): what devvm can't do, in one list.
- [CLI reference](cli/_index.md): every command, with its arguments and flags.

For contributors:

- [Contributing](contributing.md): building, testing and writing docs.
- [Testing on other hosts](testing.md): Linux and x86_64.
- [Maintaining](maintaining.md): version bumps and releases.
- [Design notes](design.md): why it's built this way, what failed, what's open.
