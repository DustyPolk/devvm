---
title: Everyday commands
weight: 3
---

This page lists devvm's commands in the groups that `devvm --help` shows them in, and each row links to the page with the details. `devvm COMMAND --help` shows a command's examples and flags, and it ends with the address of the page that covers the command. The [CLI reference](../cli/_index.md) has every command with all its flags.

## Your VM

On this machine, devvm makes the VM and opens a shell in it:

```sh
devvm                   # a new VM, its tools checked in a list, or a shell in the folder's VM
devvm --agents claude   # a new VM with Claude Code and the project's toolchains, without the list
devvm tools             # check or uncheck the folder's existing VM's toolchains and agents
devvm tools add claude  # add Claude Code to the folder's existing VM, without the list
```

You start the agent in the VM's shell, whose prompt starts with `(devvm)`, so the agent runs in the VM:

```console
(devvm) ~/some-project $ claude
```

[What a new VM gets](#what-a-new-vm-gets) explains the list and its flags, and [adding to a VM](#adding-to-a-vm) explains what adding an agent to an existing VM does.

| | |
| --- | --- |
| `devvm` | open a shell in this folder's VM, at the same path, making the VM first with the toolchains, agents and layers you check in a list, or that `--tools`, `--agents` and `--layers` name ([details](../using/images.md#what-a-new-vm-gets)) |
| `devvm restart` | stop this folder's VM, start it again and open a shell in it, for changes that apply only when it starts ([details](#restarting-and-stopping)) |
| `devvm stop` | stop this folder's VM ([details](#restarting-and-stopping)) |
| `devvm rm [--force] [--no-export] [--no-clone-check]` | delete this folder's VM after asking, exporting its agents' history first unless `--no-export` ([details](#deleting-a-vm)) |
| `devvm adopt` | use the VM made for an earlier folder at this path here ([details](#a-folder-made-again)) |
| `devvm ls` | list the project VMs ([details](#listing-vms)) |
| `devvm code` | get this folder's VM ready as `devvm host` does, then open its folder in VS Code over Remote-SSH ([VS Code](../using/ssh.md#vs-code)) |
| `devvm desktop [--clipboard] [--no-open]` | serve this folder's VM's desktop on `127.0.0.1` with its password until Ctrl+C, and open it in Screen Sharing on macOS ([details](../using/desktop.md)) |
| `devvm host` | get this folder's VM ready for ssh and editors, and show the host name they reach it by, such as `devvm-website` ([details](../using/ssh.md#devvm-host-and-devvm-code)) |
| `devvm history` | list the exports of agents' history that devvm saved in `~/.local/share/devvm/exports` ([details](../using/coding-agents.md#history)) |
| `devvm history export [FILE]` | save the history of this folder's VM's agents to FILE, or to a new `.tar.gz` in `~/.local/share/devvm/exports` ([details](../using/coding-agents.md#history)) |

In a folder without a VM of its own, every command that acts on this folder's VM, except `devvm adopt`, uses the VM of the nearest folder with one that it's inside, and devvm says which VM that is. `devvm rm` also asks before it deletes that VM, and so does each command that gives that VM more access before it changes the VM ([a subfolder](#a-subfolder)).

devvm makes a new VM from its image of base and the layers you chose, building the image first if it isn't built yet, then starts the VM and opens a shell in it in devvm's place, so devvm ends when the shell does. The VM mounts only the project folder, read-write, and the shared tools and its [agents' config folders](../using/coding-agents.md), read-only, or with `--clone` it gets its own clone of the repo instead of the folder. On macOS, devvm also locks the folder's `.git` hooks and config against changes from the VM ([.git protection](../safety/git-protection.md)). For a VM that exists, devvm starts it if it's stopped and opens it as it is. [Overview](../internals/overview.md) has each step devvm takes for a new VM and an existing one.

A VM is running or stopped from the time devvm makes it until you delete it.

```mermaid
flowchart TD
  none(["no VM"]) -- "devvm" --> running(["running"])
  running -- "devvm restart" --> running
  running -- "devvm stop" --> stopped(["stopped"])
  stopped -- "devvm,<br>devvm restart" --> running
  running -- "devvm rm" --> deleted(["deleted"])
  stopped -- "devvm rm" --> deleted
  outdated@{ shape: braces, label: "When its image changes, a VM<br>is outdated, and it keeps<br>its image until devvm rm." }
  running ~~~ outdated
```

These options are for a new VM, and only `devvm` takes them. Another command given one of them, such as `devvm ls --cpus 4`, fails as it does for any flag it doesn't know, and a VM that exists ignores them with a warning.

| | |
| --- | --- |
| `--tools LIST\|none` | the toolchains the VM gets, such as `--tools node,python`, in place of those the folder's files call for ([details](#what-a-new-vm-gets)) |
| `--agents LIST\|none` | the coding agents the VM gets, such as `--agents claude,codex`, in place of those `devvm setup` chose ([details](#what-a-new-vm-gets)) |
| `--layers LIST\|none` | the layers the VM's image gets, bottom first, such as `--layers docker`, in place of those `devvm setup` chose ([details](#what-a-new-vm-gets)) |
| `--network-policy` | create the VM with [network policy](../safety/network.md), which is the default: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine |
| `--no-network-policy` | create the VM without network policy, so it reaches anything this machine can |
| `--clone` | create the VM with its own clone of the repo instead of mounting the folder ([details](../using/clone.md)) |
| `--no-git-protect` | create the VM without `.git` protection |
| `--ssh-agent` | create the VM forwarding your SSH agent, so while you're connected, anything in it can use your keys ([details](../using/ssh.md#agent-forwarding)) |
| `--unsafe-folder` | mount a folder that devvm refuses, such as one that is or holds your home folder ([details](../using/settings.md#project-folders-devvm-refuses)) |
| `--cpus N`, `--memory GiB`, `--disk GiB` | the VM's size ([details](../using/ports-size.md#size-and-ports)) |
| `--ports all\|none\|LIST` | which ports the VM forwards ([details](../using/ports-size.md#size-and-ports)) |
| `--from IMAGE` | start its base from a container image, such as `node:24`, instead of Ubuntu's cloud image ([details](../using/from.md)) |
| `--no-devcontainer` | create the VM without reading the folder's `devcontainer.json` ([details](../using/devcontainer.md)) |
| `--here` | create a VM for this folder even when a folder it's inside has one, without asking whether to use that VM ([details](#a-subfolder)) |

Options that take a value also work with an equals sign, such as `--cpus=N`.

### What a new VM gets

In a terminal, devvm lists what a new VM can get under three headings, and you check what you want with the arrow keys and space, then press enter. The toolchains are node, python, rust and go, and those your folder's files call for are checked, each naming its file, such as `package.json`. The coding agents are claude, codex and opencode, and those you chose in `devvm setup` are checked. The layers are desktop, docker, nix and your own, and those you chose in `devvm setup` are checked. A layer goes into the VM's image, which devvm builds when it makes the VM. A layer a project file calls for, such as nix for a `flake.nix`, names the file but stays unchecked unless you chose it in `devvm setup`. The list comes before the network policy question.

`--tools`, `--agents` and `--layers` choose without the list, and any one of them skips it. Each is the exact list of its part, such as `--tools node,go`, or `none` for none of it, and a part whose flag you leave out gets what the list would have checked. Without a terminal, or with `--no-input`, devvm shows no list and doesn't fail: the VM gets those same defaults, with each flag you gave in place of its part, and devvm says in one line what the VM got and which flags choose otherwise. devvm checks each name before it does anything, and for a name it doesn't know, it suggests the closest one. [What a new VM gets](../using/images.md#what-a-new-vm-gets) has the details.

If you give neither network policy flag, devvm uses the answer you saved with `devvm network -g on` or `off`, or with `devvm setup`. Until you save one, devvm asks in a terminal, recommending yes, and without a terminal the VM gets network policy. [Turning it on or off](../safety/network.md#turning-it-on-or-off) covers the VMs that can't have it.

A folder with a `.devcontainer/devcontainer.json` or `.devcontainer.json` gets the VM that file describes, as far as devvm supports it. That covers its image, ports, environment and commands, and the features devvm has layers for, as [`devcontainer.json`](../using/devcontainer.md) explains. The options you give take priority over the file.

### Restarting and stopping

`devvm restart` stops this folder's VM, then starts it and opens a shell in it as devvm does. Some changes apply only when a VM starts, such as a new size for a running VM, a change to its network policy or SSH agent forwarding, or the config folders of an agent that `devvm tools add` added, and a restart applies them.

`devvm stop` stops this folder's VM and its network policy proxy ([stopping a VM](../internals/overview.md#stopping-a-vm)). Once the VM is stopped, devvm locks its `.git` again and checks it, as a launch that finds the VM stopped does, and it takes the VM's entry out of devvm's [ssh config](../using/ssh.md#ssh-access-by-name). devvm, `devvm restart`, and the commands that need the VM running, such as `devvm host`, start it again.

### Deleting a VM

`devvm rm` deletes this folder's VM together with what devvm keeps for it. That's the VM's state, its own allowlist, its network log, its `.git` locks or, for a `--clone` VM, the repo's `devvm` remote, and its ssh config entry ([deleting a VM](../internals/overview.md#deleting-a-vm)). It asks first in a terminal, and `--force` deletes without asking, which `devvm rm` needs when there's no terminal.

When the VM has an agent, devvm first exports the agents' history, unless you give `--no-export`, and when the export fails, devvm keeps the VM ([history](../using/coding-agents.md#history)). For a `--clone` VM, devvm offers to fetch the branches this repo hasn't fetched, and it deletes the VM's uncommitted changes only after you confirm. `--no-clone-check` skips that check and warns you that the clone's unfetched commits and uncommitted changes go with the VM ([deleting the VM](../using/clone.md#deleting-the-vm)).

devvm deletes the VM made for an earlier folder at this path only after you confirm ([a folder made again](#a-folder-made-again)). In a folder without a VM of its own, `devvm rm` acts on the VM of the nearest folder with one that this folder is inside, and it names that VM and deletes it only after you confirm ([a subfolder](#a-subfolder)). devvm asks these questions only in a terminal, and `--force` never answers them.

### A folder made again

A VM belongs to the folder it was made for, which devvm knows by its path and by the folder itself ([one VM per folder](../internals/security.md#one-vm-per-folder)). When that folder was deleted and a new one made at the same path, devvm, `devvm restart` and `devvm tools` refuse the VM. Two commands deal with it, and each acts only after you confirm in a terminal:

- `devvm rm` deletes the VM and never exports its history, because its folder is gone. The VM may hold that project's work, so `--force` never answers this question.
- `devvm adopt` gives the VM to this folder, with its files, secrets and allowlist. Its `.git` protection starts again, as if `.git` had just appeared. `devvm adopt --force` gives it without asking, which `devvm adopt` needs when there's no terminal.

### A subfolder

A VM mounts its whole project folder, so the folders inside it are in the VM at the same paths. When the folder you run a command in has no VM of its own, devvm looks for one in the folders it's inside, starting with the nearest. It stops before your home folder and the filesystem root, so it never uses a VM made for either. It stops at the nearest folder that has a VM, and when that VM was made for an earlier folder at its path ([a folder made again](#a-folder-made-again)), or its backend has lost it, the folder you're in has no VM, so devvm never reaches past it to a VM further out. Using a parent folder's VM mounts nothing new, because the subfolder is already inside the folder that VM mounts.

When you run `devvm` in such a subfolder in a terminal, devvm asks whether to open the parent folder's VM there instead of making a new VM, with yes as the default. If you answer yes, the shell opens in the VM at the subfolder's path. If you answer no, devvm makes a new VM for the subfolder alone. Without a terminal, devvm opens the parent folder's VM and names it and its folder first. `--tools`, `--agents` and `--layers` change nothing on the parent folder's VM, as on any VM that exists, and devvm warns that they don't ([adding to a VM](#adding-to-a-vm)). `devvm --here` makes a new VM for the subfolder without asking, and so does `devvm --clone`, which makes the subfolder's own clone VM. Once the subfolder has its own VM, devvm opens that one and asks nothing. In a parent folder's `--clone` VM, the shell opens in the subfolder when the clone has it ([subfolders](../using/clone.md#subfolders)).

The other commands that act on this folder's VM, such as `devvm stop`, `restart`, `tools`, `size`, `ports`, `host` and `code`, use the parent folder's VM in the same way, without asking. Each one first prints a line that names the VM and the folder it was made for. `devvm rm` is the exception, because it never deletes a parent folder's VM unasked. In a terminal, it names the VM and its folder and asks before deleting it, with no as the default, and `--force` never answers that question. Without a terminal, it refuses and tells you to run `devvm rm` in the parent folder instead. The commands that give the VM more access ask in the same way, because the parent folder's project may not expect them. They are `devvm secret use`, `devvm ssh-agent on` and `devvm ssh-agent devvm`, `devvm network off`, `devvm network allow`, `devvm network rm` when it removes a deny, and `devvm git-protect off`. In a terminal, each one names the VM and its folder and asks before it changes the VM, with no as the default. Without a terminal, it refuses and tells you to run it in the parent folder, or in a terminal to confirm. The commands that take access away, such as `devvm secret unuse` and `devvm network deny`, ask nothing. `devvm host` gives the subfolder as the folder to open in the VM, and `devvm code` opens the subfolder. `devvm adopt` acts only on the VM made for this folder's own path, never on a parent folder's.

A subfolder that is a git repo of its own is treated as a separate project, and so is a subfolder inside such a repo, when the repo is below the folder with the VM. That covers a nested repo, a submodule and a worktree, which each have a `.git` folder or file of their own. When you run `devvm` there in a terminal, devvm still asks, but the default is a new VM for the subfolder, so the parent folder's VM opens only if you answer yes. Without a terminal, devvm makes the new VM. The other commands never use the parent folder's VM there, and they say this folder has no VM, as in any folder without one.

### Listing VMs

`devvm ls` prints a table of the project VMs, sorted by name, with a header first. Images and build VMs aren't in it.

| Column | |
| --- | --- |
| `NAME` | the VM's name, `devvm-<folder>-<hash>` |
| `STATUS` | its state as its backend reports it, such as `Running` or `Stopped` |
| `IMAGE` | its image, base and its layers, with a base from a container image as `base (from node:24)` ([details](../using/from.md)) |
| `TOOLS` | its shared tools, joined with commas, such as `node,claude`, or `-` for none |
| `GIT` | its [`.git` protection](../safety/git-protection.md), `on`, `strict` or `off`, or `clone` for a [`--clone`](../using/clone.md) VM |
| `NET` | its [network policy](../safety/network.md), `on`, `off`, or `unenforced` ([the self-check](../internals/network.md#the-self-check)) |
| `LINKS` | how many [links](../using/links.md) it has to ports of your other VMs, or `-` for none |
| `FOLDER` | its project folder |

A value the VM's state doesn't have shows as `-`, so a VM whose creation didn't finish shows only its name and status.

### devvm ls --json

`devvm ls --json` prints the same list as one JSON object, as [output for scripts](#output-for-scripts) describes. `vms` has a VM's columns as fields, sorted by name. `image` is the image alone, `tools` is the list of its shared tools, `[]` for none, and `from` is the container image its base started from, which is left out for a base from Ubuntu's cloud image. A value the VM's state doesn't have is `""`. `links` is how many links the VM has, `0` for none.

```json
{
  "version": 1,
  "vms": [
    {
      "name": "devvm-website-71a99b1076d92c",
      "status": "Running",
      "image": "base",
      "tools": ["node", "claude"],
      "from": "node:24",
      "git": "on",
      "net": "on",
      "backend": "lima",
      "folder": "/Users/you/code/website",
      "links": 0
    }
  ]
}
```

### Working on a clone

| | |
| --- | --- |
| `devvm --clone` | make this folder's VM with its own clone of the repo, copy your branches into it and open a shell in the clone ([details](../using/clone.md)) |
| `git fetch devvm` | on this machine, fetch the VM's work as `devvm/<branch>`, to review before you merge it ([details](../using/clone.md#fetching-the-vms-work)) |
| `devvm rm` | delete the VM, first offering to keep the work you haven't fetched as `saved/devvm/<branch>` branches ([details](../using/clone.md#deleting-the-vm)) |

## Change your VM

| | |
| --- | --- |
| `devvm tools` | in a terminal, check or uncheck this folder's VM's toolchains and agents in a list, then open a shell in it, and otherwise show its shared tools, each with its version and any newer one ([details](#adding-to-a-vm)) |
| `devvm tools add TOOL...` | add shared tools to this folder's VM, such as `devvm tools add python claude`, then open a shell in it ([details](#adding-to-a-vm)) |
| `devvm tools rm TOOL...` | take shared tools out of this folder's VM, such as `devvm tools rm python claude`, then open a shell in it |
| `devvm tools update` | move this folder's VM to the newest shared tools, then open a shell in it ([details](../using/toolchains.md#adding-and-removing-tools)) |
| `devvm size [--cpus N] [--memory GiB] [--disk GiB]` | show or change this folder's VM's CPUs, memory and disk ([details](../using/ports-size.md#size)) |
| `devvm size -g ...` | show or change the size new VMs get ([details](../using/ports-size.md#size)) |
| `devvm ports [all\|none\|LIST]` | show or change which ports this folder's VM forwards to this machine's `localhost`, even while it runs ([details](../using/ports-size.md#ports)) |
| `devvm ports add\|rm LIST` | forward more ports, or stop forwarding some ([details](../using/ports-size.md#ports)) |
| `devvm ports -g ...` | show or change what new VMs forward ([details](../using/ports-size.md#ports)) |
| `devvm network [on\|off]` | show this folder's VM's network policy, `on`, `off` or `unenforced`, and then its allowlist, file by file, or change its network policy from its next start with `devvm restart` ([details](../safety/network.md#turning-it-on-or-off)) |
| `devvm network -g [on\|off\|ask]` | show or change whether new VMs get network policy, which is `ask` until you change it ([details](../safety/network.md#turning-it-on-or-off)) |
| `devvm network allow\|deny\|rm RULE... [-g]` | add allow or deny rules to this folder's VM's own, or remove them, at once, and with `-g`, change yours for every VM ([details](../safety/network.md#changing-rules)) |
| `devvm network log [--follow] [-n N] [--blocked]` | show what this folder's VM connected to and what its network policy blocked ([details](../safety/network.md#logs)) |
| `devvm ssh-agent [on\|devvm\|off]` | show this folder's VM's SSH agent forwarding, which is `off`, `on` for yours or `devvm` for devvm's own, then the keys devvm's own agent holds and those added to it, or change the forwarding from the VM's next start ([details](../using/ssh.md#agent-forwarding)) |
| `devvm ssh-agent add KEY_FILE [--confirm] [--lifetime DURATION]` | load a key into devvm's own agent, and load it again whenever a VM needs it ([details](../using/ssh.md#devvms-own-agent)) |
| `devvm ssh-agent rm KEY_FILE` | take a key out of devvm's own agent, so devvm doesn't load it again ([details](../using/ssh.md#devvms-own-agent)) |
| `devvm git-protect [on\|strict\|off]` | show or change this folder's VM's `.git` protection, while it's stopped ([details](../safety/git-protection.md#turning-it-on-or-off)) |
| `devvm secret`, `devvm secret rm NAME` | list the secrets, whether each is saved and the VMs using it, or delete a saved one ([details](../safety/network.md#saving-and-binding-secrets)) |
| `devvm secret set claude\|github [--stdin]` | save your Claude Code token or GitHub token on this machine, so that with network policy VMs get a placeholder and their proxy puts the secret in ([details](../safety/network.md#saving-and-binding-secrets)) |
| `devvm secret use\|unuse NAME` | let this folder's VM use a secret, or stop it, though a VM with Claude Code always uses `claude` ([details](../safety/network.md#saving-and-binding-secrets)) |

### Adding to a VM

`devvm tools` in a terminal shows the same list as a new VM, with the VM's toolchains and agents checked. Its layers are in the list too, greyed out, and the cursor skips them, because a VM keeps the layers it was made with. Check what you want to add, uncheck what you want to take out, and press enter. devvm then makes those changes as `devvm tools add` and `devvm tools rm` would, and opens a shell in the VM. When you change nothing, devvm says so and opens nothing. Without a terminal, or with `--json`, `devvm tools` shows the VM's shared tools instead.

`devvm tools add TOOL...` adds shared tools to the folder's VM without the list, each one a separate word, as in `devvm tools add python claude`, and then opens a shell in it. The tools are mounted rather than installed, so nothing in the VM's disk changes.

- devvm builds the shared tools that the share lacks, and they join the VM's list at once.
- devvm mounts a new agent's config folders, which needs the VM to start again. A stopped VM gets them as it starts, and for a running VM devvm asks before restarting it, because anything running in it stops. If you answer no, or there's no terminal, the VM gets them at its next start.
- devvm says which of the tools the VM has already and leaves those as they are.

`devvm tools rm TOOL...` takes shared tools out again. A layer can't be added to a VM that exists or taken out of one, because its script changes the VM's disk, so only a new VM gets a layer. To add or drop one, save your work outside the project folder, run `devvm rm`, and make the VM again with the layers you want.

## Links between VMs

| | |
| --- | --- |
| `devvm link` | list this folder's VM's links, each with its name, port, target VM and folder, the target's status, and whether the link applies ([details](../using/links.md#listing-links)) |
| `devvm link add TARGET PORT [--as NAME]` | link this folder's VM to PORT of the VM that TARGET names, so programs in it reach the port as `NAME:PORT` and `NAME.devvm.internal:PORT` ([details](../using/links.md#adding-a-link)) |
| `devvm link rm TARGET PORT` | remove one of this folder's VM's links ([details](../using/links.md#removing-a-link)) |
| `devvm link publish [PORTS]` | show the ports this folder's VM publishes, or offer PORTS to links from any of your other VMs ([details](../using/links.md#publishing-a-port)) |
| `devvm link unpublish PORTS` | stop offering ports to links ([details](../using/links.md#publishing-a-port)) |
| `devvm group` | show the group this folder's VM is in, with each of its VMs, the name the others reach it by, its folder and the ports it publishes ([details](../using/links.md#groups)) |
| `devvm group join NAME [--as NAME]` | put this folder's VM in the group NAME, which its first VM makes, so each VM in the group reaches the ports the others publish ([details](../using/links.md#joining-a-group)) |
| `devvm group leave` | take this folder's VM out of its group, removing the links the group made ([details](../using/links.md#leaving-a-group)) |

[Links between VMs](../using/links.md) explains what a link reaches, its names, groups, and why a link joins the two VMs' trust.

## Images

| | |
| --- | --- |
| `devvm images` | list the images, then the shared tool images ([details](../using/images.md#listing-images)) |
| `devvm images rm IMAGE\|KEY...` | delete images, by name or by the key `devvm images` shows ([details](../using/images.md#deleting-images)) |
| `devvm build IMAGE` | build an image and the images under it, unless they're built, such as `devvm build docker` or `devvm build foo+bar`, where IMAGE is base or layers and never a shared tool ([details](../using/images.md#layers)) |
| `devvm rebuild IMAGE` | build an image of base or layers again from its parent as it is, and delete the stale images no VM uses, while shared tools follow the tools lock instead ([details](../using/images.md#rebuilding)) |
| `devvm layer new NAME` | start a layer of your own, `~/.config/devvm/layers/NAME.sh`, with its allowlist, `NAME.policy` ([details](../using/customizing.md#your-own-layers)) |

`devvm build`, `devvm rebuild` and `devvm images rm` take `--from IMAGE` to work on a base made from that container image ([details](../using/from.md)).

## Disk space

| | |
| --- | --- |
| `devvm space` | show what devvm keeps on this machine, the disk each kind takes, and what frees it ([details](../using/space.md#devvm-space)) |
| `devvm gc` | delete the stale and unfinished images and the tool images no VM uses, after listing them, and never a VM ([details](../using/images.md#cleaning-up)) |
| `devvm retention [KEY=VALUE...]` | show the retention settings, which say what devvm deletes on its own, or change them, with `KEY=off` turning one off ([details](../using/settings.md#retention)) |

## This machine

| | |
| --- | --- |
| `devvm setup [--features LIST\|none] [--agents LIST\|none] [--layers LIST\|none]` | choose base's features and the coding agents and layers new VMs get, and in a terminal also new VMs' size and network policy ([details](../using/settings.md#devvm-setup)) |
| `devvm doctor` | check what devvm needs on this machine and say how to get what's missing ([details](install.md#checking-what-devvm-needs)) |
| `devvm completion SHELL` | print the shell completion script for `bash`, `zsh` or `fish` ([details](#shell-completion)) |
| `devvm limactl ARGS...` | run Lima's `limactl` on the VMs and images in devvm's own Lima home, not `~/.lima`, for debugging, which starts VMs without network policy ([details](../backends/lima.md#running-limactl)) |

### Shell completion

The completion script completes commands, flags and the image names that `devvm build` and `devvm rebuild` take, including layers joined with `+` after the ones you've already typed. It also completes shared tools for `devvm tools add` and `devvm tools rm`, the lists that `--tools`, `--agents` and `--layers` take, after the names you've already typed and their commas, and `all` and `none` for `--ports` and `devvm ports`. Load it from your shell's startup file:

| Shell | Line | In |
| --- | --- | --- |
| bash | `source <(devvm completion bash)` | `~/.bashrc`, with the bash-completion package installed |
| zsh | `source <(devvm completion zsh)` | `~/.zshrc`, after `compinit` |
| fish | `devvm completion fish \| source` | `~/.config/fish/config.fish` |

## Flags and values

Each flag belongs to the commands that list it in their help, and only `-h` or `--help`, `-v` or `--verbose` and `--no-input` work with every command. `-v` shows the output of the commands devvm runs as they run ([details](../using/troubleshooting.md#a-command-fails)), and `--no-input` makes devvm ask nothing ([questions devvm asks](#questions-devvm-asks)). A flag can come before or after the command's arguments.

When a flag or a setting can mean none or off, you write that as a word: `devvm setup --features none`, `devvm --agents none`, `devvm ports none`, `devvm retention history.age=off` and `devvm ssh-agent add KEY_FILE --lifetime forever`. devvm refuses an empty value, such as `--features ''`, and an empty argument, so a blank never stands for none or for a default.

Every duration is a whole number followed by a unit, which is `m` for minutes, `h` for hours, `d` for days or `w` for weeks, as in `30m`, `12h`, `30d` or `2w`. That covers `devvm gc --older-than`, the retention settings' ages and `devvm ssh-agent add --lifetime`, and only `--lifetime` also takes `forever`, for a key that devvm's agent keeps until it ends.

## Questions devvm asks

devvm asks a question only in a terminal, and each question has a flag or a command that answers it beforehand, so a script never needs to answer one. Without a terminal, devvm asks nothing. A question that only adds something, such as an agent's folders or the `Include` line, gets its usual answer or a warning that says what to do, and a command that deletes something refuses unless you give it `--force`.

| Question | Answered by |
| --- | --- |
| a new VM's toolchains, coding agents and layers | `--tools`, `--agents` and `--layers`, or nothing: without a terminal or with `--no-input`, the VM gets their defaults |
| whether a new VM gets network policy | `--network-policy` or `--no-network-policy`, or `devvm network -g on` or `off` for every new VM |
| whether to use the VM of a folder this one is inside | `--here` for a new VM, or `devvm` run in that folder |
| the CPUs and memory new VMs get, on the first run | `--cpus` and `--memory`, or `devvm size -g` |
| the coding agents new VMs get, on the first run and in `devvm setup` | `devvm setup --agents LIST` or `devvm setup --agents none` |
| the layers new VMs get, in `devvm setup` | `devvm setup --layers LIST` or `devvm setup --layers none` |
| a VM's toolchains and agents, in `devvm tools` | `devvm tools add` and `devvm tools rm`, or `--json` to show them |
| base's features, on the first build | `devvm setup --features LIST` or `devvm setup --features none` |
| new VMs' network policy, in `devvm setup` | `devvm network -g` |
| whether to load an SSH key into your agent | `devvm ssh-agent add KEY_FILE` beforehand |
| whether to restart a running VM to mount an agent's folders | `devvm restart` afterwards |
| whether to add the `Include` line to `~/.ssh/config` | `--ssh-config` on `devvm`, `devvm host` and `devvm code` |
| a Claude Code token or a GitHub token | `devvm secret set NAME --stdin` |
| whether to delete, in `devvm rm`, `devvm gc`, `devvm gc --caches`, `devvm images rm` and `devvm secret rm` | `--force` |
| whether to replace the images built on one, in `devvm rebuild` | `--force` |
| whether to use the VM made for an earlier folder here, in `devvm adopt` | `--force` |

`--force` only skips a confirmation, and it never deletes more than the command does without it. A command that would delete something asks first in a terminal, and without a terminal it refuses unless you give `--force`.

A few questions protect work that only you can judge, so no flag answers them, and devvm asks them only in a terminal. They are `devvm rm` deleting the VM of a folder this one is inside or the VM made for an earlier folder at this path, a `--clone` VM's commits that this repo hasn't fetched and its uncommitted changes, and a command giving the VM of a folder this one is inside more access. Without a terminal, each of them refuses and says what to run instead.

With `--no-input`, devvm asks nothing even in a terminal. A question it would ask fails instead, after devvm shows the question, and the error names the flag or command that answers it. A question that only a terminal answers fails and tells you to run the command in a terminal without `--no-input`. A question also fails this way when devvm reads its answer from a pipe or a file and the input ends before an answer, or when you press Ctrl+D at the prompt.

## devvm itself

| | |
| --- | --- |
| `devvm help [COMMAND]` | print the help for devvm or a command, as `--help` and `-h` do |
| `devvm --version` | print devvm's version alone, as `devvm version` also does |

A command's help starts with a few sentences on what the command does, then its usage line and its examples, the most common first, each with a comment. Then come its commands, if it has any, and its flags, and the help ends with the address of the docs page that covers the command. `devvm --help` lists the commands in the groups this page has, which are Your VM, Change your VM, Images, Disk space and This machine, with `help` under Additional Commands, and it ends with devvm's issue tracker after its docs page.

devvm itself takes no word after it but a command's name, so every word there is a command, and a layer may have any name a command has. When the word after `devvm` isn't a command, devvm refuses it as a usage error. If the word names a toolchain, an agent or a layer, your own layers included, devvm points you to the flag that chooses it for a new VM, such as `devvm --tools node` for `devvm node`, and a word that joins several with `+` or a comma gets one line with each flag, such as `devvm --tools node --agents claude` for `devvm node+claude`. Any other word gets the closest command, such as `devvm stop` for `devvm stpo`. `devvm stpo --help` and `devvm help stpo` are refused the same way.

The folders devvm keeps its state in and the environment variables it reads are in [Settings and files](../using/settings.md).

## Output for scripts

Every command that shows or lists something takes `--json`, which prints what the command shows as one JSON object instead of a table or lines, so a script can read it. The object's first field is `"version": 1`, and the page for each command lists its other fields with an example. A shape only ever gains fields. devvm adds new fields to version 1, and any other change to a shape, such as a field renamed, removed or given another type, comes with a new version.

With `--json`, standard output gets only the JSON. devvm's messages, such as warnings and notes, still go to standard error, as they do without it, and the JSON is never colored. A list with nothing in it is `[]`, never `null`. Sizes are in bytes, unless a field's name gives the unit, such as `memory_gib`. A field that doesn't apply, such as a VM's `from` when its base is Ubuntu's cloud image, is left out.

A command that shows a setting when you run it bare and changes the setting when you give it a value, such as `devvm network` or `devvm size`, only shows with `--json`. devvm refuses a value given with `--json`, such as `devvm network --json off`, and changes nothing.

| Command | Its JSON |
| --- | --- |
| `devvm ls --json` | [the project VMs](#devvm-ls---json) |
| `devvm images --json` | [the images and the shared tool images](../using/images.md#devvm-images---json) |
| `devvm tools --json` | [this folder's VM's shared tools](../using/toolchains.md#devvm-tools---json) |
| `devvm size --json`, `devvm ports --json` | [this folder's VM's size and ports](../using/ports-size.md#as-json), or with `-g`, new VMs' |
| `devvm network --json` | [this folder's VM's network policy and allowlist](../safety/network.md#devvm-network---json), or with `-g`, new VMs' choice |
| `devvm network log --json` | [this folder's VM's network log, or with `--blocked` its blocks](../safety/network.md#devvm-network-log---json) |
| `devvm secret --json` | [the secrets, whether each is saved and the VMs using it](../safety/network.md#devvm-secret---json) |
| `devvm ssh-agent --json` | [this folder's VM's SSH agent forwarding and devvm's own agent's keys](../using/ssh.md#devvm-ssh-agent---json) |
| `devvm link --json` | [this folder's VM's links](../using/links.md#devvm-link---json) |
| `devvm link publish --json` | [the ports this folder's VM publishes to links](../using/links.md#devvm-link-publish---json) |
| `devvm git-protect --json` | [this folder's VM's `.git` protection](../safety/git-protection.md#devvm-git-protect---json) |
| `devvm history --json` | [the saved exports of agents' history](../using/coding-agents.md#devvm-history---json) |
| `devvm space --json` | [what devvm keeps on this machine](../using/space.md#devvm-space---json) |
| `devvm retention --json` | [the retention settings](../using/settings.md#devvm-retention---json) |
| `devvm doctor --json` | [what devvm needs on this machine and what's missing](../internals/install.md#devvm-doctor---json), and it prints nothing else |
