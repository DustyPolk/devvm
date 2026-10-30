---
title: Ports and size
weight: 4
---

This page covers the ports a VM forwards to this machine and the CPUs, memory and disk it gets.

## Size and ports

A new VM gets its image's size unless you set one. For `base.yaml` that's 4 CPUs, 8 GiB of memory and a 100 GiB disk.

- `--cpus` takes a whole number. `--memory` and `--disk` take a size in GiB, written `16`, `16G` or `16GiB`, and decimals are allowed.
- What the flags leave unset comes from `~/.config/devvm/size` when that file sets it, with one `cpus=`, `memory=` or `disk=` line each. `devvm size -g` writes that file.
- devvm refuses more CPUs or memory than this machine has, and a disk smaller than the image's, because disks can't shrink. It refuses them before it builds anything.

devvm forwards the TCP ports the VM listens on to the same port on this machine's `localhost`. It forwards every one from 1024 up unless `--ports` says otherwise ([how](#ports)).

- `--ports` takes `all`, `none`, or ports and ranges like `5173,8000-8010`.
- Without `--ports`, the default comes from `~/.config/devvm/ports`, which holds one line: `all`, `none` or a list. `devvm ports -g` writes that file.
- `devvm ports` changes an existing VM's ports ([ports](#ports)), and `devvm size` changes its size ([size](#size)).

devvm refuses a size or ports file that doesn't hold a valid value, as it refuses a bad flag.

## Ports

A VM forwards every TCP port from 1024 up that it listens on to the same port on this machine's `localhost`, so a server on `:5173` in the VM is at `localhost:5173` here. To forward only some ports:

```sh
devvm ports                 # what this folder's VM forwards: all, none or a list
devvm ports 5173,8000-8010  # only these ports and ranges
devvm ports add 5432        # one more
devvm ports rm 5173         # one fewer, as listed; the last leaves none
devvm ports none            # nothing; devvm ports all forwards every port again
devvm ports -g none         # what new VMs forward (default: all)
```

- A new VM gets `--ports` (`devvm --ports 5173`). Without it, the VM gets what `devvm ports -g` set in `~/.config/devvm/ports`, a single line of `all`, `none` or a list, and without that file, it forwards every port.
- A change applies at once. A stopped VM forwards the new ports from its next start. On a running VM, devvm starts its forwarder again with the new ports while the VM runs on, so a server on an added port reaches this machine within a few seconds. Connections open through the old forwarder close, so a client that was connected through it has to connect again.
- `add` and `rm` take ports and ranges, not `all` or `none`. devvm refuses `rm` on a VM that forwards every port, because there's no list to remove from. `-g` also works with `add` and `rm`.
- Ports are forwarded with or without [network policy](../safety/network.md). Builds forward no ports.

devvm's forwarder forwards the ports by the same rules on every backend.

- **What it forwards.** When a VM forwards every port, the forwarder forwards each TCP port from 1024 up that a server in the VM listens on, on the VM's loopback or on all its addresses. When a VM lists its ports, it forwards those ports alone, however many there are, ports below 1024 included. Each port goes to the same port on this machine's `127.0.0.1` and `::1` and nowhere else, within a few seconds of the server starting, and the forward goes away when the server stops. At most 256 ports are forwarded at once.
- **What it never takes.** The forwarder never takes a port that a program on this machine has on any address. That program keeps the port and its `localhost` connections, and the forward waits until the VM's server stops and starts again. The forwarder also never takes a port that devvm records for a VM, even while that port is free. Those are the ssh, proxy and decider ports devvm chooses for VMs, the VM's own ssh port, and other VMs' listed ports. A Lima VM without network policy has an ssh port that Lima picks and devvm doesn't record, but Lima holds that port, so the forwarder can't take it.
- **A listed port that a program on this machine has** isn't forwarded when a start, or a change of a running VM's ports, finds it in use, and devvm warns about it (`ports_taken`). The VM starts or runs on all the same, and the next start or change that finds the port free forwards it again.
- **A program that starts on a port after it's forwarded** can still bind it on all addresses, as most servers do, and then that program's `localhost` connections reach the VM.
- **How it works.** The forwarder reaches the VM over the VM's ssh, with a key and a host key that devvm pins, and it asks for no forwarding into this machine. Every connection starts on this machine, and the VM opens none to it. The forwarder runs as a hidden `devvm _lima-ports`, `_smolvm-ports` or `_docker-ports` process, one per VM, which devvm starts after each start of the VM and again when you change a running VM's ports, and stops with `devvm stop`. Its log is `<vm>.ports.log` beside the backend's records, which on Lima are in `~/.local/share/devvm/lima-vms`. When a forwarder doesn't start, devvm warns with its log (`ports_forward_failed`), and the VM runs on.

**UDP isn't forwarded, on any backend.** Run the UDP client in the VM instead, or tunnel it over a forwarded TCP port with [socat](http://www.dest-unreach.org/socat/doc/socat.html), installed on both sides. For a UDP server on port 8125 in the VM, with TCP port 9125 forwarded:

```sh
socat TCP-LISTEN:9125,fork,reuseaddr UDP:127.0.0.1:8125   # in the VM
socat UDP-LISTEN:8125,fork TCP:127.0.0.1:9125             # on this machine
```

TCP is a stream, so datagrams can arrive merged or split. This suits low-rate request and response traffic, such as DNS, statsd or syslog, but not high-rate or latency-sensitive traffic, such as games, QUIC or WebRTC media.

## Size

A VM gets its image's size, which is `base.yaml`'s 4 CPUs, 8 GiB of memory and a 100 GiB disk, unless you give it another:

```sh
devvm size                         # what this folder's VM has: cpus=4 memory=8GiB disk=100GiB
devvm size --cpus 6 --memory 16    # change it; what isn't given stays
devvm size --disk 200              # a bigger disk; never a smaller one
devvm size -g --memory 16          # the size new VMs get (default: the image's)
devvm size -g                      # cpus=, memory= and disk= lines, empty for the image's
```

- A new VM gets `--cpus`, `--memory` and `--disk` (`devvm --cpus 6 base`). Without them, it gets what `devvm size -g` set in `~/.config/devvm/size`, and without that, the image's size ([details](#size-and-ports)).
- `--cpus` takes a whole number, and `--memory` and `--disk` take a size in GiB, written `16`, `16G` or `16GiB`, with decimals allowed. devvm refuses more CPUs or memory than this machine has.
- A stopped VM changes at once. A running VM keeps its size until it next starts (`devvm restart`), and until then `devvm size` shows what the VM has now and says what changes.
- Disks grow but never shrink, so devvm refuses a disk smaller than the VM's.
- On [Docker](../internals/docker.md#what-devvm-sets-on-a-vm), a VM's disk has no size of its own. devvm refuses `--disk`, a default disk size doesn't apply, and `devvm size` shows how much the VM holds instead (`cpus=4 memory=8GiB disk_used=1.2GiB`), with a warning past 100 GiB.

### Disk use

A VM's disk size is how far the disk can grow, not how much space it takes. A project VM starts as a copy of its image's disk that shares the image's blocks, so a new VM takes almost nothing until it writes ([clones and overlays](../internals/lima.md#clones-and-overlays)):

- **macOS:** the copy is an APFS clone. `du` counts the shared blocks as the clone's own, so it overcounts.
- **Linux:** the copy is a qcow2 overlay of the image's disk, which holds only the VM's changes, about 12 MB for a new VM instead of 2 GB.
