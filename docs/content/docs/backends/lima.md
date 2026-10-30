---
title: Lima
weight: 1
---

[Lima](https://lima-vm.io) is devvm's default backend. Each project gets a full VM, which runs on Apple's Virtualization framework (`vz`) on macOS and on QEMU on Linux.

## Requirements

- You need Lima 2.0 or newer, with `limactl` on your `PATH`. See [Lima's install guide](https://lima-vm.io/docs/installation/), or install it with Homebrew: `brew install lima`.
- The host runs macOS 13 or newer, or Linux, on arm64 or x86_64.
- On Linux, you need QEMU from your package manager: `qemu-system-aarch64` (or `qemu-system-x86_64`) and `qemu-img`.
- On Linux, you also need to be able to read and write `/dev/kvm`. That means virtualization is turned on in the firmware (or nested virtualization, in a VM), and your user is in the `kvm` group.

Every command that needs Lima checks these first and stops at the first one that's missing. `devvm doctor` lists them all, each with the command that installs it where there's one to run. That's Homebrew's command for Lima when you have Homebrew, your package manager's for QEMU (apt, dnf, pacman or zypper), and `sudo usermod -aG kvm $USER` for `/dev/kvm`, after which you log in again. devvm never runs them itself ([details](../getting-started/install.md#checking-what-devvm-needs)).

## A Lima home of its own

Lima merges `_config/default.yaml` and `_config/override.yaml` from its home into every VM it creates or starts. A network you set there for your own VMs would give a devvm VM a second network card with a route around its proxy, and an extra mount set there would go unnoticed. So devvm keeps its VMs in a Lima home of their own:

- The home is `~/.local/share/devvm/lima`, under `$XDG_DATA_HOME` when that's set, whatever `LIMA_HOME` says. Set `DEVVM_LIMA_HOME` to move it to a sandbox or a shorter path.
- devvm refuses to start a VM while that home holds either file, and it refuses a VM that has a network devvm didn't give it.
- Plain `limactl list` doesn't show devvm's VMs, so run `devvm limactl list` instead ([below](#running-limactl)).
- devvm runs `limactl` from its own data folder. When a config is invalid, Lima writes `lima.REJECTED.yaml` into the folder it runs in, and a project folder belongs to the VM to write.

devvm sets these in each VM's Lima config:

- **Mounts:** the project folder (read-write), agents' folders (read-only), and the shared tools at `/mnt/devvm-tools` (read-only). Each folder but the tools is mounted at its host path. Nothing else is mounted, not even the home folder that Lima's templates mount.
- **ssh:** agent forwarding on or off, and the host port. Lima picks the port, but confined starts get a fixed one.
- **Ports:** none. Lima's own forwards are off, and devvm's forwarder forwards the VM's ports instead ([below](#ports)).
- **Size:** CPUs, memory and disk.
- **Guest environment:** Lima's `env:`, which Lima writes to `/etc/environment` at every boot. With network policy it holds the proxy's address and its settings, and otherwise it's empty. With network policy, Lima doesn't pass the host's proxy settings in. `limactl edit` sets the proxy's URL with a stand-in for the password (`devvm-proxy-password`), and devvm then replaces the stand-in in the VM's `lima.yaml`, which it rewrites with mode 600, so the password is in no process's arguments.

Lima changes only a stopped VM's config, so a change you make while the VM runs applies at its next start.

### Running limactl

`devvm limactl ARGS...` runs `limactl` with ARGS on devvm's Lima home, in devvm's place. Its input, output, terminal and exit code are `limactl`'s, and devvm passes every option on to it, `--help` included. `devvm help limactl` shows devvm's own help for the command.

```sh
devvm limactl list
devvm limactl shell --start devvm-img-<key>
```

- It runs from devvm's data folder, as devvm's own `limactl` commands do, so give files by their absolute path.
- A VM you start this way, with `devvm limactl start` or `shell --start`, runs unconfined, without its network policy, and `devvm ls` shows it as `unenforced` ([confinement](#confinement)). Start project VMs with devvm.

## Confinement

A VM with network policy reaches nothing but its proxy on the host, and even root in the VM can't get around that. Direct connections, DNS, UDP and connections to the host's own services fail at once, while forwarded ports, mounts and ssh keep working.

- **macOS:** devvm starts the VM's Lima process, its hostagent, under a macOS sandbox. That's `sandbox-exec` with a Seatbelt profile in `~/.local/share/devvm/run/<vm>.sb`. The profile lets the process connect to the VM's ssh port and its proxy on the host's loopback, to the unix sockets in the VM's Lima folder (ssh's control socket and the VM's network), and to the ssh agent the VM forwards, which is yours or devvm's own. It connects nowhere else, so other sockets on the host, such as Docker's, stay out of reach even if the Lima process were compromised. Name lookups are blocked, because names resolve in the proxy. The guest reaches the proxy at `192.168.5.2`, which is the host as Lima's network sees it.
- **Linux:** Lima starts QEMU through a wrapper that devvm writes (`~/.local/share/devvm/run/qemu/`) and that runs devvm. The wrapper adds QEMU's `restrict=on` to Lima's network, so no traffic goes in or out but Lima's own forwards. It also adds one address, `192.168.5.4:3128`, which QEMU connects to the VM's proxy. If Lima's QEMU arguments aren't the shape devvm expects, with one `-netdev user,id=net0,…` and no other network, the wrapper refuses to start QEMU, so QEMU never runs unrestricted. DNS points at an address that nothing listens on, so lookups fail at once.

A process can't leave its sandbox, so devvm records the hostagent it started (`~/.local/share/devvm/run/<vm>.started`) and checks that the VM still runs on it ([the self-check](../safety/network.md#the-self-check)). A VM started another way, such as with `limactl start`, runs unconfined, and `devvm ls` shows it as `unenforced`. The VM's proxy follows the hostagent's pid file (`ha.pid` in the VM's Lima folder) and exits with it.

Builds are also confined, because they run code devvm didn't write. On macOS a build's Lima process runs under a profile that reaches either the internet but not the host's loopback, where other VMs' proxies and dev servers listen, or nothing at all. On Linux only a build with no network is confined. QEMU can't keep the internet while dropping the host's loopback, so other builds reach services on the host's `localhost`. Builds forward no ports.

`sandbox-exec` is deprecated. If macOS drops it, devvm needs another way to confine the hostagent, and until then the self-check catches a VM that runs unconfined.

## Clones and overlays

An image is a stopped VM in devvm's Lima home, named `devvm-img-<key>`. devvm makes base's from Ubuntu's cloud image with `base.yaml` and `--mount-none`. With `--from`, it makes base's from the converted disk instead, booting its kernel and initrd directly ([details](../using/from.md#how-its-converted)). A layer's image is a clone of its parent's, also made with `--mount-none`, so a build mounts nothing from the host. A project VM is a clone of its image's VM, made while the image is stopped.

- **macOS:** the clone's disk is an APFS copy-on-write copy. It shares the image's blocks until the VM writes them, so `du` overcounts it.
- **Linux:** Lima's disks are qcow2 files, and a clone would be a full copy. So a project VM's disk is a qcow2 overlay of its image's, readable only by you, which shares the image's blocks and holds only the VM's changes. An image's disk is read-only once it's built, and devvm can't delete an image while VMs' disks are overlays of it.

## Ports

devvm's forwarder forwards a VM's ports, every one or only its listed ones, by the same rules as on every backend ([ports](../using/images.md#ports)). This is an emulation, and it's reviewed ([design](../internals/design.md#ports-on-lima)). Lima's own forwards would listen with `SO_REUSEADDR`, and on macOS such a forward takes `127.0.0.1:PORT` while a program on the host has `*:PORT`, along with that program's `localhost` connections.

- **Lima forwards nothing:** each VM's `portForwards` are rules that ignore every guest port, TCP and UDP, on `127.0.0.1` (which also covers `::1`), `0.0.0.0` and `::`. Lima's own ssh is its VM driver's forward, not a rule, so it keeps working. devvm sets the rules when it makes the VM. A start sets them again only where they differ, because any change gives the VM new ssh host keys at its next boot.
- **Which ports a VM forwards** is a record of devvm's in `~/.local/share/devvm/lima-vms/<vm>.ports`, which holds `all`, `none` or a list. `devvm ports` changes the record and never the VM's Lima config.
- **The forwarder** is `devvm _lima-ports`, which devvm starts after each start of a project VM, and again with the new ports when you change a running VM's ports. It connects to Lima's ssh port as Lima's user, with Lima's key (`_config/user` in devvm's Lima home) and the host key devvm pinned for the VM's ssh config, and it asks for no forwarding into the host. It ends when the VM's hostagent does (`ha.pid`), `devvm stop` stops it, and `devvm rm` deletes its files with the record. Its log is `~/.local/share/devvm/lima-vms/<vm>.ports.log`.

## Limits

- **Folder names in VM names are cut short.** Lima keeps each VM's ssh socket in the VM's folder in its home, and a socket path has at most 104 characters. A long Lima home leaves less room for the name, so set `DEVVM_LIMA_HOME` to a shorter folder if devvm says the home is too long.
- **Paths can't contain commas.** Lima and QEMU split mount options on commas, so devvm refuses to mount a project folder whose path has one, and it leaves out an agent's folder whose path has one, with a warning.
- **Disks can grow but not shrink.**
- **Mounts on Linux are 9p.** A 9p share mounts once per guest.
- **A start Lima reports as failed** carries on with a warning when the VM is running but degraded, because one of Lima's boot scripts failed. A VM that doesn't answer over ssh within a minute is stopped instead, and its console log is in its Lima folder (`serial*.log`).
