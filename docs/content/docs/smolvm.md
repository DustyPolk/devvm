---
title: smolvm
weight: 32
---

[smolvm](https://github.com/smol-machines/smolvm) is an experimental backend: each project gets a microVM that runs its image as a container, with no init system. Choose it per VM with `--backend smolvm`, or for every new VM with `devvm setup --backend smolvm`.

It lacks ssh for editors (`devvm ssh`, `devvm code`, `--clone`), forwarding every port, and turning SSH agent forwarding on or off once a VM is made. [Network policy](#network-policy) is emulated, and reviewed. devvm refuses each of those it lacks in one line naming the VM ([features](backends.md#features)); they come later, as reviewed emulations.

## Requirements

- smolvm 1.32 or newer (`smolvm` on `PATH`). With Homebrew: `brew install smol-machines/tap/smolvm`.
- An arm64 Mac, or Linux on arm64 or x86_64 with `/dev/kvm` you can read and write. Only the Mac is tested.

`devvm doctor --backend smolvm` checks them, with the command that installs smolvm when you have Homebrew.

## Its machines

On macOS, smolvm keeps every machine in your own smolvm state: devvm can't give it a folder of its own (`--state-dir` is Linux only). So:

- Every machine devvm makes carries the label `devvm.data=<devvm's data folder>`, and devvm lists, starts, stops, changes and deletes only machines with its label. A machine of yours by a name devvm wants is refused (`smolvm_machine_foreign`), never touched.
- `smolvm machine ls` shows devvm's machines next to yours.
- devvm runs smolvm with its usage journal off (`SMOLVM_USAGE=0`), and never lets `SMOLVM_MACHINE_NAME` choose a machine.

On Linux, smolvm's state is devvm's own too, in `~/.local/share/devvm/smolvm/state`.

Images aren't machines: each is a pack (`smolvm pack create --from-vm`) in `~/.local/share/devvm/smolvm/images/<key>`, about 450 MB for `base`. A project VM is a machine made from its image's pack, so an image a VM was made from isn't deleted (`image_in_use`): the machine reads the pack when its disk grows. smolvm keeps caches of its own (`~/Library/Caches/smolvm` on macOS); `smolvm machine prune` and `smolvm pack prune` clean them.

## Building images

- `base` starts from Ubuntu's container image (`ubuntu:26.04`), by the digest its registry has at the time, which its record keeps. With `--from`, from that image by its recorded digest; an image only Docker has is refused (`smolvm_from_registry_only`).
- Before `layers/base.sh`, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash`, and the VM's user. It's your user name and uid, as on Lima, with its home at `/home/<you>.guest`: the project folder shows as your uid's, and only that user can write it.
- The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run: base's services are enabled the Lima way, and devvm starts them itself (below). The stand-ins are gone before the image is packed.
- Builds use smolvm's default network: the internet, not the host's loopback. A pack keeps its build machine's network, CPUs and memory, so devvm sets all three on every VM it makes.
- Base's CPUs, memory and disk are `base.yaml`'s: 4 CPUs, 8 GiB (half the host's memory on a small host), 100 GiB.

devvm's own scripts for this (`prepare.sh`, `shims.sh`) are in `base`'s key, with the backend: smolvm's images are never Lima's.

## What devvm sets on a VM

- **Mounts:** the project folder (read-write) and agents' folders (read-only) at their host paths, the shared tools at `/mnt/devvm-tools` (read-only), and `/var/lib/docker` on the machine's own disk, which Docker's storage needs. A path with a colon is refused: smolvm splits mounts on colons. Files a VM makes in the project folder are yours on the host, mode 0600.
- **Network:** smolvm's own, without network policy: the internet, not the host's loopback. With it, [below](#network-policy).
- **Size:** what you ask for, or base's; the disk only grows.
- **Ports:** those you list (`--ports 5173,8000-8010`), each to the same port on the host's `127.0.0.1` and `::1`, at most 64 ports. Only servers listening on all addresses (`0.0.0.0`) are reached. `all`, the default, is refused: pass `--ports` or set `devvm ports -g`.
- **SSH agent forwarding:** chosen when the VM is made (`--ssh-agent`), and fixed then. At each start smolvm forwards the agent `SSH_AUTH_SOCK` names (devvm's own with `devvm ssh-agent devvm`); with none, the start stops (`smolvm_no_agent`).

## Network policy

Emulated, and reviewed. A VM with [network policy](network.md) reaches nothing but its proxy, as on Lima, from three parts devvm sets while it's stopped ([design](design.md#network-policy-on-smolvm)):

- **An allow list of the host alone** (`100.96.0.1/32`, the gateway): smolvm refuses connections anywhere else, and UDP but DNS.
- **DNS closed:** smolvm answers every query `NXDOMAIN` itself, without asking anyone; names resolve in the proxy. One name, `devvm.invalid`, which smolvm needs to filter on, is still asked of your resolver: a narrow way out, as the VM chooses when and how to ask ([details](design.md#network-policy-on-smolvm)).
- **devvm's proxy deciding each connection** (smolvm's mediated egress): before smolvm connects anywhere, it asks the proxy, which takes over connections to itself and refuses the rest: other ports of the host's, which `devvm log` shows as `BLOCKED`, a direct connection. The proxy's address in the VM is `http://100.96.0.1:<port>`.

It fails closed: with the proxy gone, nothing gets out, and once a VM started so, `smolvm machine start` refuses it without a decider. A VM someone started with a decider of their own, or whose allow list someone widened while it runs, isn't confined: devvm shows it `unenforced` and stops it ([the self-check](network.md#the-self-check)). A UDP allow someone adds (`smolvm machine update --allow-udp`) lets UDP past the decider, and smolvm doesn't show it: don't add one to devvm's machines. `devvm network off` takes it all away at the next start.

## No init system

A machine runs its image as a container, and nothing starts at boot. After each start devvm runs, as root:

- the loop devices the shared tools are mounted with (`/dev` has none), then the tools' mounts;
- Docker's daemon, in the background, logging to `/var/log/dockerd.log`.

The shared tools are their own images on smolvm: uncompressed EROFS, as its kernel mounts no other. A tool image's key includes the format, so Lima's and smolvm's VMs each get theirs from the one share.

devvm's commands in a VM (its setup, your shell) run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`, which smolvm doesn't read.

## Looking inside

A failed build keeps its machine, stopped (`smolvm_script_failed`): `smolvm machine start --name devvm-img-<key>`, then `smolvm machine shell --name devvm-img-<key>` (as root). The next build, or `devvm gc`, deletes it.
