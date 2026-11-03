---
title: What devvm keeps on this machine
weight: 11
---

devvm, the tools it runs and its backends keep files on this machine outside your project folders. This page lists each of them: what it is, who makes it, where it is, whether devvm can make it again, and how it's cleaned up. Most of it is in devvm's data folder, `~/.local/share/devvm`, and devvm can make all of that again except your exports. The backends' own caches hold more, and they can also hold your own downloads.

## devvm space

`devvm space` shows what devvm keeps on this machine, on every backend, and how much disk each kind of thing takes. It changes nothing and starts no VM, and it gives each backend 30 seconds to answer. `devvm space --json` prints the same as JSON, for scripts ([devvm space --json](#devvm-space---json)). `devvm space --backend NAME` shows only that backend's rows and caches, with their total, and asks no other backend. The first table has a row for each kind of thing devvm has, followed by a total:

| Column | |
| --- | --- |
| `WHAT` | `VMs`, `images`, `tools gnu` or `tools musl` for the shared tools' builds for each C library, `history` for the exports of agents' history, `network logs`, `run logs`, `mise`, `Nix cache` for the [shared Nix cache](toolchains.md#nix-projects), `leftover state` for what devvm keeps of VMs that their backend no longer has, and `state` for the rest of devvm's data folder |
| `BACKEND` | the backend that has them, for VMs, images and shared tools |
| `COUNT`, `SIZE` | how many there are and the disk they take, in powers of 1024 (`2.1G`) |
| `KEPT` | `remade` when devvm makes it again when it's needed, `only copy` for VMs and exports, which nothing else has, `log` for logs, `state` for what devvm knows of its VMs and images, and `leftover` |
| `GC` | what `devvm gc` frees of it now, which for the Nix cache is what it takes beyond its cap |
| `FREED BY` | what frees the rest, such as `devvm rm`, `gc --backend lima`, which deletes all of a backend's images, or `gc --nix-cache` |
| `RETAIN` | the [retention setting](settings.md#retention) that applies to the row when it's on, such as `30d`, with what it would delete now in brackets, and `-` when none is on |

Sizes count each file's blocks once, as `du` does. On macOS a Lima VM's disk shares its image's blocks until the VM writes them, so those blocks count for both the VM and its image, and the total can be more than the disk they take. A shared tool's build shows `-` as its backend when devvm can't tell which backend it's for. When a backend that one of your VMs uses doesn't answer, devvm says so, and its VMs and images aren't counted.

### The backends' caches

The second table lists the caches that belong to the backends, with the backend, what each cache holds, how many entries it has, its size and where it is. These are Lima's download cache, smolvm's caches on macOS and the Docker volumes with devvm's label that aren't one VM's own, such as the shared tools' volume. devvm's builds fill them, but Lima's and smolvm's caches can also hold your own downloads, so they aren't in the total. The sections below say what each one holds.

devvm never prunes these caches on its own. `devvm gc --caches` frees devvm's share of them, which is what devvm's builds put there and nothing of devvm's needs. For each backend in turn, it lists the entries it would delete with their sizes and asks. Without a terminal it refuses unless you give `--force`, which frees them without asking. With `--backend NAME`, it asks only about that backend's cache.

### devvm space --json

`devvm space --json` prints both tables as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes, and devvm still says on stderr what it says without `--json`. `devvm` has a row of the first table each, with sizes in bytes:

| Field | |
| --- | --- |
| `what`, `backend`, `count`, `size` | the row's `WHAT`, `BACKEND`, `COUNT` and `SIZE`, with `backend` left out when the row has none |
| `sized` | whether devvm knows the size, which `SIZE` shows as `-` when it doesn't |
| `kept` | the row's `KEPT` |
| `gc` | what `devvm gc` frees of it now |
| `rest` | the row's `FREED BY` |
| `retain`, `retain_frees` | the retention setting that applies to the row and what it would delete now, both left out when none is on |

`total` and `gc` are the totals row's `SIZE` and `GC`. `backends` has the second table's rows, each with its `backend`, `name`, `path`, `count`, `size` and `sized`. `unknown` names the backends that one of your VMs uses that didn't answer, and `strays` has the [VMs devvm has no state for](#vms-devvm-has-no-state-for), each with its `backend`, `name` and the `data` folder its label names. devvm leaves both out when they're empty.

```json
{
  "version": 1,
  "devvm": [
    {
      "what": "VMs",
      "backend": "lima",
      "count": 2,
      "size": 5368709120,
      "sized": true,
      "kept": "only copy",
      "gc": 0,
      "rest": "devvm rm"
    },
    {
      "what": "history",
      "count": 3,
      "size": 3145728,
      "sized": true,
      "kept": "only copy",
      "gc": 0,
      "rest": "gc --history",
      "retain": "30d",
      "retain_frees": 1048576
    }
  ],
  "total": 5371854848,
  "gc": 0,
  "backends": [
    {
      "backend": "lima",
      "name": "download cache",
      "path": "/Users/you/Library/Caches/lima",
      "count": 1,
      "size": 629145600,
      "sized": true
    }
  ]
}
```

## devvm's data folder

devvm keeps its state in `~/.local/share/devvm`, or in `$XDG_DATA_HOME/devvm` when `XDG_DATA_HOME` is set. Only you can read it, because each time devvm runs, it makes this folder and the settings folder, `~/.config/devvm`, yours alone (mode 700), whatever mode they had, so nothing inside either one is open to others. If devvm can't, for example because someone else owns a folder, it warns and goes on. Whenever devvm writes into the folders that hold history, logs, keys and secrets, which are `exports/`, `logs/`, `logs/runs/`, `run/`, `ca/` and the settings folder's `secrets/`, it makes them only yours again as well, whatever mode they had. Everything in it is devvm's own, and devvm makes all of it again when it's needed, except the exports, which are the only copy of a deleted VM's agent history.

| Path | What it is | How it's cleaned up |
| --- | --- | --- |
| `vms/<vm>` | each VM's [state](settings.md#devvms-state): its image, folder, backend and settings | `devvm rm` deletes it with the VM |
| `images/` | each image's record, the [rebuild counters](images.md#rebuilding), the digests of the container images that `--from` bases start from, and in `locks/` an empty file for each image that the devvm building it locks | `devvm gc` and `devvm images rm` delete an image's record and its lock file with the image |
| `tools/` | the [shared tools](images.md#shared-tools), one read-only image per tool version, each with its checksum beside it in a `.sha256` file and what it's built for in a `.target` file | `devvm gc` deletes the versions that no VM has and that aren't the current ones, `devvm gc --tools` also the current ones that no VM has, except those built in the last hour, and `devvm gc --tools --recent` those too |
| `mise.lock`, `mise.lock.toml`, `mise.lock.checked` | the tools lock, which holds the tools' versions and their checksums, the config it was resolved from, and when devvm last checked upstream for newer versions | devvm replaces them when it checks again |
| `mise/<version>` | the mise that devvm runs on this machine to resolve the tools lock, one folder per mise version | `devvm gc` deletes the folders of older versions, which an upgrade of devvm leaves |
| `exports/` | the agents' [history](coding-agents.md#history) that `devvm history export` and `devvm rm` save, one `.tar.gz` per export, each with a `.folder` file beside it that names its VM's folder | `devvm gc --history` deletes them, or with `--older-than 30d` those that old, and nothing else does |
| `logs/runs/` | the [run logs](troubleshooting.md#a-command-fails), one per devvm run that runs a command | devvm keeps the newest 20 |
| `logs/<vm>.jsonl`, `logs/<vm>.proxy.log` | a VM's network log, which holds its proxy's decisions, and its proxy's own output. Before a log would grow past 10 MiB, the proxy moves it to the same name with `.1` added, replacing the older one, so each log takes at most about 20 MiB, and `devvm network log` reads both | `devvm rm` deletes them with the VM, `.1` files included |
| `ca/` | the certificate authority that VMs trust for their proxy | devvm makes a new one when the old one has less than a week of its 30 days left |
| `nix-cache/` | the [shared Nix cache](toolchains.md#nix-projects): the packages that VMs with network policy and the `nix` layer downloaded from Nix's binary cache, once for all of them | the VMs' proxies and `devvm gc` delete its least recently used files while it takes more than its cap, `nix-cache.size`, which is 10 GiB when it's off, and `devvm gc --nix-cache` deletes all of it |
| `run/` | files for running VMs and for builds, such as sandbox profiles, proxy pids and the ports lock | devvm deletes a VM's files when it deletes the VM |
| `agent.sock` | the socket of [devvm's own SSH agent](ssh.md#devvms-own-agent) | it takes no space |
| `oci-build.lock` | an empty file that the devvm building a base from a container image on Lima locks, so that one such build runs at a time | it stays, and takes no space |
| `tools.lock` | an empty file that the devvm building shared tools or updating the tools lock locks, so that one at a time writes to `tools/` | it stays, and takes no space |
| `groups.lock` | an empty file that each devvm joining or leaving a [group](links.md#groups), or changing a group's links, locks, so that they take turns | it stays, and takes no space |
| `making.lock` | an empty file that each devvm making a VM locks, so that `devvm gc --tools` can tell a VM is [being made](images.md#cleaning-up) | it stays, and takes no space |
| `retention.lock` | an empty file that the devvm applying the [retention settings](settings.md#retention) locks, so that one at a time applies them | it stays, and takes no space |
| `lima/` | devvm's own [Lima home](../backends/lima.md#devvms-lima-home), with its VMs and images, unless `DEVVM_LIMA_HOME` moves it | see [Lima](#lima) |
| `lima-vms/` | what the Lima backend sets on each VM: which ports it forwards, and its port forwarder's files | `devvm rm` deletes a VM's files with the VM |
| `smolvm/` | the smolvm backend's images, which are packs, and what it sets on each VM | see [smolvm](#smolvm) |
| `docker/` | what the Docker backend sets on each VM, the shared tools it unpacked, and devvm's Linux build | see [Docker](#docker) |

The [retention settings](settings.md#retention), which are all off until you set them, make devvm delete some of these on its own: `logs.runs` keeps that many run logs instead of 20, `history.age` and `history.count` delete old exports of agents' history, `tools.unused-age` deletes current shared tools that no VM has had for that long, and `orphans.age` forgets VMs that their backend no longer has. `nix-cache.size` sets the shared Nix cache's cap, and each time retention runs, it deletes the cache's least recently used files while it takes more than that. Retention also writes `retention.run` in the data folder, when it last ran, and keeps a `.idle` file beside each current shared tool that no VM has, which records since when.

A build also makes a temporary folder in the data folder, `tools-build.<random>` for the shared tools, and devvm deletes it when the build ends, even after a failure or Ctrl+C.

### Shared tools

Each tool version is built once for each kind of VM that uses it, so `tools/` can hold more than one build of the same version. Lima's VMs mount compressed images, smolvm's need uncompressed ones, and Docker unpacks its own builds into a volume. A base on Alpine also needs its tools built for its C library, musl, rather than glibc. A node 24 that both a Lima VM and a smolvm VM use is therefore two builds, and `devvm images` lists both. [Cleaning up](images.md#cleaning-up) says which ones `devvm gc` deletes.

### Exports

`devvm rm` saves the history of a VM whose image has an agent before it deletes the VM, so each export in `exports/` can be the only copy of that VM's transcripts and sessions. devvm deletes an export only when you ask it to. `devvm history` lists the exports, which are named `<vm>-<date>-<time>.tar.gz`, with their VM, its folder, their date and their size. `devvm gc --history` lists them by their VM and date, then deletes them, each with the `.folder` file beside it, after asking in a terminal, and `devvm gc --history --older-than 30d` deletes only those written 30 days ago or more. gc leaves anything else in the folder alone, such as an export that `devvm history export` is still writing.

### Leftovers of a VM deleted outside devvm

When you delete a VM with its backend's own command instead of `devvm rm`, such as `limactl delete`, `smolvm machine delete` or `docker rm`, devvm still keeps the VM's state, its logs, its files in `run/` and its backend's folder, its ssh config entry and its host keys, and `docker rm` leaves the VM's volumes and networks in Docker. `devvm gc` also keeps the shared tools that the VM's state names. `devvm rm` deletes these only together with the VM, so once the VM is gone it says the folder has no VM.

`devvm gc --orphans` deletes what devvm keeps for such a VM, as `devvm rm` does after it deletes a VM. That includes the VM's Docker volumes, which hold its home folder, and for a `--clone` VM the `devvm` remote in your repo. gc first asks each VM's backend whether it still has the VM, lists the VMs it would forget with their project folders, and in a terminal asks. Under a Docker VM, the list also names each volume, network, relay container and committed image of the VM's that gc deletes with it, because a gone VM's home volume holds its home folder and gc doesn't export it first. It never deletes a VM, and it leaves a VM's state alone in these cases, saying why:

- **The backend doesn't answer**, for example because it isn't installed or Docker isn't running. When a backend that one of your VMs uses can't list its images, gc warns and leaves all of that backend's VMs' state alone, as it does the rest of that backend's ([cleaning up](images.md#cleaning-up)).
- **The VM was made elsewhere.** devvm records where each new VM's backend keeps it: the Lima home, which `DEVVM_LIMA_HOME` moves, your home folder for smolvm on macOS, or the Docker engine that `DOCKER_HOST` or `DOCKER_CONTEXT` chooses. When this run looks somewhere else, the VM may still exist there. A VM whose state doesn't record it counts as made in the backend's default place, so while `DEVVM_LIMA_HOME`, `DOCKER_HOST` or `DOCKER_CONTEXT` is set, gc leaves such a VM alone.
- **Its state changed in the last hour,** because the VM may still be being made.

Forgetting a VM also removes your other VMs' [links](links.md) to it, as `devvm rm` does. `devvm gc --orphans` also removes links to VMs whose state is already gone, naming each one, and deletes the sockets that a link session leaves on your machine when it's killed, which are the ones nothing listens on.

Once a VM's state is gone, `devvm gc` treats its shared tools like any others. `devvm space` counts the state of VMs that their backend doesn't list as `leftover state`, and each socket a link session left counts there too.

### VMs devvm has no state for

A VM can also be the other way around: the backend has it, with devvm's label, but devvm has no state for it. Usually it belongs to another devvm, such as one run with another `XDG_DATA_HOME` for a test or a sandbox, which on macOS shares your smolvm machines. `devvm space` names each one, with the data folder its label names, and devvm never deletes it, because the other devvm may still use it. To delete one yourself, run `smolvm machine delete --name NAME` for smolvm, `devvm limactl delete NAME` for Lima, or `docker rm -f NAME` for Docker, followed by `docker volume rm NAME-home`.

## Your settings

`~/.config/devvm` holds your settings, your own layers and features, your company's CA certificates, the ssh config for running VMs and their host keys, and on Linux hosts your secrets. [Your settings](settings.md#your-settings) lists each file. devvm never deletes any of them.

## Lima

The Lima backend's VMs and images are in devvm's Lima home, `~/.local/share/devvm/lima`. Each image is a stopped VM named `devvm-img-<key>`, and each project VM is a clone of one. On macOS a clone's disk shares its image's blocks until the VM writes them, so `du` counts those blocks twice. On Linux hosts a VM's disk is an overlay of its image's. `devvm rm` deletes a VM, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend lima` deletes all of them.

Lima keeps a download cache of its own, `~/Library/Caches/lima` on macOS and `~/.cache/lima` on Linux. When devvm builds base, Lima downloads Ubuntu's cloud image into that cache, which is 600 to 900 MB for each release, and on macOS it also keeps a copy converted to a raw disk, which takes about 2.4 GB more. The cache is Lima's and it isn't in devvm's Lima home, so your own Lima VMs' downloads share it. devvm deletes something in it only when you run `devvm gc --caches`. Lima keeps each download in a folder named by the SHA-256 of its address, and gc frees only the folders of Ubuntu's cloud images at the address devvm downloads base from, for its Ubuntu release, that no image record of devvm's names and that haven't changed in the last hour. A cloud image that one of your own Lima VMs downloaded from that same address can be among them, and Lima downloads it again the next time it needs it. No VM needs its entry once it's made, because Lima copies the disk out of the cache. `limactl prune` deletes the whole cache, your own downloads included, so devvm never runs it.

## smolvm

The smolvm backend keeps each image as a pack in `~/.local/share/devvm/smolvm/images/<key>`, and base's is about 450 MB. A VM is a smolvm machine made from its image's pack. The machine reads the pack when its disk grows, so devvm doesn't delete an image that a VM was made from. `smolvm/vms/` holds what devvm sets on each VM. `devvm rm` deletes a VM, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend smolvm` deletes all of them but those a VM was made from.

On macOS, smolvm keeps every machine, devvm's and your own, in its own folders under `~/Library/Caches/smolvm`, and devvm marks its machines with a label. That folder also holds smolvm's caches, which grow with every pack that a machine is made from. smolvm unpacks a pack's disk once, which takes 1 to 3 GB, and keeps it for the next machine made from that pack. Those copies stay after devvm deletes the pack. Each one's name holds the SHA-256 of its pack, and devvm records the SHA-256 of every pack it makes, so `devvm gc --caches` frees the copies of devvm's packs that no machine of devvm's was made from and that haven't changed in the last hour. A copy whose pack is gone and isn't in that record can't be tied to devvm, so it stays, as do the copies of your own packs and smolvm's other caches. Because the folder also holds the machines' own disks, never delete it by hand. [Cleaning smolvm's caches](../backends/smolvm.md#cleaning-smolvms-caches) says what smolvm's own commands clean. On Linux, smolvm's state and caches belong to devvm alone, in `~/.local/share/devvm/smolvm/state`.

## Docker

The Docker backend keeps its VMs and images in Docker's engine. Each VM is a container with a volume for its home, and a `--clone` VM has another for its clone. devvm's images are Docker images named `devvm-<hash>:<key>`. The shared tools are unpacked into a volume, `devvm-tools-<hash>`. Network policy and the docker layer add networks, and the docker layer adds volumes for the VM's own engine. Everything devvm makes there has the label `devvm.data=<devvm's data folder>` ([what you'll see in Docker](../backends/docker.md#what-youll-see-in-docker)). `devvm rm` deletes a VM's container, volumes and networks, `devvm gc` and `devvm images rm` delete images, and `devvm gc --backend docker` deletes all of them but those a VM was made from.

Docker also keeps the container image that base starts from, which it pulls as it would any image you pull. devvm doesn't delete it, because it isn't labelled and you may use it.

`devvm gc --caches` deletes the volumes with devvm's label that no VM of devvm's has, such as the home volume that `docker rm` left of a VM, after listing them and asking. It keeps the shared tools' volume, every VM's own volumes, and the volumes of a VM that devvm has state for, which includes a VM that is still being made. It never prunes and never touches what doesn't have devvm's label.

`~/.local/share/devvm/docker` holds what devvm sets on each VM, which tools it unpacked into the tools volume, the sockets that network policy's relays reach the proxy through, the files of a build's proxy, which a failed build keeps together with its proxy's logs, and on macOS the copy of devvm's Linux build that a release fetches for network policy's relays.

## Outside devvm's folders

devvm also changes a few things outside its own folders:

- **The Keychain:** on macOS, the [secrets](../safety/network.md#saving-and-binding-secrets) you save are Keychain items under the service `devvm`. `devvm secret rm` deletes one.
- **`~/.ssh/config`:** when you let it, devvm adds the line `Include ~/.config/devvm/ssh_config` so that ssh reaches VMs by name ([SSH and editors](ssh.md#ssh-access-by-name)).
- **Time Machine:** on macOS, devvm keeps `ca/` in its data folder out of Time Machine's backups, because a copy of its key elsewhere could impersonate any site to VMs.
- **devvm itself:** the install script puts each version in `~/.local/lib/devvm/<version>` and links `~/.local/bin/devvm` to it. A newer install removes older versions that nothing runs ([upgrading](../getting-started/install.md#upgrading)). Homebrew keeps devvm in its own folders.

[Uninstalling](../getting-started/install.md#uninstalling) removes all of it.
