---
title: smolvm
weight: 32
---

[smolvm](https://github.com/smol-machines/smolvm) is an experimental backend: each project gets a microVM that runs its image as a container, with no init system. Choose it per VM with `--backend smolvm`, or for every new VM with `devvm setup --backend smolvm`.

[Network policy](#network-policy), [ssh for editors](#ssh-for-editors) (`devvm ssh`, `devvm code`, `--clone`), [forwarding every port](#every-port), the default for new VMs, and [turning SSH agent forwarding on or off](#ssh-agent-forwarding) once a VM is made are emulated, and reviewed ([features](backends.md#features)).

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
- Before `assets/layers/base.sh`, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash` (on Alpine, OpenRC and ssh's client too) and the VM's user, and after base's scripts `sshd` (without host keys: each VM makes its own). It's your user name and uid, as on Lima, with its home at `/home/<you>.guest`: the project folder shows as your uid's, and only that user can write it. It has no password that works (`*`).
- The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run: base's services are enabled the Lima way, and devvm starts them itself (below). The stand-ins are gone before the image is packed.
- Builds use smolvm's default network: the internet, not the host's loopback. A pack keeps its build machine's network, CPUs and memory, so devvm sets all three on every VM it makes.
- Base's CPUs, memory and disk are `base.yaml`'s: 4 CPUs, 8 GiB (half the host's memory on a small host), 100 GiB.

devvm's own scripts for this (`prepare.sh`, `shims.sh`, and `sshd.sh`, which installs sshd after base's scripts, from the Ubuntu snapshot they pin, and keeps the snapshot's package lists for layers) are in `base`'s key, with the backend: smolvm's images are never Lima's.

## What devvm sets on a VM

- **Mounts:** the project folder (read-write) and agents' folders (read-only) at their host paths, the shared tools at `/mnt/devvm-tools` (read-only), and `/var/lib/docker` on the machine's own disk, which the [docker layer](images.md#built-in-layers)'s storage needs (every VM gets it, so the layer can be added in place). A path with a colon is refused: smolvm splits mounts on colons. Files a VM makes in the project folder are yours on the host, mode 0600.
- **Network:** smolvm's own, without network policy: the internet, not the host's loopback. With it, [below](#network-policy).
- **Size:** what you ask for, or base's; the disk only grows.
- **Ports:** those you list (`--ports 5173,8000-8010`), each to the same port on the host's `127.0.0.1` and `::1`, at most 63 ports: smolvm forwards 64, and one is the VM's [ssh](#ssh-for-editors). Port 22 is that ssh's, so listing it is refused (`smolvm_port_ssh`). Only servers listening on all addresses (`0.0.0.0`) are reached. smolvm's forwards share a port a program on the host has on every address (`SO_REUSEADDR`), taking that program's `localhost` connections, so a start leaves out the listed ports in use, warned ([ports](images.md#ports)), and sets them again once a start finds them free (`machine update`, on the stopped VM). `all`, the default, is [emulated](#every-port).
- **SSH agent forwarding:** chosen when the VM is made (`--ssh-agent`), and the agent each start gives it ([below](#ssh-agent-forwarding)).
- **Environment:** the proxy's settings with network policy, as on Lima. The proxy variables come from the VM's proxy file, `~/.local/share/devvm/smolvm/vms/<vm>.proxy` (mode 600), which devvm writes at each start with network policy (and removes without): each of devvm's commands in the VM names it (`smolvm machine exec --secret-file`), and smolvm reads it, so the proxy's password is in no process's arguments. A `smolvm machine exec` of your own gets the variables only in a login shell. (`machine create --secret-file` would last, but smolvm ignores it for a VM made from a pack.)

## Network policy

Emulated, and reviewed. A VM with [network policy](network.md) reaches nothing but its proxy, as on Lima, from three parts devvm sets while it's stopped ([design](design.md#network-policy-on-smolvm)):

- **An allow list of the host alone** (`100.96.0.1/32`, the gateway): smolvm refuses connections anywhere else, and UDP but DNS.
- **DNS closed:** smolvm answers every query `NXDOMAIN` itself, without asking anyone; names resolve in the proxy. One name, `devvm.invalid`, which smolvm needs to filter on, is still asked of your resolver: a narrow way out, as the VM chooses when and how to ask ([details](design.md#network-policy-on-smolvm)).
- **devvm's proxy deciding each connection** (smolvm's mediated egress): before smolvm connects anywhere, it asks the proxy, which takes over connections to itself and refuses the rest: other ports of the host's, which `devvm log` shows as `BLOCKED`, a direct connection. The proxy's address in the VM is `http://100.96.0.1:<port>`.

It fails closed: with the proxy gone, nothing gets out, and once a VM started so, `smolvm machine start` refuses it without a decider. A VM someone started with a decider of their own, or whose allow list someone widened while it runs, isn't confined: devvm shows it `unenforced` and stops it ([the self-check](network.md#the-self-check)). A UDP allow someone adds (`smolvm machine update --allow-udp`) lets UDP past the decider, and smolvm doesn't show it: don't add one to devvm's machines. `devvm network off` takes it all away at the next start.

## ssh for editors

Emulated, and reviewed: `devvm ssh`, `devvm code`, `--clone` and the [ssh config entries](ssh.md#ssh-access-by-name) work as on Lima. How it works ([design](design.md#ssh-for-editors-on-smolvm)):

- **An sshd of devvm's,** started after each start (below) on a config of devvm's own, `/etc/devvm/ssh/sshd_config` (root's alone), whatever the image's says.
- **Host keys of the VM's own:** a new ed25519 key at each start, so no two VMs made from one image share one. devvm reads it through smolvm (`smolvm machine exec`), not over the network, and pins it in `~/.config/devvm/known_hosts`, as on Lima; a key that doesn't match is refused.
- **Only the VM's own key logs in, as the VM's user:** `~/.local/share/devvm/smolvm/vms/<vm>.key` (mode 600), made at its first start with ssh, its public half that VM's only authorized key, so it logs in to no other VM. No passwords, no root, no other user. Entries offer only that key (`IdentitiesOnly yes`), not your agent's.
- **Forwarded from the host's loopback alone:** port 22 in the VM, from a port on the host's `127.0.0.1` and `::1`, chosen at the start and kept while it stays free (on every address: never one a program on the host has on any), never one of the VM's own ports; with network policy, the one devvm records for the VM. The ssh config names it, so connect by name. devvm answers for it only once that start's sshd runs.
- **No way out through ssh, as long as your ssh doesn't ask for one:** an ssh session can carry the VM's connections out only through what your machine's ssh asks for (remote forwards, X11, a tunnel, your agent). devvm's entry turns X11, tunnels, remote SOCKS forwards and adding keys to an agent off, whatever a `Host *` of yours says, and asks for no remote forward. A `RemoteForward` of yours, or `ssh -R` you type, it can't turn off: don't forward into a VM you don't trust. The sshd also allows only local forwarding (`ssh -L`, `-W`, what VS Code uses: the VM connects, from its own network, so with network policy to the proxy alone), but that's a defence for an honest VM: root in the VM can change its sshd.
- **Agent forwarding** is the entry's, as on Lima: `ForwardAgent` as the VM's [SSH agent](ssh.md#agent-forwarding) choice says. smolvm forwards one of its own too ([below](#ssh-agent-forwarding)).
- **The same environment as devvm's own commands:** sessions get `/etc/environment` and, with network policy, the proxy's settings, a login's or not.

A VM running since a start without ssh (from an older devvm) gets no entry, and `devvm ssh` says to restart it (`smolvm_no_ssh`). A VM whose image has no sshd (made before devvm installed one) starts and runs without ssh, warned (`smolvm_no_sshd`): a VM made after `devvm rebuild` has it.

## Every port

Emulated, and reviewed ([design](design.md#every-port-on-smolvm)): a VM that forwards every port (`--ports all`, the default) gets none of smolvm's forwards, and a forwarder of devvm's on your machine instead, much as Lima forwards a VM's ports:

- **What it forwards:** every TCP port from 1024 up that a server in the VM listens on, on the VM's loopback (`127.0.0.1`, `::1`) or all its addresses, to the same port on your machine's `127.0.0.1` and `::1`, within a few seconds of the server starting; the forward goes when the server does. At most 256 at once.
- **What it never takes:** a port a program on your machine has, on any address (that program keeps it, and its connections; the forward waits for the VM's server to stop and start again), and a port devvm records for any VM (its ssh, proxy and decider ports, and the ports VMs list), even while it's free. A program that starts on a port after it's forwarded can still bind it on all addresses, and then its `localhost` connections reach the VM, as on Lima.
- **How it reaches the VM:** over the VM's [ssh](#ssh-for-editors), with the VM's own key and its host key read through smolvm, asking for no forwarding into your machine: the VM learns of each connection only as one to its own loopback, and opens none to your machine. With network policy, the VM still reaches nothing but its proxy.
- **Its process** is `devvm _smolvm-ports`, one per VM, started after the VM's sshd and ended with it; `devvm stop` stops it. Its log is `~/.local/share/devvm/smolvm/vms/<vm>.ports.log`. A VM without ssh (an image without sshd) forwards no ports, warned (`smolvm_ports_no_ssh`); one whose forwarder doesn't start runs on, warned with its log (`smolvm_ports_failed`).

## SSH agent forwarding

smolvm forwards an SSH agent into a VM made with `--ssh-agent` for as long as it runs, not only while you're connected: at `/tmp/ssh-agent.sock` in the VM, which reaches the agent `SSH_AUTH_SOCK` names when it starts. smolvm sets forwarding only when it makes a VM, so turning it on or off later (`devvm ssh-agent on`, `devvm` or `off`) is emulated, and reviewed ([design](design.md#ssh-agent-toggling-on-smolvm)):

- **Every new VM is made forwarding,** and each start gives it the agent its choice names: yours (`SSH_AUTH_SOCK` in your shell), devvm's own, or none. With yours chosen and none in your shell, the start stops (`smolvm_no_agent`).
- **None** is a path no socket can be at (`/dev/null/devvm-no-agent`): the VM's connections to its agent fail, so it lists no keys and can't add any.
- **A change** applies from the VM's next start (`devvm restart`), as on Lima.
- **Started outside devvm** (`smolvm machine start` from your shell gives it the agent your shell has, whatever its choice, and fails without one): devvm, `devvm ssh`, `devvm code` and `devvm export` find it running on a process devvm didn't start, stop it and say so (`agent_unchosen_stop`), unless it uses your agent anyway; the next devvm starts it with its own choice. Until then it has your shell's agent, so start devvm's VMs with devvm.
- **A VM made without forwarding** (by an older devvm) can't have it turned on (`agent_fixed`): `devvm rm`, then `devvm --ssh-agent`.

## No init system

A machine runs its image as a container, and nothing starts at boot. After each start devvm runs, as root:

- the loop devices the shared tools are mounted with (`/dev` has none), then the tools' mounts;
- the VM's [sshd](#ssh-for-editors), with new host keys; its log is `/var/log/devvm-sshd.log`;
- Docker's daemon, in a VM with the [docker layer](images.md#built-in-layers), in the background, logging to `/var/log/dockerd.log`. The layer's script also starts it when it's added to a running VM.

The shared tools are their own images on smolvm: uncompressed EROFS, as its kernel mounts no other. A tool image's key includes the format, so Lima's and smolvm's VMs each get theirs from the one share.

devvm's commands in a VM (its setup, your shell) run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`, which smolvm doesn't read.

## Looking inside

A failed build keeps its machine, stopped (`smolvm_script_failed`): `smolvm machine start --name devvm-img-<key>`, then `smolvm machine shell --name devvm-img-<key>` (as root). The next build, or `devvm gc`, deletes it.
