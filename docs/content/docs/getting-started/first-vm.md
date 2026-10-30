---
title: Your first VM
weight: 2
---

This page walks through making a VM for a project folder, what you get inside it, and coming back to it later. It assumes you've [installed devvm](install.md) and that `devvm doctor` finds nothing missing.

## Making the VM

Go to the project folder and run one of these, depending on whether you want a coding agent in the VM:

```sh
cd ~/some-project
devvm          # a VM with the folder's toolchains
devvm claude   # the same, with Claude Code
devvm codex    # the same, with Codex
```

devvm looks at the folder's files to choose the VM's toolchains, so a folder with `package.json` gets node and one with `pyproject.toml` gets python. An agent's name, such as `claude`, `codex` or `opencode`, goes on top of those toolchains, so `devvm claude` in a node project makes a node+claude VM. In a folder with neither a toolchain's files nor a `devcontainer.json`, `devvm claude` makes a VM with Claude Code alone, but plain devvm stops and asks you to name an image. To choose yourself, name the image, such as `devvm node+claude` for node with Claude Code, or `devvm base` for neither. [Which image a new VM gets](../using/images.md#which-image-a-new-vm-gets) has the details.

Claude Code in the VM needs a Claude Code token, which you make once, as [Coding agents](#coding-agents) below shows.

A new VM gets [network policy](../safety/network.md), which limits it to an allowlist of hosts through a proxy on your machine and keeps your secrets out of the VM. Keep it on unless you know you need the open network, because it's what keeps stolen data and secrets from leaving. In a terminal, devvm asks first and recommends yes, which is the default answer. Without a terminal, it turns network policy on. `--network-policy` answers yes up front, and `--no-network-policy` makes the VM with an open network instead. When a VM can't have network policy, for example on a backend that can't confine it to its proxy, devvm doesn't ask, but makes the VM without network policy and warns you.

The first time you run devvm, it asks which optional extras every VM gets, such as oh-my-bash and fzf, as [choosing features](../using/customizing.md#choosing-features) describes. Then it downloads Ubuntu's cloud image and builds the base image, which takes a few minutes:

```console
==> building image base (e9f517dddef1)
    a few minutes, and Ubuntu's cloud image to download the first time
    (details: ~/.local/share/devvm/logs/runs/20261024-090616-83541.log)
==> image base ready
==> creating devvm-some-project-71a99b1076d92c from base
==> starting devvm-some-project-71a99b1076d92c
```

Later VMs are copies of the image and start in seconds.

## Working on a clone instead

A VM either mounts your project folder or works on a clone of the repo of its own. By default it mounts the folder, so the VM and your editor share the same files. With `--clone`, the VM gets its own copy of your branches instead, and nothing in it can touch your working tree or `.git`. Some people will mostly use `--clone`, for example for an agent working unattended, for a repo they don't trust, or on Linux hosts, which have no `.git` protection.

You choose when devvm makes the VM, and you can't change a VM from one to the other afterwards. On a clone, the VM's work reaches your repo only when you fetch it, and you review it before you merge it:

```sh
devvm --clone                 # make the VM with its own clone, and open a shell in it
git fetch devvm               # later, on this machine, fetch the VM's work as devvm/<branch>
git log -p main..devvm/main   # review it before you merge it
```

[Working on a clone](../using/clone.md) has the details.

## What's in the VM

When the VM is ready, devvm opens a shell in it, in `~/some-project`, the same path as on your machine. The prompt starts with `(devvm)`, so you can tell the VM's shell from your own.

- **The project folder** is mounted read-write, unless the VM works on a clone, so your editor on this machine and the VM see the same files. It's the only folder of yours the VM sees, and devvm locks its `.git` hooks and config so the VM can't change what git runs on your machine ([.git protection](../safety/git-protection.md)).
- **The toolchains** are shared between VMs and read-only, so `node`, `python`, `cargo` or `go` are ready at once, and global installs go into the VM's home ([shared tools](../using/images.md#shared-tools)).
- **Your git name and email** are set as git has them in the project folder, so commits made in the VM are yours.
- **Ports** that a server in the VM listens on, from 1024 up, are forwarded to the same port on your machine's `localhost`, so a dev server on `:5173` opens at `localhost:5173` ([ports](../using/ports-size.md#ports)).
- **The VM's size** is 4 CPUs, 8 GiB of memory and a 100 GiB disk, which `devvm size` changes ([size](../using/ports-size.md#size)).

The VM's user has passwordless sudo, so you can install anything with `apt` as you would on any Ubuntu machine.

## Coding agents

An image with an agent, such as `devvm claude`, gets your settings, skills and MCP servers for it from this machine, but never your history, as [coding agents](../using/coding-agents.md) explains. Claude Code needs a long-lived Claude Code token, which you make once on this machine:

```sh
claude setup-token   # approve in the browser, then copy the sk-ant-oat01-… token it prints
devvm auth claude    # paste it
```

With network policy, the VM gets a placeholder instead of your Claude Code token, and the proxy puts the real one into Claude's requests.

## Coming back to it

The VM belongs to its folder. Running `devvm` in the folder again opens a new shell in the same VM, and starts the VM first if it's stopped.

```sh
devvm          # a shell in this folder's VM
devvm claude   # add Claude Code to this folder's VM, after asking
devvm code     # this folder's VM in VS Code, over Remote-SSH
devvm stop     # stop it
devvm ls       # every VM, with its image, network policy and folder
devvm rm       # delete it, saving its agents' history first
```

When the folder's VM doesn't have the agent you name, as with `devvm claude` in a VM made with plain devvm, devvm adds the agent to that VM in place. In a terminal, it asks you first. When this machine has config folders for the agent, such as `~/.claude`, devvm has to stop the VM once to mount them, so before it asks, it warns you that anything running in the VM stops, and afterwards it starts the VM again. Without a terminal, devvm changes nothing and tells you to run the command in one. [Adding to a VM](commands.md#adding-to-a-vm) has the details.

[Everyday commands](commands.md) lists the rest. Before you let an agent loose in the VM, read [staying safe](../safety/_index.md#the-habits), which lists the habits that close most of what devvm leaves open.
