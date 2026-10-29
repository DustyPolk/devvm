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
  An isolated sandbox with the project's toolchain, ready for you or an agent to start coding.
{{< /hextra/hero-subtitle >}}

{{< hextra/hero-button text="Get started" link="docs/install/" >}}
{{< hextra/hero-button text="Read the docs" link="docs/" >}}

```console
$ cd ~/some-project
$ devvm
==> creating devvm-some-project-71a99b1076d92c from base
==> starting devvm-some-project-71a99b1076d92c
```

Then you're in a shell in the VM, at the same path.

## What it does

### Isolated

- Only the project folder is mounted, at the same path. Nothing else from your home folder ([security model](docs/security.md)).
- `.git`'s hooks and config are locked, so the VM can't change what git runs on your machine ([.git protection](docs/git-protection.md)). Or `--clone` gives it a clone of its own.
- With [network policy](docs/network.md), the VM reaches only an allowlist of hosts, through a proxy on your machine.

### Ready in seconds

- The first VM builds a base image. Every later one is a copy, and starts in seconds ([images](docs/images.md)).
- devvm detects the project's toolchain from its files, or you name it: `devvm python+claude`.
- Toolchains are built once and shared read-only: ten VMs with Node share one copy.
- Docker inside when you need it: `devvm node+docker`.

### Made for coding agents

- Claude Code, Codex and opencode get your settings, skills and MCP servers, never your history ([coding agents](docs/coding-agents.md)).
- With network policy, their tokens stay on your machine: the proxy adds them to requests.
- `devvm ssh` and `devvm code` open the VM in a shell or VS Code ([SSH](docs/ssh.md)).

## Get started

Install with Homebrew, which also installs Lima:

```sh
brew install w3cj/tap/devvm
```

Or with the install script, which checks the release's signature and offers to install what's missing:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor` to check your machine, and `devvm` in any project folder. [Installing](docs/install.md) has the details.
