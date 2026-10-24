# devvm

Create an isolated VM in any project folder with everything needed for you or an agent to start coding.

```sh
cd ~/some-project
devvm
```

devvm detects the project's toolchain, creates a VM from a cached image, mounts only the project folder, and opens a shell at the same path. Toolchains and coding agents are shared read-only between VMs, network access goes through an allowlist enforced on the host, and API keys stay on the host.

## Building from source

devvm is built with Go, pinned in `mise.toml`. With [mise](https://mise.jdx.dev) installed:

```sh
mise install
mise exec -- go build -o devvm ./cmd/devvm
./devvm --help
```

The binary finds its files in the folder it's in, following links, so build it at the root of the repo and link to it from your `PATH`:

```sh
ln -s "$PWD/devvm" ~/.local/bin/devvm
```

## License

[MIT](LICENSE)
