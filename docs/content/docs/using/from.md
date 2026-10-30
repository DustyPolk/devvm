---
title: Starting from a container image
weight: 8
---

By default `base` starts from [Ubuntu 26.04's cloud image](../internals/images.md#ubuntus-cloud-image). `--from IMAGE` starts it from a Debian-, Ubuntu- or Alpine-based container image instead, such as `node:24`, `python:3.13`, `debian:13`, `alpine:3` or an image you built yourself.

```sh
devvm --from node:24 claude          # a new VM whose base is node:24
devvm --from alpine:3 build base     # just the image
DEVVM_FROM=node:24 devvm             # a default for every new VM and build
```

Everything above `base` builds and runs as usual: `assets/layers/base.sh`, the layers (`docker`, yours) and the [shared tools](images.md#shared-tools). Images from different `--from` images are cached side by side, each `base` and the images on it apart from the others, and each image's record keeps the image its base came from and its digest.

`--from` applies to new VMs and to `devvm build` and `devvm rebuild`. An existing VM keeps what it was made from: its state records it (`from=`), and adding to it, `devvm update` and `devvm remove` build on that `base`. Naming another `--from` for it is ignored, with a warning.

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

Lima converts the image into a disk as above. [Backends](../internals/backends.md#features) lists the `from` feature for each backend.
