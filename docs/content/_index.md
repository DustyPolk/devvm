---
title: devvm
layout: hextra-home
---

<!-- markdownlint-disable-next-line MD034 -- a shortcode's parameter, not a bare URL -->
{{< hextra/hero-badge link="https://github.com/w3cj/devvm" >}}
  Open source · macOS and Linux
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  One VM for every folder
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  A developer-friendly, isolated sandbox, ready for you or an agent to start coding. It runs on any of three backends: [Lima](https://lima-vm.io/), [smolvm](https://smolmachines.com/) and [Docker](https://docs.docker.com/get-started/get-docker/).
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Install" link="docs/install/" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" >}}

All it takes is one command, and you're ready to start coding.

{{< asciinema file="home/devvm.cast" theme="devvm" autoplay="true" loop="true" >}}

## Get started

Install with Homebrew:

```sh
brew install w3cj/tap/devvm
```

Or with the install script:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor` to check your machine, and run `devvm` in any project folder.

[Installing](docs/install.md) has more details.

## How it works

### Developer ready, zero config

- devvm detects the [project's toolchain](docs/images.md#shared-tools) from its files, so you write no config files and do no setup. It supports node, python, rust and go, with more to come, and you can also name the tools yourself: `devvm python+claude`.
- The base image can also include [optional tools](docs/customizing.md#base-image-features) such as oh-my-bash, fzf, zoxide (`z`), tree and tldr.
{{< art "zero-config" >}}

### Isolated sandbox

- The VM mounts your project folder at the same path, and your agents' config folders read-only. It sees nothing else from your home folder ([security model](docs/security.md)).
- devvm locks `.git`'s hooks and config, so the VM can't change what git runs on your machine ([.git protection](docs/git-protection.md)). With `--clone`, the VM gets a clone of its own instead.
- With [network policy](docs/network.md), the VM reaches only an allowlist of hosts, through a proxy on your machine.
{{< art "isolated" >}}

### Ready in seconds

- The first VM builds a base image, and every later VM is a copy of it that starts in seconds ([images](docs/images.md)).
- devvm builds each toolchain once and shares it read-only, so all VMs use the same copy ([shared tools](docs/images.md#shared-tools)).
- Docker runs inside the VM when you need it: `devvm node+docker`.
{{< art "copies" >}}

### Made for coding agents

- Claude Code, Codex and opencode get your settings, skills and MCP servers ([coding agents](docs/coding-agents.md)).
- With network policy, their tokens stay on your machine, because the proxy adds them to the VM's requests on the way out.
- `devvm ssh` and `devvm code` open the VM in a shell or in VS Code ([SSH](docs/ssh.md)).
{{< art "agents" >}}
