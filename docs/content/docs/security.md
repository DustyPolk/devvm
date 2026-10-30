---
title: Security model
weight: 25
---

devvm treats everything in a VM as untrusted, including a coding agent with root, because the VM's user has passwordless sudo. This page describes what that code can and can't reach on your machine, and [Attack vectors](attack-vectors.md) goes through common attacks one by one.

All of it rests on the VM's boundary. On the experimental [Docker](docker.md) backend, a VM is a container, not a VM. On Linux it shares your machine's kernel, and on macOS it shares Docker Desktop's one VM with every other container. What this page describes holds there only as far as a container's boundary does ([isolation](docker.md#isolation-a-container-is-not-a-vm)).

| | |
| --- | --- |
| The project folder | read and write; with `--clone`, not mounted: the VM has its own clone ([details](clone.md)) |
| `.git` hooks and config | locked (macOS); what a lock can't stop is reported, not prevented ([details](git-protection.md)); with `--clone`, out of reach |
| Your agents' config (skills, settings, logins) | its folders read-only, its files copied; never its history ([details](coding-agents.md)) |
| The rest of your home folder, other projects | not mounted |
| Your SSH keys | only with `devvm ssh-agent on`, while you're connected (on smolvm, while the VM runs; on Docker, in ssh sessions alone); with `devvm ssh-agent devvm`, only the keys you added to devvm's own agent ([details](ssh.md#agent-forwarding), [smolvm](smolvm.md#ssh-agent-forwarding)) |
| The network, with network policy | allowlisted hosts only, through a proxy on this machine ([details](network.md)) |
| Claude and GitHub tokens, Codex and opencode logins, Claude Code's MCP secrets, with network policy | kept on this machine; the proxy puts them into requests ([details](network.md#secrets)) |
| The network, without network policy | everything this machine reaches, its own services on `localhost` included |

## What devvm protects, and how

### Only the project folder

A VM mounts its project folder read-write at the same path. The only other things it mounts from the host are the shared tools and its [agents' config folders](coding-agents.md#mounts), both read-only, so it never mounts your home folder, which Lima's own templates do. A [`--clone`](clone.md) VM doesn't mount the project folder either. It has its own clone, and its work reaches the repo only when you fetch it.

devvm refuses a project folder that would hand the VM more than the project:

- A folder that is or holds your home folder, devvm's settings (`~/.config/devvm`), its state (`~/.local/share/devvm`, which holds its binaries and other VMs' disks) or its Lima home.
- A folder inside devvm's own folders. A VM in `~/.config/devvm/layers` could rewrite the scripts that every image build runs, and a VM in devvm's data folder could rewrite the files devvm runs on the host.

`--unsafe-folder` mounts such a folder anyway, and devvm says so. A `--clone` VM mounts nothing, so devvm doesn't refuse its folder.

### One VM per folder

A VM belongs to the folder it was made for, which devvm knows by its path and by the folder itself: its device and inode, and on Linux its birth time. When you delete a folder and make it again at the same path, or clone another repo where an old one was, the new folder never quietly gets the old folder's VM and what's in its disk, because devvm refuses it. `devvm rm` deletes the old VM and `devvm adopt` gives it to the new folder, and each does so only after you confirm ([a folder made again](commands.md#a-folder-made-again)).

### A project's devcontainer.json

A project's [`devcontainer.json`](devcontainer.md) belongs to the project, and so to the VM, which can rewrite it for the folder's next VM. devvm reads it only when it makes a VM, and uses only what it can do inside the VM. Nothing in it runs on this machine, so `initializeCommand` never runs. Nothing in it reaches this machine's files, because devvm doesn't follow a link out of the folder, or this machine's environment, because devvm refuses `${localEnv:…}` before it makes anything. It can't add a mount or change network policy or other confinement.

Its commands run in the VM as the VM's user, confined like anything else in it. Its features can only be devvm's own layers, so devvm never fetches or runs a feature's script, and it refuses `docker-outside-of-docker`. devvm uses its `image` only when the image's registry gives it to anyone who asks anonymously, so it never uses an image that only your Docker has, that only your registry login reaches, or that is on this machine or its network. Its docker-in-docker never takes network policy away. On Docker, where a VM with the docker layer can't have network policy, devvm still asks, and answering yes refuses the VM. An existing VM never changes when the file does.

### Read-only and confined builds

devvm builds images with nothing from the host mounted, and it confines their builds so that they reach the internet but not your machine's own services ([details](lima.md#confinement)). On Docker Desktop that confinement is emulated and reviewed, through a proxy of devvm's for each build ([details](docker.md#building-images)). Credentials never go into images. A tool build writes into a folder of its own and never into the shared tools, and devvm checks each tool image, moves it in and records its hash ([details](images.md#building-tools)). devvm's VMs live in a Lima home of their own, so Lima settings you made for other VMs never reach them. devvm also refuses a VM with a network it didn't give the VM, because that network would reach around the proxy ([details](lima.md#a-lima-home-of-its-own)).

### `.git` protection

The VM can't change the parts of `.git` that run code on the host, because devvm locks hooks and config with a flag the VM can't clear ([details](git-protection.md)).

### Network policy

A VM with network policy reaches nothing but its own proxy on the host. On macOS its Lima process runs under a macOS sandbox, and on Linux QEMU's network is restricted ([details](lima.md#confinement)). Even root in the VM can't get around that.

On [smolvm](smolvm.md#network-policy), which is experimental, network policy is emulated and reviewed. smolvm's allow list holds only the host, and its DNS answers only one name, `devvm.invalid`. smolvm still asks your resolver about that name, which is a narrow way out ([details](smolvm.md#network-policy)). smolvm asks the proxy about every connection before it makes the connection.

On [Docker](docker.md#network-policy), which is experimental, network policy is emulated and reviewed. The container is on an internal network of its own, and the only other thing on that network is a relay of devvm's that passes the container's connections to the proxy's socket on your machine. No DNS query leaves the network. The proxy and your tokens stay on your machine, and the container gets no added capability and no `NET_RAW`. A Docker VM with the [docker layer](docker.md#the-docker-layer) gets no network policy.

The proxy applies the allowlist, and it refuses the host's own addresses and internal addresses unless a rule names them. After each start, devvm checks that the VM still runs confined and can't connect out directly, and it stops a VM that fails the check instead of opening it ([the self-check](network.md#the-self-check)).

### ssh into VMs

ssh and editors reach a running VM by name, and ssh checks the VM against its own host keys, which devvm reads through the VM's backend instead of over the network. ssh refuses a key that doesn't match ([details](ssh.md#ssh-access-by-name)). Your agent is forwarded only as the VM's [SSH agent](ssh.md#agent-forwarding) choice says.

A session carries the VM's connections through your machine only where your ssh asks for it. devvm's entries turn off X11, tunnels, remote SOCKS forwards and adding keys to an agent, whatever your own `Host *` says, but they can't turn off a `RemoteForward` of yours or an `ssh -R` you type. The VM's sshd also allows only local forwarding, but root in the VM can change that.

On [smolvm](smolvm.md#ssh-for-editors), ssh is emulated and reviewed. devvm runs the VM's sshd itself, with new host keys at each start, and a key of the VM's own is the only login. On [Docker](docker.md#ssh-for-editors), ssh is emulated and reviewed in the same way, but no sshd listens and the container publishes nothing. Each connection gets an sshd of its own, run through `docker exec`, which only those who can run `docker` can reach. The agent is forwarded only into ssh sessions and is never mounted into the container.

### Tokens stay on the host

devvm saves tokens in the macOS Keychain, or elsewhere in files only you can read, and it never puts a token in a process's arguments ([details](network.md#saving-and-binding-tokens)). A VM uses only `claude`'s token, when its image has `claude`, and the tokens you bind to it. With network policy, a VM's tokens and agents' logins stay on your machine, and its proxy puts them only into requests for their own hosts.

The tokens work only for that VM. Its proxy needs the VM's own password, and the VM's placeholders end in a random value of the VM's own, which isn't the password, so a placeholder the VM sends elsewhere gives nothing away. Another VM, or a program on the host that reaches the proxy's port, gets no token. The password is in files only you can read and never in a process's arguments or devvm's run logs, and only you can read the proxy's logs.

The proxy's CA (`~/.local/share/devvm/ca`) is how the proxy reads traffic to the hosts it puts tokens in for. Your machine never trusts the CA, but inside a VM it vouches for any site, so someone with a copy of its key and a place on your network could impersonate sites to your VMs. Only you can read the key, and devvm keeps it out of Time Machine. The CA lasts 30 days, and devvm makes a new one when the current one has under a week left, so a copy goes stale. Keep that folder out of other backup and sync tools as well.

## What it doesn't protect

- **What a project's devcontainer.json runs in its VM.** Its commands run as soon as devvm makes the VM, its image and features decide what's in the VM, and devvm forwards its ports as far as your own settings allow, so the VM is the project's from the start. `devvm --no-devcontainer` skips the file for a project you don't trust that far.
- **Files the VM writes into the project folder.** Review changes before you run project code on the host, such as `npm install`, `make`, editor tasks or hooks in the working tree. `--clone` keeps the VM's changes out until you fetch and merge them, and you should review those commits the same way.
- **VMs without network policy**, where you answered no or passed `--no-network-policy`. They reach anything this machine can, including its services on `localhost` (other VMs' ports, dev servers, databases) and your local network. They get the tokens they use themselves, and those tokens still work wherever the VM kept them after `devvm secret unuse`, `devvm secret rm` or `devvm network on`, so rotate them if that matters.
- **Codex and opencode logins, and Claude Code's MCP secrets, without network policy.** The VM gets `auth.json` as it is, and it gets `~/.claude.json`'s MCP servers with their env values and headers. devvm names the servers that have them ([details](coding-agents.md#copies)).
- **A VM started outside devvm** with `limactl start`, or one running since before `devvm network on`, runs unconfined and reaches anything this machine can ([the self-check](network.md#the-self-check)). On Docker, a VM started with `docker start`, or connected to another network, runs unconfined until devvm next finds it and stops it. On smolvm, `smolvm machine start` refuses a VM that devvm started with network policy, unless it's given a decider of its own. It also gives any VM your shell's SSH agent, whatever the VM's choice, until devvm next finds the VM running and stops it ([details](smolvm.md#ssh-agent-forwarding)).
- **What allowed hosts and tokens permit.** The VM can push with a GitHub token it may use, spend Claude quota, or upload to any host on its allowlist. Use fine-grained tokens and keep allowlists short.
- **What a VM made with a token.** The proxy puts the token into any request to that service, so a VM can use it to make credentials of its own. The proxy refuses GitHub REST calls that add a key to your account, a deploy key, a self-hosted runner or a webhook (`403`), but it can't refuse everything, such as GitHub's GraphQL API, an OAuth app grant or other services. If a VM may have done that, rotate the token and check the service's keys, tokens and runners.
- **Which server an allowed name reaches, on connections the proxy doesn't read.** The proxy checks the name that a VM's TLS connection asks for (its SNI), not the `Host` inside the encrypted traffic. A CDN that serves many sites on one address can route such a connection to a site that isn't on the allowlist, which is called domain fronting. The proxy reads connections to hosts with a bound token, and it refuses a request in them that names another host.
- **Ports the VM listens on**, which devvm forwards to this machine's `localhost`: all of them from 1024 up, unless you list them with [`--ports`](commands.md#size-and-ports). Only TCP ports are forwarded. A VM could take a port that a program on the host trusts while the port is free, and what the VM serves there reaches whatever connects to it. For example, a page it serves to your browser can reach the internet from your machine, past network policy. devvm's forwarder works the same on every backend. It never takes a port that a program on your machine has on any address, or a port that devvm records for a VM (the ssh, proxy and decider ports it chooses, and other VMs' listed ports), and every connection through it starts on your machine ([ports](images.md#ports)). A program that binds a port on all addresses after devvm has forwarded it gets its `localhost` connections sent to the VM.
- **Remote forwards you ask for** into a VM, with `RemoteForward` in your ssh config or `ssh -R`. The VM can connect through them to whatever they name on your machine. The VM's sshd refuses them only while root in the VM leaves that setting alone ([ssh into VMs](#ssh-into-vms)).
- **What VS Code relays** over Remote-SSH (`devvm code`), which is the ports that programs in the VM listen on and your GitHub sign-in for git's prompts. devvm turns both off in the VM's VS Code settings each time it sets the VM up, but the VM can turn them back on, so turn them off in your own VS Code settings as well ([VS Code](ssh.md#vs-code)).
- **Set-id bits on Linux hosts.** Root in a VM can set the set-user-id and set-group-id bits on files in the project folder. On macOS these bits don't reach the host, because virtiofs drops them, but on Linux they do. A file that a Lima or smolvm VM writes is yours, so a set-user-id file runs as you for anyone else on the machine who runs it. A file that root writes in a Docker VM belongs to your machine's root, so a set-user-id file it makes runs as root for anyone who runs it, you included. devvm names such files at each launch and stop. It lists root's files apart with `setid_root_files`, and you shouldn't run them but delete them with `sudo rm`. It lists the rest with `setid_files`, and `chmod u-s,g-s FILE` clears their bits. Don't run programs from the project folder outside the VM.
- **Escapes from the VM itself**, through bugs in Lima, Apple's Virtualization framework or QEMU, or on smolvm through bugs in smolvm and libkrun, which also enforce its network policy.
- **Escapes from a container, on Docker.** A kernel or Docker flaw reaches your machine on Linux, and on macOS it reaches Docker Desktop's VM, with every other container and what Desktop shares with them. Root in a container has the host kernel's system calls, filtered by Docker's seccomp profile, and Docker's default capabilities. Docker VMs without network policy reach the internet and your network, and on Docker Desktop they also reach your Mac's services on `localhost` through `host.docker.internal` ([details](docker.md#what-devvm-sets-on-a-vm)). Whoever can run `docker` on the engine can exec into any of them as root and reach their sshd as devvm does, and on Docker Desktop that is every program of yours.
- **Docker VMs with the docker layer**, which is experimental and runs under Sysbox on a Linux engine. Such a VM has a container's boundary with a user namespace of its own, but Sysbox's code is part of that boundary, and root in it writes to the project folder as your machine's root, as in any Docker VM. It gets no network policy, because root in it can change its own network. It reaches whatever a VM without network policy does, except other containers and your machine's loopback ([details](docker.md#the-docker-layer)). devvm never runs a container `--privileged`.

Found a way out of a VM? See [SECURITY.md](https://github.com/w3cj/devvm/blob/main/SECURITY.md).
