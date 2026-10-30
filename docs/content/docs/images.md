---
title: Images and VMs
weight: 20
---

## Base

`base` is the image that every other image is built on. It is Ubuntu 26.04's cloud image with git, build-essential, ripgrep, jq and mise added. Docker comes as a [layer of devvm's](#built-in-layers), so you ask for it with `devvm node+docker`. With `--from`, base starts from a container image instead of Ubuntu's ([details](from.md)). `assets/base.yaml` holds base's VM settings (4 CPUs, 8 GiB of memory and a 100 GiB disk), and `assets/layers/base.sh` installs it, followed by the optional [features](customizing.md) you chose.

base drops the server extras that Ubuntu's cloud image comes with, which are snapd, multipath, ModemManager, udisks, firmware updates, crash reporting, tracing tools, kernel headers, and apt's background updates and upgrades. That saves 0.9 GB of disk and 100 MB of memory per VM. The everyday tools stay, such as htop, tmux, lsof, strace, tcpdump, rsync and nano.

Each download in `assets/layers/base.sh` is pinned to a version and a SHA-256 at the top of the script, and the build fails if a checksum doesn't match. While base builds, apt's packages come from a dated snapshot of Ubuntu's archive (`apt_snapshot`, from snapshot.ubuntu.com). Project VMs use the live archive, so `apt upgrade` in a VM gets the current updates.

### Ubuntu's cloud image

On Lima, base starts from a pinned, dated release of Ubuntu 26.04's cloud image (`release-YYYYMMDD` on cloud-images.ubuntu.com). devvm pins the image's SHA-256 for arm64 and amd64 with it, in `CloudRelease`, `CloudSHA256Arm64` and `CloudSHA256Amd64` in `internal/ubuntu/ubuntu.go`. The pin moves with base's package snapshot to the newest release dated before the snapshot, as smolvm's and Docker's container image does ([maintaining](maintaining.md#updating-pinned-versions)). A release with newer packages than the snapshot's would leave apt unable to install the packages that depend on their exact versions.

devvm checks the image before it uses it:

- The release's `SHA256SUMS` must list the pinned SHA-256 for the host's image, and the download must match it. devvm refuses any other checksum (`ubuntu_checksum_unpinned`).
- When `gpgv` is installed (`brew install gnupg`, or `apt install gpgv`), `SHA256SUMS` must be signed by Ubuntu's cloud image key, which is pinned in `assets/layers/ubuntu-cloud.gpg` (fingerprint `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`). No other key counts. When the signature is missing or doesn't match, devvm refuses the image.
- When `gpgv` isn't installed, devvm says so, and the pinned SHA-256 still decides.

Ubuntu removes its dated releases after a few months. Once the pinned release is gone, base can't be built on Lima (`no_ubuntu_image`) until you update devvm to a version with a newer pin.

The pinned release and its SHA-256 are part of base's key, so a new pin rebuilds base. base's record keeps the address and SHA-256 of the image it was built from.

## Layers

An image is base with layers on it, named bottom first and joined with `+`, so `foo+bar` is `bar` on `foo` on `base`. You can leave `base` itself out, which makes `base+foo` the same as `foo`. An image name can also hold [shared tools](#shared-tools), which aren't layers.

A layer is a script that bash runs in the image's VM as the VM's user, who has sudo. It is either one of devvm's [built-in layers](#built-in-layers) or one of yours, at `~/.config/devvm/layers/<name>.sh`. A layer's name is made of lowercase letters, digits and dashes, starts with a letter or digit, and is neither `base` nor a shared tool's name. devvm refuses any other name in an image, and it refuses a layer whose script is missing. `devvm layer new NAME` starts a layer of your own ([details](customizing.md#your-own-layers)).

```sh
devvm build foo+bar   # builds base, foo and foo+bar, unless they're built
devvm rebuild foo     # builds foo again, and deletes the old foo and foo+bar
```

### Built-in layers

devvm ships one layer, `docker`. You choose it like any other layer (`devvm node+docker`, `devvm build docker`), and you can add it to an existing VM in place ([adding to a VM](commands.md#adding-to-a-vm)). Its script is `assets/layers/docker.sh`, embedded in devvm, and its [allowlist](network.md#allowlists) is `assets/layers/docker.policy`. Its name is reserved, so devvm refuses `devvm layer new docker` and never reads a `docker.sh` of yours.

| Layer | Brings |
| --- | --- |
| docker | Docker's engine, CLI, BuildKit and Compose from Docker's apt repository, its signing key's checksum and each package's version pinned at the top of the script (Alpine's own packages on `--from alpine`, unpinned); the VM's user in the `docker` group; the daemon started at each boot |

A VM without the layer has no Docker. Running Docker's engine in a VM is a backend feature of its own, `docker-layer` ([features](backends.md#features)). On the [Docker backend](docker.md#the-docker-layer), the layer needs Sysbox on a Linux engine, where devvm makes a VM for it, or makes the VM again when you add the layer in place. Anywhere else, devvm refuses the layer before it changes anything, and you need a VM backend for it.

## Shared tools

The toolchains and coding agents are shared tools, not image layers. Any image can have them, in any mix (`node+python+claude`, `go+foo+codex`).

| Name | Brings |
| --- | --- |
| node | Node 24, npm, pnpm, yarn (corepack), bun |
| python | CPython, uv |
| rust | rustup, cargo, the stable toolchain with `rust-src` and `rust-analyzer` |
| go | Go |
| claude, codex, opencode | the coding agents |

devvm builds each tool version once into a compressed, read-only image (EROFS) in `~/.local/share/devvm/tools`, with its hash next to it in `<image>.sha256`. That folder is mounted read-only in every VM at `/mnt/devvm-tools`, and each VM mounts the images its tools need at `/opt/devvm/tools/<tool>/<version>`, at boot and on every launch. On [Docker](docker.md#shared-tools), devvm unpacks the images into a volume, mounts that volume instead, and links each tool's folder where its image would be mounted. Any mix of tools works without building anything new, and ten VMs with node share one copy of it. [mise](https://mise.jdx.dev) runs the tools:

- **They work as usual.** `node`, `npm i -g` (into `~/.npm-global`), `pnpm`, `yarn`, `pip` and `uv`, `go install` (into `~/go/bin`) and `cargo` work from any shell, including `bash -c` from agents and from editors over SSH.
- **A project's own version files win.** `.nvmrc`, `.node-version`, `.python-version`, `rust-toolchain.toml` and the `toolchain` line in `go.mod` pick their version, which installs into the VM the first time it's used.
- **They're read-only.** A VM can't change the shared images, even as root. The tools can't update themselves, and `rustup component add` can't change the shared toolchain.

### Versions

devvm doesn't pin the tools' versions, because agents like Claude Code release several times a week. Instead, `assets/shared-tools/mise.toml` says which releases to follow, which is the latest of each tool, and Node 24. The first time a tool is needed, devvm resolves it to upstream's newest release and writes that into the tools lock, `~/.local/share/devvm/mise.lock`, with each download's checksum for every platform that VMs run (Linux, with glibc and with musl). Builds install exactly the lock's versions and check them against those checksums. Resolving needs this machine to be online once, and until it has been, devvm says so.

The resolving runs on this machine with a mise of devvm's own. That mise is a pinned release, which devvm downloads once into `~/.local/share/devvm/mise/<version>` and checks against its SHA-256 before it runs. devvm never uses the mise you have installed, and nothing it does reaches your own mise config.

Sometimes a tool's newest version isn't published for every platform yet, for example when a release is still uploading, when a tag has no release, or for a few days after a Node release until its musl builds appear. devvm then takes the newest of the next five older versions that is published everywhere, and says so. The next check tries the newest version again.

A new VM gets the lock's versions, and devvm checks the lock again first when the last check was more than a day ago. When `assets/shared-tools/mise.toml` changes, as it can in a devvm upgrade, devvm resolves the tools again. A VM lists its tool images in its state, so a newer version never changes a VM that has an older one. Older versions that no VM has are unused, and `devvm gc` deletes them ([cleaning up](#cleaning-up)). `DEVVM_NO_TOOL_CHECK=1` turns off the daily check.

### Building tools

devvm builds the tool images that an image name needs together with the image (`devvm build node+claude`), unless the share already has them. They build in a throwaway VM made from base and confined like an image build. That VM never sees the share. It writes into a folder of its own, `~/.local/share/devvm/tools-build.<random>`, and devvm deletes it once it's done. devvm then checks that each image is a plain file and not a link, moves it into the share and records its hash. Go builds in a VM of its own, never together with other tools. Only one build at a time writes to the share, so another devvm waits for it and gives up after ten minutes.

Each tool image is named `<tool>-<version>-<key>.erofs`. Its key hashes the tool's entry in the tools lock (its version, its downloads and their checksums), `assets/shared-tools/build.sh` and the C library the image is built for. A new version, a new download or an edited build script builds the image again, and a base with a different C library, such as Alpine's musl, gets an image of its own. When the lock has no download of a tool for the VM's platform, devvm doesn't build it and says to try again later.

Shared tools aren't images, so `devvm rebuild` refuses them, because their versions come from the lock.

### Adding and removing tools

A new VM gets the tools its image names and keeps those versions. On each launch, devvm warns when the lock has newer ones. The commands below don't change the VM's disk, because its tools are mounted from the share, so a change takes effect on that launch.

- `devvm update` checks upstream for newer versions, builds what the share lacks, and moves the VM to them.
- `devvm IMAGE` with tools the VM lacks adds them, after asking ([adding to a VM](commands.md#adding-to-a-vm)).
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

## Building

devvm builds each image in a VM of its own, and that VM then is the image:

1. For base, devvm makes a new VM from [Ubuntu's cloud image](#ubuntus-cloud-image) with `base.yaml`'s settings, or, with `--from`, from a container image [converted into a disk](from.md#how-its-converted). A layer's VM is a copy of its parent image.
2. The VM starts confined. It reaches the internet but not the host's own services on loopback, and it forwards no ports. How that's enforced depends on the backend, so see [Lima](lima.md#confinement) and [Docker](docker.md#building-images), which can't build on Docker Desktop yet.
3. The script runs. For base, that's `assets/layers/base.sh` and then the [features](customizing.md) you chose, and for a layer, it's the layer's script.
4. The VM stops. devvm writes the image's record last, with its chain, what base started from, and its C library.

Nothing from the host is mounted into a build, because layer scripts run third-party installers, which mustn't see your files. base gets `base.yaml`'s memory, or half the host's memory when `base.yaml`'s would leave the host less than 2 GiB. The images built on base, and their VMs, keep that amount.

## Caching

devvm builds images in layers and caches them, as Docker does. `base` comes first, and each custom layer is built on its parent, so `foo+bar` is `bar` on `foo` on `base`. A layer starts from its parent and runs its script inside, and devvm stores it under a key, which is a hash of everything that goes into it. When an image with that key already exists, devvm uses it, and otherwise it builds one.

| Image | Its key hashes |
| --- | --- |
| `base` | the backend, `assets/base.yaml`, `assets/layers/base.sh`, the chosen features in order (each name and its script), its rebuild counter, and with `--from`, the image, its digest and the scripts that convert it; on Lima, Ubuntu's pinned cloud image for the host (its release and SHA-256); on smolvm and Docker, their scripts for containers and Ubuntu's container image, pinned by digest |
| a built-in layer | its parent's key, its name, its script (`assets/layers/<name>.sh`) and its rebuild counter |
| a custom layer | its parent's key, its name, its script (`~/.config/devvm/layers/<name>.sh`) and its rebuild counter |

This means:

- When you edit a script, its layer and every layer above it rebuild the next time they're used. The layers below are reused.
- Each backend builds its own images and never uses another backend's.
- With `--from`, base's key has the image's digest, which devvm records when it looks the image up ([digests](from.md#digests)). When the tag moves to a new image, base and everything on it rebuild.
- A layer whose script is gone hashes as missing, so its images no longer match.
- Nothing else counts. Features you haven't chosen change no key, and neither do network policy files (`*.policy`), which the proxy reads at run time and which never go into an image.

An image whose key no longer matches its inputs is stale. It stays until it's deleted (`devvm gc`, [cleaning up](#cleaning-up)), and the next use builds a new one.

### Rebuilding

`devvm rebuild <image>` builds an image again from its parent as the parent is now, which is how you get the newest packages that a script installs without a pin. It changes the image's rebuild counter, which the image's key hashes, so the image gets a new key, and so does every image built on it. The counter is `~/.local/share/devvm/images/rebuilds/<image>`, or `<image>@<from>` with `--from`, so each image that base starts from has a counter of its own.

When other images are built on the one you rebuild, devvm lists them first and, in a terminal, asks before it goes on. After the build, devvm deletes every stale image that no VM uses, which includes the old image and those built on it. They're built again, on the new image, the next time they're needed. On Linux hosts, an image whose disk VMs are overlays of stays, and devvm says which VMs use it.

### Outdated VMs

A VM keeps the image it was made from, and that image's chain. When a key in the chain changes, after an edited script or a rebuild, devvm warns on each launch that the VM is outdated and leaves the VM as it is. To get the new image, run `devvm rm` and make a new VM. Shared tool images have keys of their own, described in [Building tools](#building-tools).

A key is the first 12 hex digits of a SHA-256. Each input goes into the hash with its length, so different inputs never hash the same bytes. An image's record keeps its chain, which is the key of each image it's built on, from `base` up.

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
| `BACKEND` | the [backend](backends.md) that has it |

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

`devvm gc` deletes three things. The first is what `devvm images` lists as stale or unfinished, on every backend, with their records. The second is the shared tool images it lists as `unused`, with their hashes. The third is the copies of devvm's own files that older versions [wrote out](install.md#installing-devvm), except those a VM's proxy was started with. gc lists them all first and, in a terminal, asks. Without a terminal, it goes ahead.

- gc never deletes VMs. It also keeps current images, and every tool image that any VM's state lists, including a stopped VM's and that of a VM deleted outside devvm whose state is left.
- On Linux hosts, a stale image whose disk VMs are overlays of stays, and devvm says which VMs use it.
- gc holds the share's lock throughout, so no tool build or `devvm update` changes the share meanwhile.
- A tool image is unused only when compared with the tools lock, so before there is a lock, gc deletes none. `current` is for the C library of the base that new VMs get, so after an Alpine VM is gone, its musl tool images are unused unless you give gc that `--from`.

## Ports

A VM forwards every TCP port from 1024 up that it listens on to the same port on this machine's `localhost`, so a server on `:5173` in the VM is at `localhost:5173` here. To forward only some ports:

```sh
devvm ports                 # what this folder's VM forwards: all, none or a list
devvm ports 5173,8000-8010  # only these ports and ranges
devvm ports add 5432        # one more
devvm ports rm 5173         # one fewer, as listed; the last leaves none
devvm ports none            # nothing; devvm ports all forwards every port again
devvm ports -g none         # what new VMs forward (default: all)
```

- A new VM gets `--ports` (`devvm --ports 5173`). Without it, the VM gets what `devvm ports -g` set in `~/.config/devvm/ports`, a single line of `all`, `none` or a list, and without that file, it forwards every port.
- A stopped VM changes at once. A running VM keeps what it forwards until it next starts (`devvm restart`), and until then `devvm ports` shows what it forwards now and says what it will forward.
- `add` and `rm` take ports and ranges, not `all` or `none`. devvm refuses `rm` on a VM that forwards every port, because there's no list to remove from. `-g` also works with `add` and `rm`.
- Ports are forwarded with or without [network policy](network.md). Builds forward no ports.

devvm's forwarder forwards the ports by the same rules on every backend.

- **What it forwards.** When a VM forwards every port, the forwarder forwards each TCP port from 1024 up that a server in the VM listens on, on the VM's loopback or on all its addresses. When a VM lists its ports, it forwards those ports alone, however many there are, ports below 1024 included. Each port goes to the same port on this machine's `127.0.0.1` and `::1` and nowhere else, within a few seconds of the server starting, and the forward goes away when the server stops. At most 256 ports are forwarded at once.
- **What it never takes.** The forwarder never takes a port that a program on this machine has on any address. That program keeps the port and its `localhost` connections, and the forward waits until the VM's server stops and starts again. The forwarder also never takes a port that devvm records for a VM, even while that port is free. Those are the ssh, proxy and decider ports devvm chooses for VMs, the VM's own ssh port, and other VMs' listed ports. A Lima VM without network policy has an ssh port that Lima picks and devvm doesn't record, but Lima holds that port, so the forwarder can't take it.
- **A listed port that a program on this machine has** isn't forwarded from a start that finds it in use, and devvm warns about it (`ports_taken`). The VM starts all the same, and the next start that finds the port free forwards it again.
- **A program that starts on a port after it's forwarded** can still bind it on all addresses, as most servers do, and then that program's `localhost` connections reach the VM.
- **How it works.** The forwarder reaches the VM over the VM's ssh, with a key and a host key that devvm pins, and it asks for no forwarding into this machine. Every connection starts on this machine, and the VM opens none to it. The forwarder runs as a hidden `devvm _lima-ports`, `_smolvm-ports` or `_docker-ports` process, one per VM, which devvm starts after each start of the VM and stops with `devvm stop`. Its log is `<vm>.ports.log` beside the backend's records, which on Lima are in `~/.local/share/devvm/lima-vms`. When a forwarder doesn't start, devvm warns with its log (`ports_forward_failed`), and the VM runs on.

**UDP isn't forwarded, on any backend.** Run the UDP client in the VM instead, or tunnel it over a forwarded TCP port with [socat](http://www.dest-unreach.org/socat/doc/socat.html), installed on both sides. For a UDP server on port 8125 in the VM, with TCP port 9125 forwarded:

```sh
socat TCP-LISTEN:9125,fork,reuseaddr UDP:127.0.0.1:8125   # in the VM
socat UDP-LISTEN:8125,fork TCP:127.0.0.1:9125             # on this machine
```

TCP is a stream, so datagrams can arrive merged or split. This suits low-rate request and response traffic, such as DNS, statsd or syslog, but not high-rate or latency-sensitive traffic, such as games, QUIC or WebRTC media.

## Size

A VM gets its image's size, which is `base.yaml`'s 4 CPUs, 8 GiB of memory and a 100 GiB disk, unless you give it another:

```sh
devvm size                         # what this folder's VM has: cpus=4 memory=8GiB disk=100GiB
devvm size --cpus 6 --memory 16    # change it; what isn't given stays
devvm size --disk 200              # a bigger disk; never a smaller one
devvm size -g --memory 16          # the size new VMs get (default: the image's)
devvm size -g                      # cpus=, memory= and disk= lines, empty for the image's
```

- A new VM gets `--cpus`, `--memory` and `--disk` (`devvm --cpus 6 base`). Without them, it gets what `devvm size -g` set in `~/.config/devvm/size`, and without that, the image's size ([details](commands.md#size-and-ports)).
- `--cpus` takes a whole number, and `--memory` and `--disk` take a size in GiB, written `16`, `16G` or `16GiB`, with decimals allowed. devvm refuses more CPUs or memory than this machine has.
- A stopped VM changes at once. A running VM keeps its size until it next starts (`devvm restart`), and until then `devvm size` shows what the VM has now and says what changes.
- Disks grow but never shrink, so devvm refuses a disk smaller than the VM's.
- On [Docker](docker.md#what-devvm-sets-on-a-vm), a VM's disk has no size of its own. devvm refuses `--disk`, a default disk size doesn't apply, and `devvm size` shows how much the VM holds instead (`cpus=4 memory=8GiB disk_used=1.2GiB`), with a warning past 100 GiB.

### Disk use

A VM's disk size is how far the disk can grow, not how much space it takes. A project VM starts as a copy of its image's disk that shares the image's blocks, so a new VM takes almost nothing until it writes ([clones and overlays](lima.md#clones-and-overlays)):

- **macOS:** the copy is an APFS clone. `du` counts the shared blocks as the clone's own, so it overcounts.
- **Linux:** the copy is a qcow2 overlay of the image's disk, which holds only the VM's changes, about 12 MB for a new VM instead of 2 GB.

## Where state lives

devvm writes nothing into the project folder. It keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/<key>`), rebuild counters (`images/rebuilds/`) and `--from` digests (`images/from-digests`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`), ports (`ports`) and [backend](backends.md#choosing-a-backend) (`backend`) of new VMs, your own layers and features, the keys in devvm's SSH agent (`agent-keys`), and the [ssh config](ssh.md) for VMs and their host keys (`ssh_config`, `known_hosts`) |

A VM's state file records the image the VM came from, its project folder, its backend and its settings, one `key=value` per line. The file lasts as long as the VM, because `devvm rm` also deletes it, so the next devvm in that folder starts fresh.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Each write replaces a file whole, so a file is never left half written, and only you can read the files. When a file can't be read, devvm names the file and the line, and you fix the line or delete the file.

## Build logs and interrupts

The output of the commands devvm runs goes to a log instead of the terminal. devvm prints its own steps, and it prints the end of the log when a step fails. `DEVVM_VERBOSE=1` prints the commands' output instead.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, which only you can read, and devvm keeps the newest 20. Each command appears as a `+` line with its arguments, followed by its output. A numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

Ctrl+C, or closing the terminal, stops the command that's running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.

devvm deletes an interrupted new VM, with its state, until its first shell opens, because nothing of yours is in it yet.

An interrupted build deletes the image it was building. So does a build whose create or clone fails, once devvm has shown the end of the log, because there's nothing in the image to look at.

A build that fails later, in its script or as the VM starts or stops, keeps the image's VM, stopped and without a record, so you can look inside. devvm's error names the VM and says how to open a shell in it with [`devvm limactl`](lima.md#running-limactl):

```sh
devvm limactl shell --start devvm-img-<key>
```

`--start` starts the VM first, outside the build's confinement. The next build of that image deletes the VM and starts over, and `devvm gc` also deletes it.

Either way, the images under it, which are already built, stay. devvm deletes and builds again any image it finds without its record, such as a failed build's or one devvm couldn't clean up after a crash or a power cut.
