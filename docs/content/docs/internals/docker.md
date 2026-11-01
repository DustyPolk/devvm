---
title: Docker
weight: 11
---

This page explains what devvm makes in Docker's engine, how it builds images and runs containers as VMs, and how it emulates what Docker lacks. [Docker (experimental)](../backends/docker.md) covers what you need to run it, what a container's isolation means for you and what Docker VMs can't do.

Docker's container [features](backends.md#features) are native: building images, `--from` any image Docker has or pulls, the project folder, agents' folders, the shared tools, and CPUs and memory. On Linux, builds are confined natively, and on Docker Desktop through a proxy of devvm's ([building images](#building-images)).

devvm emulates the rest, and each emulation is reviewed:

- [ssh for editors](#ssh-for-editors), and so `devvm host`, `devvm code` and `--clone`, [forwarding ports](#ports), both the listed ones and every one, and [SSH agent forwarding](#ssh-agent-forwarding), including turning it on or off later. All of them go over one transport, which is `docker exec` into the container.
- [Network policy](#network-policy), and build confinement on Docker Desktop, through an internal network of the VM's or the build's own and a relay to devvm's proxy on your machine.
- [The docker layer](#the-docker-layer), under Sysbox on a Linux engine that has it, and nowhere else.

Some tools need more than a container gets ([what Docker VMs can't do](../backends/docker.md#what-docker-vms-cant-do)), and a disk size isn't there at all ([below](#what-devvm-sets-on-a-vm)).

## Engines

devvm builds images and confines VMs on two kinds of engine, which are Docker Desktop and an engine on this Linux machine. On any other engine, such as colima, OrbStack, a rootless engine or another machine's, builds aren't confined and network policy isn't there, and `devvm doctor --backend docker` says so (`docker_engine_other`). devvm can't tell that builds on a rootless engine are confined, so it builds nothing there.

## devvm's Linux build

On macOS, [network policy](#network-policy)'s relays and Docker Desktop's [builds](#building-images) run devvm's Linux build in containers, for your Mac's processor and of the same version as devvm. On Linux, devvm runs itself.

devvm looks for the Linux build as `devvm-linux-arm64` or `devvm-linux-amd64` beside `devvm` or in `../libexec`, where the Homebrew formula builds it from source. A release installed with `install.sh` fetches it instead, the first time a Docker VM or build needs it, without asking (`docker_linux_build_fetching`, a status step). It fetches the build from that release's own Linux archive on GitHub, checks it against the SHA-256 the release has of it before it uses it, and keeps it in `~/.local/share/devvm/docker/bin`. devvm refuses a build that doesn't match and keeps nothing (`docker_linux_build_mismatch`), and a build it can't fetch is `docker_linux_build_fetch_failed`.

A build of devvm that isn't a release fetches nothing, so build devvm for Linux beside it (`docker_linux_build_missing`). A Linux build that belongs to another build of devvm is `docker_linux_build_other`.

## What's devvm's in Docker

Docker's engine is yours, and Docker Desktop's especially, so devvm marks and names what it makes there:

- Everything devvm makes in the engine, which is containers, images, volumes and networks, carries the label `devvm.data=<devvm's data folder>`. devvm acts only on what has its label ([what you'll see in Docker](../backends/docker.md#what-youll-see-in-docker)). devvm refuses a container, volume or network of yours that has a name it wants (`docker_container_foreign`, `docker_volume_foreign`, `docker_network_foreign`), and it never touches it.
- Names that Docker keeps for everyone have a hash of that folder in them (`<hash>` below), so two devvms never share one. Images are `devvm-<hash>:<key>`, the shared tools' volume is `devvm-tools-<hash>`, and network policy's empty image is `devvm-<hash>:proxy`.
- A VM is a container named as the VM, with a volume for its home, `<vm>-home`, and one for `--clone`'s clone, `<vm>-clone`. With the [docker layer](#the-docker-layer), it also has `<vm>-docker` and `<vm>-containerd` for its Docker engine's files, and a network `<vm>-docker-net`. With [network policy](#network-policy), it has a network `<vm>-net` and a relay container `<vm>-proxy`. A Docker Desktop build's network and relay are `devvm-img-<key>-<hash>-net` and `-proxy`, for the build alone.
- `docker ps` and `docker images` show devvm's next to yours.
- Docker pulls the image that base starts from, Ubuntu's or `--from`'s, by its digest, as it does any image you pull, and it keeps the image. devvm doesn't delete that image, because it isn't labelled and you may use it.

## Building images

`base` is a container from Ubuntu's container image (`ubuntu:26.04`), by the digest devvm pins with base's package snapshot (`ContainerDigest` in `internal/ubuntu/ubuntu.go`), which is in base's key and its record. With `--from`, it's a container from that image by its recorded digest, which may be one only Docker has. Each layer is a container from its parent's image. devvm commits each container as its image (`docker commit`), so an image shares its layers with its parent's.

Before `assets/layers/base.sh` runs, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash` and the VM's user, and on Alpine also OpenRC and ssh's client. After base's scripts, it adds `sshd` without host keys. These are the same scripts as on [smolvm](smolvm.md#building-images), and they're in `base`'s key. The user has your name and uid, and on Linux your group's id too, so the files it makes in the project folder are yours. Its home is `/home/<you>.guest`.

The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run, because containers run no init system. Base's CPUs and memory come from `base.yaml`, which sets 4 CPUs and 8 GiB. When the engine's machine has fewer than 4 CPUs, base and the VMs you give no size get as many CPUs as it has, and when 8 GiB would leave it less than 2 GiB, they get half of its memory ([base's size](images.md#bases-size)).

How devvm confines a build depends on the engine:

- **On Linux,** a build container is on Docker's default network, which reaches the internet but not your machine's loopback. It does reach your machine's servers that listen on every address. Behind a company proxy (`HTTPS_PROXY` or `HTTP_PROXY` in devvm's environment), a build gets what a build on Docker Desktop gets instead, below, so that it reaches the company proxy only through devvm's proxy, which alone has the company proxy's password ([a company proxy](design.md#a-company-proxy)). Such a build needs devvm's Linux build for its relay, as network policy does.
- **On Docker Desktop,** Desktop's default network reaches your Mac's loopback (`host.docker.internal`), so devvm confines a build through a proxy of its own. This is emulated, and reviewed ([design](design.md#build-confinement-on-docker-desktop)). A build container gets what a VM with [network policy](#network-policy) gets, for the build alone: an internal network of its own, no `NET_RAW`, a relay, and devvm's proxy on your Mac. That proxy allows any host on ports 80 and 443 but your Mac's own addresses and internal ones (its [floor](network.md#the-proxy)), and it reaches no other port.
- **On other engines,** devvm can't tell that a build is confined, so it refuses the build in one line (`backend_lacks ... feature=build-confinement`). It still uses an image that's already built.

On Docker Desktop, the scripts get the proxy's address in their environment from a file (`docker exec --env-file`), never in a process's arguments, and `sudo` keeps it while they run (`/etc/sudoers.d/devvm-build-proxy`, which is gone before the commit). Names don't resolve in the container, so tools that connect without the proxy variables fail. The build reaches any public host on ports 80 and 443 rather than an allowlist, which is a known limit to revisit, because `base.policy` and each layer's policy would need to list every host a build fetches from, and builds on the other backends reach the whole internet. When the build ends, devvm removes the proxy, the relay and the network. A failed build keeps the proxy's logs beside its container's, in `~/.local/share/devvm/docker/builds/<container>/` (`proxy.log`, `events.jsonl`). A build with no network needs no proxy.

A failed build keeps its container, stopped (`docker_script_failed`), so you can [look inside](../backends/docker.md#looking-inside).

## What devvm sets on a VM

- **Mounts:** the project folder, read-write, and agents' folders, read-only, at their host paths. Root in the container can't unmount them. The shared tools' volume is at `/mnt/devvm-tools`, read-only, and the VM's home is a volume of its own, which starts with the image's home in it. Docker reads mounts as CSV, so devvm refuses paths with a comma or a quote (`docker_mount_path`).
- **Docker Desktop's file sharing:** devvm refuses a project or agent folder outside the folders Desktop shares (`docker_not_shared`), when it can read Desktop's settings ([file sharing](../backends/docker.md#docker-desktops-file-sharing)).
- **Network:** Docker's default network. On Docker Desktop that reaches your Mac's own services on `localhost` (`host.docker.internal`). On Linux it reaches your machine's servers on every address, but not those on its loopback. With [network policy](#network-policy), the VM is on its own internal network instead, and with the [docker layer](#the-docker-layer), on a network of its own that isn't internal.
- **No raw sockets:** devvm makes the container without `NET_RAW`, even for root, except with the [docker layer](#the-docker-layer). So tools that need raw or packet sockets (`tcpdump`, `nmap`'s raw scans, `arping`) don't run in Docker VMs, nor in Docker Desktop's builds. `ping` works, through ICMP datagram sockets, which Docker allows every group in its containers (`net.ipv4.ping_group_range`).
- **Size:** CPUs and memory, what you ask for or base's, which devvm changes with `docker update` on a stopped VM. The CPUs are a quota, which coreutils' `nproc` counts and BusyBox's doesn't, and `free` shows the whole machine.
- **Disk:** none of its own. Docker Desktop refuses a size, and Docker's containerd image store on Linux ignores one, so a VM's disk is the engine's. devvm refuses a VM that asks for a disk size, in one line naming the VM (`backend_lacks`). A default disk size you set (`devvm size -g --disk`) doesn't apply to Docker VMs, and devvm says so (`size_disk_default_unused`). `devvm size` shows what a VM holds (`disk_used`), which is its container's writes and its volumes, and each launch warns when that's past base's 100 GiB (`disk_over`).
- **Hostname:** the folder's part of the VM's name, which devvm's setup sets on other backends, because a container can't change its own.

### Changing what Docker fixes

Docker sets a container's mounts and capabilities when it makes the container, and they can't change. So each start compares them with what the VM wants, which devvm keeps as a hash in the label `devvm.config`. Where they differ, such as when agents' folders are added or removed, or when a container has `NET_RAW`, devvm makes the container again (`docker_remaking`):

1. It commits the old container (`devvm-<hash>:<vm>-<n>`).
2. It makes a new container from that commit, under a name of its own (`<vm>-next`), with the same CPUs, memory and volumes.
3. It deletes the old container and renames the new one.

The VM's disk and home are kept, and anything running in it stops, as on any restart. devvm deletes the commit from the time before. It flattens a commit with more than 100 layers first (`docker export` and `import`), and a flattened commit doesn't share its image's layers. A run that stops part way leaves the VM's disk in one of the two containers, and the next run finishes the rename.

## Network policy

Network policy is emulated, and reviewed ([design](design.md#network-policy-on-docker)), on Docker Desktop and on an engine of this Linux machine. Other engines can't reach devvm's proxy, so they have none. On such an engine devvm makes a new VM without network policy and warns you (`net_off_backend`), and it refuses `--network-policy`.

A VM with [network policy](../safety/network.md) reaches nothing but its proxy, as on Lima. devvm adds no capability to the VM, so it has no `NET_ADMIN`, and it has no `NET_RAW` either ([above](#what-devvm-sets-on-a-vm)).

- **Its own network.** The VM is on a network of its own, `<vm>-net`, and on no other. The network is internal, so it has no route out, and Docker's resolver answers only its members' names, so no DNS query leaves it. It has no address of your machine's on it (`com.docker.network.bridge.inhibit_ipv4`), so on Linux its gateway doesn't reach your machine's servers. It has no IPv6, and the VM has IPv6 off on its interface, which root in the VM can't turn on. Its only other member is the VM's relay. devvm refuses a network with another container on it before the VM starts (`docker_network_shared`). To fix that, take the container off it (`docker network disconnect <vm>-net <container>`) and launch again.
- **The relay,** `<vm>-proxy`, is a container of devvm's from an empty image that devvm imports itself (`devvm-<hash>:proxy`), with nothing in it and nothing downloaded. It runs devvm's own Linux build, mounted read-only, as `devvm _proxy -relay`. Its root is read-only, and it has no capabilities, no new privileges and no port published. It passes each connection the VM makes to it, unread, to the VM's [proxy](network.md#the-proxy) on your machine, through a unix socket the proxy also listens on (`~/.local/share/devvm/docker/sock/<hash>/proxy.sock`), which is mounted into the relay alone. On Docker Desktop devvm mounts the socket itself, which Desktop reaches by its path at each connection. On Linux it mounts the socket's folder, because the relay runs as you. That socket is the one path from your machine into the relay, and the relay is the VM's one way to the proxy.
- **The proxy stays on your machine,** because it reads your secrets from the Keychain (or your files), refreshes agents' logins in your own login files, and holds devvm's CA key, which can impersonate any site to every VM. In a container all of that would have to enter Docker, which on Docker Desktop is a VM every container shares, or reach back to your machine some other way. On your machine none of it enters Docker, because the relay carries bytes it doesn't read, and the proxy checks the VM's password and policy on each connection, as on every backend.
- **The proxy's address** is `http://devvm:<password>@devvm-proxy:<port>`, which uses the relay's name on the VM's network. devvm writes it into the VM's `/etc/environment` at each start, through `docker exec`'s input and never in an argument, and devvm's commands, ssh sessions and your shell read it there. [The setup](../safety/network.md#inside-the-vm) does the rest, as on Lima.
- **The proxy follows the VM's start.** Its lifeline is `docker container wait` on the container, a process on your machine that starts once the container runs and ends when the container stops, `docker stop` included, or when Docker's engine goes. A VM whose proxy is gone reaches nothing, because the relay finds no socket.
- **Started confined** means that this start of the container, by its ID and start time, is on its own network alone, that the network is still internal with no address of your machine's, and that nothing else is on it but the relay that start made, by its container's ID rather than its name. Anything else is `unenforced`, such as a VM started outside devvm (`docker start`), one connected to another network (`docker network connect`), or one with another container on its network, and its next launch stops it ([the self-check](network.md#the-self-check)).
- **On or off** (`devvm network`) applies at the VM's next start, which moves the stopped container between its own network and Docker's default one (`docker network disconnect` and `connect`). When network policy is off, its relay, network and proxy settings go. devvm doesn't make the container again.
- `devvm stop` stops the VM, its relay and its proxy, and `devvm rm` also deletes the relay and the network.

## ssh for editors

ssh for editors is emulated, and reviewed ([design](design.md#ssh-for-editors-on-docker)). `devvm host`, `devvm code`, `--clone` and the [ssh config entries](../using/ssh.md#ssh-access-by-name) work as they do on Lima, and the container publishes nothing.

No sshd listens in the VM. After each start, devvm writes the VM's sshd config (`/etc/devvm/ssh/sshd_config`, which only root can change) and makes it a new ed25519 host key, and it starts no daemon. Each ssh connection is an sshd of its own in the container (`sshd -i`, as root), whose input and output are the connection. `docker exec -i` runs it, so nothing else in the container, or on any network, can connect to it.

The way in is `devvm _docker-connect`, which devvm's ssh config names as the VM's `ProxyCommand`. The entry gives it `docker`'s path, devvm's data folder and the engine devvm reaches (`DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`), so an editor started without them reaches the same engine. It checks that the container is devvm's (by its label), that it's a VM's and that it's running, and then it runs `docker exec` on the container's ID. A path with a quote, a backslash or a `%` can't go in the entry (`docker_ssh_path`), and a failure to connect is `docker_connect_failed`.

The VM has host keys of its own, which devvm reads through `docker exec` and pins in `~/.config/devvm/known_hosts`, as on Lima, and ssh refuses a key that doesn't match.

Only the VM's own key logs in, and only as the VM's user. The key is `~/.local/share/devvm/docker/vms/<vm>.key` (mode 600), and its public half is that VM's only authorized key. Passwords, root and other users can't log in. The entries offer only that key, and never your agent's keys.

An ssh session gives the VM no way out as long as your ssh doesn't ask for one, as on [smolvm](smolvm.md#ssh-for-editors). devvm's entry turns off X11, tunnels, remote SOCKS forwards and adding keys to an agent, and the sshd allows local forwarding and remote forwards only on `127.77.0.0/16`, which protects only an honest VM.

A VM started without ssh, or started outside devvm since (`docker start`), gets no entry from devvm's next launch, and `devvm host` tells you to restart it (`vm_no_ssh`). Its shell is then a `docker exec`. A VM whose image has no sshd starts and runs without ssh, with a warning (`vm_no_sshd`), and a VM made after `devvm rebuild` has it.

## Ports

Forwarding ports is emulated, and reviewed ([design](design.md#ports-on-docker)). A container's ports reach your machine only when it publishes them, which Docker fixes when it makes the container, and even then only servers on its network address reach your machine, never those on its loopback. So a VM publishes nothing, and devvm's forwarder on your machine forwards its ports, by the rules that [Port forwarding](ports.md) sets out for every backend.

The forwarder reaches the VM over its [ssh](#ssh-for-editors), through `devvm _docker-connect`, with the VM's own key and its pinned host key.

Which ports a VM forwards is in devvm's record for the VM. [Its process and files](ports.md#its-process-and-files) lists the record's path and the forwarder's log.

devvm starts the forwarder after each start with ssh, and again for the same start of the container when you change a running VM's ports. No process on your machine runs the container, so the forwarder follows that start of it instead: it asks Docker every 30 s, and whenever its connection ends, and it exits once Docker no longer runs that start of the container. A VM without ssh forwards no ports, with a warning (`ports_no_ssh`).

## SSH agent forwarding

SSH agent forwarding is emulated, and reviewed ([design](design.md#ssh-agent-forwarding-on-docker)). You choose it when you make a VM (`--ssh-agent`) and can change it later (`devvm ssh-agent on`, `devvm` or `off`). devvm mounts no socket into a container, so only ssh sessions forward an agent, as Lima's do.

- **Your shell** is an ssh session once the start ran ssh, and it forwards the agent the VM's start was made with: yours (`SSH_AUTH_SOCK`), devvm's own, or none.
- **The ssh config's entry,** which editors, `ssh` and `git` use, forwards the same one (`ForwardAgent`, and `IdentityAgent` for devvm's), and it says no explicitly when there's none, as on Lima.
- **Nothing else in the VM has an agent.** devvm's setup and other commands run through `docker exec`, and the port forwarder forwards none. A program in the VM reaches the agent only while a session that forwards it is open.
- **A change** applies from the VM's next start (`devvm restart`), to your shell and the ssh config's entry alike, as on Lima.

## No init system

A container runs Docker's init (`--init`) with a sleep under it, and nothing starts at boot. After each start, devvm runs as root what base's services do on Lima. It links the shared tools ([below](#shared-tools)), and with [ssh](#ssh-for-editors) it writes the sshd's config and a new host key. With the [docker layer](#the-docker-layer), it starts Docker's daemon. No sshd daemon runs, because each ssh connection runs its own.

devvm's commands in a VM, such as its setup, run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`. So does your shell, either through `docker exec` or, once that start ran ssh, over the VM's ssh, as a session with the [agent](#ssh-agent-forwarding) the VM's choice names.

## Shared tools

Containers can't mount the tools' images, because they get no loop devices. So on Docker, after the tools build, a container of devvm's with no network unpacks each tool image into one volume, `devvm-tools-<hash>`, with a folder for each image. VMs mount the volume read-only and link each of their tools' folders where other backends mount the image, and mise takes the tool as installed.

A tool image's key includes the format, so Docker's tool images are built for it. The share keeps their images as the record of what's built. When the volume is gone, devvm unpacks it again from them, and when `devvm gc` deletes an image, its folder goes from the volume at the next start.

The tools build runs in a container from base's image with only its own output folder mounted, as on other backends, so it never sees the volume.

## The docker layer

The docker layer is emulated, and reviewed ([design](design.md#the-docker-layer-on-docker)). [The Docker page](../backends/docker.md#the-docker-layer) says what it means for you.

A container runs `dockerd` only with more than devvm gives its other containers. Sysbox gives it that without `--privileged`, in a user namespace of its own, so root in it is an unprivileged user on your machine. Sysbox ID-maps the container's mounts, so your files are yours in it, and what you write there is yours. What root in it writes in the project folder is your machine's root's, as in any Docker VM, so a set-user-id file it makes there runs as root on your machine for anyone who runs it. devvm names such files at each launch (`setid_root_files`, [details](security.md#what-it-doesnt-protect)). Sysbox's own code is part of the boundary.

- **Where it runs:** on a Linux engine on your machine with Sysbox's runtime (`sysbox-runc`), which `devvm doctor --backend docker` reports (`docker_sysbox`). A VM with the layer runs under Sysbox, and devvm says so (`docker_layer_runtime`). Only a new VM gets the layer, as with any layer ([adding to a VM](../getting-started/commands.md#adding-to-a-vm)), so a VM is made for it from the start. On a Linux engine without Sysbox, devvm refuses the layer in one line, before anything changes (`docker_layer_no_sysbox`), and it does the same on Docker Desktop and other engines, such as another machine's (`docker_layer_needs_vm`).
- **Its own network:** the VM is on a network of its own, `<vm>-docker-net`, instead of Docker's default one. The network is labelled, isn't internal, and has nothing else on it. It reaches the internet, but no other container, on Docker's default network or any other, even as root. On Linux it reaches your machine's servers on every address but its loopback, as any Docker VM without network policy does, and with IPv6 turned on, those on IPv6 too. devvm refuses the network if another container is on it (`docker_network_shared`).
- **Its engine's files** are in volumes of their own, `<vm>-docker` at `/var/lib/docker` and `<vm>-containerd` at `/var/lib/containerd`, because Docker's storage doesn't stack on the container's own. devvm keeps them when it makes the container again, and `devvm rm` deletes them and the VM's network.
- **Docker's daemon** starts after each start, from base's services, and the VM's user is in the `docker` group.
- **No network policy:** root in the VM can change its own network. In its own network namespace it can turn IPv6 on and reach your machine's servers on the bridge's link-local address. So devvm doesn't ask about network policy for a new VM with the layer, whether you named the layer or a `devcontainer.json`'s docker-in-docker asked for it. It makes the VM without network policy and warns you (`net_off_docker_layer`). It refuses `--network-policy` and `devvm network on` (`net_docker_layer`).
- **`NET_RAW` and `NET_ADMIN`:** the VM has them, in its own namespaces, because Sysbox gives root every capability there, and Docker's engine needs `NET_ADMIN`. Its own network keeps what it sends away from other containers, and your machine drops what it routes to its loopback.
- **Docker's userland proxy stays on,** which is its default. `"userland-proxy": false` in `daemon.json` turns `route_localnet` on for Docker's bridges, and then root in the VM could reach your machine's loopback. devvm reads its network's bridge setting at each create and start, and it refuses the VM in one line if the setting is on (`docker_route_localnet`).
- `devvm ls` shows its backend as `docker (sysbox)`, and its state has `docker_layer=sysbox`.
