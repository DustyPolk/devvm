---
title: Ports and size
weight: 5
---

This page covers the ports a VM forwards to this machine and the CPUs, memory and disk it gets. You choose both when devvm makes a VM, and you can change both later with `devvm ports` and `devvm size`. [Port forwarding](../internals/ports.md) explains how devvm's forwarder works.

## Size and ports

A new VM gets its image's size unless you set one with `--cpus`, `--memory` and `--disk`, as in `devvm --cpus 6 base`. For `base` that's 4 CPUs, 8 GiB of memory and a 100 GiB disk. On a machine with fewer than 4 CPUs, base gets as many CPUs as the machine has, and when 8 GiB would leave the machine less than 2 GiB of its own, base gets half of the machine's memory.

- `--cpus` takes a whole number. `--memory` and `--disk` take a size in GiB, written `16`, `16G` or `16GiB`, and decimals are allowed.
- What the flags leave unset comes from `~/.config/devvm/size` when that file sets it, with one `cpus=`, `memory=` or `disk=` line each. `devvm size -g` and `devvm setup` write that file ([choosing new VMs' size](#choosing-new-vms-size)).
- devvm refuses more CPUs than this machine has, and a disk smaller than the image's, because disks can't shrink. It refuses them before it builds anything.
- devvm also refuses more memory than a VM can have, because a VM that asks for all of this machine's memory doesn't start. A VM can have this machine's memory less 2 GiB, counted in whole GiB, so 6 GiB on an 8 GiB machine. On a machine with 4 GiB or less, it can have half, so 2 GiB on a 4 GiB machine. When you ask for more, devvm says how much memory this machine has and the most you can give a VM, so you can run the command again with that.

A new VM forwards every TCP port from 1024 up that it listens on, unless `--ports` says otherwise, as in `devvm --ports 5173`.

- `--ports` takes `all`, `none`, or ports and ranges such as `5173,8000-8010`.
- Without `--ports`, the default comes from `~/.config/devvm/ports`, which holds one line of `all`, `none` or a list. `devvm ports -g` writes that file, and without it, a new VM forwards every port.

devvm refuses a size or ports file that doesn't hold a valid value, as it refuses a bad flag.

## Ports

A VM forwards the TCP ports it listens on to the same port on this machine's `localhost`, so a server on `:5173` in the VM is at `localhost:5173` here. To change which ports a VM forwards:

```sh
devvm ports                 # what this folder's VM forwards: all, none or a list
devvm ports 5173,8000-8010  # only these ports and ranges
devvm ports add 5432        # one more
devvm ports rm 5173         # one fewer, as listed; the last leaves none
devvm ports none            # nothing; devvm ports all forwards every port again
devvm ports -g none         # what new VMs forward (default: all)
```

- A change applies at once. A stopped VM forwards the new ports from its next start. On a running VM, devvm starts its forwarder again with the new ports while the VM runs on, so a server on an added port reaches this machine within a few seconds. The connections that were open through the old forwarder close, so a client that was connected through it has to connect again.
- `add` and `rm` take ports and ranges, not `all` or `none`. devvm refuses `rm` on a VM that forwards every port, because there's no list to remove from. `-g` also works with `add` and `rm`.
- devvm forwards ports with or without [network policy](../safety/network.md), and it forwards no ports from builds.

When a VM forwards every port, devvm forwards each TCP port from 1024 up that a server in the VM listens on, whether on the VM's loopback or on all its addresses. When a VM lists its ports, devvm forwards those ports alone, however many there are, ports below 1024 included. A port reaches this machine within a few seconds of the server starting, only on `localhost` (`127.0.0.1` and `::1`), and the forward goes away when the server stops. At most 256 ports are forwarded at once.

A VM never takes a port that a program on this machine already has, a port that another VM lists, or a port that devvm uses for its VMs' ssh and network policy. When a port you listed is taken by a program on this machine as the VM starts, or as you change its ports, devvm warns you and the VM runs on without that port. The next start or change that finds the port free forwards it. A program that starts on a port after the VM's forward does can still bind it on all addresses, as most servers do, and then that program's `localhost` connections reach the VM.

If the forwarder can't start, devvm warns you and names its log, and the VM runs on without forwarded ports.

**UDP isn't forwarded, on any backend.** Run the UDP client in the VM instead, or tunnel it over a forwarded TCP port with [socat](http://www.dest-unreach.org/socat/doc/socat.html), installed on both sides. For a UDP server on port 8125 in the VM, with TCP port 9125 forwarded:

```sh
socat TCP-LISTEN:9125,fork,reuseaddr UDP:127.0.0.1:8125   # in the VM
socat UDP-LISTEN:8125,fork TCP:127.0.0.1:9125             # on this machine
```

TCP is a stream, so datagrams can arrive merged or split. This suits low-rate request and response traffic, such as DNS, statsd or syslog, but not high-rate or latency-sensitive traffic, such as games, QUIC or WebRTC media.

## Size

To see or change a VM's size:

```sh
devvm size                         # what this folder's VM has: cpus=4 memory=8GiB disk=100GiB
devvm size --cpus 6 --memory 16    # change it; what isn't given stays
devvm size --disk 200              # a bigger disk; never a smaller one
devvm size -g --memory 16          # the size new VMs get (default: the image's)
devvm size -g                      # cpus=, memory= and disk= lines, empty for the image's
```

- The flags take the same values as for a new VM and have the same limits, so devvm refuses more CPUs than this machine has and more memory than a VM can have ([size and ports](#size-and-ports)). `devvm size -g` checks a default size in the same way when you set it. A VM that already has more memory keeps it and still starts, because devvm checks a size only when you set one.
- A stopped VM changes at once. A running VM keeps its size until it next starts, for example with `devvm restart`, and until then `devvm size` shows what the VM has now and says what changes.
- Disks grow but never shrink, so devvm refuses a disk smaller than the VM's.
- On the [Docker backend](../backends/docker.md#what-docker-vms-cant-do), a VM's disk has no size of its own. devvm refuses `--disk`, a default disk size doesn't apply, and `devvm size` shows how much the VM holds instead (`cpus=4 memory=8GiB disk_used=1.2GiB`), with a warning past 100 GiB.

### Choosing new VMs' size

devvm asks for the CPUs and memory that new VMs get on your first run, before it asks which features base gets, and again whenever you run `devvm setup`:

```sh
devvm setup                         # asks for the backend, base's features, then the CPUs and memory
devvm setup --cpus 6 --memory 16    # without asking; the backend and features stay as they are
```

Before the two questions, devvm says how many CPUs and how much memory this machine has. Each question shows in brackets the value new VMs get now. That's what `devvm size -g` saved, or else base's own size fitted to this machine: 4 CPUs and 8 GiB, or less on a machine that doesn't have room for them ([size and ports](#size-and-ports)). Press Enter to keep the value, or type another one. devvm checks what you type as it checks `--cpus` and `--memory`, and when this machine can't give it, devvm says why and asks again.

devvm saves only the values you type, in the same file as `devvm size -g`, and keeps a saved disk size as it is. When you keep both values, devvm saves nothing, so new VMs go on getting the image's size. Either way, devvm prints the size new VMs get. A new size changes only the VMs made after it, and nothing is rebuilt, so existing VMs keep their size until you change it with `devvm size`.

The first run asks only in a terminal, and only when no size is saved and you didn't give `--cpus` or `--memory`. It asks once, because that run also chooses base's features, and devvm asks only before they're chosen. Without a terminal, devvm asks nothing, and new VMs get the image's size. `devvm setup` asks only in a terminal and only when you give it no flags. `--cpus` and `--memory` on `devvm setup` take the same values as on `devvm size -g` and have the same limits, and with only them, devvm asks nothing else.

### Disk use

A VM's disk size is how far the disk can grow, not how much space it takes. A project VM starts as a copy of its image's disk that shares the image's blocks, so a new VM takes almost nothing until it writes ([clones and overlays](../internals/lima.md#clones-and-overlays)):

- **macOS:** the copy is an APFS clone. `du` counts the shared blocks as the clone's own, so it overcounts.
- **Linux:** the copy is a qcow2 overlay of the image's disk, which holds only the VM's changes, about 12 MB for a new VM instead of 2 GB.

### Free space

Before devvm builds an image or makes a VM, it checks the free space on the disk that will hold it, and it refuses to start the step when there's less free than the step needs:

| Step | Free space it needs |
| --- | --- |
| building `base` | 12 GiB |
| building any other image | 8 GiB |
| building shared tool images | 8 GiB |
| making a VM | 4 GiB |

Each build checks again before it starts, so the first VM on a machine, which builds base, needs 12 GiB free. The numbers leave a margin over what each step takes. base takes 3 to 5 GiB, depending on its features. On macOS, an image built on another and a new VM take only what they write, but on Linux hosts an image starts as a full copy of its parent's disk. A VM then grows as you work in it, and its disk never shrinks.

devvm checks the disk that holds the backend's images and VMs. On Lima, that's the disk of devvm's Lima home. On smolvm, it's the disk of devvm's data folder, and on macOS also that of your home folder, where smolvm keeps its machines. Shared tool images come out into devvm's data folder on every backend, so a tools build also checks that folder's disk. The Docker backend keeps its images and VMs in the engine's own storage, which on Docker Desktop is a disk inside Desktop's VM, so devvm doesn't check free space for them.

`devvm doctor` also warns when that disk has less free space than building base needs, but the warning doesn't make doctor fail. When devvm refuses, see [Not enough free space](troubleshooting.md#not-enough-free-space) for how to make room.
