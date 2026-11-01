---
title: devvm
layout: hextra-home
# Mostly shortcodes: no Copy page menu (hugo.yaml gives the home page no Markdown copy).
contextMenu: false
---

<!-- markdownlint-disable-next-line MD034 -- a shortcode's parameter, not a bare URL -->
{{< hextra/hero-badge link="https://github.com/w3cj/devvm" >}}
  Free and open source
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  One VM for every project
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  An isolated sandbox with your project's toolchain, ready for you or a coding agent to start coding.
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Install" link="docs/getting-started/install/" icon="download" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" icon="open-book" >}}

## Quick start {icon="rocket"}

{{< install step="1" >}}

```sh
# 2. Go to a project folder
cd ~/some-project
# 3. Open its VM
devvm
```

For a VM with a coding agent, run `devvm claude` or `devvm codex` instead. [Your first VM](docs/getting-started/first-vm.md) walks you through it, including the Claude Code token.

## What's supported {icon="view-grid"}

{{< features columns="3" >}}

{{< feature title="Toolchains" icon="code" level="3" art="toolchains" >}}

`devvm` picks the toolchain from the project's files, and you can combine them ([toolchains](docs/using/toolchains.md)).

{{< /feature >}}

{{< feature title="Coding agents" icon="robot" level="3" art="coding-agents" >}}

Claude Code, Codex and opencode are ready in the VM ([coding agents](docs/using/coding-agents.md)).

{{< /feature >}}

{{< feature title="Backends" icon="cube" level="3" art="backends" >}}

[Lima](https://lima-vm.io/) runs the VMs by default, and [smolvm](https://smolmachines.com/) and [Docker](https://docs.docker.com/get-started/get-docker/) are {{< experimental >}} backends ([backends](docs/backends/_index.md)).

{{< /feature >}}

{{< /features >}}

## Why use devvm? {icon="shield"}

{{< beside art="blast-radius" >}}

- Developers face attacks every day, such as [supply chain attacks](https://en.wikipedia.org/wiki/Supply_chain_attack), leaked secrets, [prompt injections](https://en.wikipedia.org/wiki/Prompt_injection) and rogue agents.
- A sandbox limits the [blast radius](https://en.wikipedia.org/wiki/Blast_radius#Uses_in_software) of an attack to what's inside it.
- You know you _should_ run your code and agents in a sandbox, but most tools need configuration and setup that's hard to get right.
- `devvm` ships with sensible defaults so you can start coding in a sandbox with a single command.

{{< /beside >}}

{{< features >}}

{{< feature title="Developer ready, zero config" icon="prompt" art="zero-config" >}}

- The base image comes with [optional extras](docs/using/customizing.md#base-image-features) that you choose, such as oh-my-bash, fzf, zoxide (`z`), tree and tldr.
- Every VM gets your git name and email, so the commits made in it are authored by you or your agent.
- Ports forward on their own, so a dev server on `:5173` in the VM opens at `localhost:5173` on your machine ([ports](docs/using/ports-size.md#ports)).
- `devvm code` opens the VM in VS Code over Remote-SSH ([devvm code](docs/using/ssh.md#devvm-ssh-and-devvm-code)).

{{< /feature >}}

{{< feature title="Ready in seconds" icon="high-voltage" art="copies" >}}

- The first VM builds a base image, and every later VM is a copy of it that starts in seconds ([images](docs/using/images.md)).
- `devvm` builds each version of a toolchain once and shares it read-only, so all VMs use the same copy ([shared tools](docs/using/images.md#shared-tools)).

{{< /feature >}}

{{< feature title="Isolated sandbox" icon="locked" art="isolated" >}}

- The VM mounts your project folder at the same path. Apart from your agents' config, which it gets read-only, it sees nothing else in your home folder ([what a VM can reach](docs/safety/_index.md#what-a-vm-can-reach)).
- New VMs get [network policy](docs/safety/network.md) by default, so they reach only an allowlist of hosts, through a proxy on your machine.
- On macOS, `devvm` locks `.git`'s hooks and config, so the VM can't change what git runs on your machine ([.git protection](docs/safety/git-protection.md)).
- With [`--clone`](docs/using/clone.md), the VM works on a clone of the repo of its own instead of your folder, so nothing in it can touch your working tree or `.git`, and its work reaches your repo only when you fetch it.
- No sandbox closes every gap, so [what devvm doesn't protect](docs/internals/security.md#what-it-doesnt-protect) lists what's left open, and [staying safe](docs/safety/_index.md#the-habits) lists the habits that close most of it.

{{< /feature >}}

{{< feature title="Made for coding agents" icon="robot" art="agents" >}}

- `devvm claude`, `devvm codex` and `devvm opencode` make a VM with that agent, which gets your settings, skills and MCP servers but never your history ([coding agents](docs/using/coding-agents.md)).
- With network policy, their secrets, such as your Claude Code token and your Codex login, stay on your machine, because the VM gets placeholders and devvm's proxy puts the real ones into its requests on the way out.

{{< /feature >}}

{{< feature title="Make it yours" icon="wrench" column="right" art="make-it-yours" >}}

- Your own layers are setup scripts that `devvm` builds into an image: `devvm layer new mailpit` starts one, and `devvm node+mailpit` makes a VM with it ([your own layers](docs/using/customizing.md#your-own-layers)).
- Your own features are scripts that run in the base image, so every VM gets your shell setup, aliases or dotfiles ([your own features](docs/using/customizing.md#your-own-features)).
- `--from` starts the base image from a container image instead of Ubuntu's, such as `node:24` or your team's own ([starting from a container image](docs/using/from.md)).
- When the project has a `devcontainer.json`, `devvm` sets up its VM from the file's container image, ports, environment and commands ([devcontainer.json](docs/using/devcontainer.md)).

{{< /feature >}}

{{< /features >}}

## FAQ {icon="speech-balloon-question"}

### What does it run on?

`devvm` runs on macOS 13 or newer and on Linux, on arm64 or x86_64. It needs one of the supported [backends](docs/backends/_index.md): Lima 2.0 or newer, the default, which Homebrew installs along with devvm and which on Linux also needs QEMU and access to `/dev/kvm`, or smolvm or Docker, which are {{< experimental >}}. [Installing](docs/getting-started/install.md) lists every requirement, and [known limits](docs/using/limits.md#linux-hosts) lists what Linux hosts lack, such as `.git` protection.

### Is it free?

Yes. `devvm` is open source under the [MIT license](https://github.com/w3cj/devvm/blob/main/LICENSE).

## Start coding in a sandbox {icon="prompt"}

Install `devvm`, then run `devvm` in any project folder.

{{< hextra/hero-button text="Install" link="docs/getting-started/install/" icon="download" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" icon="open-book" >}}
