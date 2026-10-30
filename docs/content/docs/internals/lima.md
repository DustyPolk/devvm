---
title: Lima
weight: 9
---

This page explains how devvm sets up Lima for its VMs, how it confines them, and how it forwards their ports.

## A Lima home of its own

Lima merges `_config/default.yaml` and `_config/override.yaml` from its home into every VM it creates or starts. A network you set there for your own VMs would give a devvm VM a second network card with a route around its proxy, and an extra mount set there would go unnoticed. So devvm keeps its VMs in a Lima home of their own:

- The home is `~/.local/share/devvm/lima`, under `$XDG_DATA_HOME` when that's set, whatever `LIMA_HOME` says. Set `DEVVM_LIMA_HOME` to move it to a sandbox or a shorter path.
- devvm refuses to start a VM while that home holds either file, and it refuses a VM that has a network devvm didn't give it.
- [devvm's Lima home](../backends/lima.md#devvms-lima-home) says what that means for you.
- devvm runs `limactl` from its own data folder. When a config is invalid, Lima writes `lima.REJECTED.yaml` into the folder it runs in, and a project folder belongs to the VM to write.

devvm sets these in each VM's Lima config:

- **Mounts:** the project folder (read-write), agents' folders (read-only), and the shared tools at `/mnt/devvm-tools` (read-only). Each folder but the tools is mounted at its host path. Nothing else is mounted, not even the home folder that Lima's templates mount.
- **ssh:** agent forwarding on or off, and the host port. devvm chooses the port when it makes the VM and records it, so no other VM's forwarded ports take it. The port changes only when another program on the host has it, on any address, while the VM is stopped, or when the VM lists it to forward.
- **Ports:** none. Lima's own forwards are off, and devvm's forwarder forwards the VM's ports instead ([below](#ports)).
- **Size:** CPUs, memory and disk.
- **Guest environment:** Lima's `env:`, which Lima writes to `/etc/environment` at every boot. With network policy it holds the proxy's address and its settings, and otherwise it's empty. With network policy, Lima doesn't pass the host's proxy settings in. `limactl edit` sets the proxy's URL with a stand-in for the password (`devvm-proxy-password`), and devvm then replaces the stand-in in the VM's `lima.yaml`, which it rewrites with mode 600, so the password is in no process's arguments.

Lima changes only a stopped VM's config, so a change you make while the VM runs applies at its next start.

## Confinement

A VM with network policy reaches nothing but its proxy on the host, and even root in the VM can't get around that. Direct connections, DNS, UDP and connections to the host's own services fail at once, while forwarded ports, mounts and ssh keep working.

- **macOS:** devvm starts the VM's Lima process, its hostagent, under a macOS sandbox. That's `sandbox-exec` with a Seatbelt profile in `~/.local/share/devvm/run/<vm>.sb`. The profile lets the process connect to the VM's ssh port and its proxy on the host's loopback, to the unix sockets in the VM's Lima folder (ssh's control socket and the VM's network), and to the ssh agent the VM forwards, which is yours or devvm's own. It connects nowhere else, so other sockets on the host, such as Docker's, stay out of reach even if the Lima process were compromised. The profile also lets the process listen only on the VM's ssh port, over TCP, and on unix sockets in the VM's Lima folder, so it can't open a port of the VM's on the host even if Lima ignored its rules ([ports](#ports)). Names resolve in the proxy, so Lima's own DNS server for the VM is off, the guest's DNS points at an address that nothing listens on, and lookups fail at once. The guest reaches the proxy at `192.168.5.2`, which is the host as Lima's network sees it.
- **Linux:** Lima starts QEMU through a wrapper that devvm writes (`~/.local/share/devvm/run/qemu/`) and that runs devvm. The wrapper adds QEMU's `restrict=on` to Lima's network, so no traffic goes in or out but Lima's own forwards. It also adds one address, `192.168.5.4:3128`, which QEMU connects to the VM's proxy. If Lima's QEMU arguments aren't the shape devvm expects, with one `-netdev user,id=net0,…` and no other network, the wrapper refuses to start QEMU, so QEMU never runs unrestricted. DNS points at an address that nothing listens on, so lookups fail at once.

A process can't leave its sandbox, so devvm records the hostagent it started (`~/.local/share/devvm/run/<vm>.started`) and checks that the VM still runs on it ([the self-check](network.md#the-self-check)). A VM started another way, such as with `limactl start`, runs unconfined, and `devvm ls` shows it as `unenforced`. The VM's proxy follows the hostagent's pid file (`ha.pid` in the VM's Lima folder) and exits with it.

