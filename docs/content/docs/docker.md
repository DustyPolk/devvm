---
title: Docker
weight: 32
---

Docker is an experimental backend: each project gets a container, made from an image devvm builds by running its scripts in a container and committing it. Choose it per VM with `--backend docker`, or for every new VM with `devvm setup --backend docker`.

## Isolation: a container is not a VM

Read this before you choose Docker. Lima and smolvm give each project a virtual machine with a kernel of its own. Docker gives each project a container:

- **On Linux, every container shares your machine's kernel.** A flaw in that kernel, or in Docker's engine, lets code in a VM reach your machine itself, and every other VM.
- **On macOS, every container shares Docker Desktop's one VM.** A flaw there lets code in one VM reach every other VM, and all your containers, and what Docker Desktop shares with them; your Mac stays behind that VM's boundary.
- **Root in a container is closer to root on the host** than in a VM: it has the same kernel's system calls (filtered by Docker's seccomp profile) and Docker's default capabilities.

Everything else devvm promises (only the project folder mounted, agents' folders read-only, builds without your files) holds on Docker too, as far as the container boundary holds. The [security model](security.md) assumes a VM's boundary; on Docker it's a container's.

## What it has

Docker's container features are native: building images, `--from` any image Docker has or pulls, the project folder, agents' folders, the shared tools, and CPUs and memory ([features](backends.md#features)). On Linux, builds are confined; on Docker Desktop they aren't yet, so it can't build ([below](#building-images)).

ssh for editors (and so `devvm ssh`, `devvm code` and `--clone`), forwarding ports (listed, and every one) and SSH agent forwarding (and turning it on or off later) are emulated, and reviewed, all over one transport, `docker exec` into the container ([below](#ssh-for-editors)).

The rest aren't emulated yet, so devvm refuses a VM that needs them, in one line naming the VM: network policy, a disk size and the [docker layer](images.md#built-in-layers). Make Docker VMs with:

```sh
devvm --backend docker --no-network-policy
```

A default disk size you set (`devvm size -g --disk`) doesn't apply to them, and devvm says so (`size_disk_default_unused`).

## Requirements

- Docker 29.1 or newer, `docker` on `PATH`, and its engine running: Docker Desktop on macOS, or Docker's engine on Linux. Its containers run Linux, on this machine's processor.
- On Linux, access to Docker's socket: being in the `docker` group (`sudo usermod -aG docker $USER`, then log in again), which is as good as root on that machine. devvm can't tell builds on a rootless engine are confined, so it builds nothing there.

`devvm doctor --backend docker` checks them, and says which engine devvm uses: Docker Desktop, an engine on this Linux machine, or another one (colima, OrbStack, a rootless engine, another machine's), where builds aren't confined. On Docker Desktop it lists the folders Desktop shares with containers.

devvm uses the engine your `docker` reaches: its current context, or `DOCKER_HOST`.

## What's devvm's in Docker

Docker's engine is yours (Docker Desktop's especially), so:

- Everything devvm makes in it, containers, images and volumes, carries the label `devvm.data=<devvm's data folder>`. devvm lists, starts, stops, changes and deletes only what has its label, and never prunes. A container or volume of yours by a name devvm wants is refused (`docker_container_foreign`, `docker_volume_foreign`), never touched.
- Names Docker keeps for everyone have a hash of that folder in them (`<hash>` below), so two devvms never share one: images are `devvm-<hash>:<key>`, the shared tools' volume `devvm-tools-<hash>`.
- A VM is a container named as the VM, with a volume for its home, `<vm>-home`, and for `--clone`'s clone, `<vm>-clone`.
- `docker ps` and `docker images` show devvm's next to yours.
- The image base starts from (Ubuntu's, or `--from`'s) is pulled by its digest, as any image you pull, and Docker keeps it. devvm doesn't delete it: it isn't labelled, and you may use it.

## Building images

- `base` is a container from Ubuntu's container image (`ubuntu:26.04`), by the digest its registry has at the time, which its record keeps; with `--from`, from that image by its recorded digest, which may be one only Docker has. Each layer is a container from its parent's image. Each image is the container committed (`docker commit`), its layers shared with its parent's.
- Before `assets/layers/base.sh`, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash` (on Alpine, OpenRC and ssh's client too) and the VM's user, and after base's scripts `sshd` (without host keys), as on [smolvm](smolvm.md#building-images): the same scripts, in `base`'s key. The user has your name and uid, and on Linux your group's id too, so the files it makes in the project folder are yours; home is `/home/<you>.guest`.
- The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run: containers run no init system.
- Base's CPUs and memory are `base.yaml`'s: 4 CPUs, 8 GiB (half the engine's memory on a small one).
- **Confined on Linux:** a build container is on Docker's default network, which reaches the internet but not your machine's loopback (its servers on every address, it does). **Refused on Docker Desktop**, whose containers reach your Mac's loopback (`host.docker.internal`): builds there need the internet, and Docker can't confine them yet, so devvm stops in one line (`backend_lacks ... feature=build-confinement`). The same on an engine devvm can't tell is confined. An image already built is used.

A failed build keeps its container, stopped (`docker_script_failed`): `docker start devvm-img-<key>-<hash>`, then `docker exec -it devvm-img-<key>-<hash> bash` to look inside. `devvm gc` deletes it.

## What devvm sets on a VM

- **Mounts:** the project folder, read-write, and agents' folders, read-only (root in the container can't unmount them), at their host paths; the shared tools' volume at `/mnt/devvm-tools`, read-only; the VM's home, a volume of its own, which starts with the image's home in it. Paths with a comma or a quote are refused (`docker_mount_path`): Docker reads mounts as CSV.
- **Docker Desktop's file sharing:** it mounts only folders under those its settings list (Settings > Resources > File sharing; `/Users`, `/Volumes`, `/private`, `/tmp` and `/var/folders` by default). devvm refuses a project or agent folder outside them (`docker_not_shared`), where it can read Desktop's settings.
- **Network:** Docker's default network. On Docker Desktop that reaches your Mac's own services on `localhost` (`host.docker.internal`); on Linux, your machine's servers on every address, not those on its loopback.
- **Size:** CPUs and memory, what you ask for or base's, changed with `docker update` on a stopped VM. The CPUs are a quota: coreutils' `nproc` counts it, BusyBox's doesn't, and `free` shows the whole machine.
- **Disk:** none of its own. Docker Desktop refuses a size, and Docker's containerd image store on Linux ignores one, so a VM's disk is the engine's. `devvm size` shows what it holds (`disk_used`: its container's writes and its volumes), and each launch warns when that's past base's 100 GiB (`disk_over`).
- **Hostname:** the folder's part of the VM's name, as devvm's setup sets it elsewhere: a container can't change its own.

### Changing what Docker fixes

Docker sets a container's mounts and network when it makes it, and they can't change. So each start compares them with what the VM wants (a hash in the label `devvm.config`), and where they differ, after agents' folders are added or removed, it makes the container again (`docker_remaking`): the old one committed (`devvm-<hash>:<vm>-<n>`), a new one made from that commit under a name of its own (`<vm>-next`) with the same CPUs, memory and volumes, the old one deleted and the new one renamed. Its disk and home are kept; anything running in it stops, as on any restart. The commit before is deleted; one with more than 100 layers is flattened first (`docker export` and `import`), and no longer shares its image's layers. A run stopped part way leaves the VM's disk in one of them: the next one finishes the rename.

## No init system

A container runs Docker's init (`--init`) and a sleep under it; nothing starts at boot. After each start devvm runs, as root, what base's services do on Lima: the shared tools linked (below), and, with [ssh](#ssh-for-editors), its sshd's config and new host key. No sshd daemon runs (each ssh connection runs its own), nor Docker's daemon (`dockerd` needs a privileged container).

devvm's commands in a VM (its setup) run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`. So does your shell: through `docker exec`, or over the VM's ssh once that start ran it, an ssh session with the [agent](#ssh-agent-forwarding) the VM's choice names.

## ssh for editors

Emulated, and reviewed ([design](design.md#ssh-for-editors-on-docker)): `devvm ssh`, `devvm code`, `--clone` and the [ssh config entries](ssh.md#ssh-access-by-name) work as on Lima, and the container publishes nothing.

- **No sshd listens:** after each start devvm writes the VM's sshd config (`/etc/devvm/ssh/sshd_config`, root's alone) and makes it a new ed25519 host key, and starts no daemon. Each ssh connection is an sshd of its own in the container (`sshd -i`, as root), its input and output the connection: `docker exec -i` runs it, and nothing else in the container, or on any network, can connect to it.
- **The way in:** devvm's ssh config names `devvm _docker-connect` as the VM's `ProxyCommand`, with `docker`'s path, devvm's data folder and the engine devvm reaches (`DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`), so an editor started without them reaches the same one. It checks the container is devvm's (its label), a VM's, and running, then runs `docker exec` on its ID. A path with a quote, a backslash or a `%` can't go in the entry (`docker_ssh_path`); a failure is `docker_connect_failed`.
- **Host keys of the VM's own,** read through `docker exec` and pinned in `~/.config/devvm/known_hosts`, as on Lima; a key that doesn't match is refused.
- **Only the VM's own key logs in, as the VM's user:** `~/.local/share/devvm/docker/vms/<vm>.key` (mode 600), its public half that VM's only authorized key. No passwords, no root, no other user. Entries offer only that key, not your agent's.
- **No way out through ssh, as long as your ssh doesn't ask for one,** as on [smolvm](smolvm.md#ssh-for-editors): devvm's entry turns X11, tunnels, remote SOCKS forwards and adding keys to an agent off, and the sshd allows only local forwarding, a defence for an honest VM.

A VM started without ssh, or started outside devvm since (`docker start`), gets no entry from devvm's next launch, and `devvm ssh` says to restart it (`vm_no_ssh`); its shell is a `docker exec`. A VM whose image has no sshd starts and runs without ssh, warned (`vm_no_sshd`): a VM made after `devvm rebuild` has it.

## Ports

Emulated, and reviewed ([design](design.md#ports-on-docker)): a container's ports reach your machine only when it publishes them, which Docker fixes when it makes the container, and then only servers on its network address, never its loopback. So a VM publishes nothing, and a forwarder of devvm's on your machine forwards its ports, as [smolvm's](smolvm.md#every-port) does:

- **What it forwards:** with every port (`--ports all`, the default), each TCP port from 1024 up that a server in the VM listens on, on its loopback or all its addresses; with a list (`--ports 5173,8000-8010`), those ports alone, however many you list, less those a program on your machine has when the VM starts (warned, `ports_taken`). To the same port on your machine's `127.0.0.1` and `::1`, within a few seconds of the server starting; the forward goes when the server does. At most 256 at once.
- **What it never takes:** a port a program on your machine has, on any address, and a port devvm records for another VM.
- **How it reaches the VM:** over the VM's [ssh](#ssh-for-editors), through `devvm _docker-connect`, with the VM's own key and its pinned host key, asking for no forwarding into your machine.
- **Its process** is `devvm _docker-ports`, one per VM, started after each start with ssh; it ends once Docker no longer runs that start of the container (asked every 30 s, and whenever its connection ends), and `devvm stop` stops it. Its log is `~/.local/share/devvm/docker/vms/<vm>.ports.log`. A VM without ssh forwards no ports, warned (`ports_no_ssh`); one whose forwarder doesn't start runs on, warned with its log (`ports_forward_failed`).

## SSH agent forwarding

Emulated, and reviewed, chosen when a VM is made (`--ssh-agent`) and changed later (`devvm ssh-agent on`, `devvm` or `off`) ([design](design.md#ssh-agent-forwarding-on-docker)): no socket is mounted into a container, so only ssh sessions forward an agent, as Lima's do.

- **Your shell,** an ssh session once the start ran ssh, forwards the agent the VM's choice names: yours (`SSH_AUTH_SOCK`), devvm's own, or none.
- **The ssh config's entry,** for editors, `ssh` and `git`, forwards the one the VM's choice names (`ForwardAgent`, and `IdentityAgent` for devvm's), an explicit no included, as on Lima.
- **Nothing else in the VM has one:** devvm's setup and other commands run through `docker exec`, and the port forwarder forwards none. A program in the VM reaches the agent only while a session that forwards it is open.
- **A change** applies to your shell's next session; turning forwarding on, from the VM's next start (`devvm restart`). The ssh config's entry follows the VM's choice from devvm's next launch, any folder's, as on Lima: turned on, `ssh` by name and editors forward the agent before the restart.

## Shared tools

Containers can't mount the tools' images (they get no loop devices without `--privileged`), so on Docker each tool image is unpacked into one volume, `devvm-tools-<hash>`, a folder per image, by a container of devvm's with no network, after the tools build. VMs mount the volume read-only and link each of their tools' folders where other backends mount the image, which mise takes as installed. A tool image's key includes the format, so Docker's are built for it, and the share keeps their images as the record of what's built: when the volume is gone, devvm unpacks it again from them, and when `devvm gc` deletes one, its folder goes from the volume at the next start.

The tools build runs in a container from base's image with only its own output folder mounted, as on other backends; it never sees the volume.
