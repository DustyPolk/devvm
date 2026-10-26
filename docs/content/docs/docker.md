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

The rest aren't emulated yet, so devvm refuses a VM that needs them, in one line naming the VM: network policy, ssh for editors (and so `devvm ssh`, `devvm code` and `--clone`), forwarding ports, SSH agent forwarding, a disk size and the [docker layer](images.md#built-in-layers). Until they are, make Docker VMs with:

```sh
devvm --backend docker --no-network-policy --ports none
```

`--ports none` because every port is forwarded by default; `devvm ports -g none` makes it the default for new VMs. A default disk size you set (`devvm size -g --disk`) doesn't apply to them, and devvm says so (`size_disk_default_unused`).

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

A container runs Docker's init (`--init`) and a sleep under it; nothing starts at boot. After each start devvm runs, as root, what base's services do on Lima: the shared tools linked (below). It never runs the VM's sshd (ssh isn't emulated yet) or Docker's daemon (`dockerd` needs a privileged container).

devvm's commands in a VM (its setup, your shell) run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`.

## Shared tools

Containers can't mount the tools' images (they get no loop devices without `--privileged`), so on Docker each tool image is unpacked into one volume, `devvm-tools-<hash>`, a folder per image, by a container of devvm's with no network, after the tools build. VMs mount the volume read-only and link each of their tools' folders where other backends mount the image, which mise takes as installed. A tool image's key includes the format, so Docker's are built for it, and the share keeps their images as the record of what's built: when the volume is gone, devvm unpacks it again from them, and when `devvm gc` deletes one, its folder goes from the volume at the next start.

The tools build runs in a container from base's image with only its own output folder mounted, as on other backends; it never sees the volume.