Builds are also confined, because they run code devvm didn't write. On macOS a build's Lima process runs under a profile that reaches either the internet but not the host's loopback, where other VMs' proxies and dev servers listen, or nothing at all. On Linux only a build with no network is confined. QEMU can't keep the internet while dropping the host's loopback, so other builds reach services on the host's `localhost`. Builds forward no ports.

`sandbox-exec` is deprecated. If macOS drops it, devvm needs another way to confine the hostagent, and until then the self-check catches a VM that runs unconfined.

## Clones and overlays

An image is a stopped VM in devvm's Lima home, named `devvm-img-<key>`. devvm makes base's from Ubuntu's cloud image with `base.yaml` and `--mount-none`. With `--from`, it makes base's from the converted disk instead, booting its kernel and initrd directly ([details](images.md#how-its-converted)). A layer's image is a clone of its parent's, also made with `--mount-none`, so a build mounts nothing from the host. A project VM is a clone of its image's VM, made while the image is stopped.

- **macOS:** the clone's disk is an APFS copy-on-write copy. It shares the image's blocks until the VM writes them, so `du` overcounts it.
- **Linux:** Lima's disks are qcow2 files, and a clone would be a full copy. So a project VM's disk is a qcow2 overlay of its image's, readable only by you, which shares the image's blocks and holds only the VM's changes. An image's disk is read-only once it's built, and devvm can't delete an image while VMs' disks are overlays of it.

## Ports

devvm's forwarder forwards a VM's ports, every one or only its listed ones, by the same rules as on every backend ([ports](../using/ports-size.md#ports)). This is an emulation, and it's reviewed ([design](design.md#ports-on-lima)). Lima's own forwards would listen with `SO_REUSEADDR`, and on macOS such a forward takes `127.0.0.1:PORT` while a program on the host has `*:PORT`, along with that program's `localhost` connections.

- **Lima forwards nothing:** each VM's `portForwards` are rules that ignore every guest port, TCP and UDP, on `127.0.0.1` (which also covers `::1`), `0.0.0.0` and `::`. Lima's own ssh is its VM driver's forward, not a rule, so it keeps working. devvm sets the rules when it makes the VM. A start sets them again only where they differ, because any change gives the VM new ssh host keys at its next boot. On macOS, a VM with network policy doesn't rely on the rules alone, because its sandbox refuses the Lima process every port but its ssh port ([confinement](#confinement)).
- **Which ports a VM forwards** is a record of devvm's in `~/.local/share/devvm/lima-vms/<vm>.ports`, which holds `all`, `none` or a list. `devvm ports` changes the record and never the VM's Lima config.
- **The forwarder** is `devvm _lima-ports`, which devvm starts after each start of a project VM, and again with the new ports when you change a running VM's ports. It connects to Lima's ssh port as Lima's user, with Lima's key (`_config/user` in devvm's Lima home) and the host key devvm pinned for the VM's ssh config, and it asks for no forwarding into the host. It ends when the VM's hostagent does (`ha.pid`), `devvm stop` stops it, and `devvm rm` deletes its files with the record. Its log is `~/.local/share/devvm/lima-vms/<vm>.ports.log`.
