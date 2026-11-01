---
title: devvm doctor
weight: 7
---

Check what devvm needs on this machine

## Synopsis

Check what the backend new VMs get (--backend for another) needs on this machine, and say how to get what's missing: devvm never installs it. The docker backend also says which engine it uses, and on Docker Desktop which folders it shares. Then list each backend's features (native, emulated, unreviewed: emulated but refused until reviewed, or none) and whether Docker is installed, which only --from with an image only Docker has needs. When HTTPS_PROXY is set and ~/.config/devvm/ca has no CA certificates, it says that a proxy that inspects TLS needs its CA there. It warns when another devvm comes first on PATH, when the disk the backend's images and VMs go on has less free space than building base needs, and when this machine has less than 8 GiB of memory or fewer than 4 CPUs. Fails when something is missing, but not for any of these warnings.

```text
devvm doctor [flags]
```

## Options

```text
      --backend NAME   the NAME of the backend to use (default: devvm setup --backend, else lima)
  -h, --help           help for doctor
      --json           print what it found as one JSON object, for scripts, and nothing else; succeeds whatever is missing
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
