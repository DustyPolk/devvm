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
| goreleaser | release builds ([Maintaining](maintaining.md#releasing)) |
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
| `deadcode` | lists functions nothing calls outside the tests |
| `cli-docs` | generates the CLI reference |
| `docs` | generates the CLI reference, then serves the docs site |
| `e2e` | runs the end-to-end tests against real VMs |

`mise run test` needs a C compiler for the race detector: Xcode Command Line Tools on macOS, gcc on Linux.

Before and after every change, `mise run fmt && mise run lint && mise run test` must pass.

CI runs `lint` and `test` on macOS and Linux.

## CLI tests

CLI tests are [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) scripts in `testdata/script/*.txtar`, run by `internal/cli/script_test.go` against a freshly built `devvm`. Call it with `exec devvm …`:

```txtar
# An unknown flag fails.
! exec devvm --nope
stderr 'unknown flag: --nope'
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
| `DEVVM_KVM` | `stub/kvm`, a file standing in for Linux's `/dev/kvm` |
| `DEVVM_HOST_MISE` | the `mise` stub, which resolves the shared tools in place of devvm's own mise |
| `MISE_STUB_LOCK` | `testdata/mise.lock`, the lock the `mise` stub writes |
| `DEVVM_BIN` | the folder of the built `devvm`, with copies of `base.yaml`, `layers/`, `tools/`, `guest/` and `oci/` next to it |
| `DEVVM_REPO` | the checkout, for the scripts in `scripts/`, which tests copy into a throwaway repo before running |

`$WORK`, the script's own folder, is the current folder.

Tests never touch the network. Downloads are served from a test server (`net/http/httptest`) or read from a `file://` folder. `DEVVM_UBUNTU_URL` replaces `https://cloud-images.ubuntu.com/releases/resolute`, where devvm lists Ubuntu's dated releases: point it at a folder of `release-YYYYMMDD/` folders, each with its `SHA256SUMS` and `SHA256SUMS.gpg`, as `file:///path/to/folder`. Container registries (`--from`) are a TLS test server in Go tests; scripts use pinned digests, which need no lookup, or set `HTTPS_PROXY` to a closed port, so lookups fail as offline ones do.

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

`limactl` keeps its instances in `$STUB_DIR/instances` and fails or hangs on request (its header lists how). Its `shell` is also the shell devvm opens in a VM, which then ends, so a script sees it as devvm's last call. In the shared tools' build VM, its `shell` writes the images `build.sh` is asked for into the build's folder (or, on request, a symlink instead), and it keeps what a VM's setup is sent as `$STUB_DIR/sync.tar`. In the `--from` build VM, its `shell` writes what `oci/build.sh` would (or, on request, a hostile disk, command line, C library or symlink), and a call naming `$STUB_DIR/interrupt-on`'s text interrupts devvm, as Ctrl+C would. `docker` has an image only when `$STUB_DIR/docker-image` holds its ID, and its `save` writes a tarball naming it (or `$STUB_DIR/docker-saved`'s, a tag that moved); it isn't running with `$STUB_DIR/docker-down`. A direct connection from a VM (the self-check's) fails, as a confined VM's does, unless `$STUB_DIR/net-direct` exists. A `--clone` VM's git runs on the host, on `$STUB_DIR/vmfs/<folder>`, and `ssh` pushes to and fetches from there (it fails with `$STUB_DIR/ssh-fails`). `security` stands in for the Keychain: an item is `$STUB_DIR/keychain/<account>`. `tmutil` records exclusions in `$STUB_DIR/tm-excluded`, never in Time Machine. `mise` writes `$MISE_STUB_LOCK` as the lock (`$MISE_STUB_OLDER_LOCK` for a list of versions), lists `$MISE_STUB_VERSIONS`, and fails like mise offline with `MISE_STUB_FAIL` set. `gpgv` accepts every signature unless `$STUB_DIR/gpgv-bad` exists. `chflags` sets no flags, so `.git` protection leaves test folders deletable. `qemu-img` and QEMU are there so Linux hosts pass devvm's checks.

