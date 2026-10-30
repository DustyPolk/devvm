---
title: devvm
layout: hextra-home
---

<!-- markdownlint-disable-next-line MD034 -- a shortcode's parameter, not a bare URL -->
{{< hextra/hero-badge link="https://github.com/w3cj/devvm" >}}
  Free and Open source
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  One VM for every project
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  A developer-friendly, isolated sandbox, ready for you or an agent to start coding. It runs on any of three backends: [Lima](https://lima-vm.io/), [smolvm](https://smolmachines.com/) and [Docker](https://docs.docker.com/get-started/get-docker/).

  Install with Homebrew:

```sh
brew install w3cj/tap/devvm
```

Or with the install script:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor` to check your machine, and run `devvm` in any project folder.

[Installing](docs/getting-started/install.md) has more details.
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Read the docs" link="docs/" >}}
{{< hextra/hero-button text="Install" link="docs/getting-started/install/" >}}

All it takes is one command, and you're ready to start coding.

{{< asciinema file="home/devvm.cast" theme="devvm" autoplay="true" loop="true" >}}

## Why use devvm?

- Developers are exposed daily to several attack vectors including [supply chain attacks](https://en.wikipedia.org/wiki/Supply_chain_attack), leaked secrets, [prompt injections](https://en.wikipedia.org/wiki/Prompt_injection), rogue agents and more.
- A sandbox limits the [blast radius](https://en.wikipedia.org/wiki/Blast_radius#Uses_in_software) of any potential attack.
- We all know we _should_ be sandboxing our code and agents, but most tools require configs and setup that are hard to get right.
- `devvm` ships with sensible defaults so you can start coding in a sandbox right away.

## Developer ready, zero config

- `devvm` detects the [project's toolchain](docs/using/images.md#shared-tools) from its files, so you write no config files and do no setup. It supports node, python, rust and go, with more to come.
- Docker runs inside the VM when you need it: `devvm node+docker`.
- Every instance automatically sets your git identity so commits inside the VM are authored by you (or your agent).
- The base image includes [optional tools](docs/using/customizing.md#base-image-features) such as oh-my-bash, fzf, zoxide (`z`), tree, tldr and more.
{{< art "zero-config" >}}

## Ready in seconds

- The first VM builds a base image, and every later VM is a copy of it that starts in seconds ([images](docs/using/images.md)).
- `devvm` builds each toolchain once and shares it read-only, so all VMs use the same copy ([shared tools](docs/using/images.md#shared-tools)).
{{< art "copies" >}}

## Isolated sandbox

- The VM mounts your project folder at the same path. It sees nothing else from your home folder ([security model](docs/safety/security.md)).
- `devvm` locks `.git`'s hooks and config, so the VM can't change what git runs on your machine ([.git protection](docs/safety/git-protection.md)). With `--clone`, the VM gets a clone of its own instead.
- With [network policy](docs/safety/network.md), the VM reaches only an allowlist of hosts, through a proxy on your machine.
- A few habits close most of what's left, and [staying safe](docs/safety/attack-vectors.md#staying-safe) lists them.
{{< art "isolated" >}}

## Made for coding agents

- Claude Code, Codex and opencode get your settings, skills and MCP servers ([coding agents](docs/using/coding-agents.md)).
- With network policy enabled, their API keys stay on your machine because the `devvm` proxy adds them to the VM's requests on the way out.
{{< art "agents" >}}
