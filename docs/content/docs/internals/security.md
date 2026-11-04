---
title: Security model
weight: 7
---

devvm treats everything in a VM as untrusted, including a coding agent with root, because the VM's user has passwordless sudo. This page explains how devvm keeps that code away from the rest of your machine and what it doesn't protect. [Staying safe](../safety/_index.md) lists [what a VM can reach](../safety/_index.md#what-a-vm-can-reach) and the habits that [keep you safe](../safety/_index.md#the-habits), and [Attack vectors](../safety/attack-vectors.md) goes through common attacks one by one.

All of it rests on the VM's boundary.

## What devvm protects, and how

### Only the project folder

A VM mounts its project folder read-write at the same path. The only other things it mounts from the host are the shared tools and its [agents' config folders](../using/coding-agents.md#mounts), both read-only, so it never mounts your home folder, which Lima's own templates do. A [`--clone`](../using/clone.md) VM doesn't mount the project folder either. It has its own clone, and its work reaches the repo only when you fetch it. When you run devvm in a subfolder and it uses the project folder's VM, that VM mounts nothing more than it already did ([a subfolder's VM](overview.md#a-subfolders-vm)).

devvm refuses a project folder that would hand the VM more than the project:

- A folder that is or holds your home folder, devvm's settings (`~/.config/devvm`), its state (`~/.local/share/devvm`, which holds its binaries and other VMs' disks) or its Lima home (`unsafe_folder`).
- A folder inside devvm's own folders (`unsafe_folder_inside`). A VM in `~/.config/devvm/layers` could rewrite the scripts that every image build runs, and a VM in devvm's data folder could rewrite the files devvm runs on the host.

`--unsafe-folder` mounts such a folder anyway, and devvm says so. A `--clone` VM mounts nothing, so devvm doesn't refuse its folder.

### One VM per folder

A VM belongs to the folder it was made for, which devvm knows by its path and by the folder itself: its device and inode, and on Linux its birth time. When you delete a folder and make it again at the same path, or clone another repo where an old one was, devvm refuses to give the new folder the old folder's VM, so the new folder never quietly gets what's in the old VM's disk. `devvm rm` deletes the old VM and `devvm adopt` gives it to the new folder, and each does so only after you confirm ([a folder made again](../getting-started/commands.md#a-folder-made-again)).

### A project's devcontainer.json

A project's [`devcontainer.json`](../using/devcontainer.md) belongs to the project, and so to the VM, which can rewrite it for the folder's next VM. devvm reads it only when it makes a VM, and uses only what it can do inside the VM. Nothing in it runs on this machine, so `initializeCommand` never runs. Nothing in it reaches this machine's files, because devvm doesn't follow a link out of the folder, or this machine's environment, because devvm refuses `${localEnv:…}` before it makes anything. It can't add a mount or change network policy or other confinement.

Its commands run in the VM as the VM's user, confined like anything else in it. Its features can only be devvm's own layers, so devvm never fetches or runs a feature's script, and it refuses `docker-outside-of-docker`. devvm uses its `image` only when the image's registry gives it to anyone who asks anonymously, so it never uses a container image that only your Docker has, that only your registry login reaches, or that is on this machine or its network.

An existing VM never changes when the file does.

### Read-only and confined builds

devvm builds images with nothing from the host mounted, and it confines their builds so that they reach the internet but not your machine's own services ([details](lima.md#confinement)). Credentials never go into images.

A tool build writes into a folder of its own and never into the shared tools, and devvm checks each tool image, moves it in and writes its record, with its hash ([details](images.md#building-tools)). devvm's VMs live in a Lima home of their own, so Lima settings you made for other VMs never reach them. devvm also refuses a VM with a network it didn't give the VM, because that network would reach around the proxy ([details](lima.md#a-lima-home-of-its-own)).

### `.git` protection

The VM can't change the parts of `.git` that run code on the host, because devvm locks hooks and config with a flag the VM can't clear ([details](git-protection.md)).

### Network policy

A VM with network policy reaches nothing but its own proxy on the host and the [links](ports.md#links-between-vms) you added to it. On macOS its Lima process runs under a macOS sandbox, and on Linux QEMU's network is restricted ([details](lima.md#confinement)). Even root in the VM can't get around that. A link's connections don't use the VM's network, because this machine carries them over ssh, and only to a port that another VM with network policy publishes.

The proxy applies the allowlist, and it refuses the host's own addresses and internal addresses unless a rule names them ([the proxy](network.md#the-proxy)). The one thing that VMs with network policy share is the [shared Nix cache](network.md#the-shared-nix-cache), for those with the nix layer. Only the host writes it, with packages from cache.nixos.org whose signature it checked, and nothing a VM sends is stored, so a VM can make the host fetch but can't change what another VM gets. After each start, devvm checks that the VM still runs confined and can't connect out directly, and it stops a VM that fails the check instead of opening it ([the self-check](network.md#the-self-check)).

### ssh into VMs

ssh and editors reach a running VM by name through the entries that devvm writes ([ssh access by name](../using/ssh.md#ssh-access-by-name)). Each entry is built so that ssh connects only to that VM and the connection gives the VM nothing else of this machine's:

- **Each entry checks the VM's own host keys,** which devvm keeps in the VM's folder, `~/.local/share/devvm/vms/<hash>/run/known_hosts`, as a record of the VM's current start. devvm reads them from the VM through its backend, instead of over the network, once each start, after the VM runs, and the record names the start it was read for, so the keys of an earlier start are read again rather than trusted, whoever started the VM. devvm also removes the record before each start of its own. A VM whose Lima config changed gets new host keys when it boots. A VM whose keys devvm can't read gets no entry, and ssh refuses a host key that doesn't match. devvm's port forwarder, the sessions of links between VMs and a `--clone` VM's git all check the VM against the same record.
- **Only host keys go into the record.** devvm keeps a line the VM shows only when its key decodes and names its own type, which works for every key type ssh has, and it drops certificates, markers such as `@cert-authority`, patterns and anything else. So the VM can't add a line that would make ssh trust another host, or trust a key for another name.
- **Each connection is its own.** The entries set `ControlMaster no`, so a session never rides on a connection that another program opened. Of Lima's own ssh settings for the VM, devvm copies only the address, port, user and login key, because Lima's settings turn host key checks off.
- **The entries turn off everything else your ssh could share with the VM.** They set `ForwardX11 no`, `ForwardX11Trusted no`, `Tunnel no`, `PermitRemoteOpen none`, `AddKeysToAgent no`, `ProxyJump none`, `ProxyCommand none`, `UpdateHostKeys no` and `HashKnownHosts no`, whatever your own `Host *` says. That turns off X11, tunnels, remote SOCKS forwards and adding keys to an agent. `PermitRemoteOpen` needs OpenSSH 8.7 or newer (`ssh -V`), and an older ssh refuses the entries.
- **Your agent is forwarded only as the VM's [SSH agent](../using/ssh.md#agent-forwarding) choice says.**

A session carries the VM's connections through your machine only where your ssh asks for it. The entries can't turn off a `RemoteForward` of yours without also refusing an `ssh -R` you type, so a `RemoteForward` still applies, and you should keep remote forwards out of `Host *`. The VM's sshd allows local forwarding (`-L` and `-D`, which VS Code uses), and it allows remote forwards only on addresses in `127.77.0.0/16`, which devvm keeps for links between VMs, so a remote forward on the VM's `localhost`, as `ssh -R` asks by default, is refused. It allows no tunnels or X11. Root in the VM can change all of that, so it's your ssh that decides.

### Secrets stay on the host

devvm saves secrets in the macOS Keychain, or elsewhere in files only you can read, and it never puts a secret in a process's arguments ([where secrets are saved](network.md#where-secrets-are-saved)). A VM uses only the Claude Code token, when its image has `claude`, and the secrets you bind to it. With network policy, a VM's secrets and its agents' logins stay on your machine, and its proxy puts them only into requests for their own hosts.

The secrets work only for that VM. Its proxy needs the VM's own password, and the VM's placeholders end in a random value of the VM's own, which isn't the password, so a placeholder that the VM sends elsewhere gives nothing away. Another VM, or a program on the host that reaches the proxy's port, gets no secret. The password is in files only you can read, and never in a process's arguments or in devvm's run logs, and only you can read the proxy's logs.

Each VM's proxy reads traffic to the hosts it puts secrets in for with the VM's own CA, which devvm keeps in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/ca/` ([the VM's CA](network.md#the-vms-ca)). Your machine never trusts it, but inside its VM it vouches for any site, so someone with a copy of its key and a place on your network could impersonate sites to that VM. No other VM trusts it, so a copy reaches one VM. Only you can read the keys, they never leave this machine, and devvm keeps them out of Time Machine. Each CA lasts 30 days, and the next one, which the VM already trusts, takes over a week before the current one ends, so a copy goes stale. The certificates that the proxy makes never outlast their CA. Keep devvm's `vms/` folder out of other backup and sync tools as well.

## What it doesn't protect

- **What a project's devcontainer.json runs in its VM.** Its commands run as soon as devvm makes the VM, its image and features decide what's in the VM, and devvm forwards its ports as far as your own settings allow, so the VM is the project's from the start. `devvm --no-devcontainer` skips the file for a project you don't trust that far.
- **Files the VM writes into the project folder.** Review changes before you run project code on the host, such as `npm install`, `make`, editor tasks or hooks in the working tree. `--clone` keeps the VM's changes out until you fetch and merge them, and you should review those commits the same way.
- **VMs without network policy,** because you answered no or passed `--no-network-policy`. They reach anything this machine can, including its services on `localhost` (other VMs' ports, dev servers, databases) and your local network. They get the secrets they use themselves, and those secrets still work wherever the VM kept them after `devvm secret unuse`, `devvm secret rm` or `devvm network on`, so rotate them if that matters.
- **Codex and opencode logins, and Claude Code's MCP secrets, without network policy.** The VM gets `auth.json` as it is, and it gets `~/.claude.json`'s MCP servers with their env values and headers. devvm names the servers that have them ([details](../using/coding-agents.md#copies)).
- **A VM started outside devvm** with `limactl start`, or one running since before `devvm network on`, runs unconfined and reaches anything this machine can ([the self-check](network.md#the-self-check)).
- **What allowed hosts and secrets permit.** The VM can push with a GitHub token it may use, spend Claude quota, or upload to any host on its allowlist. Use fine-grained GitHub tokens and keep allowlists short.
- **What a VM made with a secret.** The proxy puts the secret into any request to that service, so a VM can use it to make credentials of its own. The proxy refuses GitHub REST calls that add a key to your account, a deploy key, a self-hosted runner or a webhook (`403`), but it can't refuse everything, such as GitHub's GraphQL API, an OAuth app grant or other services. If a VM may have done that, rotate the secret and check the service's keys, credentials and runners.
- **What one VM learns of another through the shared Nix cache.** A VM can tell by how fast the cache answers whether some VM fetched a public package, and a VM that fetches many packages fills the cache up to its cap and pushes other VMs' packages out, so they download them again ([the shared Nix cache](network.md#the-shared-nix-cache)).
- **Which server an allowed name reaches, on connections the proxy doesn't read.** The proxy checks the name that a VM's TLS connection asks for (its SNI), not the `Host` inside the encrypted traffic. A CDN that serves many sites on one address can route such a connection to a site that isn't on the allowlist, which is called domain fronting. The proxy reads connections to hosts with a bound secret, and it refuses a request in them that names another host.
- **Ports the VM listens on,** which devvm forwards to this machine's `localhost`: all the TCP ports from 1024 up, unless you list them with [`--ports`](../using/ports-size.md#size-and-ports). A VM could take a port that a program on the host trusts while the port is free, and what the VM serves there reaches whatever connects to it. For example, a page it serves to your browser can reach the internet from your machine, past network policy. A program that binds a port on all addresses after devvm has forwarded it gets its `localhost` connections sent to the VM. [Port forwarding](ports.md) has the forwarder's rules, including the ports it never takes.
- **What a VM does through a link.** A [link between VMs](../using/links.md) joins the two VMs' trust: code in the client can often run code in the target through the service it reaches, such as Postgres's `COPY … TO PROGRAM`, and it then has the target's network, the secrets it may use and its project folder. devvm limits a link to the ports the target publishes, on the target's loopback, and a client with network policy to targets with network policy, but it doesn't limit what the service does ([links between VMs](../safety/network.md#links-between-vms)).
- **Remote forwards you ask for** into a VM, with `RemoteForward` in your ssh config or `ssh -R`. The VM can connect through them to whatever they name on your machine. The VM's sshd refuses them only while root in the VM leaves that setting alone, and it never refuses one on an address in `127.77.0.0/16` ([ssh into VMs](#ssh-into-vms)).
- **What VS Code relays** over Remote-SSH (`devvm code`), which is the ports that programs in the VM listen on and your GitHub sign-in for git's prompts. devvm turns both off in the VM's VS Code settings each time it sets the VM up, but the VM can turn them back on, so turn them off in your own VS Code settings as well ([VS Code](../using/ssh.md#vs-code)).
- **Set-id bits on Linux hosts.** Root in a VM can set the set-user-id and set-group-id bits on files in the project folder. On macOS these bits don't reach the host, because virtiofs drops them, but on Linux they do. A file that a Lima VM writes is yours, so a set-user-id file runs as you for anyone else on the machine who runs it. A set-user-id file that your machine's root owns runs as root for anyone who runs it, you included, and since the VM's writes are yours, such a file came from something else on this machine. devvm names such files when it makes a VM and once a VM is stopped, by `devvm stop` or found stopped at a launch, but not at a launch into a running VM, because the search reads every file's metadata and takes about a second for 200,000 files. It skips `.git` folders, which hold most of a big project's files and whose files nothing runs by their path. It lists root's files apart with `setid_root_files`, and you shouldn't run them but delete them with `sudo rm`. It lists the rest with `setid_files`, and `chmod u-s,g-s FILE` clears their bits. Don't run programs from the project folder outside the VM.
- **Escapes from the VM itself**, through bugs in Lima, Apple's Virtualization framework or QEMU.

Found a way out of a VM? See [SECURITY.md](https://github.com/w3cj/devvm/blob/main/SECURITY.md).