devvm runs in `$WORK` or a folder made in it (`mkdir`, then `cd`), never in `HOME` or devvm's own folders, which it refuses as project folders.

## End-to-end tests

`tests/e2e.sh` runs devvm against real VMs, so it needs Lima and takes minutes: run it in the background. It builds `devvm` from the checkout into its sandbox, with `base.yaml`, `layers/`, `tools/`, `guest/` and `oci/` next to it, and runs every check through that binary:

| | |
| --- | --- |
| Lima home | `~/.lt-e2e-go` (or `$DEVVM_E2E_LIMA_HOME`); it refuses `~/.lima` and devvm's own |
| State | `/tmp/devvm-go-e2e` (`XDG_DATA_HOME`, `XDG_CONFIG_HOME`) |
| Keychain | service `devvm-go-e2e` (`DEVVM_SECRET_SERVICE`), fake tokens only, deleted at the end |

```sh
tests/e2e.sh                # every section but from
tests/e2e.sh --from         # every section, from too: the longest
tests/e2e.sh --only images  # just these sections
tests/e2e.sh --clean        # delete the sandbox's cached images and tool images first
tests/e2e.sh --backend smolvm  # the smolvm section alone (needs smolvm, not Lima)
```

The sections so far:

- `images`: building base and fixture layers, rebuilds, a failed script and Ctrl+C during a build.
- `project`: devvm in a new project folder (under `/tmp/devvm-go-e2e/p`) makes a VM from base and opens a shell there, which reads its commands from devvm's input. The folder and the shared tools are mounted and nothing else is, there's no SSH agent and the hostname is the folder's. The ssh config lists it as `devvm-web` with its host keys pinned; through an `~/.ssh/config` of the run's own that includes it under a `Host *` forwarding the agent, `ssh devvm-web` works, forwards no agent (a throwaway one), and refuses a changed host key. `.git` is locked (macOS) and git in the VM can still commit. `devvm stop` stops it and drops its entry, and on macOS reports and locks a submodule git folder the VM added. `TestE2EExecQuoting` in `internal/backend/lima`, which runs only there (`DEVVM_E2E_VM` names the VM), checks that arguments with spaces, quotes, `$`, newlines and globs, a folder and input reach the guest as given. Then a claude VM with a fake agent config, history, git identity and token in a home of its own: the agent's folder is mounted read-only and linked into the VM's home, its settings are copied and its history isn't, the VM gets the git identity, the timezone and its hostname, and the token itself. Then SSH agents, with a home of their own and throwaway keys: a `--ssh-agent` VM uses a throwaway agent standing for yours; after `devvm ssh-agent add` and `devvm ssh-agent devvm` it uses only devvm's own agent's key, in its shells and by name; after `devvm ssh-agent off`, none.
- `gc`: a VM from a folder named `img-keep`, which is named like an image, and a fixture layer edited after its build: `devvm gc` deletes the stale image and its record and an old tool image no VM lists (once there's a tools lock), and keeps base and the VM; a second gc finds nothing.
- `tools`: first, the shared tools resolve to upstream's newest versions, with devvm's pinned mise (downloaded once), each download with its checksum; the rest of the run doesn't check again (`DEVVM_NO_TOOL_CHECK`). Then a node project's VM: node, npm, pnpm, bun and yarn at the locked versions, each a read-only EROFS image, the share read-only even for root, `npm i -g` into the VM's home, a non-login shell over SSH, an `.nvmrc`'s own version, the tools mounted at boot without devvm, and a second node VM that builds nothing.
- `network`: a claude+codex VM with `--network-policy`, a fake token in devvm's environment and a fake `~/.claude.json` and Codex login in a home of its own. Its proxy runs and the self-check passed; `ssh devvm-net` reaches it; direct connections fail, even as root, to an address and to other ports; an allowed host works and others, or other ports, are blocked; DNS fails at once; the host's loopback services are out of reach, directly or through the proxy; the proxy refuses requests without the VM's password; the token is nowhere in the VM, which has devvm's CA, its own placeholder and logins' copies with placeholders only; the proxy puts the token in for its host (for the VM's own placeholder only), an MCP server's header and Codex's key for theirs, over TLS the VM verifies; the host's login files are unchanged. `devvm secret set github --stdin` saves a fake token (in the Keychain on macOS, under the run's service, found through a link to `~/Library/Keychains` in the VM's home) and `secret get` reads it back; `secret use github` binds it to the running proxy at once, and on the next launch the VM has its own placeholder in `GH_TOKEN`, never the token, which the proxy puts in for `api.github.com`; `secret unuse` and `rm` take the binding and the token away. `TestE2ESelfCheck` in `internal/netpolicy` runs the self-check on the VM: it passes, then, once the VM is started outside devvm, it fails and stops the VM. `devvm stop` stops the VM and its proxy at once. `devvm network off` makes its next start direct, without a proxy; `on` while it runs shows it `unenforced` until `devvm restart` starts it confined; `off` while it runs confined keeps it on its proxy until it next starts.
- `smolvm` (`--backend smolvm`): the [smolvm backend](smolvm.md) on smolvm's own machines, which on macOS share yours: the run looks only at machines with its sandbox's label, deletes only those it made, and checks every other machine is as it was. base and a fixture layer built as packs, a rebuild, `images rm`, and a failed script's machine kept stopped; a new VM refused in one line for every port (the default), network policy and `--clone`; a project VM with its size, the folder read-write and files the VM makes yours, the tools read-only even for root, its hostname, `TestE2EExecQuoting` in `internal/backend/smolvm` (arguments, folder, input, the VM's user and login environment), Docker, a listed port on the host's loopback, `devvm ssh`, `network on`, `ports all` and `ssh-agent on` refused, a new size and ports applied on restart, stop, start and rm; a `--ssh-agent` VM using a throwaway agent, and turning it off refused; a node VM's uncompressed tool images.
- `from`: run with a `docker` that isn't running, so nothing comes from Docker. A node+python+claude VM from `node:24` (Debian 12, in a home of its own) boots, with its base keyed by the registry's digest, and its node, python and claude at the locked versions; an `alpine:3` VM runs node and pnpm from musl tool images and Docker on OpenRC, with no crashed services; base builds from `ubuntu:26.04`; and the build VM and its temporary folders are gone.

