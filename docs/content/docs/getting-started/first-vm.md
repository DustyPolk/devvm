---
title: Your first VM
weight: 2
---

This page walks through making a VM for a project folder, what you get inside it, and coming back to it later. It assumes you've [installed devvm](install.md) and that `devvm doctor` finds nothing missing.

## Making the VM

Run devvm in the project folder:

```sh
cd ~/some-project
devvm --network-policy
```

devvm looks at the folder's files to choose the VM's toolchains, so a folder with `package.json` gets node and one with `pyproject.toml` gets python ([which image a new VM gets](../using/images.md#which-image-a-new-vm-gets)). To choose yourself, name the image, such as `devvm node+claude` for node with Claude Code, or `devvm base` for neither.

`--network-policy` limits the VM to an allowlist of hosts, through a proxy on your machine, and keeps your tokens out of the VM ([network policy](../safety/network.md)). If you leave the flag out, devvm asks, with no as the default. Turn it on unless you know you need the open network, because it's what keeps stolen data and tokens from leaving.

The first time you run devvm, it asks which optional extras every VM gets, such as oh-my-bash and fzf ([choosing features](../using/customizing.md#choosing-features)). Then it builds the base image, which takes a few minutes, and downloads Ubuntu's cloud image first:

```console
==> building image base (e9f517dddef1)
    a few minutes, and Ubuntu's cloud image to download the first time
    (details: ~/.local/share/devvm/logs/runs/20261024-090616-83541.log)
==> image base ready
==> creating devvm-some-project-71a99b1076d92c from base
==> starting devvm-some-project-71a99b1076d92c
```

Later VMs are copies of the image and start in seconds.

## What's in the VM

When the VM is ready, devvm opens a shell in it, in `~/some-project`, the same path as on your machine. The prompt starts with `(devvm)`, so you can tell the VM's shell from your own.

- **The project folder** is mounted read-write, so your editor on this machine and the VM see the same files. It's the only folder of yours the VM sees, and devvm locks its `.git` hooks and config so the VM can't change what git runs on your machine ([.git protection](../safety/git-protection.md)).
- **The toolchains** are shared between VMs and read-only, so `node`, `python`, `cargo` or `go` are ready at once, and global installs go into the VM's home ([shared tools](../using/images.md#shared-tools)).
- **Your git name and email** are set as git has them in the project folder, so commits made in the VM are yours.
- **Ports** that a server in the VM listens on, from 1024 up, are forwarded to the same port on your machine's `localhost`, so a dev server on `:5173` opens at `localhost:5173` ([ports](../using/images.md#ports)).
- **The VM's size** is 4 CPUs, 8 GiB of memory and a 100 GiB disk, which `devvm size` changes ([size](../using/images.md#size)).

The VM's user has passwordless sudo, so you can install anything with `apt` as you would on any Ubuntu machine.

## Coding agents

An image with an agent, such as `devvm claude`, gets your settings, skills and MCP servers for it from this machine, but never your history ([coding agents](../using/coding-agents.md)). Claude Code needs a long-lived token, which you make once on this machine:

```sh
claude setup-token   # approve in the browser, then copy the sk-ant-oat01-… token it prints
devvm auth claude    # paste it
```

With network policy, the VM gets a placeholder instead of the token, and the proxy puts the real token into Claude's requests.

## Coming back to it

The VM belongs to its folder. Running `devvm` in the folder again opens a new shell in the same VM, and starts the VM first if it's stopped.

```sh
devvm          # a shell in this folder's VM
devvm code     # this folder's VM in VS Code, over Remote-SSH
devvm stop     # stop it
devvm ls       # every VM, with its image, network policy and folder
devvm rm       # delete it, saving its agents' history first
```

[Everyday commands](commands.md) lists the rest. Before you let an agent loose in the VM, read [staying safe](../safety/attack-vectors.md#staying-safe), which lists the habits that close most of what devvm leaves open.
