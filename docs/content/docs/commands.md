---
title: Commands
weight: 15
---

This page lists devvm's commands by task. The [CLI reference](cli/_index.md) has every command with all its flags.

## VMs

| | |
| --- | --- |
| `devvm [IMAGE]` | open a shell in this folder's VM, at the same path, creating the VM first: from IMAGE, else the image the folder's files call for ([details](images.md#which-image-a-new-vm-gets)) |
| `devvm restart` | stop this folder's VM, then start it and open a shell in it: for what applies only when it starts |
| `devvm stop` | stop this folder's VM ([details](#stopping-a-vm)) |
| `devvm update` | move this folder's VM to the newest shared tools, then open a shell in it ([details](images.md#adding-and-removing-tools)) |
| `devvm remove TOOLS` | take shared tools out of this folder's VM (`devvm remove python+claude`), then open a shell in it |
| `devvm export [FILE]` | save this folder's VM's agents' history to a `.tar.gz`: FILE, else a new file in `~/.local/share/devvm/exports` ([details](coding-agents.md#history)) |
| `devvm rm [-f]` | delete this folder's VM, exporting its agents' history first ([details](#deleting-a-vm)) |
| `devvm adopt` | use the VM made for an earlier folder at this path here ([details](#a-folder-made-again)) |
| `devvm ls` | list the project VMs ([details](#listing-vms)) |
| `devvm ports [all\|none\|LIST]` | show or change which ports this folder's VM forwards to this machine's `localhost`: at once when it's stopped, from its next start when it runs ([details](images.md#ports)) |
| `devvm ports add\|rm LIST` | forward more ports, or stop forwarding some |
| `devvm ports -g ...` | the same for what new VMs forward |
| `devvm size [--cpus N] [--memory GiB] [--disk GiB]` | show or change this folder's VM's CPUs, memory and disk: at once when it's stopped, from its next start when it runs; disks only grow ([details](images.md#size)) |
| `devvm size -g ...` | the same for the size new VMs get |
| `devvm git-protect [on\|strict\|off]` | show or change this folder's VM's `.git` protection, while it's stopped ([details](git-protection.md#turning-it-on-or-off)) |

devvm makes a new VM from its image, building the image first with its [shared tools](images.md#shared-tools) if it isn't built yet. The VM mounts only the project folder, read-write, and the shared tools and its [agents' config folders](coding-agents.md), read-only. devvm locks the folder's `.git` hooks and config ([`.git` protection](git-protection.md)). With `--clone`, devvm doesn't mount the folder, and the VM gets its own clone of the repo instead ([working on a clone](clone.md)).

Then devvm starts the VM, sets it up ([the VM's setup](coding-agents.md#the-vms-setup)), makes it reachable [by name over SSH](ssh.md), and opens a shell in it in devvm's place, so devvm ends when the shell does. Without a terminal, the shell reads its commands from devvm's input.

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
| `--no-devcontainer` | create the VM without reading the folder's `devcontainer.json` ([details](devcontainer.md)) |

A folder with a `.devcontainer/devcontainer.json` or `.devcontainer.json` gets the VM that file describes, as far as devvm supports it. That covers its image, ports, environment and commands, and the features devvm has layers for ([`devcontainer.json`](devcontainer.md)). The options you give take priority over the file.

If you give neither network policy flag, devvm asks in a terminal, with no as the default. Without a terminal, the VM gets no network policy.

Options that take a value also work with an equals sign, such as `--cpus=N`.

### Size and ports

A new VM gets its image's size unless you set one. For `base.yaml` that's 4 CPUs, 8 GiB of memory and a 100 GiB disk.

- `--cpus` takes a whole number. `--memory` and `--disk` take a size in GiB, written `16`, `16G` or `16GiB`, and decimals are allowed.
- What the flags leave unset comes from `~/.config/devvm/size` when that file sets it, with one `cpus=`, `memory=` or `disk=` line each. `devvm size -g` writes that file.
- devvm refuses more CPUs or memory than this machine has, and a disk smaller than the image's, because disks can't shrink. It refuses them before it builds anything.

devvm forwards the TCP ports the VM listens on to the same port on this machine's `localhost`. It forwards every one from 1024 up unless `--ports` says otherwise ([how](images.md#ports)).

- `--ports` takes `all`, `none`, or ports and ranges like `5173,8000-8010`.
- Without `--ports`, the default comes from `~/.config/devvm/ports`, which holds one line: `all`, `none` or a list. `devvm ports -g` writes that file.
- `devvm ports` changes an existing VM's ports ([ports](images.md#ports)), and `devvm size` changes its size ([size](images.md#size)).

devvm refuses a size or ports file that doesn't hold a valid value, as it refuses a bad flag.

### An existing VM

When you run `devvm` in a folder whose VM exists, devvm opens the VM as it is and starts it if it's stopped. Each time, it does these steps in this order:

1. When you give an IMAGE that names something the VM lacks, such as `devvm go` in a base VM, devvm asks before adding it in place ([below](#adding-to-a-vm)).
2. devvm warns when an image the VM was made from has changed since ([outdated VMs](images.md#outdated-vms)), or when there are newer shared tools than the VM's, which `devvm update` moves it to. It also checks for newer tools once a day.
3. devvm matches the VM's [agent folder mounts](coding-agents.md#mounts) to the folders on this machine.
4. devvm ignores the options for new VMs, with a warning for each. The VM keeps its network policy, which `devvm network` changes, and its `.git` protection, which `devvm git-protect` changes. It keeps its size, which `devvm size` changes, its ports, which `devvm ports` changes, and its SSH agent forwarding, which `devvm ssh-agent` changes. It also keeps its backend, and ignores `--from` and `--no-devcontainer`.
5. devvm warns when the folder's `devcontainer.json` isn't the one the VM was made from ([details](devcontainer.md#when-its-read)). It checks `.git` ([`.git` protection](git-protection.md#on-each-launch)) and, on Linux hosts, names set-id files in the folder. It skips both for a `--clone` VM, which mounts neither.
6. A stopped VM gets the size, ports and [SSH agent forwarding](ssh.md#agent-forwarding) that its state asks for. A running VM gets them when it next starts, and devvm says so. When the VM forwards your agent, devvm checks that the agent has keys. When it uses devvm's own agent, devvm loads the keys the agent lacks.
7. devvm starts the VM if it's stopped. A running VM with network policy keeps its proxy, unless what the proxy was started with has changed ([the proxy](network.md#the-proxy)). A VM whose network policy was turned off after it started confined also keeps its proxy, until it next starts.
8. The [self-check](network.md#the-self-check) runs, and devvm stops a VM whose network policy doesn't hold.
9. devvm sets the VM up and opens the shell, as for a new VM. A `--clone` VM whose clone wasn't made yet gets it first. On the first launch after the VM started, the `postStartCommand` of the `devcontainer.json` it was made from runs before the shell opens ([commands](devcontainer.md#commands)).

devvm never starts a VM whose creation didn't finish, because it would start without network policy. That happens when devvm was stopped after making the VM but before it recorded the network choice. `devvm rm` deletes such a VM.

### Adding to a VM

`devvm IMAGE` names an image as it does for a new VM, with agents alone on top of the folder's toolchains. devvm adds what the VM's image lacks of it in place, after you confirm in a terminal. Without a terminal, devvm stops and changes nothing.

- devvm builds shared tools that the share lacks, and they join the VM's list. They're mounted at once, and nothing is installed in the VM's disk.
- devvm mounts an agent's config folders, which stops the VM first, so anything running in it stops.
- A layer's script runs in the VM, as when devvm builds an image. With network policy, the VM's proxy allows the layer's hosts for it. If the script fails, the VM is otherwise left as it was.

The VM's image is then its old one with these added. `devvm remove TOOLS` takes shared tools out again. Layers can't be taken out, because their scripts changed the VM's disk, so save your work outside the project folder, run `devvm rm`, and make the VM again.

### Stopping a VM

`devvm stop` stops this folder's VM, then its [proxy](network.md#the-proxy). Once the VM is stopped, devvm checks `.git` as a launch that finds the VM stopped does, and sets the locks that a running VM was waiting for ([on each launch](git-protection.md#on-each-launch)). On Linux it also names set-id files in the folder. It skips both for a `--clone` VM. devvm removes the VM's [ssh config entry](ssh.md). A VM that's already stopped gets the same checks.

### Deleting a VM

`devvm rm` deletes this folder's VM and what devvm keeps for it: its state, its own allowlist and secret files, its network log, its `.git` locks, its pinned host keys and its ssh config entry. Other VMs, images and the project folder stay.

When the VM's image has an agent, devvm first exports the agents' history from the VM, starting it if it's stopped ([history](coding-agents.md#history)). If the export fails, devvm keeps the VM. Before that, devvm checks a `--clone` VM's clone for commits this repo hasn't fetched and for uncommitted changes. The repo's remote for the VM goes with it, and devvm keeps its fetched work as local branches ([deleting the VM](clone.md#deleting-the-vm)). `devvm rm -f` deletes the VM without an export or those checks.

### A folder made again

A VM belongs to the folder it was made for, by its path and by the folder itself ([one VM per folder](security.md#one-vm-per-folder)). When that folder was deleted and a new one made at the same path, devvm, `devvm restart`, `update` and `remove` refuse the VM. Two commands deal with it, each only after you confirm in a terminal:

- `devvm rm` deletes the VM, with `-f` or without it, and never exports its history, because its folder is gone. The VM may hold that project's work.
- `devvm adopt` gives the VM to this folder, with its files, tokens and allowlist. Its `.git` protection starts again, as if `.git` had just appeared.

### Listing VMs

`devvm ls` prints a table of the project VMs on every backend, sorted by name, with a header first. Images and build VMs aren't in it.

| Column | |
| --- | --- |
| `NAME` | the VM's name: `devvm-<folder>-<hash>` |
| `STATUS` | as its backend reports it: `Running`, `Stopped`, or another state it has |
| `IMAGE` | its image, with what it added in place; a base from a container image as `base (from node:24)` ([details](from.md)) |
| `GIT` | its [`.git` protection](git-protection.md): `on`, `strict` or `off`; `clone` for a [`--clone`](clone.md) VM |
| `NET` | its [network policy](network.md): `on`, `off`, or `unenforced` ([the self-check](network.md#the-self-check)) |
| `BACKEND` | the [backend](backends.md) that runs it |
| `FOLDER` | its project folder |

A value the VM's state doesn't have shows as `-`, so a VM whose creation didn't finish shows only its name, status and backend. A backend that can't list its VMs, for example because Lima isn't installed, fails `devvm ls` only when a VM's state names that backend.

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
| `devvm ssh` | get this folder's VM ready for ssh and editors, started if it's stopped, and show the name they reach it by: `devvm-website` ([details](ssh.md#devvm-ssh-and-devvm-code)) |
| `devvm code` | the same, then open its folder in VS Code over Remote-SSH ([VS Code](ssh.md#vs-code)) |

## Images

| | |
| --- | --- |
| `devvm build IMAGE` | build an image and the images under it, unless they're built, and the shared tool images it names (`devvm build foo+bar`, `devvm build node+claude`) |
| `devvm rebuild IMAGE` | build an image again, from its parent as it is, and delete the stale images no VM uses, the old one and those built on it among them ([details](images.md#rebuilding)); shared tools follow the tools lock instead |
| `devvm layer new NAME` | start a layer of your own: `~/.config/devvm/layers/NAME.sh`, a script to edit, and its allowlist, `NAME.policy` ([details](customizing.md#your-own-layers)) |
| `devvm images` | list the images on every backend, with their state and size, then the shared tool images, with how many VMs use each ([details](images.md#listing-images)) |
| `devvm images rm IMAGE\|KEY...` | delete images, by name or by the key `devvm images` shows ([details](images.md#deleting-images)) |
| `devvm gc` | delete the stale and unfinished images on every backend, the tool images no VM uses that aren't the tools lock's, and old copies of devvm's files; it lists them first, and in a terminal asks. VMs are never deleted ([details](images.md#cleaning-up)) |

With `--backend NAME`, these commands build or delete on that backend instead of the one new VMs get. With `--from IMAGE`, they work on a base made from that container image ([details](from.md)).

## devvm itself

| | |
| --- | --- |
| `devvm doctor` | check what devvm needs on this machine and say how to get what's missing, then list each backend's features ([details](install.md#checking-what-devvm-needs)) |
| `devvm --version` | print devvm's version, alone (`devvm version` does too) |
| `devvm help [COMMAND]` | print the help for devvm or a command, as `--help` and `-h` do |
| `devvm completion SHELL` | print the shell completion script for `bash`, `zsh` or `fish` ([details](#shell-completion)) |
| `devvm limactl ARGS...` | run Lima's `limactl` on the lima backend's VMs and images, which live in devvm's own Lima home, not `~/.lima` ([details](lima.md#running-limactl)) |

devvm never reads a command's name as an IMAGE, so `devvm build` runs the build command. To use one of your layers that's named like a command, write it as `base+NAME`.

### Shell completion

The completion script completes commands, flags and image names, including names joined with `+` after the parts you've already typed. It also completes shared tools for `devvm remove`, `--backend` values, and `all` and `none` for `--ports` and `devvm ports`. Load it from your shell's startup file:

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