base, the `--from` bases and the shared tool images stay cached between runs, except what a rebuild or gc deletes: every stale image in the sandbox (other selections' bases, whose features differ, among them) and every unused tool image (an earlier `from` run's musl ones); every other VM a run makes is deleted at its end, and its project folders are unlocked and deleted. Each check prints `[n/N]`, how long it took and the time left as the last complete run measured it, and a heartbeat every minute shows the latest output. A check a run has nothing for is counted as `SKIP`: base's features, with base cached. A run with fewer checks than the last complete one fails, as a section ended early. Checks look for devvm's messages with `devvm _msg`, never by their wording.

[Testing on other hosts](testing.md) covers Linux hosts, x86_64 and the checks to run by hand.

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

### The CLI reference

The [CLI reference](cli/_index.md) in `docs/content/docs/cli/` is generated from devvm's commands with Cobra's docs generator, and checked in. Don't edit it: change the command's `Use`, `Short`, `Long` or flags, then run:

```sh
mise run cli-docs
```

It runs `internal/cli/docgen`, which replaces every page in that folder: `_index.md`, listing the commands, and a page per command named after its path (`devvm_build.md`), with `title` and `weight` front matter and links to the other pages as files. Hidden commands (`_msg`, `_qemu`) and Cobra's help command are left out. The pages carry no dates, so the same commands give the same files.

The tests fail when the checked-in pages are stale: `TestReferenceIsCurrent` in `internal/cli/docgen` generates the reference into a temporary folder and compares it with `docs/content/docs/cli/`. `mise run docs` regenerates it before serving the site.

### Building the site

`mise run docs` serves the site at `http://localhost:1313/devvm/` and reloads on every change. `mise exec -- hugo -s docs --minify` builds it into `docs/public/`, as the Pages workflow does on every push to `main`.
