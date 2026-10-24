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
| `lint` | runs golangci-lint, shellcheck, the docs linters and the message link check |
| `test` | runs the tests with the race detector and coverage |
| `vuln` | runs govulncheck |
| `docs` | serves the docs site |
| `e2e` | runs the end-to-end tests against real VMs |

`mise run test` needs a C compiler for the race detector: Xcode Command Line Tools on macOS, gcc on Linux.

Before and after every change, `mise run fmt && mise run lint && mise run test` must pass.

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
| `DEVVM_UBUNTU_URL` | `ubuntu/`, with one dated release |
| `DEVVM_BIN` | the folder of the built `devvm`, with copies of `base.yaml` and `layers/` next to it |

`$WORK`, the script's own folder, is the current folder.

Tests never touch the network. Downloads are served from a test server (`net/http/httptest`) or read from a `file://` folder. `DEVVM_UBUNTU_URL` replaces `https://cloud-images.ubuntu.com/releases/resolute`, where devvm lists Ubuntu's dated releases: point it at a folder of `release-YYYYMMDD/` folders, each with its `SHA256SUMS` and `SHA256SUMS.gpg`, as `file:///path/to/folder`.

A command left running in the background (`exec devvm … &name&`) can be interrupted with `kill -INT name`. `wait_file FILE` waits up to 10 seconds for a file to appear, such as one a stub writes when it gets somewhere.

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

`limactl` keeps its instances in `$STUB_DIR/instances` and fails or hangs on request (its header lists how). `gpgv` accepts every signature unless `$STUB_DIR/gpgv-bad` exists. `qemu-img` and QEMU are there so Linux hosts pass devvm's checks.

## End-to-end tests

`tests/e2e.sh` runs devvm against real VMs, so it needs Lima and takes minutes: run it in the background. It builds `devvm` from the checkout into its sandbox, with `base.yaml` and `layers/` next to it, and runs every check through that binary:

| | |
| --- | --- |
| Lima home | `~/.lt-e2e-go` (or `$DEVVM_E2E_LIMA_HOME`); it refuses `~/.lima` and devvm's own |
| State | `/tmp/devvm-go-e2e` (`XDG_DATA_HOME`, `XDG_CONFIG_HOME`) |

```sh
tests/e2e.sh                # every section
tests/e2e.sh --only images  # just these sections
tests/e2e.sh --clean        # delete the sandbox's cached images first
```

The sections so far: `images` (building base and fixture layers, rebuilds, a failed script and Ctrl+C during a build). base stays cached between runs; every other VM a run makes is deleted at its end. Each check prints `[n/N]`, how long it took and the time left as the last complete run measured it, and a heartbeat every minute shows the latest output. Checks look for devvm's messages with `devvm _msg`, never by their wording.

## Messages

Every message devvm prints is a table in `locales/en.toml`, and Go code calls it through a function generated from it. `internal/msg` renders messages but never prints them; `internal/ui` does.

The locale files are in [go-i18n](https://github.com/nicksnyder/go-i18n)'s TOML format, with only go-i18n's own message fields, so go-i18n's loader reads them as they are and its `goi18n` tools work on them:

```toml
[no_vm]
other = "no VM for {{.dir}}: run devvm there to create one"

[vm_count]
description = "how many VMs a folder has"
one = "{{.count}} VM in {{.dir}}"
other = "{{.count}} VMs in {{.dir}}"
```

- **The ID** is the table's name, in snake_case.
- **`other`** is the text: a Go template whose fields are snake_case (`{{.vm_name}}`). Only plain fields are allowed, no logic.
- **`one`** is the text for a count of 1, when the wording changes with the count. Both texts then take `{{.count}}`.
- **`description`** is an optional note for translators.
- **Nothing else:** the generator refuses keys go-i18n doesn't know, nested tables, plural forms English doesn't use (`zero`, `two`, `few`, `many`) and `leftDelim`/`rightDelim`.
- **Style:** succinct and direct. Say what is being done, or what failed and what to do; link a page when more is needed. An error is one line.

### Doc links

A message links a docs page through the `docs` map in `internal/msg/docs.go`, from message ID to a page in `docs/content/docs` and a heading on it:

```go
var docs = map[string]string{
    "no_vm": "images#where-state-lives",
}
```

devvm prints the page's address on the docs site on the line after the text.

After editing `en.toml`, run:

```sh
mise exec -- go generate ./internal/msg
```

It writes `internal/msg/messages_gen.go`, with one function per message. The parameters are the template's fields in sorted order, in camelCase, and `count int` for plural messages: `[no_vm]` becomes `msg.NoVM(dir string) msg.M`, and a value that's missing or extra is a compile error. `msg.M` is also an `error`.

The tests fail when:

- `messages_gen.go` is stale;
- a message doesn't render with sample values, or shows `<no value>`;
- a generated function is never called outside the generated file and the tests;
- a doc link names a message that isn't in `en.toml`, or a page or heading that doesn't exist. `mise run lint` runs this check too.

### Names mode

With `DEVVM_MESSAGES=names`, a message prints as its ID and its values in sorted key order, so tests check which message devvm printed and with what, never its wording or language:

```txtar
env DEVVM_MESSAGES=names
! exec devvm _msg nope
stderr 'msg_unknown name=nope$'
```

`devvm _msg NAME key=value…` prints a message's text (never its names), for end-to-end tests that look for it in a VM's output. It's hidden from the help.

### Translations

A translation is `locales/<language>.toml` (`de.toml`) with the same tables, `one` and `other` translated (and the language's other plural forms, such as `few`), loaded by go-i18n the same way. Doc links are the same in every language: the docs site is English. devvm picks the language from the first of `LC_ALL`, `LC_MESSAGES` and `LANG` that is set (`de_DE.UTF-8` is German); `C` and `POSIX` are English. A message missing from a translation is shown in English.

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
