---
title: devvm
layout: hextra-home
---

<!-- markdownlint-disable-next-line MD034 -- a shortcode's parameter, not a bare URL -->
{{< hextra/hero-badge link="https://github.com/w3cj/devvm" >}}
  Free and open source
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  One VM for every project
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  An isolated sandbox with the project's toolchain, ready for you or a coding agent to start coding. devvm runs its VMs on [Lima](https://lima-vm.io/) by default, or on the experimental [smolvm](https://smolmachines.com/) and [Docker](https://docs.docker.com/get-started/get-docker/) backends.
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Install" link="docs/getting-started/install/" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" >}}

## Quick start

{{< install step="1" >}}

```sh
# 2. Go to a project folder
cd ~/some-project
# 3. Open its VM
devvm
```

For a VM with a coding agent, run `devvm claude` or `devvm codex` instead. The first VM builds the base image, which takes a few minutes once, and [your first VM](docs/getting-started/first-vm.md) walks you through it, including the Claude Code token.

## What's supported

{{< features columns="3" >}}

{{< feature title="Toolchains" level="3" >}}

devvm picks node, python, rust or go from the project's files, and you can combine them ([toolchains](docs/using/toolchains.md)).

{{< /feature >}}

{{< feature title="Coding agents" level="3" >}}

Claude Code, Codex and opencode run in the VM with your settings, skills and MCP servers ([coding agents](docs/using/coding-agents.md)).

{{< /feature >}}

{{< feature title="Backends" level="3" >}}

Lima runs the VMs by default, and smolvm and Docker are experimental backends ([backends](docs/backends/_index.md)).

{{< /feature >}}

{{< /features >}}

## Why use devvm?

- Developers face attacks every day, such as [supply chain attacks](https://en.wikipedia.org/wiki/Supply_chain_attack), leaked secrets, [prompt injections](https://en.wikipedia.org/wiki/Prompt_injection) and rogue agents.
- A sandbox limits the [blast radius](https://en.wikipedia.org/wiki/Blast_radius#Uses_in_software) of an attack to what's inside it.
- You know you _should_ run your code and agents in a sandbox, but most tools need configuration and setup that's hard to get right.
- devvm comes with sensible defaults, so you can start coding in a sandbox right away.

{{< features >}}

{{< feature title="Developer ready, zero config" art="zero-config" >}}

- devvm detects the [project's toolchain](docs/using/images.md#which-image-a-new-vm-gets) from its files, so you write no config files and do no setup.
- Docker runs inside the VM when you need it, with `devvm node+docker`.
- Every VM gets your git name and email, so the commits made in it are authored by you or your agent.
- `devvm code` opens the VM in VS Code over Remote-SSH ([devvm code](docs/using/ssh.md#devvm-ssh-and-devvm-code)).
- When the project has a `devcontainer.json`, devvm sets up its VM from the file's container image, ports, environment and commands ([devcontainer.json](docs/using/devcontainer.md)).
- The base image comes with [optional extras](docs/using/customizing.md#base-image-features) that you choose, such as oh-my-bash, fzf, zoxide (`z`), tree and tldr.

{{< /feature >}}

{{< feature title="Ready in seconds" art="copies" >}}

- The first VM builds a base image, and every later VM is a copy of it that starts in seconds ([images](docs/using/images.md)).
- devvm builds each version of a toolchain once and shares it read-only, so all VMs use the same copy ([shared tools](docs/using/images.md#shared-tools)).

{{< /feature >}}

{{< feature title="Isolated sandbox" art="isolated" >}}

- The VM mounts your project folder at the same path. Apart from your agents' config, which it gets read-only, it sees nothing else in your home folder ([what a VM can reach](docs/safety/_index.md#what-a-vm-can-reach)).
- On macOS, devvm locks `.git`'s hooks and config, so the VM can't change what git runs on your machine ([.git protection](docs/safety/git-protection.md)).
- With [`--clone`](docs/using/clone.md), the VM works on a clone of the repo of its own instead of your folder, so nothing in it can touch your working tree or `.git`, and its work reaches your repo only when you fetch it.
- New VMs get [network policy](docs/safety/network.md) by default, so they reach only an allowlist of hosts, through a proxy on your machine.
- No sandbox closes every gap, so [what devvm doesn't protect](docs/internals/security.md#what-it-doesnt-protect) lists what's left open, and [staying safe](docs/safety/_index.md#the-habits) lists the habits that close most of it.

{{< /feature >}}

{{< feature title="Made for coding agents" art="agents" >}}

- `devvm claude`, `devvm codex` and `devvm opencode` make a VM with that agent, which gets your settings, skills and MCP servers but never your history ([coding agents](docs/using/coding-agents.md)).
- With network policy, their secrets, such as your Claude Code token and your Codex login, stay on your machine, because the VM gets placeholders and devvm's proxy puts the real ones into its requests on the way out.

{{< /feature >}}

{{< /features >}}

## FAQ

### Does it run on Linux?

Yes. devvm runs on macOS 13 or newer and on Linux, on arm64 or x86_64. On Linux hosts it has a few limits, such as no `.git` protection, which [known limits](docs/using/limits.md#linux-hosts) lists.

### Is it free?

Yes. devvm is open source under the [MIT license](https://github.com/w3cj/devvm/blob/main/LICENSE).

### What do I need?

devvm needs Lima 2.0 or newer, which Homebrew installs along with it, and on Linux also QEMU and access to `/dev/kvm`. `devvm doctor` checks your machine, and [installing](docs/getting-started/install.md) lists every requirement.

## Install devvm

{{< install >}}

```sh
# Check that your machine has what devvm needs
devvm doctor
```

{{< hextra/hero-button text="Install" link="docs/getting-started/install/" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" >}}
