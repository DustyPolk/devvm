---
title: Using devvm
weight: 20
---

These pages cover what you change as you work, such as the images VMs are made from, their ports and size, your coding agents, and the editors you connect with.

- [Images and tools](images.md): base, the docker layer and your own layers, the shared tools, which image a new VM gets, and rebuilding, listing and deleting images.
- [Working on a clone](clone.md): `--clone`, which gives a VM its own clone of the repo instead of mounting the folder.
- [Customizing images](customizing.md): your own layers, and the optional features in every VM, such as oh-my-bash and fzf.
- [Ports and size](ports-size.md): the ports a VM forwards to your machine, and its CPUs, memory and disk.
- [Coding agents](coding-agents.md): the config that Claude Code, Codex and opencode get in a VM, their logins, and their history.
- [SSH and editors](ssh.md): reaching VMs by name from ssh and VS Code, and forwarding your SSH agent.
- [devcontainer.json](devcontainer.md): what a new VM takes from a project's `devcontainer.json`, and what devvm refuses.
- [Starting from a container image](from.md): `--from`, which starts base from `node:24`, `alpine:3` or your own image.
- [Settings and files](settings.md): your settings and the commands that write them, devvm's state, and the environment variables devvm reads.
- [Troubleshooting](troubleshooting.md): what to do when a command or a build fails, a request is blocked, or a VM won't start.
- [Known limits](limits.md): what devvm can't do, in one list.
