---
title: Images and caching
weight: 3
---

This page explains how devvm builds images and shared tools, how it keys and caches them, and how it checks what they start from: Ubuntu's cloud image, a container image from `--from`, and each tool's downloads. [Images and tools](../using/images.md) covers choosing, rebuilding and cleaning up images, and [Starting from a container image](../using/from.md) covers `--from`.

## Building

devvm builds each image in a VM of its own, and that VM then is the image:

1. For base, devvm makes a new VM from [Ubuntu's cloud image](#ubuntus-cloud-image) with `base.yaml`'s settings, or, with `--from`, from a container image [converted into a disk](#how-its-converted). A layer's VM is a copy of its parent image.
2. The VM starts confined, so it reaches the internet but not the host's own services on loopback, and it forwards no ports. How that's enforced depends on the backend, as [Lima](lima.md#confinement) and [Docker](docker.md#building-images) describe. devvm can't build images on Docker Desktop yet.
3. The script runs. For base, that's `assets/layers/base.sh` and then the [features](../using/customizing.md) you chose, and for a layer, it's the layer's script.
4. The VM stops. devvm writes the image's record last, with its chain, what base started from, and its C library.

Nothing from the host is mounted into a build, because layer scripts run third-party installers, which mustn't see your files. base gets `base.yaml`'s memory, or half the host's memory when `base.yaml`'s would leave the host less than 2 GiB. The images built on base, and their VMs, keep that amount.

## Keys and caching

devvm builds images in layers and caches them, as Docker does. `base` comes first, and each layer is built on its parent, so `foo+bar` is `bar` on `foo` on `base`. A layer starts from its parent and runs its script inside it, and devvm stores it under a key, which is a hash of everything that goes into it. When an image with that key already exists, devvm uses it, and otherwise it builds one.

| Image | Its key hashes |
| --- | --- |
| `base` | the backend, `assets/base.yaml`, `assets/layers/base.sh`, the chosen features in order (each name and its script), its rebuild counter, and with `--from`, the image, its digest and the scripts that convert it; on Lima, Ubuntu's pinned cloud image for the host (its release and SHA-256); on smolvm and Docker, their scripts for containers and Ubuntu's container image, pinned by digest |
| a built-in layer | its parent's key, its name, its script (`assets/layers/<name>.sh`) and its rebuild counter |
| a custom layer | its parent's key, its name, its script (`~/.config/devvm/layers/<name>.sh`) and its rebuild counter |

A key is the first 12 hex digits of a SHA-256. Each input goes into the hash with its length, so different inputs never hash the same bytes. An image's record keeps its chain, which is the key of each image it's built on, from `base` up.

This means:

- When you edit a script, its layer and every layer above it rebuild the next time they're used, and the layers below are reused.
- Each backend builds its own images and never uses another backend's.
- With `--from`, base's key has the image's [digest](#digests), so when the tag moves to a new image, base and everything on it rebuild.
- A layer whose script is gone hashes as missing, so its images don't match any more.
- Nothing else counts. Features you haven't chosen change no key, and neither do network policy files (`*.policy`), which the proxy reads at run time and which never go into an image.

An image whose key doesn't match its inputs any more is stale. It stays until it's deleted ([cleaning up](../using/images.md#cleaning-up)), and the next use builds a new one.

## Rebuilding

`devvm rebuild` works through the rebuild counter, which each image's key hashes. The counter is the file `~/.local/share/devvm/images/rebuilds/<image>`, or `<image>@<from>` with `--from`, so each image that base starts from has a counter of its own. A rebuild writes a new value into it, made of the time and a random part, so the image gets a new key, and so does every image built on it. devvm lists those images first (`rebuild_replaces`). After the build, it deletes every stale image that no VM uses, which includes the old image and those built on it, and they're built again, on the new image, the next time they're needed. On Linux hosts, an image whose disk VMs are overlays of stays, and devvm says which VMs use it.

A VM keeps the image it was made from, and its state keeps that image's chain. When a key in the chain changes, after an edited script or a rebuild, devvm warns on each launch that the VM is outdated (`vm_outdated`) and leaves the VM as it is ([outdated VMs](../using/images.md#outdated-vms)). Shared tool images have keys of their own, which [building tools](#building-tools) describes.

## Ubuntu's cloud image

On Lima, base starts from a pinned, dated release of Ubuntu 26.04's cloud image (`release-YYYYMMDD` on cloud-images.ubuntu.com), and devvm pins the image's SHA-256 for arm64 and amd64 with it. The pin moves with base's package snapshot to the newest release dated before the snapshot, as smolvm's and Docker's container image does ([maintaining](../contributing/maintaining.md#updating-pinned-versions)). A release with newer packages than the snapshot's would leave apt unable to install the packages that depend on their exact versions.

devvm checks the image before it uses it:

- The release's `SHA256SUMS` must list the pinned SHA-256 for the host's image, and the download must match it. devvm refuses any other checksum (`ubuntu_checksum_unpinned`).
- When `gpgv` is installed (`brew install gnupg`, or `apt install gpgv`), `SHA256SUMS` must be signed by Ubuntu's cloud image key, which is pinned in `assets/layers/ubuntu-cloud.gpg` (fingerprint `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`). No other key counts. When the signature is missing or doesn't match, devvm refuses the image (`ubuntu_bad_signature`).
- When `gpgv` isn't installed, devvm says so (`ubuntu_unsigned`), and the pinned SHA-256 still decides.

Ubuntu removes its dated releases after a few months. Once the pinned release is gone, base can't be built on Lima (`no_ubuntu_image`) until you update devvm to a version with a newer pin ([troubleshooting](../using/troubleshooting.md)).

The pinned release and its SHA-256 are part of base's key, so a new pin rebuilds base. base's record keeps the address and SHA-256 of the image it was built from.

## Base's settings and pins

`assets/base.yaml` holds base's VM settings, which are 4 CPUs, 8 GiB of memory and a 100 GiB disk. `assets/layers/base.sh` installs base, and the optional [features](../using/customizing.md) you chose follow it.

Each download in `assets/layers/base.sh` is pinned to a version and a SHA-256 at the top of the script, and the build fails if a checksum doesn't match. Each feature pins its downloads in the same way, at the top of its script, `assets/features/<name>.sh` in the repo, which is built into devvm. While base builds, apt's packages come from a dated snapshot of Ubuntu's archive (`apt_snapshot`, from snapshot.ubuntu.com), and project VMs use the live archive ([base](../using/images.md#base)).

## The docker layer

The built-in [`docker` layer](../using/images.md#built-in-layers)'s script is `assets/layers/docker.sh`, embedded in devvm, and its [allowlist](../safety/network.md#allowlists) is `assets/layers/docker.policy`. The script pins the checksum of the signing key of Docker's apt repository and each package's version at the top. On `--from alpine`, it installs Alpine's own packages, which aren't pinned. Running Docker's engine in a VM is a backend feature of its own, `docker-layer` ([features](backends.md#features)).

## How it's converted

With `--from`, devvm turns the container image into a disk in a throwaway VM, `devvm-oci-build`, which it makes from Ubuntu's cloud image with `base.yaml`'s settings. No Docker is needed:

1. [crane](https://github.com/google/go-containerregistry/tree/main/cmd/crane) downloads exactly the [digest](#digests) in `base`'s key, for this machine's architecture, and flattens its layers. crane is pinned like every download: its version and SHA-256 are at the top of `assets/oci/build.sh`, checked with `sha256sum`.
2. `assets/oci/bootify.sh`, in a chroot of the image, adds what a container lacks to boot as a VM: a kernel and initramfs, an init system (systemd, or OpenRC on Alpine), cloud-init, sshd and sudo, and your user in the VM.
3. `mke2fs` writes the image's files into a raw ext4 disk, and `base` grows it to `base.yaml`'s size.

devvm then deletes the build VM and creates `base` from the disk, booting its kernel and initrd directly. One conversion runs at a time, so another devvm's conversion waits for it. A conversion also takes over the turn of a devvm that ended without giving it up.

## Digests

`base` is keyed by the container image's digest, not its tag. When devvm builds, it asks the image's registry for the digest anonymously, with a `HEAD` request for the image's manifest, which doesn't count against Docker Hub's pull limit. When the registry asks for a token, devvm gets one from the registry's token service, at an `https` address only. The digest goes into `base`'s key, and devvm records it for each image in `~/.local/share/devvm/images/from-digests`. An existing VM's state records what its base was made from (`from=`).

devvm refuses a reference it can't read (`bad_from`). When it can't reach the registry, it uses the digest it recorded last, with a warning (`from_cached_unreachable`), and with none recorded, it fails (`from_unreachable`). [Tags and digests](../using/from.md#tags-and-digests) covers when a moved tag rebuilds base, how `@sha256:` pins an image, and how devvm reads a reference.

## The image is untrusted

The container image's own code runs as root in the build VM, so devvm treats what it writes as hostile:

- The build VM is [confined](lima.md#confinement) like any build, so it reaches the internet, for crane, but not this machine's own services. It mounts only devvm's `oci/` folder ([written out](install.md#devvms-own-files) in `~/.local/share/devvm/assets/<hash>`), read-only, and one writable folder of devvm's own, `vm/` in a temporary folder, `devvm-oci.<random>`. devvm writes nothing there itself except an [image only Docker has](#images-only-docker-has), before the VM starts.
- devvm checks what comes out once the build VM is gone. Each file must be a regular file, because a link could point Lima at a file on this machine. The kernel command line can hold only letters, digits, spaces and `=,./:_-`, because it goes into Lima's config. The C library must be `gnu` or `musl`. The disk must be raw ext4, because another format, such as qcow2, can name a file on this machine as its backing file. devvm refuses anything else (`convert_bad_output`), and it doesn't create `base`.
- devvm deletes the build VM and the temporary folder however the run ends, Ctrl+C included.

## Images only Docker has

When the registry doesn't give devvm the image ([images only Docker has](../using/from.md#images-only-docker-has)), devvm keys `base` by Docker's image ID instead of a digest. devvm exports the image with `docker save` into the build VM's folder, where crane reads it, and it checks that the export is the image it looked up. The image is saved by name, so if the tag has moved since, for example after a rebuild, devvm stops and asks you to run it again. devvm only asks Docker about images, with `docker image inspect`, `docker info` and `docker save`.

## Versions

devvm doesn't pin the shared tools' versions ([shared tools](../using/images.md#shared-tools)). Instead, `assets/shared-tools/mise.toml` says which releases to follow, which is the latest of each tool, and Node 24. The first time a tool is needed, devvm resolves it to upstream's newest release and writes that into the tools lock, `~/.local/share/devvm/mise.lock`, with each download's checksum for every platform that VMs run (Linux, with glibc and with musl). Builds install exactly the lock's versions and check them against those checksums. Resolving needs this machine to be online once, and until it has been, devvm says so (`tools_need_resolve`).

The resolving runs on this machine with a mise of devvm's own. That mise is a pinned release, which devvm downloads once into `~/.local/share/devvm/mise/<version>` and checks against its SHA-256 before it runs it (`mise_checksum`). devvm never uses the mise you have installed, and nothing it does reaches your own mise config.

Sometimes a tool's newest version isn't published for every platform yet, for example when a release is still uploading, when a tag has no release, or for a few days after a Node release until its musl builds appear. devvm then takes the newest of the next five older versions that is published everywhere, and says so (`tool_stepped_back`). When none of them is, devvm says that too (`tool_no_download`). The next check tries the newest version again.

A new VM gets the lock's versions, and devvm checks the lock again first when the last check was more than a day ago. When a check fails, devvm uses the versions it found last (`tools_check_failed`). When `assets/shared-tools/mise.toml` changes, as it can in a devvm upgrade, devvm resolves the tools again. A VM lists its tool images in its state, so a newer version never changes a VM that has an older one. Older versions that no VM has are unused, and `devvm gc` deletes them.

## Building tools

devvm builds the tool images that an image name needs together with the image (`devvm build node+claude`), unless the share already has them. They build in a throwaway VM that devvm makes from base and confines like an image build. That VM never sees the share. It writes into a folder of its own, `~/.local/share/devvm/tools-build.<random>`, which devvm deletes once it's done. devvm then checks that each image is a plain file and not a link (`tool_bad_output`), moves it into the share and records its hash. Go builds in a VM of its own, never together with other tools. Only one build at a time writes to the share, so another devvm waits for it and gives up after ten minutes (`share_lock_held`).

Each tool image is named `<tool>-<version>-<key>.erofs`. Its key hashes the tool's entry in the tools lock (its version, its downloads and their checksums), `assets/shared-tools/build.sh` and the C library the image is built for. A new version, a new download or an edited build script builds the image again, and a base with a different C library, such as Alpine's musl, gets an image of its own. When the lock has no download of a tool for the VM's platform, devvm doesn't build it and says to try again later (`tool_unbuilt`).

`devvm rebuild` refuses shared tools, because they aren't images and their versions come from the lock (`tools_not_images`).

## The share

devvm builds each tool version once into a compressed, read-only image (EROFS) in `~/.local/share/devvm/tools`, with its hash next to it in `<image>.sha256`. That folder is mounted read-only in every VM at `/mnt/devvm-tools`, and each VM mounts the images its tools need at `/opt/devvm/tools/<tool>/<version>`, at boot and on every launch. On [Docker](docker.md#shared-tools), devvm unpacks the images into a volume, mounts that volume instead, and links each tool's folder where its image would be mounted. A VM can't change the shared images, even as root.

## Listing and cleaning up

[Listing images](../using/images.md#listing-images) describes the columns of `devvm images`. An `unfinished` image is a build that didn't finish, so it has no record. A tool image's `STATE` compares it with the tools lock and with the VMs' state:

- `current` is the tools lock's version, for the C library of the base that new VMs get, which `--from` changes.
- `in use` is another version, which a VM has in its state.
- `unused` is neither.
- `not built` is the lock's version of a tool that the share has other versions of.
- `not installed` means the share has nothing of that name's.

Listing never resolves the tools lock. Before there is a lock, no tool image is current, and the tool images that no VM has show no state.

`devvm images rm` deletes images with their records. `devvm gc` deletes stale and unfinished images with their records, and unused tool images with their hashes. gc holds the share's lock throughout, so no tool build or `devvm update` changes the share meanwhile. A tool image is unused only when compared with the tools lock, so before there is a lock, gc deletes none. Because `current` is for the C library of the base that new VMs get, the musl tool images of an Alpine VM are unused once it's gone, unless you give gc that `--from`.
