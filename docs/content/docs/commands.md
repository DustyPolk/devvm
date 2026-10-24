---
title: Commands
weight: 15
---

Every command, with all its flags: the [CLI reference](cli/_index.md).

## VMs

| | |
| --- | --- |
| `devvm [IMAGE]` | open a shell in this folder's VM, at the same path, creating the VM first: from IMAGE, else the image the folder's files call for ([details](images.md#which-image-a-new-vm-gets)) |

A new VM is made from its image (built first if it isn't, with its [shared tools](images.md#shared-tools)), with only the project folder mounted, read-write, the shared tools and its [agents' config folders](coding-agents.md) read-only, and its `.git` hooks and config locked ([`.git` protection](git-protection.md)). Then devvm starts it, sets it up ([the VM's setup](coding-agents.md#the-vms-setup)) and opens a shell in it, in place of devvm: when the shell ends, so does devvm. Without a terminal, the shell reads its commands from devvm's input.

devvm doesn't open a VM that already exists yet.

Options for a new VM:

| | |
| --- | --- |
| `--network-policy` | create the VM with [network policy](network.md): it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here |
| `--no-network-policy` | create the VM without network policy, so it reaches anything this machine can |
| `--no-git-protect` | create the VM without `.git` protection |
| `--unsafe-folder` | mount a folder devvm refuses (one that is or holds your home folder or devvm's own, or is inside devvm's own) anyway ([details](security.md#only-the-project-folder)) |
| `--cpus N`, `--memory GiB`, `--disk GiB` | the VM's size ([details](#size-and-ports)) |
| `--ports all\|none\|LIST` | which ports the VM forwards ([details](#size-and-ports)) |
| `--backend NAME` | the [backend](backends.md#choosing-a-backend) that runs it |

Without either network policy flag devvm asks in a terminal, no by default; without a terminal the VM gets no network policy.

Options that take a value also work as `--cpus=N` and so on.

### Size and ports

A new VM gets its image's size (`base.yaml`'s: 4 CPUs, 8 GiB of memory and a 100 GiB disk) unless you set one:

- `--cpus` takes a whole number, `--memory` and `--disk` a size in GiB: `16`, `16G` or `16GiB`, decimals allowed.
- What the flags leave unset comes from `~/.config/devvm/size`, one `cpus=`, `memory=` or `disk=` line each, if it has one.
- devvm refuses more CPUs or memory than this machine has, and a disk smaller than the image's (disks can't shrink), before it builds anything.

Ports the VM listens on are forwarded to the same port on this machine's `localhost`: all of them, unless `--ports` says otherwise:

- `all`, `none`, or ports and ranges like `5173,8000-8010`.
- Without `--ports`, `~/.config/devvm/ports` holds the default, one line: `all`, `none` or a list.

A size or ports file that doesn't hold one is refused, like a bad flag.

## Images

| | |
| --- | --- |
| `devvm build IMAGE` | build an image and the images under it, unless they're built, and the shared tool images it names (`devvm build foo+bar`, `devvm build node+claude`) |
| `devvm rebuild IMAGE` | build an image again, from its parent as it is, and delete the stale images no VM uses, the old one and those built on it among them ([details](images.md#rebuilding)); shared tools follow the tools lock instead |

`--backend NAME` builds on that backend instead of the one new VMs get.

## Environment

| | |
| --- | --- |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](images.md#build-logs-and-interrupts)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](lima.md#a-lima-home-of-its-own)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#versions)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm reads tokens from, instead of `devvm` ([details](network.md#secrets)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude token VMs' proxies put in, over a saved one ([details](network.md#secrets)) |
