# devvm

Create an isolated VM in any project folder with everything needed for you or an agent to start coding.

```sh
cd ~/some-project
devvm
```

devvm detects the project's toolchain, creates a VM from a cached image, mounts only the project folder, and opens a shell at the same path. Toolchains and coding agents are shared read-only between VMs, network access goes through an allowlist enforced on the host, and API keys stay on the host.

## Quickstart

devvm runs on macOS and Linux, with [Lima](https://lima-vm.io) 2.0 or newer.

```console
$ cd ~/some-project
$ devvm --no-network-policy base
==> building image base (e9f517dddef1)
    a few minutes, and Ubuntu's cloud image to download the first time
    (details: ~/.local/share/devvm/logs/runs/20261024-090616-83541.log)
==> image base ready
==> creating devvm-some-project-71a99b1076d92c from base
==> starting devvm-some-project-71a99b1076d92c
```

Then you're in a shell in the VM, in `~/some-project`. The first VM builds the base image. Later VMs are copies of it and start in seconds. Network policy isn't there yet, so devvm creates a VM only with `--no-network-policy`, and that VM reaches anything your machine can.

## Docs

[w3cj.github.io/devvm](https://w3cj.github.io/devvm/), from `docs/`: [commands](https://w3cj.github.io/devvm/docs/commands/), [the security model](https://w3cj.github.io/devvm/docs/security/). Building, testing and writing docs: [Contributing](https://w3cj.github.io/devvm/docs/contributing/).

## License

[MIT](LICENSE)
