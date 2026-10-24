---
title: Contributing
weight: 100
---

The rules for working in the repo, for people and coding agents alike, are in [AGENTS.md](https://github.com/w3cj/devvm/blob/main/AGENTS.md).

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

## Tools and tasks

mise installs every tool at the version pinned in `mise.toml`, checked against `mise.lock`:

| Tool | Used for |
| --- | --- |
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
| --- | --- |
| `fmt` | formats Go code |
| `lint` | runs golangci-lint, shellcheck and the docs linters |
| `test` | runs the tests with the race detector and coverage |
| `vuln` | runs govulncheck |
| `docs` | serves the docs site |
| `e2e` | runs the end-to-end tests against real VMs |

`mise run test` needs a C compiler for the race detector: Xcode Command Line Tools on macOS, gcc on Linux.

CI runs `lint` and `test` on macOS and Linux.

## CLI tests

CLI tests are [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) scripts in `testdata/script/*.txtar`, run by `internal/cli/script_test.go` against a freshly built `devvm`. Call it with `exec devvm …`:

```txtar
# An unknown command fails.
! exec devvm nope
stderr 'unknown command "nope"'
```

Run one script with `mise exec -- go test ./internal/cli -run TestScript/<name>`.

Each script runs in a sandbox under `/tmp`, never the real folders:

| Variable | Points to |
| --- | --- |
| `HOME` | `home/` |
| `XDG_DATA_HOME` | `data/` |
| `XDG_CONFIG_HOME` | `config/` |
| `DEVVM_LIMA_HOME`, `LIMA_HOME` | `lima/` (short, for Lima's socket paths) |
| `STUB_DIR` | `stub/`, for stubs' state |
| `STUB_LOG` | `stub/calls`, the stubs' call log |
| `DEVVM_SECRET_SERVICE` | `devvm-test`, the Keychain service name |

`$WORK`, the script's own folder, is the current folder.

### Stubs

`testdata/stubs/` is on `PATH` ahead of the real commands, so tests never run the real tools. A stub is a shell script that appends its command line to `$STUB_LOG` and keeps any state in `$STUB_DIR`. `stub_calls` prints the log for `stdout` or `cmp` to check, and `! stub_calls` fails if anything was logged:

```txtar
exec devvm …
stub_calls
cmp stdout calls

-- calls --
limactl list
```

`uname` and `sw_vers` pretend to be another host when these are set, and run the real command otherwise:

| Variable | Printed by |
| --- | --- |
| `STUB_UNAME_S` | `uname -s` |
| `STUB_UNAME_M` | `uname -m` |
| `STUB_SW_VERS` | `sw_vers -productVersion` |

## Writing docs

The site is [Hugo](https://gohugo.io) with the [Hextra](https://imfing.github.io/hextra/) theme, built from `docs/`. Hextra is a Hugo module pinned in `docs/go.mod`; update it with `hugo mod get github.com/imfing/hextra@<version>` in `docs/`.

- **Pages** are Markdown files in `docs/content/docs/`, one per topic. Update the page for any behavior you change.
- **Front matter** sets `title`, shown as the page's heading, and `weight`, the page's place in the sidebar. Start the body at `##`.
- **Links** to other pages point at their files (`[Contributing](contributing.md)`), so the linters can check them.
- **Style:** succinct and direct. Commands are written as devvm x or in backticks, never in quotes. No `please`, `simply` or `note that`.

`mise run lint` checks `docs/` with:

| Linter | Checks | Config |
| --- | --- | --- |
| markdownlint-cli2 | Markdown | `.markdownlint-cli2.yaml` |
| Vale | the style rules | `.vale.ini`, `.vale/styles/devvm/` |
| lychee (`--offline`) | links to pages and files | |

`mise run docs` serves the site at `http://localhost:1313/devvm/` and reloads on every change. `mise exec -- hugo -s docs --minify` builds it into `docs/public/`, as the Pages workflow does on every push to `main`.
