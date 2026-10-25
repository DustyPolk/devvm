---
title: devvm images rm
weight: 10
---

Delete images, by name or by key

## Synopsis

Delete images, with their records: each by its name (base, foo+bar), or by the key devvm images shows, for a stale or unfinished one; with --from, a name is its image on a base from that container image. Every one must be built, or nothing is deleted. VMs made from an image keep working, except on Linux hosts, where an image whose disk VMs use is refused. Shared tools aren't images.

```text
devvm images rm IMAGE|KEY... [flags]
```

## Options

```text
  -h, --help   help for rm
```

## Options inherited from parent commands

```text
      --backend NAME        the NAME of the backend a new VM or image gets (default: the default backend file's, else lima)
      --cpus N              the N CPUs a new VM gets (default: the size file's, else the image's)
      --disk GiB            a new VM's disk in GiB, at least the image's (default: the size file's, else the image's)
      --from IMAGE          the container IMAGE a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)
      --memory GiB          a new VM's memory in GiB: 16, 16G or 16GiB (default: the size file's, else the image's)
      --network-policy      create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)
      --no-network-policy   create the VM without network policy: it reaches anything this machine can
      --ports PORTS         which PORTS a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)
```

## See also

- [devvm images](devvm_images.md): List the images and the shared tools
