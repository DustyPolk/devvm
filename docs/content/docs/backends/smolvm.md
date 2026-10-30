---
title: smolvm (experimental)
weight: 2
---

[smolvm](https://github.com/smol-machines/smolvm) is an experimental backend. Each project gets a microVM that runs its image as a container, with no init system. You choose it for one VM with `--backend smolvm`, or for every new VM with `devvm setup --backend smolvm`.

devvm emulates some of the features smolvm lacks, and each emulation has been reviewed ([features](../internals/backends.md#features)). They are [network policy](../internals/smolvm.md#network-policy), [ssh for editors](../internals/smolvm.md#ssh-for-editors) (`devvm ssh`, `devvm code` and `--clone`), [forwarding ports](../internals/smolvm.md#ports), every one or only the ones you list, and [turning SSH agent forwarding on or off](../internals/smolvm.md#ssh-agent-forwarding) after a VM is made.

## Requirements

- smolvm 1.32 or newer, with `smolvm` on your `PATH`. With Homebrew, run `brew install smol-machines/tap/smolvm`.
- An arm64 Mac, or Linux on arm64 or x86_64 with a `/dev/kvm` you can read and write. Only the Mac is tested.

`devvm doctor --backend smolvm` checks both, and when you have Homebrew it shows the command that installs smolvm.

## Looking inside

When a build fails, devvm keeps its machine, stopped (`smolvm_script_failed`). To look inside, run `smolvm machine start --name devvm-img-<key>`, then `smolvm machine shell --name devvm-img-<key>`, which gives you a root shell. The next build, or `devvm gc`, deletes the machine.
