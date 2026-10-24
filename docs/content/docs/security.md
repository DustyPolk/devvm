---
title: Security model
weight: 25
---

devvm treats everything in a VM as untrusted, a coding agent with root included: the VM's user has passwordless sudo. This is what that code can and can't reach on your machine.

| | |
| --- | --- |
| The project folder | read and write; with `--clone`, not mounted: the VM has its own clone ([details](clone.md)) |
| `.git` hooks and config | locked (macOS); what a lock can't stop is reported, not prevented ([details](git-protection.md)); with `--clone`, out of reach |
| Your agents' config (skills, settings, logins) | its folders read-only, its files copied; never its history ([details](coding-agents.md)) |
| The rest of your home folder, other projects | not mounted |
| Your SSH keys | not reachable: VMs have no SSH agent ([details](ssh.md#agent-forwarding)) |
| The network, with network policy | allowlisted hosts only, through a proxy on this machine ([details](network.md)) |
| Claude and GitHub tokens, Codex and opencode logins, Claude Code's MCP secrets, with network policy | kept on this machine; the proxy puts them into requests ([details](network.md#secrets)) |
| The network, without network policy | everything this machine reaches, its own services on `localhost` included |

## What devvm protects, and how

### Only the project folder

A VM mounts its project folder, read-write, at the same path, and nothing else from the host but the shared tools and its [agents' config folders](coding-agents.md#mounts), read-only: not your home folder, which Lima's own templates mount. A [`--clone`](clone.md) VM doesn't mount the project folder either: it has its own clone, and its work reaches the repo only when you fetch it.

devvm refuses a project folder that would hand the VM more than the project:

- one that is or holds your home folder, devvm's settings (`~/.config/devvm`), its state (`~/.local/share/devvm`, with its binaries and other VMs' disks) or its Lima home;
- one inside devvm's own folders: a VM in `~/.config/devvm/layers` could rewrite the scripts every image build runs, one in its data folder the files devvm runs on the host.

`--unsafe-folder` mounts such a folder anyway, saying so. `--clone` mounts nothing, so it isn't refused.

### One VM per folder

A VM belongs to the folder it was made for: its path, and the folder itself (its device and inode, and on Linux its birth time). A folder deleted and made again at the same path, another repo cloned where an old one was, never quietly gets the old folder's VM and what's in its disk: devvm refuses it. `devvm rm` deletes it and `devvm adopt` gives it to the new folder, each only after you confirm ([a folder made again](commands.md#a-folder-made-again)).

### Read-only and confined builds

Images are built with nothing from the host mounted, and their builds are confined: they reach the internet but not your machine's own services ([details](lima.md#confinement)). Credentials never go into them. devvm's VMs live in a Lima home of their own, so Lima settings you made for other VMs never reach them, and devvm refuses a VM with a network it didn't give it, which would reach around the proxy ([details](lima.md#a-lima-home-of-its-own)).

### `.git` protection

The VM can't change the parts of `.git` that run code on the host: devvm locks hooks and config with a flag the VM can't clear ([details](git-protection.md)).

### Network policy

A VM with network policy reaches nothing but its own proxy, on the host: on macOS its Lima process runs under a macOS sandbox, on Linux QEMU's network is restricted ([details](lima.md#confinement)). Even root in the VM can't get around that. The proxy applies the allowlist and refuses the host's own and internal addresses unless a rule names them. After each start devvm checks that the VM still runs confined and can't connect out directly, and stops a VM that fails the check rather than open it ([the self-check](network.md#the-self-check)).

### Tokens stay on the host

With network policy, a VM's tokens and agents' logins stay on your machine, and its proxy puts them into requests for their own hosts only. They work only for that VM: its proxy needs the VM's own password, and its placeholders end in a random value of the VM's own (not the password: a placeholder the VM sends elsewhere gives nothing away), so another VM or a program on the host that reaches the proxy's port gets no token. The password is in files only you can read, never in a process's arguments or devvm's run logs, and the proxy's logs are yours alone.

The proxy's CA (`~/.local/share/devvm/ca`) is how it reads traffic to the hosts it puts tokens in for. Your machine never trusts it, but inside a VM it vouches for any site: someone with a copy of its key and a place on your network could impersonate sites to your VMs. The key is readable only by you and kept out of Time Machine, and the CA lasts 30 days (devvm makes a new one when it has under a week left), so a copy goes stale. Keep that folder out of other backup and sync tools too.

## What it doesn't protect

- **Files the VM writes into the project folder.** Review changes before you run project code on the host (`npm install`, `make`, editor tasks, hooks in the working tree). `--clone` keeps them out until you fetch and merge them; review those commits the same way.
- **VMs without network policy**, where you answered no or passed `--no-network-policy`: they reach anything this machine can, its services on `localhost` (other VMs' ports, dev servers, databases) and your local network included.
- **A VM started outside devvm** (`limactl start`), or running since before `devvm network on`, runs unconfined, and reaches anything this machine can ([the self-check](network.md#the-self-check)).
- **What allowed hosts and tokens permit.** The VM can push with a GitHub token it may use, spend Claude quota, or upload to any host on its allowlist. Use fine-grained tokens and keep allowlists short.
- **What a VM made with a token.** The proxy puts the token into any request to that service, so a VM can use it to make credentials of its own. The proxy refuses GitHub REST calls that add a key to your account, a deploy key, a self-hosted runner or a webhook (`403`), but not everything: GitHub's GraphQL API, an OAuth app grant, other services. If a VM may have done that, rotate the token and check the service's keys, tokens and runners.
- **Which server an allowed name reaches, on connections the proxy doesn't read.** The proxy checks the name a VM's TLS connection asks for (its SNI), not the `Host` inside the encrypted traffic. A CDN that serves many sites on one address (domain fronting) can route such a connection to a site that isn't on the allowlist. Connections to hosts with a bound token are read, and a request in them that names another host is refused.
- **Ports the VM listens on**, which are forwarded to this machine's `localhost`: all of them, unless you list them with [`--ports`](commands.md#size-and-ports). A VM could take a port a program on the host trusts.
- **Set-id bits on Linux hosts.** Root in a VM can set the set-user-id and set-group-id bits on files in the project folder. On macOS they don't reach the host (virtiofs drops them). On Linux they do, and the files are yours, so a set-user-id one runs as you for anyone else on the machine who runs it. devvm names such files when it creates a VM for the folder (`chmod u-s,g-s FILE` clears them). Don't run programs from the project folder as another user.
- **Escapes from the VM itself**: bugs in Lima, Apple's Virtualization framework or QEMU.
