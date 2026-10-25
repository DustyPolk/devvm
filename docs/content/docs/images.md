---
title: Images and VMs
weight: 20
---

## Base

`base` is the image every other one is built on: Ubuntu 26.04's cloud image with git, build-essential, ripgrep, jq, Docker and mise. `--from` starts it from a container image instead ([details](from.md)). `base.yaml` holds its VM settings (4 CPUs, 8 GiB of memory, a 100 GiB disk) and `layers/base.sh` installs it, then the optional [features](customizing.md) you chose.

It drops the server extras Ubuntu's cloud image comes with: snapd, multipath, ModemManager, udisks, firmware updates, crash reporting, tracing tools, kernel headers, and apt's background updates and upgrades. That saves 0.9 GB of disk and 100 MB of memory per VM. The everyday tools stay (htop, tmux, lsof, strace, tcpdump, rsync, nano and so on).

Each download in `layers/base.sh` is pinned to a version and SHA-256 at the top of the script, and the build fails if a checksum doesn't match. While base builds, apt's packages come from a dated snapshot of Ubuntu's archive (`apt_snapshot`, from snapshot.ubuntu.com), and Docker's packages are pinned by version. Project VMs use the live archive, so `apt upgrade` gets current updates.

### Ubuntu's cloud image

Ubuntu removes its dated cloud images after a few months, so base doesn't pin one. Building base takes the newest dated release of Ubuntu 26.04 (`release-YYYYMMDD` on cloud-images.ubuntu.com) that has an image for the host, arm64 or amd64. A newer release whose files aren't there yet is skipped.

The image is checked before it's used:

- Its SHA-256 comes from that release's `SHA256SUMS`, and the download must match it. A checksum that isn't 64 hex digits is ignored.
- When `gpgv` is installed (`brew install gnupg`, or `apt install gpgv`), `SHA256SUMS` must be signed by Ubuntu's cloud image key, pinned in `layers/ubuntu-cloud.gpg` (fingerprint `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`). No other key counts. When the signature is missing or doesn't match, devvm refuses the image.
- Without `gpgv`, devvm says so, and only the HTTPS download vouches for the checksums.

The Ubuntu image isn't part of base's key: a newer one doesn't make base stale. base's record keeps the one it was built from (its address and SHA-256), and `devvm rebuild base` takes the newest.

## Layers

