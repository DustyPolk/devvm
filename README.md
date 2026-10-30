# devvm

One VM for every folder: an isolated sandbox with the project's toolchain, ready for you or an agent to start coding.

```sh
cd ~/some-project
devvm
```

devvm detects the project's toolchain, creates a VM from a cached image, mounts only the project folder, and opens a shell at the same path. Toolchains and coding agents are shared read-only between VMs, network access goes through an allowlist enforced on the host, and API keys stay on the host.

## Quickstart

devvm runs on macOS and Linux, with [Lima](https://lima-vm.io) 2.0 or newer. Install it with Homebrew (`brew install w3cj/tap/devvm`) or the install script, which offers to install what's missing ([Installing](https://devvm.dev/docs/getting-started/install/)). `devvm doctor` checks what's missing and how to get it.

```console
$ cd ~/some-project
$ devvm --network-policy base
==> building image base (e9f517dddef1)
    a few minutes, and Ubuntu's cloud image to download the first time
    (details: ~/.local/share/devvm/logs/runs/20261024-090616-83541.log)
==> image base ready
==> creating devvm-some-project-71a99b1076d92c from base
==> starting devvm-some-project-71a99b1076d92c
```

Then you're in a shell in the VM, in `~/some-project`. The first VM builds the base image. Later VMs are copies of it and start in seconds. The VM gets [network policy](https://devvm.dev/docs/safety/network/) by default, so it reaches only an allowlist of hosts, through a proxy on your machine, and devvm asks first in a terminal. Before you trust a VM with a project, read [staying safe](https://devvm.dev/docs/safety/attack-vectors/#staying-safe).

## Docs

[devvm.dev](https://devvm.dev/), from `docs/`: [your first VM](https://devvm.dev/docs/getting-started/first-vm/), [commands](https://devvm.dev/docs/getting-started/commands/), [the security model](https://devvm.dev/docs/safety/security/), [attack vectors](https://devvm.dev/docs/safety/attack-vectors/). Building, testing and writing docs: [Contributing](https://devvm.dev/docs/contributing/).

## License

[MIT](LICENSE)
