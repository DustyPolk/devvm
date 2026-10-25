---
title: Commands
weight: 15
---

Every command, with all its flags: the [CLI reference](cli/_index.md).

## VMs

| | |
| --- | --- |
| `devvm [IMAGE]` | open a shell in this folder's VM, at the same path, creating the VM first: from IMAGE, else the image the folder's files call for ([details](images.md#which-image-a-new-vm-gets)) |
| `devvm restart` | stop this folder's VM, then start it and open a shell in it: for what applies only when it starts |
| `devvm update` | move this folder's VM to the newest shared tools, then open a shell in it ([details](images.md#adding-and-removing-tools)) |
| `devvm remove TOOLS` | take shared tools out of this folder's VM (`devvm remove python+claude`), then open a shell in it |
| `devvm rm [-f]` | delete this folder's VM, exporting its agents' history first ([details](#deleting-a-vm)) |
| `devvm adopt` | use the VM made for an earlier folder at this path here ([details](#a-folder-made-again)) |
| `devvm ls` | list the project VMs ([details](#listing-vms)) |

A new VM is made from its image (built first if it isn't, with its [shared tools](images.md#shared-tools)), with only the project folder mounted, read-write, the shared tools and its [agents' config folders](coding-agents.md) read-only, and its `.git` hooks and config locked ([`.git` protection](git-protection.md)); with `--clone`, the folder isn't mounted and the VM gets its own clone of the repo ([working on a clone](clone.md)). Then devvm starts it, sets it up ([the VM's setup](coding-agents.md#the-vms-setup)), makes it reachable [by name over SSH](ssh.md) and opens a shell in it, in place of devvm: when the shell ends, so does devvm. Without a terminal, the shell reads its commands from devvm's input.

Options for a new VM:

| | |
| --- | --- |
| `--network-policy` | create the VM with [network policy](network.md): it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here |
| `--no-network-policy` | create the VM without network policy, so it reaches anything this machine can |
| `--clone` | create the VM with its own clone of the repo instead of mounting the folder ([details](clone.md)) |
| `--no-git-protect` | create the VM without `.git` protection |
| `--ssh-agent` | create the VM forwarding your SSH agent: while you're connected, anything in it can use your keys ([details](ssh.md#agent-forwarding)) |
| `--unsafe-folder` | mount a folder devvm refuses (one that is or holds your home folder or devvm's own, or is inside devvm's own) anyway ([details](security.md#only-the-project-folder)) |
| `--cpus N`, `--memory GiB`, `--disk GiB` | the VM's size ([details](#size-and-ports)) |
| `--ports all\|none\|LIST` | which ports the VM forwards ([details](#size-and-ports)) |
| `--backend NAME` | the [backend](backends.md#choosing-a-backend) that runs it |
| `--from IMAGE` | start its base from a container image (`node:24`) instead of Ubuntu's cloud image ([details](from.md)) |

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

### An existing VM

`devvm` in a folder whose VM exists opens it as it is, starting it if it's stopped. Each time, in this order:

1. With an IMAGE that names what the VM lacks (`devvm go` in a base VM), devvm asks before adding it in place ([below](#adding-to-a-vm)).
2. It warns when an image the VM was made from has changed since ([outdated VMs](images.md#outdated-vms)), or there are newer shared tools than the VM's (`devvm update` moves it to them), and checks for newer tools once a day.
3. It matches the VM's [agent folder mounts](coding-agents.md#mounts) to the folders on this machine.
4. The options for new VMs are ignored, each with a warning: the VM keeps its network policy (`devvm network` changes it), `.git` protection, size, ports, SSH agent forwarding (`devvm ssh-agent` changes it), backend and `--from`.
5. It checks `.git` ([`.git` protection](git-protection.md#on-each-launch)) and, on Linux hosts, names set-id files in the folder; not for a `--clone` VM, which mounts neither.
6. A stopped VM gets the size, ports and [SSH agent forwarding](ssh.md#agent-forwarding) its state asks for; a running one gets them when it next starts, and devvm says so. A VM that forwards your agent has it checked for keys; one that uses devvm's own gets the keys it lacks loaded.
7. It starts the VM if it's stopped. A running VM with network policy keeps its proxy, unless what the proxy was started with has changed ([the proxy](network.md#the-proxy)); so does one whose network policy was turned off since it started confined, until it next starts.
8. The [self-check](network.md#the-self-check) runs: a VM whose network policy doesn't hold is stopped.
9. It sets the VM up and opens the shell, as for a new VM. A `--clone` VM whose clone wasn't made yet gets it first.

A VM whose creation didn't finish (devvm was stopped after making it, before it recorded the network choice) is never started: it would start without network policy. `devvm rm` deletes it.

### Adding to a VM

`devvm IMAGE` names an image as for a new VM, agents alone on top of the folder's toolchains. What the VM's image lacks of it is added in place, after you confirm in a terminal; without one, devvm stops and changes nothing:

- Shared tools are built if the share lacks them, and join the VM's list: they're mounted at once, nothing is installed in its disk.
- An agent's config folders are mounted, which stops the VM first: anything running in it stops.
- A layer's script runs in the VM, as when it builds an image. A script that fails leaves the VM as it was otherwise.

The VM's image is then its old one with them. `devvm remove TOOLS` takes shared tools out again; layers can't be, as their scripts changed the VM's disk: save your work outside the project folder, `devvm rm`, and make the VM again.

### Deleting a VM

`devvm rm` deletes this folder's VM and what devvm keeps for it: its state, its own allowlist and secret files, its network log, its `.git` locks, its pinned host keys and its ssh config entry. Nothing else: other VMs, images and the project folder stay.

When the VM's image has an agent, devvm first exports the agents' history from it, started if it's stopped ([history](coding-agents.md#history)). An export that fails keeps the VM. A `--clone` VM's clone is checked before that, for commits this repo hasn't fetched and uncommitted changes, and the repo's remote for it goes with it, its fetched work kept as local branches ([deleting the VM](clone.md#deleting-the-vm)). `devvm rm -f` deletes it without an export or those checks.

### A folder made again

A VM belongs to the folder it was made for: its path, and the folder itself ([one VM per folder](security.md#one-vm-per-folder)). When that folder was deleted and a new one made at the same path, devvm, `devvm restart`, `update` and `remove` refuse the VM. Two commands deal with it, each only after you confirm in a terminal:

- `devvm rm` deletes it, without an export (its folder is gone), with `-f` or not. It may hold that project's work.
- `devvm adopt` gives it to this folder, with its files, tokens and allowlist. Its `.git` protection starts again, as if `.git` had just appeared.

### Listing VMs

`devvm ls` prints a table of the project VMs on every backend, sorted by name, a header first. Images and build VMs aren't in it.

| Column | |
| --- | --- |
| `NAME` | the VM's name: `devvm-<folder>-<hash>` |
| `STATUS` | as its backend reports it: `Running`, `Stopped`, or another state it has |
| `IMAGE` | its image, with what it added in place; a base from a container image as `base (from node:24)` ([details](from.md)) |
| `GIT` | its [`.git` protection](git-protection.md): `on` or `off`; `clone` for a [`--clone`](clone.md) VM |
| `NET` | its [network policy](network.md): `on`, `off`, or `unenforced` ([the self-check](network.md#the-self-check)) |
| `BACKEND` | the [backend](backends.md) that runs it |
| `FOLDER` | its project folder |

A value the VM's state doesn't have is `-`: a VM whose creation didn't finish has only its name, status and backend. A backend that can't list its VMs (Lima not installed, say) fails `devvm ls` only when a VM's state names it.

## Network policy

| | |
| --- | --- |
| `devvm network [on\|off]` | show this folder's VM's network policy (`on`, `off` or `unenforced`), or change it from its next start: `devvm restart` ([details](network.md#turning-it-on-or-off)) |
| `devvm policy` | show the allowlist of this folder's VM, file by file ([details](network.md#changing-rules)) |
| `devvm policy allow\|deny\|rm RULE... [-g]` | add rules to this folder's VM's own, as allows or denies, or remove them; `-g`: yours for every VM. Changes apply at once |
| `devvm log [-f] [-n N] [--blocked]` | show what this folder's VM connected to and what its [network policy](network.md) blocked: the last 50 unless `-n`; `-f` follows; `--blocked` lists the blocks by host, with the command that allows each ([details](network.md#logs)) |
| `devvm secret set claude\|github [--stdin]`, `devvm auth claude` | save a token on this machine; with network policy, VMs get a placeholder and their proxy puts the token in ([details](network.md#saving-and-binding-tokens)) |
| `devvm secret use\|unuse NAME` | let this folder's VM use a token, or stop it (`claude`: automatic for an image with `claude`) |
| `devvm secret ls`, `devvm secret rm NAME` | list the tokens, whether each is saved and the VMs using it; delete a saved one |

## SSH

| | |
| --- | --- |
| `devvm ssh-agent [on\|devvm\|off]` | show this folder's VM's SSH agent forwarding (`off`, `on`: yours, `devvm`: devvm's own), or change it from its next start: `devvm restart` ([details](ssh.md#agent-forwarding)) |
| `devvm ssh-agent add KEY_FILE [--confirm] [--lifetime DURATION]` | load a key into devvm's own agent, and record it, so it's loaded again when a VM needs it ([details](ssh.md#devvms-own-agent)) |
| `devvm ssh-agent keys` | list the keys devvm's own agent holds, then those added to it |

## Images

| | |
| --- | --- |
| `devvm build IMAGE` | build an image and the images under it, unless they're built, and the shared tool images it names (`devvm build foo+bar`, `devvm build node+claude`) |
| `devvm rebuild IMAGE` | build an image again, from its parent as it is, and delete the stale images no VM uses, the old one and those built on it among them ([details](images.md#rebuilding)); shared tools follow the tools lock instead |

`--backend NAME` builds on that backend instead of the one new VMs get, and `--from IMAGE` builds on a base from that container image ([details](from.md)).

## devvm itself

| | |
| --- | --- |
| `devvm doctor` | check what devvm needs on this machine and say how to get what's missing, then list each backend's features ([details](install.md#checking-what-devvm-needs)) |
| `devvm --version` | print devvm's version, alone (`devvm version` does too) |
| `devvm help [COMMAND]` | print the help for devvm or a command, as `--help` and `-h` do |
| `devvm completion SHELL` | print the shell completion script for `bash`, `zsh` or `fish` ([details](#shell-completion)) |

A command's name is never an IMAGE: `devvm build` builds. For one of your layers named like a command, write it as `base+NAME`.

### Shell completion

The script completes commands, flags, image names (with `+`, after the parts already given), shared tools for `devvm remove`, and `--backend` and `--ports` values. Load it from your shell's startup file:

| Shell | Line | In |
| --- | --- | --- |
| bash | `source <(devvm completion bash)` | `~/.bashrc`, with the bash-completion package installed |
| zsh | `source <(devvm completion zsh)` | `~/.zshrc`, after `compinit` |
| fish | `devvm completion fish \| source` | `~/.config/fish/config.fish` |

## Environment

| | |
| --- | --- |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](images.md#build-logs-and-interrupts)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image new VMs' and builds' base starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#versions)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads tokens in, instead of `devvm` ([details](network.md#saving-and-binding-tokens)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude token VMs' proxies put in, over a saved one ([details](network.md#secrets)) |
