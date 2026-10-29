---
title: devvm
layout: hextra-home
---

<!-- markdownlint-disable-next-line MD034 -- a shortcode's parameter, not a bare URL -->
{{< hextra/hero-badge link="https://github.com/w3cj/devvm" >}}
  Open source · macOS and Linux
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
  One Virtual Machine for every folder
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  An isolated sandbox with your project's toolchain, ready for you or an agent to start coding.
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Install" link="docs/install/" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" >}}

One command, then you're in a shell in the VM.

{{< asciinema file="home/devvm.cast" theme="devvm" autoplay="true" loop="true" >}}

## Get started

Install with Homebrew, which also installs Lima:

```sh
brew install w3cj/tap/devvm
```

Or with the install script, which checks the release's signature and offers to install what's missing:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor` to check your machine, and `devvm` in any project folder. More details in [Installing](docs/install.md).

## What you get out of the box

### Developer Ready, Zero Config

{{< art "zero-config" >}}

- `devvm` auto detects the [project's toolchain](docs/images/#shared-tools) (node, python, rust and go supported now, more to come), no config files or setup required.
- The base image [optionally includes](docs/customizing/#base-image-features) useful tools like oh-my-bash, fzf, zoxide (z), tree and tldr and more.

### Isolated

{{< art "isolated" >}}

- Only the project folder is mounted, at the same path. Nothing else from your home folder ([security model](docs/security.md)).
- `.git`'s hooks and config are locked, so the VM can't change what git runs on your machine ([.git protection](docs/git-protection.md)). Or `--clone` gives it a clone of its own.
- With [network policy](docs/network.md), the VM reaches only an allowlist of hosts, through a proxy on your machine.

### Ready in seconds

{{< art "copies" >}}

- The first VM builds a base image. Every later one is a copy, and starts in seconds ([images](docs/images.md)).
- devvm detects the project's toolchain from its files, or you name it: `devvm python+claude`.
- Toolchains are built once and shared read-only: all VMs share one copy ([Shared tools](docs/images/#shared-tools)).
- Docker inside when you need it: `devvm node+docker`.

### Made for coding agents

{{< art "agents" >}}

- Claude Code, Codex and opencode get your settings, skills and MCP servers ([coding agents](docs/coding-agents.md)).
- With network policy, their tokens stay on your machine: the proxy adds them to requests.
- `devvm ssh` and `devvm code` open the VM in a shell or VS Code ([SSH](docs/ssh.md)).
