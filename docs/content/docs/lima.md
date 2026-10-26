---
title: Lima
weight: 31
---

[Lima](https://lima-vm.io) is devvm's default backend. Each project gets a full VM: on macOS it runs on Apple's Virtualization framework (`vz`), on Linux on QEMU.

## Requirements

- Lima 2.0 or newer (`limactl` on `PATH`). See [Lima's install guide](https://lima-vm.io/docs/installation/), or with Homebrew `brew install lima`.
- macOS 13 or newer, or Linux, on arm64 or x86_64.
- On Linux, QEMU: `qemu-system-aarch64` (or `qemu-system-x86_64`) and `qemu-img`, from your package manager.
- On Linux, `/dev/kvm`, which you can read and write: virtualization turned on in the firmware (or nested virtualization, in a VM), and your user in the `kvm` group.

Every command that needs Lima checks these first and stops at the first one missing. `devvm doctor` lists them all, with the command that installs each one where there's one to run: Homebrew's for Lima when you have Homebrew, your package manager's for QEMU (apt, dnf, pacman or zypper), `sudo usermod -aG kvm $USER` for `/dev/kvm` (then log in again). devvm never runs them itself ([details](install.md#checking-what-devvm-needs)).

## A Lima home of its own

Lima merges `_config/default.yaml` and `_config/override.yaml` of its home into every VM it creates or starts. A network set there for your own VMs would give a devvm VM a second network card with a route around its proxy, and an extra mount would go unnoticed. So devvm's VMs live in a Lima home of their own:

- It's `~/.local/share/devvm/lima` (under `$XDG_DATA_HOME` when that's set), whatever `LIMA_HOME` says. `DEVVM_LIMA_HOME` moves it, to a sandbox or a shorter path.
- devvm refuses to start a VM while that home holds either file, and refuses a VM that has a network devvm didn't give it.
- Plain `limactl list` doesn't show devvm's VMs: run `devvm limactl list` ([below](#running-limactl)).
- devvm runs `limactl` from its own data folder. Lima writes `lima.REJECTED.yaml` into the folder it runs in when a config is invalid, and a project folder is the VM's to write.

What devvm sets in each VM's Lima config:

- **Mounts:** the project folder (read-write), agents' folders (read-only), and the shared tools at `/mnt/devvm-tools` (read-only), each at its host path otherwise. Nothing else, not even the home folder Lima's templates mount.
- **ssh:** agent forwarding on or off; the host port Lima picks, or a fixed one for confined starts.
- **Ports:** which guest ports Lima forwards to the host's loopback (below).
- **Size:** CPUs, memory and disk.
- **Guest environment** (Lima's `env:`, written to `/etc/environment` at every boot): with network policy, the proxy's address and its settings; otherwise nothing. Lima doesn't pass the host's proxy settings in with network policy. `limactl edit` sets the proxy's URL with a stand-in for the password (`devvm-proxy-password`), which devvm then replaces in the VM's `lima.yaml`, rewritten mode 600: the password is in no process's arguments.

Lima only changes a stopped VM's config. A change made while the VM runs applies at its next start.

### Running limactl

`devvm limactl ARGS...` runs `limactl` with ARGS on devvm's Lima home, in devvm's place: its input, output, terminal and exit code are `limactl`'s, and its options are passed on, `--help` included (`devvm help limactl` shows devvm's own help).

```sh
devvm limactl list
devvm limactl shell --start devvm-img-<key>
```

- It runs from devvm's data folder, like devvm's own `limactl` commands: give files by absolute path.
- A VM started this way (`devvm limactl start`, `shell --start`) runs unconfined, without its network policy, and `devvm ls` shows it `unenforced` ([confinement](#confinement)). Start project VMs with devvm.

## Confinement

A VM with network policy reaches nothing but its proxy on the host. Even root in the VM can't get around it: direct connections, DNS, UDP and the host's own services fail at once, while forwarded ports, mounts and ssh keep working.

- **macOS:** devvm starts the VM's Lima process (its hostagent) under a macOS sandbox (`sandbox-exec`, a Seatbelt profile in `~/.local/share/devvm/run/<vm>.sb`). It lets the process connect only to the VM's ssh port and its proxy on the host's loopback, to the unix sockets in the VM's Lima folder (ssh's control socket, the VM's network), and to the ssh agent the VM forwards (yours, or devvm's own). Other sockets on the host, such as Docker's, are out of reach even if the Lima process were compromised. Name lookups are blocked: names resolve in the proxy. The guest reaches the proxy at `192.168.5.2`, the host as Lima's network sees it.
- **Linux:** Lima starts QEMU through a wrapper devvm writes (`~/.local/share/devvm/run/qemu/`), which runs devvm. It adds QEMU's `restrict=on` to Lima's network (no traffic in or out but Lima's own forwards), and one address, `192.168.5.4:3128`, that QEMU connects to the VM's proxy. If Lima's QEMU arguments aren't the shape devvm expects (one `-netdev user,id=net0,…`, no other network), the wrapper refuses to start QEMU rather than run it unrestricted. DNS points at an address nothing listens on, so lookups fail at once.

A process can't leave its sandbox, so devvm records the hostagent it started (`~/.local/share/devvm/run/<vm>.started`) and checks the VM still runs on it ([the self-check](network.md#the-self-check)). A VM started another way (`limactl start`) runs unconfined, and `devvm ls` shows it `unenforced`. The VM's proxy follows the hostagent's pid file (`ha.pid` in the VM's Lima folder) and exits with it.

Builds are confined too: they run code devvm didn't write. On macOS a build's Lima process runs under a profile that reaches the internet but not the host's loopback (other VMs' proxies, dev servers), or nothing at all. On Linux only a build with no network is confined: QEMU can't keep the internet and drop the host's loopback, so other builds reach services on the host's `localhost`. Builds forward no ports.

`sandbox-exec` is deprecated. If macOS drops it, devvm needs another way to confine the hostagent; until then, the self-check catches a VM that runs unconfined.

## Clones and overlays

An image is a stopped VM in devvm's Lima home, `devvm-img-<key>`. base's is made from Ubuntu's cloud image with `base.yaml` and `--mount-none` (with `--from`, from the converted disk, booting its kernel and initrd directly: [details](from.md#how-its-converted)), and a layer's is a clone of its parent's, made with `--mount-none` too, so a build mounts nothing from the host. A project VM is a clone of its image's VM, made with the image stopped.

- **macOS:** the clone's disk is an APFS copy-on-write copy: it shares the image's blocks until written, so `du` overcounts it.
- **Linux:** Lima's disks are qcow2 files, and a clone would be a full copy. A project VM's disk is a qcow2 overlay of its image's instead, readable only by you: the image's blocks are shared and the VM's disk holds only its changes. An image's disk is read-only once it's built, and an image whose disk VMs are overlays of can't be deleted.

## Ports

Lima forwards every port a VM listens on to the same port on the host's `localhost`, unless told otherwise. devvm gives it a rule per port or range a VM forwards, then rules ignoring everything else. A VM forwarding all ports gets Lima's own behavior.

Lima's forwards listen with `SO_REUSEADDR`, so on macOS one takes `127.0.0.1:PORT` even while a program on the host has `*:PORT`, and that program's `localhost` connections with it. So each start checks a VM's listed ports first, listening on each without `SO_REUSEADDR`, and leaves out those in use ([ports](images.md#ports)): the rules for that start are the list less those ports. The rules change only where they differ, as any change gives the VM new ssh host keys at its next boot, which devvm pins again: a start that leaves a port out, and the next one that forwards it again. A VM forwarding all ports isn't checked: Lima makes its forwards as servers in it start, whatever the host has.

## Limits

- **Folder names in VM names are cut short.** Lima keeps each VM's ssh socket in the VM's folder in its home, and a socket path has at most 104 characters. A long Lima home leaves less room for the name: set `DEVVM_LIMA_HOME` to a shorter folder if devvm says it's too long.
- **Paths can't contain commas.** Lima and QEMU split mount options on commas, so devvm refuses to mount a project folder with one, and leaves out an agent's folder with one, with a warning.
- **Disks can grow but not shrink.**
- **Mounts on Linux are 9p.** A 9p share mounts once per guest.
- **A start Lima reports as failed** for a VM that's running but degraded (one of Lima's boot scripts failed) carries on with a warning. A VM that doesn't answer over ssh within a minute is stopped instead: its console log is in its Lima folder (`serial*.log`).
