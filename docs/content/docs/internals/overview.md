---
title: Overview
weight: 1
---

This page explains what devvm does, step by step, when it opens, stops and deletes a VM.

## A new VM

devvm makes a new VM from its image, building the image first with its [shared tools](../using/images.md#shared-tools) if it isn't built yet. The VM mounts only the project folder, read-write, and the shared tools and its [agents' config folders](../using/coding-agents.md), read-only. devvm locks the folder's `.git` hooks and config ([`.git` protection](../safety/git-protection.md)). With `--clone`, devvm doesn't mount the folder, and the VM gets its own clone of the repo instead ([working on a clone](../using/clone.md)).

Then devvm starts the VM, sets it up ([the VM's setup](../using/coding-agents.md#the-vms-setup)), makes it reachable [by name over SSH](../using/ssh.md), and opens a shell in it in devvm's place, so devvm ends when the shell does. Without a terminal, the shell reads its commands from devvm's input.

## Adding to a VM

When `devvm IMAGE` adds to an existing VM in place ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)), the shared tools it builds are mounted at once, and nothing is installed in the VM's disk. With network policy, the VM's proxy allows a layer's hosts for its script.

## Listing VMs

A backend that can't list its VMs, for example because Lima isn't installed, fails `devvm ls` only when a VM's state names that backend.

## An existing VM

When you run `devvm` in a folder whose VM exists, devvm opens the VM as it is and starts it if it's stopped. Each time, it does these steps in this order:

1. When you give an IMAGE that names something the VM lacks, such as `devvm go` in a base VM, devvm asks before adding it in place ([below](../getting-started/commands.md#adding-to-a-vm)).
2. devvm warns when an image the VM was made from has changed since ([outdated VMs](images.md#outdated-vms)), or when there are newer shared tools than the VM's, which `devvm update` moves it to. It also checks for newer tools once a day.
3. devvm matches the VM's [agent folder mounts](../using/coding-agents.md#mounts) to the folders on this machine.
4. devvm ignores the options for new VMs, with a warning for each. The VM keeps its network policy, which `devvm network` changes, and its `.git` protection, which `devvm git-protect` changes. It keeps its size, which `devvm size` changes, its ports, which `devvm ports` changes, and its SSH agent forwarding, which `devvm ssh-agent` changes. It also keeps its backend, and ignores `--from` and `--no-devcontainer`.
5. devvm warns when the folder's `devcontainer.json` isn't the one the VM was made from ([details](../using/devcontainer.md#when-its-read)). It checks `.git` ([`.git` protection](git-protection.md#on-each-launch)) and, on Linux hosts, names set-id files in the folder. It skips both for a `--clone` VM, which mounts neither.
6. A stopped VM gets the size and [SSH agent forwarding](../using/ssh.md#agent-forwarding) that its state asks for. A running VM gets them when it next starts, and devvm says so. When the VM forwards your agent, devvm checks that the agent has keys. When it uses devvm's own agent, devvm loads the keys the agent lacks.
7. devvm starts the VM if it's stopped. A running VM with network policy keeps its proxy, unless what the proxy was started with has changed ([the proxy](network.md#the-proxy)). A VM whose network policy was turned off after it started confined also keeps its proxy, until it next starts.
8. The [self-check](network.md#the-self-check) runs, and devvm stops a VM whose network policy doesn't hold.
9. devvm sets the VM up and opens the shell, as for a new VM. A `--clone` VM whose clone wasn't made yet gets it first. On the first launch after the VM started, the `postStartCommand` of the `devcontainer.json` it was made from runs before the shell opens ([commands](../using/devcontainer.md#commands)).

devvm never starts a VM whose creation didn't finish, because it would start without network policy. That happens when devvm was stopped after making the VM but before it recorded the network choice. `devvm rm` deletes such a VM.

## Stopping a VM

`devvm stop` stops this folder's VM, then its [proxy](network.md#the-proxy). Once the VM is stopped, devvm checks `.git` as a launch that finds the VM stopped does, and sets the locks that a running VM was waiting for ([on each launch](git-protection.md#on-each-launch)). On Linux it also names set-id files in the folder. It skips both for a `--clone` VM. devvm removes the VM's [ssh config entry](../using/ssh.md). A VM that's already stopped gets the same checks.

## Deleting a VM

`devvm rm` deletes this folder's VM and what devvm keeps for it: its state, its own allowlist and secret files, its network log, its `.git` locks, its pinned host keys and its ssh config entry. Other VMs, images and the project folder stay.

When the VM's image has an agent, devvm first exports the agents' history from the VM, starting it if it's stopped ([history](../using/coding-agents.md#history)). If the export fails, devvm keeps the VM. Before that, devvm checks a `--clone` VM's clone for commits this repo hasn't fetched and for uncommitted changes. The repo's remote for the VM goes with it, and devvm keeps its fetched work as local branches ([deleting the VM](../using/clone.md#deleting-the-vm)). `devvm rm -f` deletes the VM without an export or those checks.

### Saved branches

The local branches are named `saved/<remote>/<branch>`, after the remote. devvm never overwrites a branch. If a branch named `saved/<remote>` exists, or one under it, or one it would sit under, devvm uses `saved/<remote>-2` instead, then `-3`, and so on up to `-99`. If none of those names is free, for example because a branch is named `saved`, `devvm rm` refuses before fetching and keeps the VM. When that happens as devvm is about to remove the remote, it leaves the remote and its branches in place. Because branch names come from the VM, devvm quotes them when it shows them and skips any that git refuses as branch names.

## State files

devvm writes nothing into the project folder. It keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/<key>`), rebuild counters (`images/rebuilds/`) and `--from` digests (`images/from-digests`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`), ports (`ports`) and [backend](../backends/_index.md#choosing-a-backend) (`backend`) of new VMs, your own layers and features, the keys in devvm's SSH agent (`agent-keys`), and the [ssh config](../using/ssh.md) for VMs and their host keys (`ssh_config`, `known_hosts`) |

A VM's state file records the image the VM came from, its project folder, its backend and its settings, one `key=value` per line. The file lasts as long as the VM, because `devvm rm` also deletes it, so the next devvm in that folder starts fresh.

Each write replaces a file whole, so a file is never left half written, and only you can read the files. When a file can't be read, devvm names the file and the line, and you fix the line or delete the file.
