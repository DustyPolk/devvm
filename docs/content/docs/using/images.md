---
title: Images and tools
weight: 1
---

Every VM is a copy of an image. An image is `base` with your choice of layers on it, and a VM also gets the shared tools its image names, such as node or Claude Code. This page covers what base and the layers bring, how the shared tools work, which image a new VM gets, and how to rebuild, list and delete images. [Images and caching](../internals/images.md) explains how devvm builds, pins and caches them, and [toolchains](toolchains.md) and [coding agents](coding-agents.md#supported-agents) list the shared tools.

```mermaid
flowchart TD
  ubuntu["Ubuntu's cloud image,<br>or a --from image"] --> base["base, with<br>the features you chose"]
  base --> docker["docker, a layer<br>on base"]
  base -. "a copy" .-> vm1["VM: node"]
  docker -. "a copy" .-> vm2["VM: node+docker"]
  tools[("shared tools:<br>node, claude…")] -- "mounted<br>read-only" --> vm1 & vm2
```

## Base

`base` is the image that every other image is built on. It is Ubuntu 26.04's cloud image with git, build-essential, ripgrep, jq and mise added, followed by the optional [features](customizing.md#base-image-features) you chose, such as oh-my-bash and fzf. Docker comes as a [layer of devvm's](#built-in-layers), so you ask for it with `devvm node+docker`. With `--from`, base starts from a container image instead of Ubuntu's ([details](from.md)).

A VM made from base gets 4 CPUs, 8 GiB of memory and a 100 GiB disk unless you give it another [size](ports-size.md#size). On a small machine it gets fewer CPUs and less memory, which [the size section](ports-size.md#size-and-ports) explains.

base drops the server extras that Ubuntu's cloud image comes with, which are snapd, multipath, ModemManager, udisks, firmware updates, crash reporting, tracing tools, kernel headers, and apt's background updates and upgrades. That saves 0.9 GB of disk and 100 MB of memory per VM. The everyday tools stay, such as htop, tmux, lsof, strace, tcpdump, rsync and nano.

devvm builds base from pinned packages, but your VMs use Ubuntu's live archive, so `apt upgrade` in a VM gets the current updates.

## Layers

An image is base with layers on it, named bottom first and joined with `+`, so `foo+bar` is `bar` on `foo` on `base`. You can leave `base` itself out, which makes `base+foo` the same as `foo`. An image name can also hold [shared tools](#shared-tools), which aren't layers.

A layer is a script that bash runs in the image's VM as the VM's user, who has sudo. It is either one of devvm's [built-in layers](#built-in-layers) or one of yours, at `~/.config/devvm/layers/<name>.sh`. A layer's name is made of lowercase letters, digits and dashes, starts with a letter or digit, and is neither `base` nor a shared tool's name. devvm refuses any other name in an image, and it refuses a layer whose script is missing. `devvm layer new NAME` starts a layer of your own ([details](customizing.md#your-own-layers)).

```sh
devvm build foo+bar   # builds base, foo and foo+bar, unless they're built
devvm rebuild foo     # builds foo again, and deletes the old foo and foo+bar
```

### Built-in layers

devvm ships two layers, `docker` and `nix`. You choose them like any other layer, as in `devvm node+docker` or `devvm build nix`, and you can also add them to an existing VM in place ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)). Their names are reserved, so devvm refuses `devvm layer new docker` and never reads a `docker.sh` or `nix.sh` of yours. The hosts that each layer needs are in its own [allowlist](../safety/network.md#allowlists).

#### docker

The `docker` layer installs Docker's engine, CLI, BuildKit and Compose from Docker's apt repository, adds the VM's user to the `docker` group, and starts the daemon at each boot. On a base made `--from alpine`, it installs Alpine's own packages instead. A VM without the layer has no Docker.

On the [Docker backend](../backends/docker.md#the-docker-layer), the layer needs Sysbox on a Linux engine, where devvm makes a VM for it, or makes the VM again when you add the layer in place. Anywhere else on that backend, devvm refuses the layer before it changes anything, and you need a VM backend for it.

#### nix

The `nix` layer installs [Nix](https://nixos.org) with flakes and the `nix` command turned on, and [direnv](https://direnv.net) with [nix-direnv](https://github.com/nix-community/nix-direnv), so a project's `flake.nix`, `shell.nix` or `devenv.nix` can supply its toolchain. Each VM has a Nix store and daemon of its own, which no other VM and nothing on this machine shares. [Nix projects](toolchains.md#nix-projects) explains how a flake's dev shell and the shared tools work together.

The `nix` layer works on Lima and smolvm, on a base from Ubuntu, Debian or Alpine. On a base made `--from alpine`, it installs Alpine's `xz` package, which isn't pinned, and runs Nix's daemon as an OpenRC service. devvm refuses the layer on the Docker backend, whose containers can't make the sandbox that Nix builds packages in ([what Docker VMs can't do](../backends/docker.md#what-docker-vms-cant-do)).

## Shared tools

The toolchains and coding agents are shared tools, not image layers. Any image can have them, in any mix, such as `node+python+claude` or `go+foo+codex`. [Toolchains](toolchains.md) lists node, python, rust and go, with what each brings, and [supported agents](coding-agents.md#supported-agents) lists claude, codex and opencode.

devvm builds each tool version once and mounts it read-only into every VM that names it, so any mix of tools works without building anything new, and ten VMs with node share one copy of it. The builds are in `~/.local/share/devvm/tools`. Each backend and each C library gets a build of its own, so when a Lima VM and a smolvm VM both have node 24, or an Alpine VM has it too, `devvm images` lists more than one build of that version ([what devvm keeps](space.md#shared-tools)). [mise](https://mise.jdx.dev) runs the tools:

- **They work as usual.** `node`, `npm i -g` (into `~/.npm-global`), `pnpm`, `yarn`, `pip` and `uv`, `go install` (into `~/go/bin`) and `cargo` work from any shell, including `bash -c` from agents and from editors over SSH.
- **A project's own version files win.** `.nvmrc`, `.node-version`, `.python-version`, `rust-toolchain.toml` and the `toolchain` line in `go.mod` pick their version, which installs into the VM the first time it's used. mise also reads a `mise.toml` in the project folder, and devvm trusts it without asking, because the folder is yours.
- **They're read-only.** A VM can't change the shared tools, even as root. The tools can't update themselves, and `rustup component add` can't change the shared toolchain.
- **They're mounted, or the launch stops.** When one of a VM's tools is missing or the VM can't mount it, devvm stops the launch and names the tool, and the VM never downloads its own copy in its place ([a shared tool doesn't mount](troubleshooting.md#a-shared-tool-doesnt-mount)). Only the versions that a project's own version files pick install into the VM.

devvm doesn't pin the tools' versions, because agents like Claude Code release several times a week. It follows the newest release of each tool, and Node 24 for node. The first time a tool is needed, devvm looks up its newest release, which needs this machine to be online once. When it makes a new VM, devvm also checks for newer releases if its last check was more than a day ago, and `DEVVM_NO_TOOL_CHECK=1` turns that check off.

### Adding and removing tools

A new VM gets the tools its image names and keeps those versions. On each launch, devvm warns when there are newer ones. The commands below don't change the VM's disk, because its tools are mounted, so a change takes effect on that launch.

- `devvm update` checks upstream for newer versions, builds what's missing, and moves the VM to them. When the VM already has the newest versions, it builds any of them that are missing.
- `devvm IMAGE` with tools the VM lacks adds them, after asking ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)).
- `devvm remove TOOLS` takes tools out, as in `devvm remove python+claude`. What the VM installed itself, such as other versions or global packages, stays.

## Which image a new VM gets

```mermaid
flowchart TD
  run["devvm, or devvm IMAGE"] --> named{{"names a toolchain,<br>a layer or base?"}}
  named -- "yes" --> order
  named -- "no: nothing,<br>or only agents" --> detect{{"toolchain or Nix files<br>in the folder?"}}
  detect -- "yes" --> found["those toolchains and nix,<br>agents on top"] --> order
  detect -- "no" --> other{{"agents named, or<br>a devcontainer.json?"}}
  other -- "yes" --> order
  other -- "no" --> list["devvm lists the choices"]
  order["adds a devcontainer.json's features,<br>then orders the name:<br>toolchains, layers, agents"]
  order --> copy["built if it isn't,<br>and the VM is a copy of it"]
```

`devvm IMAGE` names the image. `devvm` alone looks at the folder's top level for the files that call for a toolchain, and names those toolchains, in this order:

| Toolchain | Files |
| --- | --- |
| node | `package.json`, `bun.lock`, `bun.lockb` |
| python | `pyproject.toml`, `requirements.txt`, `uv.lock`, `setup.py`, `Pipfile` |
| rust | `Cargo.toml` |
| go | `go.mod` |

If more than one toolchain matches, they all go in, in that order, so `package.json` and `pyproject.toml` give `node+python`. A Nix project's files also call for the [`nix` layer](#nix), which goes after the toolchains, so `package.json` and `flake.nix` give `node+nix`. Those files are `flake.nix`, `shell.nix` and `devenv.nix`, and a `default.nix` that makes a shell with `mkShell`. In a folder with none of these files, devvm doesn't guess, and lists what there is to choose from instead.

Agents go on top of the folder's toolchains. `devvm claude` in a folder with `package.json` makes a `node+claude` VM, and it makes a `claude` VM (base and Claude) when nothing is detected. Naming a toolchain or a layer skips detection, as in `devvm python+claude`, and so does `devvm base`.

A folder's `devcontainer.json` adds what its features map to, such as `docker` or toolchains, under what's named or detected. With a `devcontainer.json`, a folder where nothing is named or detected gets `base` ([`devcontainer.json`](devcontainer.md)).

devvm puts image names in one order: the toolchains (node, python, rust, go), then the layers as given, then the agents. So `claude+node` and `node+claude` are the same image.

If the image (base and its layers) isn't built yet, devvm builds it first, with the shared tools its name has, and the VM is a copy of the image. A VM keeps the image and tools it was made from.

## Rebuilding

When you edit a layer's script, devvm builds that layer again the next time it's used, together with every layer built on it, and it reuses the layers below. A new choice of base's features rebuilds base and everything on it in the same way.

`devvm rebuild IMAGE` builds an image again from its parent as the parent is now, which is how you get the newest packages that a script installs without a pin. devvm also builds the images on it again, on the new image, the next time they're needed. When there are such images, devvm lists them first and, in a terminal, asks before it goes on. After the build, devvm deletes the old images that no VM uses.

Shared tools aren't images, so `devvm rebuild` refuses them, and `devvm update` gets their newer versions instead. [Rebuilding](../internals/images.md#rebuilding) explains how devvm tells which images to build again.

## Outdated VMs

A VM keeps the image it was made from. When that image, or one it's built on, changes after an edited script or a rebuild, devvm warns on each launch that the VM is outdated, and it leaves the VM as it is. To get the new image, save any work the VM holds outside the project folder, then run `devvm rm` and `devvm` to make a new VM.

## Listing images

`devvm images` prints two tables, each with a header, and it shows what a row lacks as `-`. The first table lists the images on every backend by name, with unfinished builds last:

| Column | |
| --- | --- |
| `IMAGE` | its name, such as `base` or `foo+bar` |
| `FROM` | what its base started from: the `--from` image, or Ubuntu's cloud image with its release's date, `ubuntu 26.04 (20261001)` |
| `DIGEST` | the first 12 hex digits of that image's SHA-256 |
| `KEY` | its key, which `devvm images rm` also takes |
| `STATE` | `current`, `stale` when what it's built from has changed since it was built, or `unfinished` for a build that didn't finish |
| `SIZE` | the disk it takes, in powers of 1024 (`2.1G`). On macOS an image is a clone of its parent's disk, and the blocks they share count for both |
| `BACKEND` | the [backend](../backends/_index.md) that has it |

The second table lists the [shared tools](#shared-tools) that devvm has built, by the name that image names use for them, such as `node` for node, pnpm and bun. The toolchains come first, then the agents, with each tool's versions newest first:

| Column | |
| --- | --- |
| `NAME` | `node`, `python`, `rust`, `go`, `claude`, `codex` or `opencode` |
| `TOOL`, `VERSION` | the tool and its version |
| `BACKEND`, `LIBC` | the backend and the C library (`gnu`, or `musl` for an Alpine base) that this build is for, because each gets a build of its own, or `-` when devvm can't tell |
| `SIZE` | the disk it takes |
| `VMS` | how many VMs have it |
| `STATE` | `current` for the version new VMs get, `in use` for another version a VM has, `unused` for neither, `not built` for the current version of a tool devvm has other versions of, and `not installed` when devvm has no version of it |

After the tables, devvm says on stderr how many images are stale or unfinished and how much disk the unused tools take, when there are any. It ends with the disk that the images and shared tools take together, and `devvm space` shows the rest of what devvm keeps ([what devvm keeps](space.md#devvm-space)).

### Deleting images

`devvm images rm IMAGE|KEY...` deletes images. You name each one by its name or by the key that `devvm images` shows:

- A name means its image as it is now, on base from `--from` when you give it. A stale or unfinished image has only its key.
- Each image must be built, or devvm deletes nothing. Shared tools aren't images, so devvm refuses a name that has any, such as `node+foo`.
- A deleted image is built again the next time it's needed. VMs made from it keep working, because on macOS they're clones of its disk. On Linux hosts, devvm refuses to delete an image whose disk VMs are overlays of, and names those VMs.

### Cleaning up

`devvm gc` deletes four things. The first is what `devvm images` lists as stale or unfinished, on every backend. The second is the shared tools it lists as `unused`. The third is the copies of devvm's own files that older versions [wrote out](../internals/install.md#devvms-own-files), except those a VM's proxy was started with. The fourth is the older versions of the mise that devvm runs on this machine to look up the tools' versions, which an upgrade of devvm leaves behind, except one that changed in the last hour, because another devvm may be downloading it. gc lists them all first and, in a terminal, asks. Without a terminal, it goes ahead.

- `devvm gc --tools` also deletes the current shared tools that no VM has, such as a toolchain you tried once, on every backend and C library. `devvm gc --tools node claude` deletes only those tools' builds, by the names images use, so `node` also covers pnpm and bun. A tool that a VM's state lists stays, including a stopped VM's, and so does one built in the last hour, because the VM it was built for may not list it yet. A VM whose image names a deleted tool builds it again when it needs it.
- `devvm gc --exports` also deletes the [exports](coding-agents.md#history) of agents' history that `devvm export` and `devvm rm` saved, which are the only copy of a deleted VM's history, and `--older-than AGE` keeps those newer than `AGE`, such as `30d`, `2w` or `12h`. gc never deletes exports without `--exports`.
- `devvm gc --orphans` also deletes what devvm keeps for VMs that their backend no longer has, such as one you deleted with `limactl delete` instead of `devvm rm`, and with it the hold their state has on their shared tools ([leftovers](space.md#leftovers-of-a-vm-deleted-outside-devvm)).
- `devvm gc --caches` then frees devvm's share of the backends' own caches, such as Lima's downloads of Ubuntu's cloud image, asking for each backend apart, and only in a terminal ([the backends' caches](space.md#the-backends-caches)).
- `devvm gc --retention` then applies the [retention settings](settings.md#retention) at once, which devvm otherwise applies at most once a day. They're all off until you set them.
- `devvm gc --backend NAME` acts on that backend alone, for example once you stop using smolvm. It deletes all of that backend's images, the current ones too, and its shared tools that are `unused`, and it leaves every other backend's images and devvm's own files as they are. Without `--backend`, gc acts on every backend, whichever backend new VMs get by default. An image that a smolvm or Docker VM was made from stays, and devvm names the VMs that use it.
- When a backend that one of your VMs uses can't list its images, for example because Docker isn't running, gc warns and leaves that backend's images, shared tools, VM leftovers and caches alone. It also leaves the shared tools whose backend devvm can't tell, because they may be that backend's. gc goes on with the other backends and succeeds, so run `devvm doctor` to see what's wrong with that backend, then run gc again once it answers. `devvm images` fails instead, because it lists every backend's images or none.
- gc never deletes VMs. Without `--backend` it keeps current images, and without `--tools` the current shared tools. It always keeps every shared tool that any VM has, including a stopped VM's and that of a VM deleted outside devvm whose state is left, until `--orphans` deletes that state.
- On Linux hosts, a stale image whose disk VMs are overlays of stays, and devvm says which VMs use it.
- After an Alpine VM is gone, the shared tools built for Alpine are unused, unless you give gc that `--from`.

[Images and caching](../internals/images.md#listing-and-cleaning-up) has the details of what counts as current and unused. To see what gc would free before you run it, and what takes the rest of the space, run `devvm space` ([what devvm keeps](space.md#devvm-space)).
