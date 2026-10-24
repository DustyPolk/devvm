---
title: Lima
weight: 31
---

[Lima](https://lima-vm.io) is devvm's default backend. Each project gets a full VM: on macOS it runs on Apple's Virtualization framework (`vz`), on Linux on QEMU.

## Requirements

- Lima 2.0 or newer (`limactl` on `PATH`). See [Lima's install guide](https://lima-vm.io/docs/installation/).
- On Linux, QEMU: `qemu-system-aarch64` (or `qemu-system-x86_64`) and `qemu-img`, from your package manager.
- macOS or Linux, on arm64 or x86_64.

## A Lima home of its own

Lima merges `_config/default.yaml` and `_config/override.yaml` of its home into every VM it creates or starts. A network set there for your own VMs would give a devvm VM a second network card with a route around its proxy, and an extra mount would go unnoticed. So devvm's VMs live in a Lima home of their own:

- It's `~/.local/share/devvm/lima` (under `$XDG_DATA_HOME` when that's set), whatever `LIMA_HOME` says. `DEVVM_LIMA_HOME` moves it, to a sandbox or a shorter path.
- devvm refuses to start a VM while that home holds either file, and refuses a VM that has a network devvm didn't give it.
- Plain `limactl list` doesn't show devvm's VMs: run Lima with `LIMA_HOME` set to devvm's home.
- devvm runs `limactl` from its own data folder. Lima writes `lima.REJECTED.yaml` into the folder it runs in when a config is invalid, and a project folder is the VM's to write.

What devvm sets in each VM's Lima config:

- **Mounts:** the project folder (read-write), agents' folders (read-only), and the shared tools at `/mnt/devvm-tools` (read-only), each at its host path otherwise. Nothing else, not even the home folder Lima's templates mount.
- **ssh:** agent forwarding on or off; the host port Lima picks, or a fixed one for confined starts.
- **Ports:** which guest ports Lima forwards to the host's loopback (below).
- **Size:** CPUs, memory and disk.
- **Guest environment** (Lima's `env:`, written to `/etc/environment` at every boot): with network policy, the proxy's address and its settings; otherwise nothing. Lima doesn't pass the host's proxy settings in with network policy.

Lima only changes a stopped VM's config. A change made while the VM runs applies at its next start.

## Confinement

A VM with network policy reaches nothing but its proxy on the host. Even root in the VM can't get around it: direct connections, DNS, UDP and the host's own services fail at once, while forwarded ports, mounts and ssh keep working.

- **macOS:** devvm starts the VM's Lima process (its hostagent) under a macOS sandbox (`sandbox-exec`, a Seatbelt profile in `~/.local/share/devvm/run/<vm>.sb`). It lets the process connect only to the VM's ssh port and its proxy on the host's loopback, to the unix sockets in the VM's Lima folder (ssh's control socket, the VM's network), and to the ssh agent the VM forwards (yours, or devvm's own). Other sockets on the host, such as Docker's, are out of reach even if the Lima process were compromised. Name lookups are blocked: names resolve in the proxy. The guest reaches the proxy at `192.168.5.2`, the host as Lima's network sees it.
- **Linux:** Lima starts QEMU through a wrapper devvm writes (`~/.local/share/devvm/run/qemu/`), which runs devvm. It adds QEMU's `restrict=on` to Lima's network (no traffic in or out but Lima's own forwards), and one address, `192.168.5.4:3128`, that QEMU connects to the VM's proxy. If Lima's QEMU arguments aren't the shape devvm expects (one `-netdev user,id=net0,…`, no other network), the wrapper refuses to start QEMU rather than run it unrestricted. DNS points at an address nothing listens on, so lookups fail at once.

A process can't leave its sandbox, so devvm records the hostagent it started (`~/.local/share/devvm/run/<vm>.started`) and checks the VM still runs on it ([the self-check](network.md#the-self-check)). A VM started another way (`limactl start`) runs unconfined. The VM's proxy follows the hostagent's pid file (`ha.pid` in the VM's Lima folder) and exits with it.

Builds are confined too: they run code devvm didn't write. On macOS a build's Lima process runs under a profile that reaches the internet but not the host's loopback (other VMs' proxies, dev servers), or nothing at all. On Linux only a build with no network is confined: QEMU can't keep the internet and drop the host's loopback, so other builds reach services on the host's `localhost`. Builds forward no ports.

`sandbox-exec` is deprecated. If macOS drops it, devvm needs another way to confine the hostagent; until then, the self-check catches a VM that runs unconfined.

## Clones and overlays

An image is a stopped VM in devvm's Lima home, `devvm-img-<key>`. base's is made from Ubuntu's cloud image with `base.yaml` and `--mount-none`, and a layer's is a clone of its parent's, made with `--mount-none` too, so a build mounts nothing from the host. A project VM is a clone of its image's VM, made with the image stopped.

- **macOS:** the clone's disk is an APFS copy-on-write copy: it shares the image's blocks until written, so `du` overcounts it.
- **Linux:** Lima's disks are qcow2 files, and a clone would be a full copy. A project VM's disk is a qcow2 overlay of its image's instead, readable only by you: the image's blocks are shared and the VM's disk holds only its changes. An image's disk is read-only once it's built, and an image whose disk VMs are overlays of can't be deleted.

## Ports

Lima forwards every port a VM listens on to the same port on the host's `localhost`, unless told otherwise. devvm gives it a rule per port or range a VM forwards, then rules ignoring everything else. A VM forwarding all ports gets Lima's own behavior.

## Limits

- **Folder names in VM names are cut short.** Lima keeps each VM's ssh socket in the VM's folder in its home, and a socket path has at most 104 characters. A long Lima home leaves less room for the name: set `DEVVM_LIMA_HOME` to a shorter folder if devvm says it's too long.
- **Paths can't contain commas.** Lima and QEMU split mount options on commas, so devvm refuses to mount a folder with one.
- **Disks can grow but not shrink.**
- **Mounts on Linux are 9p.** A 9p share mounts once per guest.
- **A start Lima reports as failed** for a VM that's running but degraded (one of Lima's boot scripts failed) carries on with a warning. A VM that doesn't answer over ssh within a minute is stopped instead: its console log is in its Lima folder (`serial*.log`).
