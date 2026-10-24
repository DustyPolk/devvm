# devvm

Create an isolated VM in any project folder with everything needed for you or an agent to start coding.

```sh
cd ~/some-project
devvm
```

devvm detects the project's toolchain, creates a VM from a cached image, mounts only the project folder, and opens a shell at the same path. Toolchains and coding agents are shared read-only between VMs, network access goes through an allowlist enforced on the host, and API keys stay on the host.

## Docs

[w3cj.github.io/devvm](https://w3cj.github.io/devvm/), from `docs/`. Building, testing and writing docs: [Contributing](https://w3cj.github.io/devvm/docs/contributing/).

## License

[MIT](LICENSE)
