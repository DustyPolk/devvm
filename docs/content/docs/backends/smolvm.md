---
title: smolvm (experimental)
weight: 2
---

{{< experimental >}}

[smolvm](https://github.com/smol-machines/smolvm) is an experimental backend. Each project gets a microVM that runs its image as a container, with no init system, so devvm starts the VM's services itself after each start. You choose it for one VM with `--backend smolvm`, or for every new VM with `devvm setup --backend smolvm`.

Network policy, `devvm host`, `devvm code`, `--clone`, forwarding ports and turning SSH agent forwarding on or off after a VM is made all work as they do on Lima. smolvm lacks some of these, so devvm builds them from other parts of smolvm, and [How it works › smolvm](../internals/smolvm.md) explains how.

## Requirements

- smolvm 1.32 or newer, with `smolvm` on your `PATH`. With Homebrew, run `brew install smol-machines/tap/smolvm`.
- An arm64 Mac, or Linux on arm64 or x86_64 with a `/dev/kvm` you can read and write. Only the Mac is tested.

`devvm doctor --backend smolvm` checks both, and when you have Homebrew it shows the command that installs smolvm.

## What's different

- **devvm's machines are next to yours.** On macOS, `smolvm machine ls` shows devvm's machines along with your own. Each of devvm's carries the label `devvm.data=<devvm's data folder>`, and devvm touches only the machines with its label.
- **Docker in a VM works.** The [docker layer](../using/images.md#built-in-layers) runs Docker's engine in the VM, as on Lima, and you can add it to an existing VM.
- **Nix works.** The [nix layer](../using/images.md#nix) runs Nix's daemon in the VM, with each build in Nix's sandbox, as on Lima.
- **The SSH agent is in the VM the whole time it runs.** With [SSH agent forwarding](../using/ssh.md#agent-forwarding) on, smolvm forwards the agent into the VM for as long as the VM runs, whether or not you're connected, and not only during your ssh sessions as on Lima.
- **File watchers see your edits, but not your deletions.** A dev server or a file watcher in the VM sees the files you make on this machine, as on Docker Desktop, and it sees a file you change here when it watches that file itself. A watcher that watches only the file's folder learns that the folder changed, but not which file, and no watcher sees the files you delete here. A watcher that needs deletions, or one that watches only folders, can poll the project folder instead, with the settings on the [Lima page](lima.md#file-watchers).
- **Paths can't contain colons.** smolvm splits mounts on colons, so devvm refuses a folder whose path has one.
- **Start VMs with devvm, not `smolvm machine start`.** A VM started outside devvm gets the SSH agent your shell has, whatever you chose for it, and devvm's ssh doesn't run in it. When devvm finds such a VM running, it stops the VM and says so, unless the VM uses your agent anyway, and the next devvm starts it again with its own choice.

## Looking inside

When a build fails, devvm keeps its machine, stopped. To look inside, run `smolvm machine start --name devvm-img-<key>`, then `smolvm machine shell --name devvm-img-<key>`, which gives you a root shell. The next build, or `devvm gc`, deletes the machine.

## Cleaning smolvm's caches

Each of devvm's smolvm images is a full pack, and `base`'s is about 450 MB. When you stop using smolvm, `devvm gc --backend smolvm` deletes all of devvm's packs and the shared tools built for smolvm that no VM has and that aren't current, except a pack that a VM was made from ([cleaning up](../using/images.md#cleaning-up)). smolvm also keeps caches of its own (`~/Library/Caches/smolvm` on macOS), which also hold your own machines and packs. The largest part is the disk smolvm unpacks of each pack that a machine is made from, which stays after the pack is gone. `devvm gc --caches` frees those of devvm's packs that no VM of devvm's was made from, after listing them and asking ([what devvm keeps](../using/space.md#smolvm)). devvm never runs `smolvm pack prune` or `smolvm machine prune`, because they also clean what belongs to your own packs and machines. Before a build or a new VM, devvm checks the free space on the disk of its data folder, and on macOS also on that of your home folder, where smolvm keeps machines ([free space](../using/ports-size.md#free-space)).
