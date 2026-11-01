---
title: smolvm
weight: 10
---

This page explains how devvm builds and runs VMs on smolvm, and how it emulates what smolvm lacks. [smolvm (experimental)](../backends/smolvm.md) covers what you need to run it and what's different for you.

devvm emulates four [features](backends.md#features) that smolvm lacks, and each emulation has been reviewed. They are [network policy](#network-policy), [ssh for editors](#ssh-for-editors) (`devvm host`, `devvm code` and `--clone`), [forwarding ports](#ports), every one or only the ones you list, and [turning SSH agent forwarding on or off](#ssh-agent-forwarding) after a VM is made.

## Its machines

On macOS, smolvm keeps every machine in your own smolvm state, because devvm can't give it a folder of its own (`--state-dir` is Linux only). So devvm keeps its machines apart from yours by a label. Every machine devvm makes carries the label `devvm.data=<devvm's data folder>`, and devvm lists, starts, stops, changes and deletes only the machines with its label. If one of your machines has a name devvm wants, devvm refuses with `smolvm_machine_foreign` and never touches it. devvm also runs smolvm with its usage journal off (`SMOLVM_USAGE=0`), and it never lets `SMOLVM_MACHINE_NAME` choose a machine.

On Linux, smolvm's state belongs to devvm alone, in `~/.local/share/devvm/smolvm/state`.

Images aren't machines. Each image is a pack (`smolvm pack create --from-vm`) in `~/.local/share/devvm/smolvm/images/<key>`. A project VM is a machine made from its image's pack, and the machine reads the pack when its disk grows, so devvm doesn't delete an image that a VM was made from (`image_in_use`). [Cleaning smolvm's caches](../backends/smolvm.md#cleaning-smolvms-caches) covers the packs' size and smolvm's own caches.

## Building images

`base` starts from Ubuntu's container image (`ubuntu:26.04`), by the digest devvm pins with base's package snapshot (`ContainerDigest` in `internal/ubuntu/ubuntu.go`). That digest is in base's key and its record. With `--from`, the build starts from that image by its recorded digest, and devvm refuses an image that only Docker has (`smolvm_from_registry_only`).

Before `assets/layers/base.sh` runs, devvm adds what Ubuntu's cloud image has and a container lacks: `sudo`, CA certificates, `bash` and the VM's user, and on Alpine also OpenRC and ssh's client. After base's scripts, it adds `sshd` without host keys, because each VM makes its own.

The VM's user has your user name and uid, as on Lima, with its home at `/home/<you>.guest`. The project folder shows as your uid's, and only that user can write it. The user has no password that works (`*`).

The scripts run as that user, with stand-ins for `systemctl` and `rc-update` while they run. Base's services are enabled the Lima way, and devvm starts them itself ([below](#no-init-system)). The stand-ins are gone before the image is packed.

Builds use smolvm's default network, which reaches the internet but not the host's loopback. A pack keeps its build machine's network, CPUs and memory, so devvm sets all three on every VM it makes. Base's CPUs, memory and disk come from `base.yaml`, which sets 4 CPUs, 8 GiB of memory and a 100 GiB disk. On a host with fewer than 4 CPUs, base and the VMs you give no size get as many CPUs as the host has, and when 8 GiB would leave the host less than 2 GiB, they get half of its memory ([base's size](images.md#bases-size)).

devvm's own scripts for these steps are in `base`'s key, together with the backend, so smolvm's images are never Lima's. They are `prepare.sh`, `shims.sh` and `sshd.sh`. `sshd.sh` installs sshd after base's scripts, from the Ubuntu snapshot they pin, and keeps the snapshot's package lists for layers.

When a build fails, devvm keeps its machine, stopped (`smolvm_script_failed`), so you can [look inside](../backends/smolvm.md#looking-inside).

## What devvm sets on a VM

- **Mounts:** devvm mounts the project folder (read-write) and agents' folders (read-only) at their host paths, and the shared tools at `/mnt/devvm-tools` (read-only). It also mounts `/var/lib/docker` on the machine's own disk, because the [docker layer](../using/images.md#built-in-layers)'s storage needs it. Every VM gets that mount. devvm refuses a path with a colon, because smolvm splits mounts on colons. Files a VM makes in the project folder are yours on the host, with mode 0600.
- **Network:** without network policy, a VM has smolvm's own network, which reaches the internet but not the host's loopback. With network policy, devvm confines it as [below](#network-policy).
- **Size:** a VM gets the size you ask for, or base's. Its disk only grows.
- **Ports:** devvm's forwarder forwards them, and smolvm forwards none of them ([below](#ports)). The only port smolvm forwards is the VM's [ssh](#ssh-for-editors).
- **SSH agent forwarding:** you choose it when the VM is made (`--ssh-agent`), and each start gives the VM an agent ([below](#ssh-agent-forwarding)).
- **Environment:** with network policy, the VM gets the proxy's settings, as on Lima. The proxy variables come from the VM's proxy file, `~/.local/share/devvm/smolvm/vms/<vm>.proxy` (mode 600), which devvm writes at each start with network policy and removes at a start without it. Each of devvm's commands in the VM names the file (`smolvm machine exec --secret-file`) and smolvm reads it, so the proxy's password is in no process's arguments. A `smolvm machine exec` of your own gets the variables only in a login shell. `machine create --secret-file` would last, but smolvm ignores it for a VM made from a pack.

## Network policy

Network policy is emulated, and reviewed. A VM with [network policy](../safety/network.md) reaches only its proxy, as on Lima. devvm builds this from three parts, which it sets while the VM is stopped ([design](design.md#network-policy-on-smolvm)):

- **An allow list of the host alone** (`100.96.0.1/32`, the gateway). smolvm refuses connections anywhere else, and all UDP but DNS.
- **DNS closed.** smolvm answers every query with `NXDOMAIN` itself, without asking anyone, and names resolve in the proxy instead. smolvm still asks your resolver about one name, `devvm.invalid`, because it needs that name to filter on. That's a narrow way out, as the VM chooses when and how to ask ([details](design.md#network-policy-on-smolvm)).
- **devvm's proxy deciding each connection** (smolvm's mediated egress). Before smolvm connects anywhere, it asks the proxy. The proxy takes over connections to itself and refuses the rest, such as connections to the host's other ports, which `devvm network log` shows as a `BLOCKED` direct connection. The proxy's address in the VM is `http://100.96.0.1:<port>`.

The policy fails closed. With the proxy gone, nothing gets out, and once a VM has started this way, `smolvm machine start` refuses to start it without a decider. A VM that someone started with a decider of their own, or whose allow list someone widened while it runs, isn't confined, so devvm shows it as `unenforced` and stops it ([the self-check](network.md#the-self-check)). A UDP allow that someone adds (`smolvm machine update --allow-udp`) lets UDP past the decider, and smolvm doesn't show it, so don't add one to devvm's machines. `devvm network off` removes all three parts at the VM's next start.

## ssh for editors

ssh for editors is emulated, and reviewed. `devvm host`, `devvm code`, `--clone` and the [ssh config entries](../using/ssh.md#ssh-access-by-name) work as they do on Lima ([design](design.md#ssh-for-editors-on-smolvm)).

devvm starts an sshd of its own after each start of the VM ([below](#no-init-system)), with a config of devvm's own, `/etc/devvm/ssh/sshd_config`, which only root can change. The image's own sshd config doesn't apply.

The VM gets a new ed25519 host key at each start, so no two VMs made from one image share one. devvm reads the key through smolvm (`smolvm machine exec`), not over the network, and pins it in `~/.config/devvm/known_hosts`, as on Lima. ssh refuses a key that doesn't match.

Only the VM's own key logs in, and only as the VM's user. The key is `~/.local/share/devvm/smolvm/vms/<vm>.key` (mode 600), which devvm makes at the VM's first start with ssh. Its public half is that VM's only authorized key, so the key logs in to no other VM. Passwords, root and other users can't log in. The ssh config entries offer only that key (`IdentitiesOnly yes`), and never your agent's keys.

Port 22 in the VM is forwarded from a port on the host's `127.0.0.1` and `::1` alone. devvm chooses that port at the start and keeps it while it stays free on every address, so it's never one a program on the host has on any address, and never one of the VM's listed ports. If the port is one of them anyway, devvm refuses the start (`smolvm_ssh_port_forwarded`). With network policy, it's the port devvm records for the VM. The ssh config names the port, so connect by name. devvm answers for the port only once that start's sshd runs.

An ssh session gives the VM no way out as long as your ssh doesn't ask for one. A session can carry the VM's connections out only through what your machine's ssh asks for, such as remote forwards, X11, a tunnel or your agent. devvm's entry turns off X11, tunnels, remote SOCKS forwards and adding keys to an agent, whatever a `Host *` of yours says, and it asks for no remote forward. devvm can't turn off a `RemoteForward` of yours or an `ssh -R` you type, so don't forward into a VM you don't trust. The sshd also allows local forwarding (`ssh -L` and `-W`, which VS Code uses), where the VM makes the connection from its own network, so with network policy it reaches only the proxy. It allows remote forwards only on addresses in `127.77.0.0/16`, which devvm keeps for links between VMs, so a remote forward on the VM's `localhost` is refused. Those settings only protect an honest VM, because root in the VM can change its sshd.

Agent forwarding follows the entry, as on Lima. `ForwardAgent` is set by the [SSH agent](../using/ssh.md#agent-forwarding) choice that the VM's current start was made with. smolvm also forwards an agent of its own ([below](#ssh-agent-forwarding)).

Sessions get the same environment as devvm's own commands: `/etc/environment` and, with network policy, the proxy's settings, in a login shell or not.

A VM that has been running since a start without ssh gets no entry, and `devvm host` tells you to restart it (`vm_no_ssh`). A VM whose image has no sshd starts and runs without ssh, with a warning (`vm_no_sshd`). A VM made after `devvm rebuild` has it.

## Ports

Forwarding ports, every one or only the ones you list, is emulated, and reviewed ([design](design.md#ports-on-smolvm)). smolvm's own forwards reach only servers listening on all of the VM's addresses (`0.0.0.0`), and a machine has at most 64 of them. Each one also shares a port that a program on your machine has on every address (`SO_REUSEADDR`), and takes that program's `localhost` connections. So smolvm forwards none of a VM's ports, and devvm's forwarder runs on your machine instead, by the rules that [Port forwarding](ports.md) sets out for every backend.

Unlike smolvm's own forwards, devvm's forwarder also reaches servers on the VM's loopback (`127.0.0.1` or `::1`), both with every port (`--ports all`, the default) and with listed ones (`--ports 5173,8000-8010`).

The forwarder reaches the VM over its [ssh](#ssh-for-editors), with the VM's own key and the host key devvm read through smolvm. The VM sees each forwarded connection only as one to its own loopback, and with network policy it still reaches only its proxy.

Which ports a VM forwards is in devvm's record for the VM, so `devvm ports` changes the record and no forward of smolvm's. [Its process and files](ports.md#its-process-and-files) lists the record's path and the forwarder's log.

The forwarder starts after the VM's sshd and ends with it. When you change a running VM's ports, devvm starts it again with the new ones, unless the VM was started outside devvm since devvm last started it. Such a VM runs no sshd of devvm's, so devvm warns about that instead (`ports_no_ssh`). A VM without ssh, because its image has no sshd, forwards no ports, with the same warning.

## SSH agent forwarding

smolvm forwards an SSH agent into a VM made with `--ssh-agent` for as long as the VM runs, whether or not you're connected. The agent is at `/tmp/ssh-agent.sock` in the VM, and it reaches the agent that `SSH_AUTH_SOCK` names when the VM starts. smolvm sets forwarding only when it makes a VM, so devvm emulates turning it on or off later (`devvm ssh-agent on`, `devvm` or `off`), and the emulation is reviewed ([design](design.md#ssh-agent-toggling-on-smolvm)):

- **Every new VM is made forwarding,** and each start gives it the agent its choice names: yours (`SSH_AUTH_SOCK` in your shell), devvm's own, or none. If you chose yours and your shell has none, the start stops (`smolvm_no_agent`).
- **None** is a path where no socket can be (`/dev/null/devvm-no-agent`). The VM's connections to its agent fail, so it lists no keys and can't add any.
- **A change** applies from the VM's next start (`devvm restart`), to smolvm's agent, your shell and the ssh config's entry alike, as on Lima.
- **A VM started outside devvm** gets the agent your shell has, whatever its choice, because `smolvm machine start` from your shell passes it on, and fails without one. When devvm, `devvm host`, `devvm code` or `devvm history export` finds the VM running on a process devvm didn't start, it stops the VM and says so (`agent_unchosen_stop`), unless the VM uses your agent anyway. The next devvm starts it with its own choice. Until then the VM has your shell's agent, so start devvm's VMs with devvm.
- **A VM made without forwarding** can't have it turned on (`agent_fixed`). Run `devvm rm`, then `devvm --ssh-agent`.

## No init system

A machine runs its image as a container, and nothing starts at boot. After each start, devvm does these as root:

- It makes the loop devices that the shared tools are mounted with, because `/dev` has none, and then mounts the tools.
- It starts the VM's [sshd](#ssh-for-editors) with new host keys, logging to `/var/log/devvm-sshd.log`.
- In a VM with the [docker layer](../using/images.md#built-in-layers), it starts Docker's daemon in the background, logging to `/var/log/dockerd.log`. The layer's script also starts the daemon when you add the layer to a running VM. devvm's setup restarts it the same way when a launch changes the VM's [network policy](../safety/network.md) settings, which on Alpine the daemon reads from `/etc/conf.d/docker`.
- In a VM with the [nix layer](images.md#the-nix-layer), it starts Nix's daemon in the background, logging to `/var/log/nix-daemon.log`, with its pid in `/run/nix-daemon.pid`. The daemon reads its settings from `/etc/conf.d/nix-daemon`, which devvm's setup writes with the VM's proxy when network policy is on and removes when it's off, and the setup restarts the daemon when that file changes. The layer's script also starts the daemon when you add the layer to a running VM.

devvm's commands in a VM, such as its setup and your shell, run as the VM's user with the environment a login would give: `HOME`, `USER`, and `/etc/environment`, which smolvm doesn't read.

## File watchers

smolvm watches each mounted folder on the host, with FSEvents on macOS, and for each change it fires an event on the matching file in the VM, so watchers there see [your edits](../backends/smolvm.md#whats-different), though not your deletions, within about a tenth of a second, even on a busy host. An event for a new file names the file to a watch on its folder. An event for a change to an existing file reaches a watch on that file, but a watch on its folder gets only an event for the folder itself, without the file's name. A change made within a few seconds of the file's creation is the exception, because macOS still reports it together with the creation, and the folder's watch sees it as a new file.

## Shared tools

On smolvm, the shared tools are images of their own, in uncompressed EROFS, because smolvm's kernel mounts no other format. A tool image's key includes the format, so Lima's and smolvm's VMs each get their own tool images from the one share.
