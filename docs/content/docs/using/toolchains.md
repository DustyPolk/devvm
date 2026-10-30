---
title: Toolchains
weight: 2
---

devvm has four toolchains: node, python, rust and go. A new VM gets the ones its folder's files call for, or the ones you name, as in `devvm node+python`. Each section below says what the toolchain brings and how devvm chooses it.

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

## How toolchains are shared and updated

The toolchains are [shared tools](images.md#shared-tools), like the [coding agents](coding-agents.md#supported-agents), so they aren't part of any image. devvm builds each version once and mounts it read-only into every VM that has it, and [mise](https://mise.jdx.dev) runs it, so `node`, `python`, `cargo` and `go` work from any shell in the VM, including `bash -c` from agents and from editors over SSH. Because the shared copy is read-only, a toolchain can't update itself, and anything you install globally goes into your home in the VM, as each section above says.

devvm follows each toolchain's newest release, and Node 24 for node. [Adding and removing tools](images.md#adding-and-removing-tools) explains how `devvm update` moves a VM to newer versions, how you add a toolchain to an existing VM, and how `devvm remove` takes one out. [Which image a new VM gets](images.md#which-image-a-new-vm-gets) explains what happens when a folder calls for more than one toolchain.
