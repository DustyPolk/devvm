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
- **One store per VM.** Each VM has its own Nix store, so a package one VM builds isn't in another's. `nix run nixpkgs#hello` works in any VM with the layer.
- **Shared downloads with network policy.** VMs with [network policy](../safety/network.md) share their downloads from Nix's binary cache. The first VM that needs a package gets it from cache.nixos.org through devvm, which keeps a copy on this machine, and the other VMs get that copy, so each package downloads once per machine instead of once per VM. Only devvm adds to this shared cache, and only what cache.nixos.org signed, so no VM can change what another VM gets. When the cache doesn't have a package or can't get it, Nix goes to cache.nixos.org itself. The cache keeps to 10 GiB, deleting the packages used least recently first, unless you set another size with `devvm space set nix-cache.size=20G` ([retention](settings.md#retention)). `devvm space` shows what it takes, and `devvm gc --nix-cache` empties it ([how it works](../internals/network.md#the-shared-nix-cache)).
- **Without network policy**, a VM gets no shared cache. Its Nix downloads from cache.nixos.org directly into its own store, and nothing runs or listens on this machine for it.
- **Trust.** Only root is a trusted Nix user, so the VM's user can't add caches or signing keys of their own, and Nix ignores `--option substituters` from them. `sudo nix` works. Nix builds every package in its sandbox, and a build fails rather than run without it.
- **Allowlist.** With network policy, the VM can reach Nix's binary cache and its channels and flake registry, which are `cache.nixos.org`, `channels.nixos.org` and `releases.nixos.org`, and GitHub, where most flakes' inputs come from. A package that isn't in Nix's binary cache builds from source, which downloads its sources from their own hosts, and those are blocked until you allow them ([Nix with network policy](../safety/network.md#nix)).

### Sharing a team's packages

When your team's projects use one flake for their dev shell, you can put that dev shell's packages into an image, so every VM made from the image starts with them in its store and downloads none of them. VMs made from one image share its disk, because on macOS each VM's disk is a copy-on-write clone of the image's, and on Linux hosts it's an overlay of the image's, so the packages take their space once. This works with the stable Nix that the `nix` layer installs, with network policy or without it, on every backend that has the `nix` layer, which are Lima and smolvm.

To do it, make a [layer of your own](customizing.md#your-own-layers) that builds the dev shell while the image builds, and name it after `nix` in the image, so that it runs on top of the `nix` layer:

```sh
devvm layer new myteam
$EDITOR ~/.config/devvm/layers/myteam.sh
devvm nix+myteam         # or devvm node+nix+myteam
```

Put these lines in `~/.config/devvm/layers/myteam.sh`, below the starter's `set -euo pipefail`, with your team's flake at the commit you want:

```bash
# The team's flake at a commit, so the image holds exactly that dev shell.
flake=github:myorg/devshell/0123456789abcdef0123456789abcdef01234567
bin=/nix/var/nix/profiles/default/bin
"$bin/nix" develop "$flake" --profile /tmp/myteam-shell --command true
# A garbage collector root in the VM's home keeps the dev shell's packages in the store.
mkdir -p ~/.cache/myteam
"$bin/nix-store" --add-root ~/.cache/myteam/dev-shell --realise "$(readlink -f /tmp/myteam-shell)"
rm -f /tmp/myteam-shell /tmp/myteam-shell-*-link
```

`nix develop` builds or downloads every package of the flake's dev shell into the image's store and records the dev shell in a profile. `nix-store --add-root` then makes `~/.cache/myteam/dev-shell` a garbage collector root, so `nix-collect-garbage` in a VM never deletes those packages. A project that loads the same dev shell, with `use flake github:myorg/devshell/<commit>` in its `.envrc`, finds every package already in its VM's store, and so does most of a project whose own flake pins the same nixpkgs revision. The image is built without your credentials, because [credentials never go into images](../internals/security.md#read-only-and-confined-builds), so the flake has to be one that anyone can fetch, such as a public repository.

What this doesn't cover:

- **Packages a VM gets after it's made** stay on that VM's own disk. With network policy, the [shared downloads](#nix-projects) at least save downloading them again.
- **A new version of the flake** needs a new image. Moving `flake` to another commit edits the script, so devvm builds the image again the next time it's used, while VMs made earlier keep the packages they have. When the script names a branch instead of a commit, a changed `flake.lock` upstream doesn't change the script, so run `devvm rebuild myteam` to build the image again.
- **A flake that pins another nixpkgs revision** shares few packages with the image, and its VM downloads the rest.
- **Nix's local overlay store**, which would let VMs share one store as it changes, is left out until Nix makes it stable.

## How toolchains are shared and updated

The toolchains are [shared tools](images.md#shared-tools), like the [coding agents](coding-agents.md#supported-agents), so they aren't part of any image. devvm builds each version once and mounts it read-only into every VM that has it, and [mise](https://mise.jdx.dev) runs it, so `node`, `python`, `cargo` and `go` work from any shell in the VM, including `bash -c` from agents and from editors over SSH. Because the shared copy is read-only, a toolchain can't update itself, and anything you install globally goes into your home in the VM, as each section above says.

devvm follows each toolchain's newest release, and Node 24 for node. [Adding and removing tools](images.md#adding-and-removing-tools) explains how `devvm update` moves a VM to newer versions, how you add a toolchain to an existing VM, and how `devvm remove` takes one out. [Which image a new VM gets](images.md#which-image-a-new-vm-gets) explains what happens when a folder calls for more than one toolchain.
