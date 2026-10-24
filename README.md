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

## Development

[mise](https://mise.jdx.dev) installs every tool at the version pinned in `mise.toml`, checked against `mise.lock`:

| Tool | Used for |
|---|---|
| go | building and testing |
| golangci-lint | Go linters and formatters (`.golangci.yml`) |
| gotestsum | running the tests |
| govulncheck | checking dependencies for known vulnerabilities |
| goreleaser | release builds |
| hugo-extended | the docs site |
| vale, markdownlint-cli2, lychee | linting the docs |
| node | running markdownlint-cli2 |
| shellcheck | linting shell scripts |
| minisign | signing releases |

Run the tasks with `mise run <task>`:

| Task | Does |
|---|---|
| `fmt` | formats Go code |
| `lint` | runs golangci-lint and shellcheck |
| `test` | runs the tests with the race detector and coverage |
| `vuln` | runs govulncheck |
| `docs` | serves the docs site |
| `e2e` | runs the end-to-end tests against real VMs |

`mise run test` needs a C compiler for the race detector: Xcode Command Line Tools on macOS, gcc on Linux.

CI runs `lint` and `test` on macOS and Linux.

## License

[MIT](LICENSE)
