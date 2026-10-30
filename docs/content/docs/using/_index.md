---
title: Using devvm
weight: 20
---

These pages cover what you change as you work: the images VMs are made from, your coding agents, and the editors you connect with.

- [Images and VMs](images.md): base, the docker layer and your own layers, the shared tools, which image a VM gets, ports and size.
- [Customizing images](customizing.md): your own layers, and the optional extras in every VM, such as oh-my-bash and fzf.
- [Starting from a container image](from.md): `--from`, a base from `node:24`, `alpine:3` or your own image.
- [devcontainer.json](devcontainer.md): what a new VM takes from a project's `devcontainer.json`, and what it refuses.
- [Coding agents](coding-agents.md): the config Claude Code, Codex and opencode get in a VM, and their history.
- [SSH and editors](ssh.md): reaching VMs by name from ssh and VS Code, their host keys, and agent forwarding.
- [Known limits](limits.md): what devvm can't do, in one list.
