---
title: Docker
weight: 3
---

Docker is an experimental backend: each project gets a container, made from an image devvm builds by running its scripts in a container and committing it. Choose it per VM with `--backend docker`, or for every new VM with `devvm setup --backend docker`.

## Isolation: a container is not a VM

Read this before you choose Docker. Lima and smolvm give each project a virtual machine with a kernel of its own. Docker gives each project a container:

- **On Linux, every container shares your machine's kernel.** A flaw in that kernel, or in Docker's engine, lets code in a VM reach your machine itself, and every other VM.
- **On macOS, every container shares Docker Desktop's one VM.** A flaw there lets code in one VM reach every other VM, and all your containers, and what Docker Desktop shares with them; your Mac stays behind that VM's boundary.
- **Root in a container is closer to root on the host** than in a VM: it has the same kernel's system calls (filtered by Docker's seccomp profile) and Docker's default capabilities.

Everything else devvm promises (only the project folder mounted, agents' folders read-only, builds without your files) holds on Docker too, as far as the container boundary holds. The [security model](../safety/security.md) assumes a VM's boundary; on Docker it's a container's.

## What it has

Docker's container features are native: building images, `--from` any image Docker has or pulls, the project folder, agents' folders, the shared tools, and CPUs and memory ([features](_index.md#features)). On Linux, builds are confined natively; on Docker Desktop, through a proxy of devvm's ([below](#building-images)).

ssh for editors (and so `devvm ssh`, `devvm code` and `--clone`), forwarding ports (listed, and every one) and SSH agent forwarding (and turning it on or off later) are emulated, and reviewed, all over one transport, `docker exec` into the container ([below](#ssh-for-editors)).

[Network policy](#network-policy), and build confinement on Docker Desktop ([below](#building-images)), are emulated, and reviewed: an internal network of the VM's (or build's) own and a relay to devvm's proxy on your machine.

The [docker layer](#the-docker-layer) is emulated, and reviewed: under Sysbox, on a Linux engine that has it, and nowhere else. Some tools need more than a container gets ([what Docker VMs can't do](#what-docker-vms-cant-do)).

A disk size isn't there, so devvm refuses a VM that asks for one, in one line naming the VM (`backend_lacks`). A default disk size you set (`devvm size -g --disk`) doesn't apply to Docker VMs, and devvm says so (`size_disk_default_unused`).

## Requirements

- Docker 29.1 or newer, `docker` on `PATH`, and its engine running: Docker Desktop on macOS, or Docker's engine on Linux. Its containers run Linux, on this machine's processor.
- On Linux, access to Docker's socket: being in the `docker` group (`sudo usermod -aG docker $USER`, then log in again), which is as good as root on that machine. devvm can't tell builds on a rootless engine are confined, so it builds nothing there.
- On macOS, devvm's Linux build for your Mac's processor, of the same version: [network policy](#network-policy)'s relays and Docker Desktop's [builds](#building-images) run it in containers. On Linux, devvm runs itself. devvm looks for it as `devvm-linux-arm64` or `devvm-linux-amd64` beside `devvm` or in `../libexec` (the Homebrew formula builds it there from source). A release installed with `install.sh` fetches it instead, the first time a Docker VM or build needs it (`docker_linux_build_fetching`, a status step, no question asked): from that release's own Linux archive on GitHub, checked against the SHA-256 the release has of it before it's used, and kept in `~/.local/share/devvm/docker/bin`. One that doesn't match is refused, and nothing kept (`docker_linux_build_mismatch`); one that can't be fetched is `docker_linux_build_fetch_failed`. A build of devvm that isn't a release fetches nothing: build devvm for Linux beside it (`docker_linux_build_missing`). One of another build is `docker_linux_build_other`.

`devvm doctor --backend docker` checks them, and says which engine devvm uses: Docker Desktop, an engine on this Linux machine, or another one (colima, OrbStack, a rootless engine, another machine's), where builds aren't confined and network policy isn't there. On Docker Desktop it lists the folders Desktop shares with containers.

devvm uses the engine your `docker` reaches: its current context, or `DOCKER_HOST`.

## What's devvm's in Docker

Docker's engine is yours (Docker Desktop's especially), so:

- Everything devvm makes in it, containers, images, volumes and networks, carries the label `devvm.data=<devvm's data folder>`. devvm lists, starts, stops, changes and deletes only what has its label, and never prunes. A container, volume or network of yours by a name devvm wants is refused (`docker_container_foreign`, `docker_volume_foreign`, `docker_network_foreign`), never touched.
- Names Docker keeps for everyone have a hash of that folder in them (`<hash>` below), so two devvms never share one: images are `devvm-<hash>:<key>`, the shared tools' volume `devvm-tools-<hash>`, network policy's empty image `devvm-<hash>:proxy`.
- A VM is a container named as the VM, with a volume for its home, `<vm>-home`, and for `--clone`'s clone, `<vm>-clone`; with the [docker layer](#the-docker-layer), `<vm>-docker` and `<vm>-containerd` for its Docker engine's files and a network `<vm>-docker-net`. With [network policy](#network-policy), a network `<vm>-net` and a relay container `<vm>-proxy` too; a Docker Desktop build's are `devvm-img-<key>-<hash>-net` and `-proxy`, for the build alone.
- `docker ps` and `docker images` show devvm's next to yours.
- The image base starts from (Ubuntu's, or `--from`'s) is pulled by its digest, as any image you pull, and Docker keeps it. devvm doesn't delete it: it isn't labelled, and you may use it.

## Building images

- `base` is a container from Ubuntu's container image (`ubuntu:26.04`), by the digest devvm pins with base's package snapshot (`ContainerDigest` in `internal/ubuntu/ubuntu.go`), which is in base's key and its record; with `--from`, from that image by its recorded digest, which may be one only Docker has. Each layer is a container from its parent's image. Each image is the container committed (`docker commit`), its layers shared with its parent's.
- Before `assets/layers/base.sh`, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash` (on Alpine, OpenRC and ssh's client too) and the VM's user, and after base's scripts `sshd` (without host keys), as on [smolvm](smolvm.md#building-images): the same scripts, in `base`'s key. The user has your name and uid, and on Linux your group's id too, so the files it makes in the project folder are yours; home is `/home/<you>.guest`.
- The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run: containers run no init system.
- Base's CPUs and memory are `base.yaml`'s: 4 CPUs, 8 GiB (half the engine's memory on a small one).
- **Confined on Linux:** a build container is on Docker's default network, which reaches the internet but not your machine's loopback (its servers on every address, it does).
- **Confined on Docker Desktop through a proxy of devvm's** (emulated, and reviewed: [design](../internals/design.md#build-confinement-on-docker-desktop)): Desktop's default network reaches your Mac's loopback (`host.docker.internal`), so a build container gets the same as a VM with [network policy](#network-policy), for the build alone: an internal network of its own, no `NET_RAW`, a relay, and devvm's proxy on your Mac, which allows any host on ports 80 and 443 but your Mac's own addresses and internal ones (its [floor](../safety/network.md#the-proxy)). The scripts get the proxy's address in their environment from a file (`docker exec --env-file`), never in a process's arguments, and `sudo` keeps it while they run (`/etc/sudoers.d/devvm-build-proxy`, gone before the commit). Names don't resolve in the container: tools that connect without the proxy variables fail. A known limit, to revisit: the build reaches any public host on 80 and 443, not an allowlist, since `base.policy` and each layer's policy would need every host a build fetches from listed, and builds on the other backends reach the whole internet; and it reaches no other port. When the build ends the proxy, relay and network go; a failed build keeps the proxy's logs beside its container's, in `~/.local/share/devvm/docker/builds/<container>/` (`proxy.log`, `events.jsonl`). A build with no network needs no proxy.
- **Refused on other engines**, which devvm can't tell are confined, in one line (`backend_lacks ... feature=build-confinement`). An image already built is used.

A failed build keeps its container, stopped (`docker_script_failed`): `docker start devvm-img-<key>-<hash>`, then `docker exec -it devvm-img-<key>-<hash> bash` to look inside. `devvm gc` deletes it.

## What devvm sets on a VM

- **Mounts:** the project folder, read-write, and agents' folders, read-only (root in the container can't unmount them), at their host paths; the shared tools' volume at `/mnt/devvm-tools`, read-only; the VM's home, a volume of its own, which starts with the image's home in it. Paths with a comma or a quote are refused (`docker_mount_path`): Docker reads mounts as CSV.
- **Docker Desktop's file sharing:** it mounts only folders under those its settings list (Settings > Resources > File sharing; `/Users`, `/Volumes`, `/private`, `/tmp` and `/var/folders` by default). devvm refuses a project or agent folder outside them (`docker_not_shared`), where it can read Desktop's settings.
- **Network:** Docker's default network. On Docker Desktop that reaches your Mac's own services on `localhost` (`host.docker.internal`); on Linux, your machine's servers on every address, not those on its loopback. With [network policy](#network-policy), its own internal network instead; with the [docker layer](#the-docker-layer), a network of its own that isn't internal.
- **No raw sockets:** the container is made without `NET_RAW` (but with the [docker layer](#the-docker-layer)), even for root, so tools that need raw or packet sockets (`tcpdump`, `nmap`'s raw scans, `arping`) don't run in Docker VMs, nor in Docker Desktop's builds. `ping` works, through ICMP datagram sockets, which Docker allows every group in its containers (`net.ipv4.ping_group_range`).
- **Size:** CPUs and memory, what you ask for or base's, changed with `docker update` on a stopped VM. The CPUs are a quota: coreutils' `nproc` counts it, BusyBox's doesn't, and `free` shows the whole machine.
- **Disk:** none of its own. Docker Desktop refuses a size, and Docker's containerd image store on Linux ignores one, so a VM's disk is the engine's. `devvm size` shows what it holds (`disk_used`: its container's writes and its volumes), and each launch warns when that's past base's 100 GiB (`disk_over`).
- **Hostname:** the folder's part of the VM's name, as devvm's setup sets it elsewhere: a container can't change its own.

### Changing what Docker fixes

Docker sets a container's mounts and capabilities when it makes it, and they can't change. So each start compares them with what the VM wants (a hash in the label `devvm.config`), and where they differ (agents' folders added or removed, or a container that has `NET_RAW`), it makes the container again (`docker_remaking`): the old one committed (`devvm-<hash>:<vm>-<n>`), a new one made from that commit under a name of its own (`<vm>-next`) with the same CPUs, memory and volumes, the old one deleted and the new one renamed. Its disk and home are kept; anything running in it stops, as on any restart. The commit before is deleted; one with more than 100 layers is flattened first (`docker export` and `import`), and no longer shares its image's layers. A run stopped part way leaves the VM's disk in one of them: the next one finishes the rename.

## Network policy

Emulated, and reviewed ([design](../internals/design.md#network-policy-on-docker)). On Docker Desktop and an engine of this Linux machine; other engines can't reach devvm's proxy, so it's none there. On such an engine devvm makes a new VM without network policy and warns you (`net_off_backend`), and it refuses `--network-policy`. A VM with [network policy](../safety/network.md) reaches nothing but its proxy, as on Lima, and the VM gets no capability added (no `NET_ADMIN`), and no `NET_RAW` ([above](#what-devvm-sets-on-a-vm)).

- **Its own network:** the VM is on a network of its own, `<vm>-net`, and no other: internal (no route out, and Docker's resolver answers only its members' names, so no DNS query leaves it) and with no address of your machine's on it (`com.docker.network.bridge.inhibit_ipv4`), so on Linux its gateway doesn't reach your machine's servers; no IPv6 on it, and the VM has IPv6 off on its interface, which root in it can't turn on. Its only other member is its relay. A network with another container on it is refused before the VM starts (`docker_network_shared`): take that container off it (`docker network disconnect <vm>-net <container>`), then launch again.
- **The relay**, `<vm>-proxy`: a container of devvm's from an empty image devvm imports itself (`devvm-<hash>:proxy`, nothing in it, nothing downloaded), running devvm's own Linux build, mounted read-only, as `devvm _proxy -relay`; its root read-only, with no capabilities, no new privileges and no port published. It passes each connection the VM makes to it, unread, to the VM's [proxy](../safety/network.md#the-proxy) on your machine, through a unix socket the proxy also listens on, mounted into the relay alone (`~/.local/share/devvm/docker/sock/<hash>/proxy.sock`): on Docker Desktop the socket itself, which Desktop reaches by its path at each connection; on Linux its folder, as the relay runs as you. That socket is the one path from your machine into the relay, and the relay the VM's one way to the proxy.
- **Why the proxy stays on your machine:** it reads your tokens from the Keychain (or your files), refreshes agents' logins in your own login files, and holds devvm's CA key, which can impersonate any site to every VM. In a container all of that would have to enter Docker (on Docker Desktop, a VM every container shares) or reach back to your machine some other way. On your machine, none of it enters Docker: the relay carries bytes it doesn't read, and the proxy checks the VM's password and policy on each connection, as on every backend.
- **The proxy's address:** `http://devvm:<password>@devvm-proxy:<port>`, the relay's name on the VM's network, in the VM's `/etc/environment` at each start (written through `docker exec`'s input, never in an argument), which devvm's commands, ssh sessions and your shell read; [the setup](../safety/network.md#inside-the-vm) does the rest, as on Lima.
- **The proxy follows the VM's start:** its lifeline is `docker container wait` on the container, a process on your machine started once it runs, which ends when the container stops (`docker stop` included) or Docker's engine goes. A VM whose proxy is gone reaches nothing: the relay finds no socket.
- **Started confined** is that start of its container (its ID and start time), on its own network alone, that network still internal with no address of your machine's, and nothing else on it but the relay that start made (by its container's ID, not its name). Anything else, such as a VM started outside devvm (`docker start`), connected to another network (`docker network connect`), or with another container on its network, is `unenforced`, and its next launch stops it ([the self-check](../safety/network.md#the-self-check)).
- **On or off** (`devvm network`): applied at the VM's next start, which moves the stopped container between its own network and Docker's default one (`docker network disconnect` and `connect`); its relay, network and proxy settings go when it's off. The container isn't made again.
- `devvm stop` stops the VM, its relay and its proxy; `devvm rm` deletes the relay and network too.

## No init system

A container runs Docker's init (`--init`) and a sleep under it; nothing starts at boot. After each start devvm runs, as root, what base's services do on Lima: the shared tools linked (below), and, with [ssh](#ssh-for-editors), its sshd's config and new host key, and with the [docker layer](#the-docker-layer), Docker's daemon. No sshd daemon runs (each ssh connection runs its own).

devvm's commands in a VM (its setup) run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`. So does your shell: through `docker exec`, or over the VM's ssh once that start ran it, an ssh session with the [agent](#ssh-agent-forwarding) the VM's choice names.

## The docker layer

Docker inside a Docker VM (the [docker layer](../using/images.md#built-in-layers), `devvm node+docker`) needs Sysbox on a Linux engine. Anywhere else, use a VM backend, Lima or smolvm, which run Docker inside with nothing more given up. Emulated, and reviewed ([design](../internals/design.md#the-docker-layer-on-docker)).

A container runs `dockerd` only with more than devvm gives its other containers. Sysbox gives it that without `--privileged`: a user namespace of its own, so root in it is an unprivileged user on your machine, but on its mounts, ID-mapped, so your files are yours in it and what you write there is yours. What root in it writes in the project folder is your machine's root's, as in any Docker VM: a set-user-id file it makes there runs as root on your machine for anyone who runs it (devvm names such files at each launch, `setid_root_files`; [details](../safety/security.md#what-it-doesnt-protect)). Sysbox's own code is part of the boundary.

devvm never runs a container `--privileged`. Root in a privileged container has every capability and every device, without Docker's seccomp or AppArmor profile, so it can load kernel modules and write the engine's disks: on Linux that's, in effect, root on your machine; on Docker Desktop, root of Desktop's VM, with every container in it and the folders Desktop shares (your home among them).

- **Where it runs:** on a Linux engine on your machine with Sysbox's runtime (`sysbox-runc`), which `devvm doctor --backend docker` reports (`docker_sysbox`). A VM with the layer runs under it, said (`docker_layer_runtime`), whether it's made with the layer or gets it in place ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)): then its container is [made again](#changing-what-docker-fixes) from a commit, its disk and home kept. On a Linux engine without Sysbox the layer is refused in one line, before anything changes (`docker_layer_no_sysbox`); on Docker Desktop and other engines (another machine's), too (`docker_layer_needs_vm`).
- **Its own network:** a network of its own, `<vm>-docker-net`, labelled, not internal, with nothing else on it, instead of Docker's default one: it reaches the internet, and no other container, on Docker's default network or any other, even as root. On Linux it reaches your machine's servers on every address but its loopback, as any Docker VM without network policy does (with IPv6 turned on, those on IPv6 too). One with another container on it is refused (`docker_network_shared`).
- **Its engine's files** are in volumes of its own, `<vm>-docker` at `/var/lib/docker` and `<vm>-containerd` at `/var/lib/containerd`: Docker's storage doesn't stack on the container's own, and they're kept when its container is made again. `devvm rm` deletes them and its network.
- **Docker's daemon** starts after each start, from base's services; the VM's user is in the `docker` group.
- **No network policy:** root in it can change its own network: in its own network namespace it can turn IPv6 on and reach your machine's servers on the bridge's link-local address. devvm doesn't ask about network policy for a new VM with the layer, whether you named the layer or a `devcontainer.json`'s docker-in-docker asked for it. It makes the VM without network policy and warns you (`net_off_docker_layer`). It refuses `--network-policy`, `devvm network on` and adding the layer to a VM with network policy (`net_docker_layer`).
- **`NET_RAW` and `NET_ADMIN`:** it has them, in its own namespaces: Sysbox gives root every capability there, and Docker's engine needs `NET_ADMIN`. Its own network keeps what it sends from other containers, and your machine drops what it routes to its loopback.
- **Docker's userland proxy stays on** (its default): `"userland-proxy": false` in `daemon.json` turns `route_localnet` on for Docker's bridges, and then root in the VM could reach your machine's loopback. devvm reads its network's bridge setting at each create and start and refuses the VM in one line if it's on (`docker_route_localnet`).
- **A script that fails** when the layer's added in place leaves the VM as it was: its container made again as any VM's, its network and engine's volumes deleted.
- `devvm ls` shows its backend as `docker (sysbox)`; its state has `docker_layer=sysbox`.

**Installing Sysbox:** its release's package for your distribution, from GitHub (`nestybox/sysbox`), which adds `sysbox-runc` to Docker's runtimes and restarts it. On Ubuntu 25.04 and later, fusermount3's AppArmor profile refuses Sysbox's FUSE mounts, and its containers don't start (`failed to pre-register with sysbox-fs`): allow them in `/etc/apparmor.d/local/fusermount3`, then reload the profile (`sudo apparmor_parser -r /etc/apparmor.d/fusermount3`):

```text
mount fstype=fuse options=(nosuid,nodev) options in (ro,rw) -> /var/lib/sysboxfs/**/,
umount /var/lib/sysboxfs/**/,
```

Checked with Sysbox 0.7.1 on Ubuntu 26.04 (arm64, kernel 7.0, Docker 29.1). Docker Desktop has no Sysbox.

## What Docker VMs can't do

A Docker VM is a container with Docker's default capabilities less `NET_RAW`, no devices added, and Docker's seccomp profile. So these don't work in one (one with the [docker layer](#the-docker-layer) has `NET_RAW` and `NET_ADMIN` in its own namespaces, and still none of the devices):

- **Raw packet tools** (`tcpdump`, `nmap`'s raw scans, `arping`): no `NET_RAW`. `ping` works.
- **Firewalls, routes and VPN clients** (`iptables`, `ip route`, WireGuard, OpenVPN): no `NET_ADMIN`, and no `/dev/net/tun`.
- **Mounting:** `mount`, loop devices, FUSE (`sshfs`, `rclone mount`): no `CAP_SYS_ADMIN`, no `/dev/fuse`.
- **Nested virtualization** (QEMU with KVM, Android emulators, Firecracker): no `/dev/kvm`.
- **Some profiling:** `perf` and eBPF tools, whose system calls the seccomp profile refuses. `strace` and `gdb` work.
- **Docker inside**, and with it Kubernetes in Docker (kind, k3d): only under Sysbox on a Linux engine ([above](#the-docker-layer)).

Use a VM backend (Lima or smolvm) for any of these.

## ssh for editors

Emulated, and reviewed ([design](../internals/design.md#ssh-for-editors-on-docker)): `devvm ssh`, `devvm code`, `--clone` and the [ssh config entries](../using/ssh.md#ssh-access-by-name) work as on Lima, and the container publishes nothing.

- **No sshd listens:** after each start devvm writes the VM's sshd config (`/etc/devvm/ssh/sshd_config`, root's alone) and makes it a new ed25519 host key, and starts no daemon. Each ssh connection is an sshd of its own in the container (`sshd -i`, as root), its input and output the connection: `docker exec -i` runs it, and nothing else in the container, or on any network, can connect to it.
- **The way in:** devvm's ssh config names `devvm _docker-connect` as the VM's `ProxyCommand`, with `docker`'s path, devvm's data folder and the engine devvm reaches (`DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`), so an editor started without them reaches the same one. It checks the container is devvm's (its label), a VM's, and running, then runs `docker exec` on its ID. A path with a quote, a backslash or a `%` can't go in the entry (`docker_ssh_path`); a failure is `docker_connect_failed`.
- **Host keys of the VM's own,** read through `docker exec` and pinned in `~/.config/devvm/known_hosts`, as on Lima; a key that doesn't match is refused.
- **Only the VM's own key logs in, as the VM's user:** `~/.local/share/devvm/docker/vms/<vm>.key` (mode 600), its public half that VM's only authorized key. No passwords, no root, no other user. Entries offer only that key, not your agent's.
- **No way out through ssh, as long as your ssh doesn't ask for one,** as on [smolvm](smolvm.md#ssh-for-editors): devvm's entry turns X11, tunnels, remote SOCKS forwards and adding keys to an agent off, and the sshd allows only local forwarding, a defence for an honest VM.

A VM started without ssh, or started outside devvm since (`docker start`), gets no entry from devvm's next launch, and `devvm ssh` says to restart it (`vm_no_ssh`); its shell is a `docker exec`. A VM whose image has no sshd starts and runs without ssh, warned (`vm_no_sshd`): a VM made after `devvm rebuild` has it.

## Ports

Emulated, and reviewed ([design](../internals/design.md#ports-on-docker)): a container's ports reach your machine only when it publishes them, which Docker fixes when it makes the container, and then only servers on its network address, never its loopback. So a VM publishes nothing, and a forwarder of devvm's on your machine forwards its ports, as [smolvm's](smolvm.md#ports) does:

- **What it forwards:** with every port (`--ports all`, the default), each TCP port from 1024 up that a server in the VM listens on, on its loopback or all its addresses; with a list (`--ports 5173,8000-8010`), those ports alone, however many you list, less those a program on your machine has when the VM starts or you change a running VM's ports (warned, `ports_taken`). To the same port on your machine's `127.0.0.1` and `::1`, within a few seconds of the server starting; the forward goes when the server does. At most 256 at once.
- **What it never takes:** a port a program on your machine has, on any address, and a port devvm records for another VM.
- **How it reaches the VM:** over the VM's [ssh](#ssh-for-editors), through `devvm _docker-connect`, with the VM's own key and its pinned host key, asking for no forwarding into your machine.
- **Its process** is `devvm _docker-ports`, one per VM, started after each start with ssh, and again for the same start of the container when you change a running VM's ports; it ends once Docker no longer runs that start of the container (asked every 30 s, and whenever its connection ends), and `devvm stop` stops it. Its log is `~/.local/share/devvm/docker/vms/<vm>.ports.log`. A VM without ssh forwards no ports, warned (`ports_no_ssh`); one whose forwarder doesn't start runs on, warned with its log (`ports_forward_failed`).

## SSH agent forwarding

Emulated, and reviewed, chosen when a VM is made (`--ssh-agent`) and changed later (`devvm ssh-agent on`, `devvm` or `off`) ([design](../internals/design.md#ssh-agent-forwarding-on-docker)): no socket is mounted into a container, so only ssh sessions forward an agent, as Lima's do.

- **Your shell,** an ssh session once the start ran ssh, forwards the agent the VM's start was made with: yours (`SSH_AUTH_SOCK`), devvm's own, or none.
- **The ssh config's entry,** for editors, `ssh` and `git`, forwards the same one (`ForwardAgent`, and `IdentityAgent` for devvm's), an explicit no included, as on Lima.
- **Nothing else in the VM has one:** devvm's setup and other commands run through `docker exec`, and the port forwarder forwards none. A program in the VM reaches the agent only while a session that forwards it is open.
- **A change** applies from the VM's next start (`devvm restart`), to your shell and the ssh config's entry alike, as on Lima.

## Shared tools

Containers can't mount the tools' images (they get no loop devices), so on Docker each tool image is unpacked into one volume, `devvm-tools-<hash>`, a folder per image, by a container of devvm's with no network, after the tools build. VMs mount the volume read-only and link each of their tools' folders where other backends mount the image, which mise takes as installed. A tool image's key includes the format, so Docker's are built for it, and the share keeps their images as the record of what's built: when the volume is gone, devvm unpacks it again from them, and when `devvm gc` deletes one, its folder goes from the volume at the next start.

The tools build runs in a container from base's image with only its own output folder mounted, as on other backends; it never sees the volume.