An image is base and your layers on it, bottom first, joined with `+`: `foo+bar` is `bar` on `foo` on `base`. `base` itself can be left out, so `base+foo` is `foo`. An image name can also hold [shared tools](#shared-tools), which aren't layers.

A layer is a script of yours, `~/.config/devvm/layers/<name>.sh`, run with bash in the image's VM as its user, who has sudo. Its name is lowercase letters, digits and dashes, starting with a letter or digit, and is neither `base` nor a shared tool's. devvm refuses any other name in an image, and a layer without its script.

```sh
devvm build foo+bar   # builds base, foo and foo+bar, unless they're built
devvm rebuild foo     # builds foo again, and deletes the old foo and foo+bar
```

## Shared tools

The toolchains and coding agents aren't image layers: they're shared tools, which any image can have, in any mix (`node+python+claude`, `go+foo+codex`).

| Name | Brings |
| --- | --- |
| node | Node 24, npm, pnpm, yarn (corepack), bun |
| python | CPython, uv |
| rust | rustup, cargo, the stable toolchain with `rust-src` and `rust-analyzer` |
| go | Go |
| claude, codex, opencode | the coding agents |

Each tool version is built once into a compressed read-only image (EROFS) in `~/.local/share/devvm/tools`, with its hash next to it (`<image>.sha256`). Every VM has that folder mounted read-only at `/mnt/devvm-tools`, and mounts the images its tools need at `/opt/devvm/tools/<tool>/<version>`, at boot and on every launch. Any mix works without building anything new, and ten VMs with node share one copy. [mise](https://mise.jdx.dev) runs them:

- **They work as usual:** `node`, `npm i -g` (into `~/.npm-global`), `pnpm`, `yarn`, `pip` and `uv`, `go install` (into `~/go/bin`), `cargo`, from any shell, including `bash -c` from agents and editors over SSH.
- **A project's own version files win:** `.nvmrc`, `.node-version`, `.python-version`, `rust-toolchain.toml` and `go.mod`'s `toolchain` line pick their version, which installs into the VM the first time it's used.
- **Read-only:** a VM can't change the shared images, even as root. The tools can't update themselves, and `rustup component add` can't change the shared toolchain.

### Versions

devvm doesn't pin the tools' versions: agents like Claude Code release several times a week. `tools/mise.toml` says which releases to follow: the latest of each, and Node 24. The first time a tool is needed, devvm resolves it to upstream's newest releases, with each download's checksum for every platform VMs run (Linux, glibc and musl), into the tools lock, `~/.local/share/devvm/mise.lock`. Builds install exactly the lock's versions, checked against those checksums. Resolving needs to be online once; until then, devvm says so.

The resolving runs on this machine, with a mise of devvm's own: a pinned release, downloaded once into `~/.local/share/devvm/mise/<version>` and checked against its SHA-256 before it runs. It never uses the mise you have installed, and nothing it does reaches your own mise config.

A tool whose newest version isn't published for every platform yet (a release still uploading, a tag with no release, Node's musl builds for a few days) gets the newest of its next five older versions that is, and devvm says so. The next check tries the newest again.

A new VM gets the lock's versions, checked again first when the last check was more than a day ago. A changed `tools/mise.toml` (a devvm upgrade) is resolved again. A VM lists its tool images in its state, so a newer version never changes a VM that has one. `DEVVM_NO_TOOL_CHECK=1` turns off the daily check.

### Building tools

The tool images an image name needs are built with it (`devvm build node+claude`), unless the share has them. They build in a throwaway VM made from base, confined like an image build, which never sees the share: it writes into a folder of its own, `~/.local/share/devvm/tools-build.<random>`, and is deleted once it's done. devvm then checks each image is a plain file (not a link), moves it into the share and records its hash. Go builds in a VM of its own, never with other tools. One build at a time writes to the share: another devvm waits for it, and gives up after ten minutes.

Each tool image is named `<tool>-<version>-<key>.erofs`, and its key hashes the tool's entry in the tools lock (its version, downloads and their checksums), `tools/build.sh` and the C library it's built for. A new version, a new download or an edited build script builds it again, and a base with a different C library (Alpine's musl) gets its own. A tool the lock has no download of for the VM's platform isn't built: devvm says to try again later.

Shared tools aren't images, so `devvm rebuild` refuses them: their versions come from the lock.

### Adding and removing tools

A new VM gets the tools its image names, and keeps those versions. On each launch devvm warns when the lock has newer ones. None of these change the VM's disk: its tools are mounted from the share, so they take effect on the launch.

- `devvm update` checks upstream for newer versions, builds what the share lacks, and moves the VM to them.
- `devvm IMAGE` with tools the VM lacks adds them, after asking ([adding to a VM](commands.md#adding-to-a-vm)).
- `devvm remove TOOLS` takes tools out (`devvm remove python+claude`). What the VM installed itself, other versions or global packages, stays.

## Which image a new VM gets

`devvm IMAGE` names it. `devvm` alone looks at the folder's top level for the files that call for a toolchain, and names those, in this order:

| Toolchain | Files |
| --- | --- |
| node | `package.json`, `bun.lock`, `bun.lockb` |
| python | `pyproject.toml`, `requirements.txt`, `uv.lock`, `setup.py`, `Pipfile` |
| rust | `Cargo.toml` |
| go | `go.mod` |

If more than one matches, they all go in, in that order (`package.json` and `pyproject.toml` give `node+python`). In a folder with none of them, devvm doesn't guess: it lists what there is to choose from.

Agents go on top of the folder's toolchains: `devvm claude` in a folder with `package.json` makes a `node+claude` VM, and a `claude` one (base and Claude) when nothing is detected. Naming a toolchain or a layer skips detection (`devvm python+claude`), and so does `devvm base`.

Image names are put in one order: the toolchains (node, python, rust, go), then your layers as given, then the agents. So `claude+node` and `node+claude` are the same.

The image (base and your layers) is built first if it isn't, with the shared tools the name has, and the VM is a copy of the image. A VM keeps the image and tools it was made from.

## Building

Each image is built in a VM of its own, which then is the image:

1. base is a new VM from [Ubuntu's cloud image](#ubuntus-cloud-image) with `base.yaml`'s settings, or with `--from`, from a container image [converted into a disk](from.md#how-its-converted). A layer's VM is a copy of its parent image.
2. The VM starts confined: it reaches the internet but not the host's own services on loopback, and forwards no ports. How that's enforced depends on the backend: see [Lima](lima.md#confinement).
3. The script runs: `layers/base.sh` for base, then its chosen [features](customizing.md), yours for a layer.
4. The VM stops. Its record is written last, with its chain, what base started from, and its C library.

Nothing from the host is mounted into a build: layer scripts run third-party installers, which mustn't see your files. base gets `base.yaml`'s memory, or half the host's when that would leave the host less than 2 GiB. The images built on it, and their VMs, keep that.

## Caching

Images are built in layers and cached, like Docker's. `base` comes first, and each custom layer is built on its parent: `foo+bar` is `bar` on `foo` on `base`. A layer starts from its parent, runs its script inside, and is stored under a key, a hash of everything that goes into it. When an image with the key already exists, devvm uses it; otherwise it builds one.

| Image | Its key hashes |
| --- | --- |
| `base` | the backend, `base.yaml`, `layers/base.sh`, the chosen features in order (each name and its script), its rebuild counter, and with `--from`, the image, its digest and the scripts that convert it |
| a custom layer | its parent's key, its name, its script (`~/.config/devvm/layers/<name>.sh`) and its rebuild counter |

So:

- Edit a script and its layer and every layer above it rebuild on next use. The layers below are reused.
- Each backend builds its own images, and never uses another's.
- With `--from`, base's key has the image's digest, recorded when devvm looks it up ([digests](from.md#digests)). When the tag moves to a new image, base and everything on it rebuild.
- A layer whose script is gone hashes as missing, so its images no longer match.
- Nothing else counts: features you haven't chosen, and network policy files (`*.policy`, which the proxy reads at run time and never go into an image), change no key.

An image whose key no longer matches its inputs is stale. It stays until it's deleted, and the next use builds a new one.

### Rebuilding

`devvm rebuild <image>` builds an image again from its parent as it is: for the newest packages a script installs without a pin, or for base, Ubuntu's newest cloud image. It changes the image's rebuild counter (`~/.local/share/devvm/images/rebuilds/<image>`, or `<image>@<from>` with `--from`, kept apart for each image base starts from), which its key hashes, so the image gets a new key, and so does every image on it.

When images are built on it, devvm lists them first, and in a terminal asks before going on. After the build, it deletes every stale image no VM uses: the old one and those built on it among them. They're built again, on the new one, the next time they're needed. On Linux hosts, an image whose disk VMs are overlays of stays, and devvm says which VMs use it.

### Outdated VMs

A VM keeps the image it was made from, and its chain. When a key in the chain changes (an edited script, a rebuild), devvm warns on each launch that the VM is outdated, and leaves it as it is: `devvm rm` and a new VM gets the new image. Shared tool images have keys of their own: see [Building tools](#building-tools).

A key is the first 12 hex digits of a SHA-256. Each input goes in with its length, so different inputs never hash the same bytes. An image's record keeps its chain: the key of each image it's built on, from `base` up.

## Ports

A VM forwards every port it listens on (TCP and UDP) to the same port on this machine's `localhost`, so a server on `:5173` in the VM is at `localhost:5173` here. To forward only some:

```sh
devvm ports                 # what this folder's VM forwards: all, none or a list
devvm ports 5173,8000-8010  # only these ports and ranges
devvm ports add 5432        # one more
devvm ports rm 5173         # one fewer, as listed; the last leaves none
devvm ports none            # nothing; devvm ports all forwards every port again
devvm ports -g none         # what new VMs forward (default: all)
```

- A new VM gets `--ports` (`devvm --ports 5173`), else what `devvm ports -g` set (`~/.config/devvm/ports`, one line: `all`, `none` or a list), else all.
- A stopped VM changes at once. A running one keeps what it forwards until it next starts (`devvm restart`): until then, `devvm ports` shows what it forwards now and says what it will.
- `add` and `rm` take ports and ranges, not `all` or `none`; `rm` on a VM that forwards every port is refused, as there's no list to remove from. `-g` works with them too.
- Ports are forwarded with or without [network policy](network.md). Builds forward none.

## Size

A VM gets its image's size, `base.yaml`'s 4 CPUs, 8 GiB of memory and a 100 GiB disk, unless it's given another:

```sh
devvm size                         # what this folder's VM has: cpus=4 memory=8GiB disk=100GiB
devvm size --cpus 6 --memory 16    # change it; what isn't given stays
devvm size --disk 200              # a bigger disk; never a smaller one
devvm size -g --memory 16          # the size new VMs get (default: the image's)
devvm size -g                      # cpus=, memory= and disk= lines, empty for the image's
```

- A new VM gets `--cpus`, `--memory` and `--disk` (`devvm --cpus 6 base`), else what `devvm size -g` set (`~/.config/devvm/size`), else the image's ([details](commands.md#size-and-ports)).
- `--cpus` takes a whole number, `--memory` and `--disk` a size in GiB: `16`, `16G` or `16GiB`, decimals allowed. More CPUs or memory than this machine has is refused.
- A stopped VM changes at once. A running one keeps its size until it next starts (`devvm restart`): until then, `devvm size` shows what it has now and says what changes.
- Disks grow but never shrink: a disk smaller than the VM's is refused.

### Disk use

A VM's disk size is how far it can grow, not what it takes. A project VM starts as a copy of its image's disk that shares its blocks, so a new VM takes almost nothing until it writes ([clones and overlays](lima.md#clones-and-overlays)):

- **macOS:** an APFS clone. `du` counts the shared blocks as its own, so it overcounts.
- **Linux:** a qcow2 overlay of the image's disk, holding only the VM's changes: about 12 MB for a new VM instead of 2 GB.

## Where state lives

Nothing is written into the project folder. devvm keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/<key>`), rebuild counters (`images/rebuilds/`) and `--from` digests (`images/from-digests`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`), ports (`ports`) and [backend](backends.md#choosing-a-backend) (`backend`) of new VMs, your own layers and features, the keys in devvm's SSH agent (`agent-keys`), and the [ssh config](ssh.md) for VMs and their host keys (`ssh_config`, `known_hosts`) |

A VM's state file records the image it came from, its project folder, its backend and its settings, one `key=value` per line. It lasts as long as the VM: `devvm rm` deletes it too, so the next devvm in that folder starts fresh.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, to a sandbox or a shorter path.

Each write replaces a file whole, so a file is never left half written. Files are readable only by you. When a file can't be read, devvm names it and the line: fix the line or delete the file.

## Build logs and interrupts

The output of the commands devvm runs goes to a log, not the terminal: devvm prints its own steps, and the end of the log when a step fails. `DEVVM_VERBOSE=1` prints the output instead.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, readable only by you. devvm keeps the newest 20. Each command is a `+` line with its arguments, then its output. A numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

Ctrl+C, or closing the terminal, stops the command running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.

An interrupted new VM is deleted, with its state, until its first shell opens: nothing of yours is in it yet.

An interrupted build deletes the image it was building. So does a build whose create or clone fails, once the end of the log is shown: there's nothing in it to look at.

A build that fails later, in its script or as it starts or stops, keeps the image's VM, stopped and without a record, so you can look inside. devvm's error names it and how to open a shell in it, with Lima pointed at devvm's Lima home:

```sh
LIMA_HOME=~/.local/share/devvm/lima limactl shell --start devvm-img-<key>
```

`--start` starts it first, outside the build's confinement. The next build of that image deletes it and starts over.

Either way, the images under it, already built, stay. Any image found without its record (a failed build's, or one devvm couldn't clean up after, such as after a crash or a power cut) is deleted and built again.
