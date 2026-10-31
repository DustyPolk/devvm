---
title: Toolchains
weight: 2
---

devvm has four toolchains: node, python, rust and go. A new VM gets the ones its folder's files call for, or the ones you name, as in `devvm node+python`. Each section below says what the toolchain brings and how devvm chooses it. A project that gets its toolchain from Nix uses the `nix` layer, which [Nix projects](#nix-projects) covers.

## node

node brings the newest release of Node 24, with npm, pnpm, yarn through corepack, and bun. devvm stays on Node 24 because newer releases drop corepack.

- **Files.** devvm chooses node for a folder with `package.json`, `bun.lock` or `bun.lockb` at its top level.
- **Versions.** A project's `.nvmrc` or `.node-version` picks its Node version, which installs into the VM the first time it's used.
- **Global installs.** `npm i -g` installs into `~/.npm-global` in the VM, which is on the `PATH`.
- **Allowlist.** With network policy, the VM can reach npm's, yarn's, pnpm's and bun's registries and Node's downloads, which are `registry.npmjs.org`, `registry.yarnpkg.com`, `repo.yarnpkg.com`, `nodejs.org` and `bun.sh`.

## python

python brings a prebuilt build of the newest CPython release, and uv, so `pip` and `uv` work as usual.

- **Files.** devvm chooses python for a folder with `pyproject.toml`, `requirements.txt`, `uv.lock`, `setup.py` or `Pipfile` at its top level.
- **Versions.** A project's `.python-version` picks its Python version, which installs into the VM the first time it's used.
- **Global installs.** The shared Python is read-only, so packages go into a project's virtual environment or into your home in the VM, and `~/.local/bin` is on the `PATH`.
- **Allowlist.** With network policy, the VM can reach PyPI and the Python downloads that uv uses, which are `pypi.org`, `files.pythonhosted.org` and `releases.astral.sh`.

## rust

rust brings rustup, cargo and the newest stable toolchain, with the `rust-src` and `rust-analyzer` components already added.

- **Files.** devvm chooses rust for a folder with `Cargo.toml` at its top level.
- **Versions.** A project's `rust-toolchain.toml` picks its toolchain, which rustup installs into the VM next to the shared one the first time it's used.
- **Global installs.** `cargo install` installs into `~/.cargo/bin` in the VM, which is on the `PATH`. `rustup component add` can't change the shared toolchain, which is why it comes with `rust-src` and `rust-analyzer`.
- **Allowlist.** With network policy, the VM can reach crates.io and Rust's toolchain downloads, which are `crates.io`, `index.crates.io`, `static.crates.io` and `static.rust-lang.org`.

## go

go brings the newest Go release, and devvm turns Go's telemetry off in the VM.

- **Files.** devvm chooses go for a folder with `go.mod` at its top level.
- **Versions.** The `toolchain` line in a project's `go.mod` picks its Go version, and Go itself downloads that version into the VM the first time it's used.
- **Global installs.** `go install` installs into `~/go/bin` in the VM, which is on the `PATH`.
- **Allowlist.** With network policy, the VM can reach Go's module proxy, its checksum database and Go's downloads, which are `proxy.golang.org`, `sum.golang.org`, `go.dev` and `dl.google.com`.

## Nix projects

A project whose toolchain comes from Nix gets the [`nix` layer](images.md#nix), which brings Nix, direnv and nix-direnv into its VM. devvm chooses it for a folder with `flake.nix`, `shell.nix` or `devenv.nix` at its top level, or with a `default.nix` that makes a shell with `mkShell`. It goes after the toolchains the folder calls for, so a folder with `package.json` and `flake.nix` gets `node+nix`, and you can also name it, as in `devvm nix`.

- **Dev shells.** Put `use flake` (or `use nix` for a `shell.nix`) in the project's `.envrc` and run `direnv allow` in the VM once. From then on, each shell that changes into the folder loads the dev shell, and leaving the folder unloads it. This works in your terminal and in the terminals of editors connected over SSH.
- **The flake's tools come first.** While the dev shell is loaded, its tools are ahead of the shared tools on the `PATH`, so a flake's `node` wins over the shared node, and the shared tools come back when you leave the folder.
- **Files in the project.** nix-direnv keeps its cache of the dev shell in your home in the VM, so the project gets no `.direnv` folder. Nix itself writes a `flake.lock` next to a `flake.nix` that has none, and in a git repository it marks that file to be added, as it does on any machine.
- **One store per VM.** Each VM has its own Nix store, so a package one VM downloads or builds isn't in another's. `nix run nixpkgs#hello` works in any VM with the layer.
- **Trust.** Only root is a trusted Nix user, so the VM's user can't add caches or signing keys of their own, and Nix ignores `--option substituters` from them. `sudo nix` works. Nix builds every package in its sandbox, and a build fails rather than run without it.
- **Allowlist.** With network policy, the VM can reach Nix's binary cache and its channels and flake registry, which are `cache.nixos.org`, `channels.nixos.org` and `releases.nixos.org`, and GitHub, where most flakes' inputs come from. A package that isn't in the cache builds from source, which downloads its sources from their own hosts, and those are blocked until you allow them ([Nix with network policy](../safety/network.md#nix)).

## How toolchains are shared and updated

The toolchains are [shared tools](images.md#shared-tools), like the [coding agents](coding-agents.md#supported-agents), so they aren't part of any image. devvm builds each version once and mounts it read-only into every VM that has it, and [mise](https://mise.jdx.dev) runs it, so `node`, `python`, `cargo` and `go` work from any shell in the VM, including `bash -c` from agents and from editors over SSH. Because the shared copy is read-only, a toolchain can't update itself, and anything you install globally goes into your home in the VM, as each section above says.

devvm follows each toolchain's newest release, and Node 24 for node. [Adding and removing tools](images.md#adding-and-removing-tools) explains how `devvm update` moves a VM to newer versions, how you add a toolchain to an existing VM, and how `devvm remove` takes one out. [Which image a new VM gets](images.md#which-image-a-new-vm-gets) explains what happens when a folder calls for more than one toolchain.
