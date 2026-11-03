---
title: Attack vectors
weight: 3
---

This page lists the common ways that code you didn't write gets to run on a developer's machine or gets data off it, and what devvm does about each one. devvm stops most of them on its own, and the [habits](_index.md#the-habits) close most of what's left.

## How devvm handles each attack

The rest of this page sorts the attacks by how much of each one devvm stops:

| | |
| --- | --- |
| [Blocked](#blocked) | The attack can't work, because what it's after isn't in the VM or the VM can't reach it. |
| [Kept inside the VM](#kept-inside-the-vm) | The attacker's code runs, but it runs in the VM, where it reaches only the project folder and what you gave the VM. |
| [Narrowed, not closed](#narrowed-not-closed) | devvm closes part of the attack, and the entry says what's still open. |
| [Left to you](#left-to-you) | devvm doesn't stop the attack, and the entry says what to do instead. |

Some protections hold only with [network policy](network.md) or only on macOS, and each entry names its conditions. Everything here rests on the VM's boundary. How devvm does each of these is in the [security model](../internals/security.md).

## Blocked

### Credentials on this machine

- **SSH keys.** Malware on a developer's machine looks in `~/.ssh` first, because a private key opens every server and repo that trusts it. A VM never mounts `~/.ssh` or anything else from your home folder, so there's no key in it to copy. [Agent forwarding](#secrets-and-keys-the-vm-may-use), which you turn on yourself, lets the VM use your keys but never copy them.
- **Cloud, cluster and registry logins.** Worms that steal credentials collect `~/.aws`, `~/.config/gcloud`, `~/.azure`, `~/.kube/config`, `~/.docker/config.json` and the npm publish token in `~/.npmrc`. A VM mounts none of them, so a package that goes looking finds nothing. Secrets you keep in the project folder are a different case, which [Left to you](#files-the-vm-writes) covers.
- **The Keychain, browsers and password managers.** Stealers read saved passwords, cookies and login sessions from browser profiles and the Keychain. The VM has no browser profile of yours, and it can't reach this machine's Keychain.
- **Your coding agents' history.** Agent transcripts hold what you pasted into them, such as secrets and code from other projects. Your history never goes into a VM, and each VM's own history stays in that VM ([coding agents](../using/coding-agents.md)).
- **Secrets, with network policy.** The VM gets a placeholder instead of each secret, such as your GitHub token or Claude Code's token, and the proxy on this machine puts the real value only into requests for that secret's own hosts ([secrets](network.md#secrets)). A placeholder copied out of the VM is worthless elsewhere, because it ends in a random value of that VM's own. Another VM or a program on this machine can't spend the secret through the proxy either, because the proxy needs the VM's own password. Codex and opencode logins and Claude Code's MCP secrets work the same way, and devvm refreshes OAuth logins on this machine, so a VM never holds a refresh token.

### Git running code on this machine

- **Git hooks and config, on macOS.** A VM can write the project folder, so without protection it could add a hook such as `.git/hooks/post-checkout`, or set a command in `.git/config`: `core.fsmonitor`, `core.hooksPath`, `core.sshCommand`, `core.pager`, `core.editor`, a `filter.*` or `diff.*.textconv` driver, a `credential.helper` or a `!` alias. Your next git command on this machine would run it. devvm locks hooks and config with a flag that even root in the VM can't clear ([.git protection](git-protection.md)). Linux hosts have no such flag, which [Left to you](#the-platform) covers.
- **The shell prompt.** Many shell prompts and editors run `git status` as soon as you `cd` into a folder or open it, and `git status` runs `core.fsmonitor`. The lock covers that as well, so opening the folder runs nothing that the VM wrote into `.git`.
- **devvm's own git commands.** devvm runs git on this machine itself, for example to read your name and email for the VM, to check a `--clone` VM's repo for uncommitted changes, and to fetch from the VM. A VM can write a repo that git on this machine reads, such as the project folder's on Linux or a repo inside a folder it mounts, so devvm's git runs none of the programs a repo's config can name. It turns off the repo's fsmonitor, its hooks, including those its config defines, and its filters, it never enters a submodule for `git status`, and it uses no transport but ssh, so `git status`, a fetch or a push of devvm's runs nothing that the VM wrote into the repo. It ignores the git variables of the shell you run devvm in, such as `GIT_DIR` and `GIT_CONFIG_GLOBAL`, so it never works on another repo or config by mistake, and it never asks you anything (`GIT_TERMINAL_PROMPT=0`). Its fetches and pushes reach the VM with ssh in batch mode (`-o BatchMode=yes`), so the VM's ssh server can't ask you for a password. They accept only the host key devvm pinned for that VM and forward neither your agent nor any port, so a repo's config that sends them to another host gets nowhere.

### Getting out over the network, with network policy

- **Sending data to the attacker's server.** Most attacks end by sending what they stole to a server the attacker runs. A VM with network policy reaches only its proxy, and the proxy allows only the hosts on the VM's allowlist. devvm leaves storage and paste sites out of the default allowlist on purpose, because they're easy ways to send data out ([allowlists](network.md#allowlists)).
- **DNS tunnelling.** An attacker can encode data into names and look them up at a DNS server of their own. A VM sends no DNS queries of its own, because names resolve in the proxy on this machine. smolvm leaves [one narrow channel](#network).
- **Services on this machine and your network.** Code in a VM could attack what developers leave open on `localhost`, such as databases, dev servers, Docker's API and other VMs' ports. It could also attack your router, a NAS, or a cloud provider's metadata service at `169.254.169.254`. The proxy refuses this machine's own addresses and internal ones, such as loopback, private, link-local and CGNAT ranges, unless a rule names the address ([the proxy](../internals/network.md#the-proxy)).
- **DNS rebinding and address tricks.** A name can resolve to a public address when it's checked and to an internal one when it's dialed, and an internal address can hide in an IPv6 or NAT64 form. The proxy dials only the address it checked, and it sees through NAT64 forms ([the proxy](../internals/network.md#the-proxy)).
- **Going around the proxy.** Root in a VM can change its routes and firewall, but devvm confines the VM from outside it, so nothing in the VM reaches anything but the proxy ([confinement](../internals/lima.md#confinement)). devvm catches a VM started outside devvm, or one given a second network, with [the self-check](../internals/network.md#the-self-check), and stops it. On smolvm, network policy is emulated and reviewed.

### Spreading beyond the VM

- **Your agents' config.** A rogue agent could add a hook to `~/.claude/settings.json` or a skill to `~/.claude/skills`, so that it runs the next time you use the agent on this machine. devvm mounts the agents' config folders read-only, which this machine enforces, and copies their files in on every launch, so nothing the VM changes goes back ([mounts](../using/coding-agents.md#mounts)).
- **Other projects and other VMs.** Each project folder gets its own VM with its own disk, and a VM mounts only its own project folder. Each VM's proxy has its own password, so one VM can't use another's secrets. A VM reaches a service in another VM only through a [link between VMs](../using/links.md) that you add in its folder, to a port that the other VM's folder publishes. A link joins the two VMs' trust, because code in one can often run code in the other through its service, so a VM with network policy links only to a VM with network policy, and nothing in a VM can add a link ([links between VMs](network.md#links-between-vms)).
- **Images, layers and devvm itself.** Malware could try to make itself permanent by editing an image, a layer script or devvm's own files. devvm builds images with nothing from this machine mounted, and the shared tools are read-only in every VM. devvm also refuses a project folder that holds its own folders or sits inside them, so a VM can't reach the layer scripts that every build runs or the files devvm runs on this machine ([only the project folder](../internals/security.md#only-the-project-folder)).
- **A folder made again.** If you delete a repo and clone another at the same path, the new folder never gets the old VM's disk or anything left in it. devvm knows a folder by its inode as well as its path, so it refuses and offers `devvm rm` or `devvm adopt` ([a folder made again](../getting-started/commands.md#a-folder-made-again)).
- **A devcontainer.json that reaches this machine.** Other tools run a devcontainer's `initializeCommand` on the host. devvm never runs it, refuses `${localEnv:…}`, adds no mounts from the file and doesn't follow a link out of the folder ([devcontainer.json](../using/devcontainer.md#security)).
- **Impersonating a VM.** ssh checks a VM against host keys that devvm reads through the VM's backend instead of over the network, so a program that answers on the VM's port in its place is refused ([ssh access by name](../using/ssh.md#ssh-access-by-name)).

## Kept inside the VM

The attacks in this section run the attacker's code, and devvm doesn't stop that code from running. It runs in the VM, as the VM's user, who has passwordless sudo. From there it reaches the project folder, or with [`--clone`](../using/clone.md) the VM's own clone of it, and the secrets the VM uses, which are placeholders with network policy. With network policy it reaches only the hosts on the allowlist, and without it the whole network. Everything in [Blocked](#blocked) still holds. What the code writes into the project folder can still reach you when a program on this machine runs it, which [Left to you](#files-the-vm-writes) covers.

### Install and build scripts

- **npm, yarn, pnpm and bun lifecycle scripts.** A package's `preinstall`, `install`, `postinstall` and `prepare` scripts run any command when the package is installed, and most npm attacks use them. pnpm's build approvals (`pnpm approve-builds`) and `--ignore-scripts` narrow which packages run scripts, and they're still worth using, but a package you approved runs its scripts all the same, even one compromised after you approved it. In a VM, those scripts reach only what this section's introduction lists.
- **Python packages.** Installing a source distribution runs its `setup.py` or its build backend, and a `.pth` file in `site-packages` runs code every time Python starts.
- **Ruby gems.** A gem with a native extension runs its `extconf.rb` when it's installed, and Bundler can install gems straight from a git repo.
- **Rust build scripts and procedural macros.** `build.rs` and procedural macros run on `cargo build` and `cargo check`, and also when rust-analyzer opens the project, so opening the folder in an editor is enough to run them. With `devvm code`, VS Code's extensions, rust-analyzer among them, run in the VM ([devvm host and devvm code](../using/ssh.md#devvm-host-and-devvm-code)).
- **Go modules.** Go has no install scripts, but cgo passes flags to the C compiler, `go generate` runs commands, a `toolchain` line in `go.mod` makes Go download another toolchain, and a module's `init` functions run in every test and program that imports it.
- **Makefiles, task runners and `curl | sh`.** A `Makefile`, a `justfile` or a project's install line from its README runs whatever its author wrote.

### Bad packages and images

- **Typosquatting, dependency confusion and made-up names.** Attackers publish packages under names one typo away from popular ones, under a company's internal package names with a higher version, and under names that AI coding agents invent and that attackers then register. Their code runs as the scripts above do.
- **Compromised maintainers.** A trusted package can turn malicious when its maintainer's account is taken over or a new maintainer joins with bad intent, as event-stream (2018) and xz (2024) did. Updating such a package runs its new code in the VM.
- **Malicious images.** An image from [`--from`](../using/from.md) or from a devcontainer's `image` runs its own code as root in the build VM ([the image is untrusted](../internals/images.md#the-image-is-untrusted)). devvm uses only images that the registry gives to anyone who asks anonymously, never one that only your Docker or your registry login reaches, and it confines builds so they reach the internet but not this machine's services ([read-only and confined builds](../internals/security.md#read-only-and-confined-builds)).
- **A devcontainer.json's commands and features.** A project's `postCreateCommand` and its other commands run in the VM as the VM's user, as soon as devvm makes the VM. Its features can only be devvm's own layers, so devvm never fetches or runs a feature's script. `devvm --no-devcontainer` skips the file for a project you don't trust that far ([devcontainer.json](../using/devcontainer.md)).

### Rogue agents

- **Prompt injection.** Instructions hidden in a README, an issue, a code comment, a web page, a dependency's docs or an MCP tool's output can turn a coding agent against you. devvm can't tell an agent's own work from injected work, so it limits what the agent reaches instead. An attack like this needs private data, untrusted input and a way to send the data out. With network policy, the only way out is the hosts on the allowlist, which [Left to you](#the-network) covers.
- **Agents with their checks off.** An agent run with `claude --dangerously-skip-permissions` or `codex --dangerously-bypass-approvals-and-sandbox` acts without asking, and a mistaken `rm -rf ~` or `sudo` command reaches only the VM. The project folder is still the agent's to change, so commit often, or use [`--clone`](../using/clone.md) to keep the agent's work apart until you fetch it.
- **Malware that stays.** Anything the attacker leaves in the VM, such as a cron job, a changed shell startup file or a backdoored program, stays in that VM and goes into no other. devvm never deletes a VM on its own, because it may hold unsaved work, so save what you need and run `devvm rm`, then make the VM again.
- **Mining and resource abuse.** A VM's CPUs, memory and disk are capped by its size, which `devvm size` shows and changes ([size](../using/ports-size.md#size)).

## Narrowed, not closed

### Secrets and keys the VM may use

- **Worms that use your GitHub token.** Shai-Hulud (2025) spread through npm install scripts. It collected npm, GitHub and cloud credentials, then used them to publish repos and new versions of packages. A VM holds no npm or cloud credentials, and with network policy its GitHub token is a placeholder. The proxy still puts the real GitHub token into every request to GitHub's hosts, so a worm in the VM can push, make repos and do whatever else the GitHub token's scope allows. Use a fine-grained GitHub token scoped to the repos the VM needs, or bind none ([saving and binding secrets](network.md#saving-and-binding-secrets)).
- **Minting credentials with a secret.** A VM that can use a secret can use it to make another credential that outlasts it. The proxy refuses GitHub REST calls that add an SSH key, a deploy key, a self-hosted runner or a webhook with `403`, but it can't refuse everything, such as GitHub's GraphQL API, an OAuth app grant or other services. If a VM may have done this, rotate the secret and check the service's keys, access tokens and runners.
- **Your SSH agent.** While [agent forwarding](../using/ssh.md#agent-forwarding) is on, anything in the VM can use the agent's keys to push or log in to servers, though it can't copy them. The VM has the agent while you're connected, and on smolvm the whole time the VM runs. `devvm ssh-agent devvm` limits it to the keys you add to [devvm's own agent](../using/ssh.md#devvms-own-agent), and an agent that asks you to approve each use, such as 1Password's or Secretive, narrows it further.

### Git

- **Submodules with their own config.** The VM writes `.git/index` to commit, so it can add a submodule whose config sets `core.fsmonitor`. Host `git status` reads that config before devvm's next launch finds the submodule and locks it. [`devvm git-protect strict`](git-protection.md#strict) stops `git status` and `git diff` from looking inside submodules, but `git submodule` commands still enter them, so use `--clone` for a repo with submodules you don't trust.
- **Replacing `.git`.** The VM can rename `.git` and put a new one in its place, or add a `commondir` that points git at another config. devvm reports these on each launch but doesn't prevent them, so check what it names before you run git in the folder ([what a lock can't stop](../internals/git-protection.md#what-a-lock-cant-stop)).

### Network

- **Domain fronting.** The proxy checks the host name that a TLS connection asks for, but not the `Host` inside the encrypted traffic, so a CDN that serves many sites on one address can route an allowed connection to a site that isn't allowed. The proxy reads the connections to hosts with a bound secret, and it refuses a request in them that names another host.
- **DNS from smolvm.** On the experimental [smolvm](../internals/smolvm.md#network-policy) backend, a VM's DNS answers only `devvm.invalid`, but smolvm still asks your resolver about that name, which is a narrow way out.
- **Ports the VM listens on.** devvm forwards the VM's ports to this machine's `localhost`, so a VM could take a port that a program on this machine trusts while that port is free. devvm's forwarder never takes a port that a program on this machine already has, or one that devvm records for a VM, but a program that binds a port on all addresses after devvm forwarded it gets its `localhost` connections sent to the VM. `devvm ports` limits what a VM forwards, and `devvm ports none` forwards nothing ([ports](../using/ports-size.md#ports)).
- **VS Code's relays.** Over Remote-SSH, VS Code forwards ports and answers git's prompts with your GitHub sign-in. devvm turns both off in the VM's VS Code settings, but the VM can turn them back on, so turn them off in your own VS Code settings as well ([VS Code](../using/ssh.md#vs-code)).

### This machine

- **Set-id files on Linux hosts.** Root in a VM can set the set-user-id bit on a file in the project folder, and on Linux hosts that bit reaches the host, so the file runs as its owner for whoever runs it. devvm names such files at each launch and stop, and [Troubleshooting](../using/troubleshooting.md#devvm-names-set-id-files) says how to clear them.
- **The proxy's CA key.** Inside a VM, devvm's CA vouches for any site, so someone with a copy of its key and a place on your network could impersonate sites to your VMs. Only you can read the key, devvm keeps it out of Time Machine, and each CA lasts 30 days, so a copy goes stale. Keep `~/.local/share/devvm/ca` out of other backup and sync tools as well ([secrets stay on this machine](../internals/security.md#secrets-stay-on-the-host)).

## Left to you

### Files the VM writes

The VM can write anything in the project folder, and devvm locks only `.git`. Any other file there that a program on this machine runs, or reads as its config, is the VM's to change. [`--clone`](../using/clone.md) keeps all of these away from this machine until you fetch the VM's work and review it.

- **Hooks in the working tree.** Hooks kept in the project, such as husky's `.husky/` with `core.hooksPath` pointing there, run on this machine, and so do an `include.path` or a hook symlink that leads into the folder. devvm warns about them when it makes the VM ([not covered](git-protection.md#not-covered)).
- **Agent config in the project.** Claude Code reads a project's `.claude/settings.json`, whose hooks run commands, and `.mcp.json`, which starts MCP servers, and agents follow a project's `CLAUDE.md` and `AGENTS.md`. Running an agent on this machine in the folder after the VM has worked in it runs what the VM wrote, so run your agents in the VM.
- **Editor and tool config.** VS Code runs `.vscode/tasks.json` tasks set to `runOn: folderOpen` and starts the interpreters and formatters that `.vscode/settings.json` names. direnv loads `.envrc`, and mise reads `mise.toml`'s environment, hooks and tasks. Keep VS Code's workspace trust on, and use direnv's and mise's own approval prompts.
- **Project code and binaries.** `npm install`, `make`, a test run or `./bin/tool` on this machine runs whatever the VM wrote. Run them in the VM instead.
- **Symlinks out of the folder.** The VM can create a link in the project folder that points at a file of yours, such as `~/.ssh/id_ed25519`. A program on this machine that follows it, such as an upload, an archive or an agent reading files, reads your file instead. devvm itself never follows a link out of the folder.
- **Backdoors in code you merge.** Treat the VM's commits as an untrusted contributor's, and review them before you merge, deploy or publish them.
- **Secrets in the project folder.** A `.env` file, a project `.npmrc` or a cloud key in the folder is in the VM. With network policy it can reach only the hosts on the allowlist, and without network policy it can reach anywhere. Keep secrets out of the project folder, or use `--clone`, which copies no untracked files into the VM.

### The network

- **Allowed hosts.** Anything an allowed host accepts can carry data out. github.com accepts a push from anyone with an account, including an attacker with an account of their own, and the npm registry accepts packages the same way. Keep allowlists short, and check what a VM reached with `devvm network log`.
- **What your secrets allow.** A VM can push with a GitHub token it may use and spend Claude quota. Use a fine-grained GitHub token, and bind only the secrets a VM needs.
- **VMs without network policy.** A VM without network policy reaches everything this machine can, including its services on `localhost` and your local network. It gets the secrets it uses as they are, and those secrets still work wherever it kept them after `devvm secret unuse`, `devvm secret rm` or `devvm network on`, so rotate them if that matters.
- **Your browser.** Programs in a VM can open URLs in this machine's browser, and a page that a VM serves on a forwarded port runs in your browser like any other. From there it can reach the internet past network policy, and your services on `localhost`.
- **Remote forwards you add.** A `RemoteForward` in your ssh config, or an `ssh -R` you type, lets the VM connect to whatever it names on this machine. devvm can't turn those off ([ssh into VMs](../internals/security.md#ssh-into-vms)).

### The platform

- **`.git` on Linux hosts.** Linux has no flag a user can set to lock a file, so VMs on Linux hosts have no `.git` protection, and the VM can change hooks and config that run on the host. Use `--clone` there ([Linux hosts](git-protection.md#linux-hosts)).
- **Escaping the VM.** A bug in Lima, Apple's Virtualization framework or QEMU, or on smolvm in smolvm or libkrun, could let code out of the VM.

Found a way out of a VM? See [SECURITY.md](https://github.com/w3cj/devvm/blob/main/SECURITY.md).
