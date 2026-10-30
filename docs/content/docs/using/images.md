---
title: Images and tools
weight: 1
---

## Base

`base` is the image that every other image is built on. It is Ubuntu 26.04's cloud image with git, build-essential, ripgrep, jq and mise added. Docker comes as a [layer of devvm's](#built-in-layers), so you ask for it with `devvm node+docker`. With `--from`, base starts from a container image instead of Ubuntu's ([details](from.md)). `assets/base.yaml` holds base's VM settings (4 CPUs, 8 GiB of memory and a 100 GiB disk), and `assets/layers/base.sh` installs it, followed by the optional [features](customizing.md) you chose.

base drops the server extras that Ubuntu's cloud image comes with, which are snapd, multipath, ModemManager, udisks, firmware updates, crash reporting, tracing tools, kernel headers, and apt's background updates and upgrades. That saves 0.9 GB of disk and 100 MB of memory per VM. The everyday tools stay, such as htop, tmux, lsof, strace, tcpdump, rsync and nano.

Each download in `assets/layers/base.sh` is pinned to a version and a SHA-256 at the top of the script, and the build fails if a checksum doesn't match. While base builds, apt's packages come from a dated snapshot of Ubuntu's archive (`apt_snapshot`, from snapshot.ubuntu.com). Project VMs use the live archive, so `apt upgrade` in a VM gets the current updates.

## Layers

An image is base with layers on it, named bottom first and joined with `+`, so `foo+bar` is `bar` on `foo` on `base`. You can leave `base` itself out, which makes `base+foo` the same as `foo`. An image name can also hold [shared tools](#shared-tools), which aren't layers.

A layer is a script that bash runs in the image's VM as the VM's user, who has sudo. It is either one of devvm's [built-in layers](#built-in-layers) or one of yours, at `~/.config/devvm/layers/<name>.sh`. A layer's name is made of lowercase letters, digits and dashes, starts with a letter or digit, and is neither `base` nor a shared tool's name. devvm refuses any other name in an image, and it refuses a layer whose script is missing. `devvm layer new NAME` starts a layer of your own ([details](customizing.md#your-own-layers)).

```sh
devvm build foo+bar   # builds base, foo and foo+bar, unless they're built
devvm rebuild foo     # builds foo again, and deletes the old foo and foo+bar
```

### Built-in layers

devvm ships one layer, `docker`. You choose it like any other layer (`devvm node+docker`, `devvm build docker`), and you can add it to an existing VM in place ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)). Its script is `assets/layers/docker.sh`, embedded in devvm, and its [allowlist](../safety/network.md#allowlists) is `assets/layers/docker.policy`. Its name is reserved, so devvm refuses `devvm layer new docker` and never reads a `docker.sh` of yours.

| Layer | Brings |
| --- | --- |
| docker | Docker's engine, CLI, BuildKit and Compose from Docker's apt repository, its signing key's checksum and each package's version pinned at the top of the script (Alpine's own packages on `--from alpine`, unpinned); the VM's user in the `docker` group; the daemon started at each boot |

A VM without the layer has no Docker. Running Docker's engine in a VM is a backend feature of its own, `docker-layer` ([features](../internals/backends.md#features)). On the [Docker backend](../backends/docker.md#the-docker-layer), the layer needs Sysbox on a Linux engine, where devvm makes a VM for it, or makes the VM again when you add the layer in place. Anywhere else, devvm refuses the layer before it changes anything, and you need a VM backend for it.

## Shared tools

The toolchains and coding agents are shared tools, not image layers. Any image can have them, in any mix (`node+python+claude`, `go+foo+codex`).

| Name | Brings |
| --- | --- |
| node | Node 24, npm, pnpm, yarn (corepack), bun |
| python | CPython, uv |
| rust | rustup, cargo, the stable toolchain with `rust-src` and `rust-analyzer` |
| go | Go |
| claude, codex, opencode | the coding agents |

devvm builds each tool version once into a compressed, read-only image (EROFS) in `~/.local/share/devvm/tools`, with its hash next to it in `<image>.sha256`. That folder is mounted read-only in every VM at `/mnt/devvm-tools`, and each VM mounts the images its tools need at `/opt/devvm/tools/<tool>/<version>`, at boot and on every launch. On [Docker](../internals/docker.md#shared-tools), devvm unpacks the images into a volume, mounts that volume instead, and links each tool's folder where its image would be mounted. Any mix of tools works without building anything new, and ten VMs with node share one copy of it. [mise](https://mise.jdx.dev) runs the tools:

- **They work as usual.** `node`, `npm i -g` (into `~/.npm-global`), `pnpm`, `yarn`, `pip` and `uv`, `go install` (into `~/go/bin`) and `cargo` work from any shell, including `bash -c` from agents and from editors over SSH.
- **A project's own version files win.** `.nvmrc`, `.node-version`, `.python-version`, `rust-toolchain.toml` and the `toolchain` line in `go.mod` pick their version, which installs into the VM the first time it's used.
- **They're read-only.** A VM can't change the shared images, even as root. The tools can't update themselves, and `rustup component add` can't change the shared toolchain.

### Adding and removing tools

A new VM gets the tools its image names and keeps those versions. On each launch, devvm warns when the lock has newer ones. The commands below don't change the VM's disk, because its tools are mounted from the share, so a change takes effect on that launch.

- `devvm update` checks upstream for newer versions, builds what the share lacks, and moves the VM to them.
- `devvm IMAGE` with tools the VM lacks adds them, after asking ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)).
- `devvm remove TOOLS` takes tools out (`devvm remove python+claude`). What the VM installed itself, such as other versions or global packages, stays.

## Which image a new VM gets

`devvm IMAGE` names the image. `devvm` alone looks at the folder's top level for the files that call for a toolchain, and names those toolchains, in this order:

| Toolchain | Files |
| --- | --- |
| node | `package.json`, `bun.lock`, `bun.lockb` |
| python | `pyproject.toml`, `requirements.txt`, `uv.lock`, `setup.py`, `Pipfile` |
| rust | `Cargo.toml` |
| go | `go.mod` |

If more than one toolchain matches, they all go in, in that order, so `package.json` and `pyproject.toml` give `node+python`. In a folder with none of these files, devvm doesn't guess, and lists what there is to choose from instead.

Agents go on top of the folder's toolchains. `devvm claude` in a folder with `package.json` makes a `node+claude` VM, and it makes a `claude` VM (base and Claude) when nothing is detected. Naming a toolchain or a layer skips detection (`devvm python+claude`), and so does `devvm base`.

A folder's `devcontainer.json` adds what its features map to, such as `docker` or toolchains, under what's named or detected. With a `devcontainer.json`, a folder where nothing is named or detected gets `base` ([`devcontainer.json`](devcontainer.md)).

devvm puts image names in one order: the toolchains (node, python, rust, go), then the layers as given, then the agents. So `claude+node` and `node+claude` are the same image.

If the image (base and its layers) isn't built yet, devvm builds it first, with the shared tools its name has, and the VM is a copy of the image. A VM keeps the image and tools it was made from.

## Listing images

`devvm images` prints two tables, each with a header, and shows what a row lacks as `-`. The first table lists the images on every backend by name, with unfinished builds last:

| Column | |
| --- | --- |
| `IMAGE` | its name: `base`, `foo+bar` |
| `FROM` | what its base started from: the `--from` image, or Ubuntu's cloud image with its release's date, `ubuntu 26.04 (20261001)` |
| `DIGEST` | the first 12 hex digits of that image's SHA-256 |
| `KEY` | its key |
| `STATE` | `current`; `stale`: its key no longer matches its inputs; `unfinished`: a build that didn't finish, with no record |
| `SIZE` | the disk it takes, in powers of 1024 (`2.1G`). On macOS an image is a clone of its parent's disk, and the blocks they share count for both |
| `BACKEND` | the [backend](../backends/_index.md) that has it |

The second table lists the [shared tool images](#shared-tools) in the share, by the name that image names use for them (`node` for node, pnpm and bun). The toolchains come first, then the agents, with each tool's versions newest first:

| Column | |
| --- | --- |
| `NAME` | `node`, `python`, `rust`, `go`, `claude`, `codex` or `opencode` |
| `TOOL`, `VERSION` | the tool and its version |
| `SIZE` | the disk it takes |
| `VMS` | how many VMs have it in their state |
| `STATE` | `current`: the tools lock's version, for the C library of the base new VMs get (`--from` changes it); `in use`: another version a VM has; `unused`: neither; `not built`: the lock's version of a tool the share has others of; `not installed`: nothing of the name's is in the share |

Listing never resolves the tools lock. Before there is a lock, no tool image is current, and the tool images that no VM has show no state. After the tables, devvm says on stderr how many images are stale or unfinished and how much disk the unused tool images take, when there are any.

### Deleting images

`devvm images rm IMAGE|KEY...` deletes images with their records. You name each one by its name or by the key that `devvm images` shows:

- A name means its image as it is now, on base from `--from` when you give it. A stale or unfinished image has only its key.
- Each image must be built, or devvm deletes nothing. Shared tools aren't images, so devvm refuses a name that has any (`node+foo`).
- A deleted image is built again the next time it's needed. VMs made from it keep working, because on macOS they're clones of its disk. On Linux hosts, devvm refuses to delete an image whose disk VMs are overlays of, and names those VMs.

### Cleaning up

`devvm gc` deletes three things. The first is what `devvm images` lists as stale or unfinished, on every backend, with their records. The second is the shared tool images it lists as `unused`, with their hashes. The third is the copies of devvm's own files that older versions [wrote out](../internals/install.md#devvms-own-files), except those a VM's proxy was started with. gc lists them all first and, in a terminal, asks. Without a terminal, it goes ahead.

- gc never deletes VMs. It also keeps current images, and every tool image that any VM's state lists, including a stopped VM's and that of a VM deleted outside devvm whose state is left.
- On Linux hosts, a stale image whose disk VMs are overlays of stays, and devvm says which VMs use it.
- gc holds the share's lock throughout, so no tool build or `devvm update` changes the share meanwhile.
- A tool image is unused only when compared with the tools lock, so before there is a lock, gc deletes none. `current` is for the C library of the base that new VMs get, so after an Alpine VM is gone, its musl tool images are unused unless you give gc that `--from`.
