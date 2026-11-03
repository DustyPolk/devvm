---
title: Starting from a container image
weight: 10
---

By default `base` starts from Ubuntu 26.04's cloud image. `--from IMAGE` starts it from a Debian-, Ubuntu- or Alpine-based container image instead, such as `node:24`, `python:3.13`, `debian:13`, `alpine:3` or an image you built yourself. Use it when your project needs a distribution or a system package set that Ubuntu doesn't give you.

```sh
devvm --from node:24                 # a new VM whose base is node:24
devvm build --from alpine:3 base     # just the image
DEVVM_FROM=node:24 devvm             # a default for every new VM and build
```

Everything above `base` builds and runs as usual, including base's own setup, the layers (`docker` and yours) and the [shared tools](images.md#shared-tools). devvm keeps the images from each container image apart, so each `base` and the images on it sit side by side with the others, and each image's record keeps the container image its base came from and that image's digest.

`--from` applies to new VMs and to `devvm build` and `devvm rebuild`, and `devvm images rm` takes it to delete an image on a base from that container image. An existing VM keeps what it was made from, and `devvm tools add` and `devvm tools update` build its shared tools on that `base`. devvm ignores another `--from` for an existing VM, with a warning.

[Images and caching](../internals/images.md#how-its-converted) explains how devvm turns a container image into a VM's disk and how it treats the image's code as untrusted.

## Tags and digests

devvm builds `base` from the exact image that a tag names when it builds, which is the image's digest, and it records that digest:

- When the tag moves to a new image, such as a new `node:24`, the next build makes a new `base`. That build happens for a new VM with `--from`, `devvm build` or `devvm rebuild`, and the old `base` is then stale.
- Offline, or when the registry doesn't have the image any more, devvm uses the digest it recorded last, with a warning. With none recorded, it fails.
- Opening an existing VM never looks anything up, because only builds do.
- `--from node:24@sha256:<digest>` pins an exact image, and devvm never looks it up.

devvm reads a reference the way Docker does, so `node:24` is Docker Hub's `library/node` with the tag `24`. A first part with a dot or a port, or `localhost`, names the registry, as in `ghcr.io/org/image:tag` or `localhost:5000/app`. A reference without a tag means `latest`. [Digests](../internals/images.md#digests) explains how devvm asks a registry for them.

## Images only Docker has

Public images work from any registry, such as Docker Hub, ghcr.io or quay.io. When a registry doesn't give devvm the image, devvm takes it from Docker, as long as Docker runs and has it. That covers an image you built and never pushed, and a private one, which you get with `docker login` and then `docker pull`. devvm itself never logs in to a registry, and it never pulls, builds or deletes Docker's images.

If the image's tag moves to another image while devvm builds, for example because you rebuilt it, devvm stops and asks you to run it again.

## Supported images

- **Debian, Ubuntu and Alpine** and the distributions based on them. devvm refuses any other, and an image without a shell or `/etc/os-release`.
- **This machine's architecture.** devvm refuses an image without an arm64 build (or amd64), and names the ones it has.
- **Alpine** uses OpenRC and musl, so its VMs get musl builds of the shared tools, built apart from the others.
- **Pinned packages where the distribution has snapshots.** While `base` builds, apt's packages come from a dated snapshot, as for Ubuntu's image. Debian 12 (and so `node:24`) and Ubuntu before 26.04 on arm64 have none, and Alpine's packages, including the [desktop layer](images.md#desktop)'s, the [docker layer](images.md#docker)'s and the [nix layer](images.md#nix)'s, come from its repositories. These build with unpinned packages, and devvm warns you.
- **Tested.** The end-to-end tests make VMs from `node:24` (Debian 12, with node, python, claude and the docker and nix layers) and `alpine:3` (with node and the docker, nix and desktop layers), and build `base` from `ubuntu:26.04`.

## Backends

Lima starts `base` from a container image by converting the image into a disk ([backends](../internals/backends.md#features)).
