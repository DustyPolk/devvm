---
title: Starting from a container image
weight: 21
---

By default `base` starts from [Ubuntu 26.04's cloud image](images.md#ubuntus-cloud-image). `--from IMAGE` starts it from a Debian-, Ubuntu- or Alpine-based container image instead, such as `node:24`, `python:3.13`, `debian:13`, `alpine:3` or an image you built yourself.

```sh
devvm --from node:24 claude          # a new VM whose base is node:24
devvm --from alpine:3 build base     # just the image
DEVVM_FROM=node:24 devvm             # a default for every new VM and build
```

Everything above `base` builds and runs as usual: `assets/layers/base.sh`, the layers (`docker`, yours) and the [shared tools](images.md#shared-tools). Images from different `--from` images are cached side by side, each `base` and the images on it apart from the others, and each image's record keeps the image its base came from and its digest.

`--from` applies to new VMs and to `devvm build` and `devvm rebuild`. An existing VM keeps what it was made from: its state records it (`from=`), and adding to it, `devvm update` and `devvm remove` build on that `base`. Naming another `--from` for it is ignored, with a warning.

## How it's converted

devvm turns the image into a disk in a throwaway VM, `devvm-oci-build`, made from Ubuntu's cloud image with `base.yaml`'s settings. No Docker is needed:

1. [crane](https://github.com/google/go-containerregistry/tree/main/cmd/crane) downloads exactly the digest in `base`'s key, for this machine's architecture, and flattens its layers. crane is pinned like every download: its version and SHA-256 are at the top of `assets/oci/build.sh`, checked with `sha256sum`.
2. `assets/oci/bootify.sh`, in a chroot of the image, adds what a container lacks to boot as a VM: a kernel and initramfs, an init system (systemd, or OpenRC on Alpine), cloud-init, sshd and sudo, and your user in the VM.
3. `mke2fs` writes the image's files into a raw ext4 disk; `base` grows it to `base.yaml`'s size.

devvm then deletes the build VM and creates `base` from the disk, booting its kernel and initrd directly. One conversion runs at a time: another devvm's waits for it, and takes over the turn of a devvm that ended without giving it up.

### The image is untrusted

The image's own code runs as root in the build VM, so devvm treats what it writes as hostile:

- The build VM is [confined](lima.md#confinement) like any build: it reaches the internet, for crane, but not this machine's own services. It mounts only devvm's `oci/` folder ([written out](install.md#installing-devvm) in `~/.local/share/devvm/assets/<hash>`), read-only, and a folder of devvm's own, writable: `vm/` in a temporary folder, `devvm-oci.<random>`. devvm writes nothing there itself but an [image only Docker has](#images-only-docker-has), before the VM starts.
- What comes out is checked once the build VM is gone. Each file must be a regular file (a link could point Lima at a file on this machine), the kernel command line can hold only letters, digits, spaces and `=,./:_-` (it goes into Lima's config), the C library must be `gnu` or `musl`, and the disk must be raw ext4 (another format, such as qcow2, can name files on this machine as its backing file). Anything else is refused, and `base` isn't created.
- The build VM and the temporary folder are deleted however the run ends, Ctrl+C included.

## Digests

`base` is keyed by the image's digest, not its tag. When devvm builds, it asks the image's registry for the digest, anonymously: a `HEAD` request for the image's manifest, which doesn't count against Docker Hub's pull limit, with a token from the registry's token service when it asks for one (an `https` address only). The digest goes into `base`'s key, and is recorded per image in `~/.local/share/devvm/images/from-digests`.

- When the tag moves to a new image (a new `node:24`), the next build, a new VM with `--from` or `devvm build` and `devvm rebuild`, makes a new `base`, and the old one is stale.
- Offline, or when the registry doesn't have the image any more, devvm uses the digest it recorded last, with a warning. With none recorded, it fails.
- Opening an existing VM never looks anything up: only builds do.
- `--from node:24@sha256:<digest>` pins an exact image, and is never looked up.

References are read the way Docker reads them: `node:24` is Docker Hub's `library/node`, tag `24`. A first part with a dot or a port, or `localhost`, names the registry (`ghcr.io/org/image:tag`, `localhost:5000/app`). No tag is `latest`.

## Images only Docker has

Public images work from any registry: Docker Hub, ghcr.io, quay.io or another. An image the registry doesn't give devvm comes from Docker, when Docker runs and has it: one you built and never pushed, or a private one (`docker login`, then `docker pull`). devvm itself never logs in to a registry.

Such a `base` is keyed by Docker's image ID. devvm exports the image with `docker save` into the build VM's folder, where crane reads it, and checks the export is the image it looked up: it's saved by name, so if the tag moved since (a rebuild), devvm stops and asks you to run it again. devvm only asks Docker for images (`docker image inspect`, `docker info`, `docker save`); it never pulls, builds or deletes them.

## Supported images

- **Debian, Ubuntu and Alpine** and the distributions based on them. devvm refuses any other, and an image without a shell or `/etc/os-release`.
- **This machine's architecture:** an image without an arm64 build (or amd64) is refused, naming the ones it has.
- **Alpine** uses OpenRC and musl, so its VMs get musl builds of the shared tools, built apart from the others.
- **Pinned packages where the distribution has snapshots:** while `base` builds, apt's packages come from a dated snapshot, as for Ubuntu's image. Debian 12 (and so `node:24`) and Ubuntu before 26.04 on arm64 have none, and Alpine's packages (the [docker layer](images.md#built-in-layers)'s too) come from its repositories: these build unpinned, with a warning.
- **Tested:** the end-to-end tests make VMs from `node:24` (Debian 12, with node, python and claude) and `alpine:3`, and build `base` from `ubuntu:26.04`.

## Backends

Lima converts the image into a disk as above. [Backends](backends.md#features) lists the `from` feature for each backend.
