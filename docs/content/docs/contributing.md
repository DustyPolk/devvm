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

The binary has devvm's own files (`assets/`) built in, so it runs from anywhere: link to it from your `PATH`, or copy it there.

```sh
ln -s "$PWD/devvm" ~/.local/bin/devvm
```

After editing anything in `assets/`, build again before a manual run: a binary keeps the files it was built with.

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

## Go lint rules

golangci-lint enforces the [Go conventions](https://github.com/w3cj/devvm/blob/main/AGENTS.md#go-conventions) in `.golangci.yml`:

| Rule | Enforces |
| --- | --- |
| forbidigo | only `internal/ui` prints (`fmt.Print*`, `os.Stdout`, `os.Stderr`); only `cmd/devvm` calls `os.Exit`; only `internal/ui` and `cmd/devvm` make a context (`context.Background`, `context.TODO`), every other function takes its caller's |
| depguard | the allowed modules; `internal/proxy` uses only the standard library; only `cmd/devvm` and `internal/cli` import `internal/ui` |
| nolintlint | every `//nolint` names its linters and says why, and is still needed |
| reassign | no package reassigns another package's variables, tests included |
| revive | at most 100 statements or 160 lines a function (`function-length`), and a cognitive complexity of 75 (`cognitive-complexity`) |

A fresh context elsewhere, such as for a cleanup that outlives a canceled run, or a lookup that many requests share, takes `//nolint:forbidigo // <why>`.

## The structure test

`internal/structure` parses every Go file in the repo, tests and every platform's included, generated files left out, and checks what linters don't. `mise run test` runs it. Each failure starts with its rule and the file and line:

| Rule | Checks |
| --- | --- |
| `max-methods` | no type has more than 24 methods, value and pointer receivers together; a larger one splits by what its methods do |
| `no-init` | no `func init()` |
| `package-vars` | outside `internal/proxy`, a package-level `var` is a blank interface check, an `Err*`/`err*` sentinel from `errors.New` or `fmt.Errorf`, a `regexp.MustCompile`, `template.Must` or `sync.Once*` result, a `//go:embed` file, or listed in `allowedVars`; anything else becomes a field, a const or a local |
| `layers` | which packages import which, in non-test files (tests may wire packages together) |

The layers:

- Only `cmd/devvm` and `internal/cli` import `internal/ui`; only `internal/cli` imports `internal/vm`; only `cmd/devvm` imports `internal/cli`.
- `internal/backend/...` imports none of `internal/vm`, `build`, `tools`, `netpolicy`, `secrets`, `project`, `gitprotect`, `sshconfig` or `sshagent`.
- Only `internal/backend/registry` and `internal/cli` import a backend (`lima`, `smolvm`, `docker`); only tests import `backendtest`.
- `internal/proxy` imports no other devvm package.
- `assets`, `locales`, `internal/platform` and `internal/msg/gen` import no devvm package; `internal/msg` only `locales`; `internal/paths` only `internal/msg`; `internal/state` only `internal/msg` and `internal/paths`; `internal/image` only `assets`, `internal/msg` and `internal/paths`.

Everything it holds the code to is in `devvm()` in `internal/structure/structure_test.go`:

- **`largeTypes`** lists the types allowed more methods (the backends, which implement all of `backend.Backend`, among them), each with its count as its maximum and why. One that grows fails; one that shrinks fails until its maximum is lowered.
- **`allowedVars`** lists the other package-level vars, by package folder and name, each with why: values set by `-ldflags`, fixed tables, test fixtures. An entry whose var is gone fails.
- **`layers`** is the layering table: a layer's packages (`from`, but not `exceptFrom`) import none of `to` (but `exceptTo`); `x/...` is `x` and the packages below it.

`TestRulesFail` breaks each rule, and each layer, on small sources and checks the failure names it.

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
| `DEVVM_BIN` | the folder of the built `devvm`, alone |
| `DEVVM_REPO` | the checkout, for the scripts in `scripts/`, which tests copy into a throwaway repo before running, and the scripts in `assets/`, which tests source to check their pieces |

The built `devvm` has the checkout's `assets/` embedded, as a release has, and no test build reads them from elsewhere: a script that needs other files for devvm (layers, features, policies) puts them in the user's folders in the sandbox, as a user would.

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

`limactl` keeps its instances in `$STUB_DIR/instances` and fails or hangs on request (its header lists how). Its `shell` is also the shell devvm opens in a VM, which then ends, so a script sees it as devvm's last call. In the shared tools' build VM, its `shell` writes the images `build.sh` is asked for into the build's folder (or, on request, a symlink instead), and it keeps what a VM's setup is sent as `$STUB_DIR/sync.tar`. In the `--from` build VM, its `shell` writes what `oci/build.sh` would (or, on request, a hostile disk, command line, C library or symlink), and a call naming `$STUB_DIR/interrupt-on`'s text interrupts devvm, as Ctrl+C would. `docker` has an image only when `$STUB_DIR/docker-image` holds its ID, and its `save` writes a tarball naming it (or `$STUB_DIR/docker-saved`'s, a tag that moved); it isn't running with `$STUB_DIR/docker-down`. As the Docker backend's engine, a container has an ID and a start time, and `exec` answers the sshd probe (none with `$STUB_DIR/no-sshd`) and the host key's `cat`. `ssh` answers a port forwarder's master with `$STUB_DIR/guest-ports`, and a Docker VM's shell over ssh reads its input and ends. A direct connection from a VM (the self-check's) fails, as a confined VM's does, unless `$STUB_DIR/net-direct` exists. A `--clone` VM's git runs on the host, on `$STUB_DIR/vmfs/<folder>`, and `ssh` pushes to and fetches from there (it fails with `$STUB_DIR/ssh-fails`). `security` stands in for the Keychain: an item is `$STUB_DIR/keychain/<account>`. `tmutil` records exclusions in `$STUB_DIR/tm-excluded`, never in Time Machine. `mise` writes `$MISE_STUB_LOCK` as the lock (`$MISE_STUB_OLDER_LOCK` for a list of versions), lists `$MISE_STUB_VERSIONS`, and fails like mise offline with `MISE_STUB_FAIL` set. `gpgv` accepts every signature unless `$STUB_DIR/gpgv-bad` exists. `chflags` sets no flags, so `.git` protection leaves test folders deletable. `qemu-img` and QEMU are there so Linux hosts pass devvm's checks.

devvm runs in `$WORK` or a folder made in it (`mkdir`, then `cd`), never in `HOME` or devvm's own folders, which it refuses as project folders.

## End-to-end tests

`tests/e2e.sh` runs devvm against real VMs, so it needs Lima and takes minutes: run it in the background. It builds `devvm` from the checkout into its sandbox, its files embedded, and runs every check through that binary:

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
tests/e2e.sh --backend docker  # the Docker section alone: on macOS, a Linux engine in a VM, then Docker Desktop
tests/e2e.sh --backend docker --engine desktop  # one of the two
```

The sections so far:

- `images`: building base and fixture layers, rebuilds, a failed script and Ctrl+C during a build.
- `project`: devvm in a new project folder (under `/tmp/devvm-go-e2e/p`) makes a VM from base and opens a shell there, which reads its commands from devvm's input. The folder and the shared tools are mounted and nothing else is, there's no SSH agent, no Docker (no docker layer) and the hostname is the folder's. The ssh config lists it as `devvm-web` with its host keys pinned; through an `~/.ssh/config` of the run's own that includes it under a `Host *` forwarding the agent, `ssh devvm-web` works, forwards no agent (a throwaway one), and refuses a changed host key. `.git` is locked (macOS) and git in the VM can still commit. `devvm stop` stops it and drops its entry, and on macOS reports and locks a submodule git folder the VM added. `TestE2EExecQuoting` in `internal/backend/lima`, which runs only there (`DEVVM_E2E_VM` names the VM), checks that arguments with spaces, quotes, `$`, newlines and globs, a folder and input reach the guest as given. Then a claude VM with a fake agent config, history, git identity and token in a home of its own: the agent's folder is mounted read-only and linked into the VM's home, its settings are copied and its history isn't, the VM gets the git identity, the timezone and its hostname, and the token itself. Then SSH agents, with a home of their own and throwaway keys: a `--ssh-agent` VM uses a throwaway agent standing for yours; after `devvm ssh-agent add` and `devvm ssh-agent devvm` it keeps yours, in its shell and by name, until a restart, then uses only devvm's own agent's key, in its shells and by name; after `devvm ssh-agent off`, none.
- `gc`: a VM from a folder named `img-keep`, which is named like an image, and a fixture layer edited after its build: `devvm gc` deletes the stale image and its record and an old tool image no VM lists (once there's a tools lock), and keeps base and the VM; a second gc finds nothing.
- `tools`: first, the shared tools resolve to upstream's newest versions, with devvm's pinned mise (downloaded once), each download with its checksum; the rest of the run doesn't check again (`DEVVM_NO_TOOL_CHECK`). Then a node project's VM: node, npm, pnpm, bun and yarn at the locked versions, each a read-only EROFS image, the share read-only even for root, `npm i -g` into the VM's home, a non-login shell over SSH, an `.nvmrc`'s own version, the tools mounted at boot without devvm, and a second node VM that builds nothing.
- `network`: a claude+codex VM with `--network-policy`, a fake token in devvm's environment and a fake `~/.claude.json` and Codex login in a home of its own. Its proxy runs and the self-check passed; its password was in no host process's arguments while devvm launched it (`ps`, sampled), and the guest has it; `ssh devvm-net` reaches it; direct connections fail, even as root, to an address and to other ports; an allowed host works and others, or other ports, are blocked; DNS fails at once; the host's loopback services are out of reach, directly or through the proxy; the proxy refuses requests without the VM's password; the token is nowhere in the VM, which has devvm's CA, its own placeholder and logins' copies with placeholders only; the proxy puts the token in for its host (for the VM's own placeholder only), an MCP server's header and Codex's key for theirs, over TLS the VM verifies; the host's login files are unchanged. `devvm secret set github --stdin` saves a fake token (in the Keychain on macOS, under the run's service, found through a link to `~/Library/Keychains` in the VM's home) and `secret get` reads it back; `secret use github` binds it to the running proxy at once, and on the next launch the VM has its own placeholder in `GH_TOKEN`, never the token, which the proxy puts in for `api.github.com`; `secret unuse` and `rm` take the binding and the token away. `TestE2ESelfCheck` in `internal/netpolicy` runs the self-check on the VM: it passes, then, once the VM is started outside devvm, it fails and stops the VM. `devvm stop` stops the VM and its proxy at once. `devvm network off` makes its next start direct, without a proxy; `on` while it runs shows it `unenforced` until `devvm restart` starts it confined; `off` while it runs confined keeps it on its proxy until it next starts.
- `smolvm` (`--backend smolvm`): the [smolvm backend](smolvm.md) on smolvm's own machines, which on macOS share yours: the run looks only at machines with its sandbox's label, deletes only those it made, and checks every other machine is as it was. base and a fixture layer built as packs, a rebuild, `images rm`, and a failed script's machine kept stopped; a new VM refused in one line for every port (the default); a project VM with its size, the folder read-write and files the VM makes yours, the tools read-only even for root, its hostname, `TestE2EExecQuoting` in `internal/backend/smolvm` (arguments, folder, input, the VM's user and login environment), no Docker (no docker layer), a listed port on the host's loopback, `ports all` and `ssh-agent on` refused, a new size and ports applied on restart, a listed port a host program has on every address left to it until it's freed, stop, start and rm; a `--ssh-agent` VM using a throwaway agent, and turning it off refused; ssh for editors' emulation ([its checks](design.md#ssh-for-editors-on-smolvm)): pinned host keys of each VM's own, its own key alone, no forwarding out even past a hostile sshd, `devvm ssh` and `code`, a VM `--from alpine:3` reached over ssh, a `--clone` VM's push and fetch; network policy's emulation, on a docker VM (no agent's login read): the VM behind its proxy and decider, the proxy's password in no host process's arguments while devvm launched it and in the guest's environment from its proxy file and Docker's daemon, Docker pulling through the proxy, its allow list and closed DNS, allowed and blocked hosts, direct connections failing even as root (to the host's loopback too), DNS failing at once and every probe query (TXT, ANY, AAAA, HTTPS, EDNS0, under `devvm.invalid`) refused in smolvm's decision log with no flow beyond the host's loopback (nettop, macOS), `devvm.invalid` as the log's control, UDP and ICMP getting nowhere, the decider's token nowhere in the VM or a process's arguments, `devvm log`, `policy allow` and `rm` at once, an allow list widened while it runs making it `unenforced`, nothing out with the proxy gone or a decider that answers garbage or nothing, no start without a decider, a start with someone else's decider stopped, and `network off` and `on`, where the direct, DNS and UDP probes get out (their controls); a docker VM `--from alpine:3` with network policy, its Docker restarted by devvm's setup and pulling through the proxy; a node VM's uncompressed tool images.
- `docker` (`--backend docker`): the [Docker backend](docker.md), with state under `/tmp/devvm-go-e2e-docker`, on an engine where only what has its sandbox's label is touched, and what hasn't must be as it was at the end. On a Linux engine of the machine's own (on Linux; on macOS in the docker host VM below, from a `devvm` built here for it, its results counted here): doctor, base and a fixture layer built by committing containers, the layer's build reaching the internet and the host's server on every address but not its loopback-only one, a rebuild, `images rm`, a failed script's container kept stopped; a project VM with its CPUs and memory, the folder read-write and what the VM writes yours (uid and gid), its home a volume of its own, the tools' volume read-only even for root, its hostname, no Docker, `devvm size`'s `disk_used`, stop and start keeping its disk; refused in one line, nothing made: network policy, the docker layer, a disk size; ssh for editors', ports' and agent forwarding's emulations ([their checks](design.md#ssh-for-editors-on-docker)): no port published and no sshd listening, pinned host keys of each VM's own, its own key alone, no forwarding out even past a hostile sshd, `devvm ssh` and `code`, `devvm _docker-connect` refusing another devvm's container, a VM started outside devvm without ssh, every port and listed ports through devvm's forwarder, never a port of this machine's programs (on its `127.0.0.1`, `::1` or every address) or another VM's, the agent in ssh sessions alone (a throwaway one standing for yours, devvm's, none), a `--clone` VM's push and fetch; an agent VM in a home of its own whose added and removed folders make its container again from a commit, its disk and home kept, the commit before deleted, rm deleting its commit and volumes; a node VM whose tools are linked from Docker's tools volume, read-only even for root. Then on Docker Desktop: doctor naming it and its file sharing, a build refused in one line with nothing made, base and the shared tools copied from the Linux engine's run (Desktop can't build yet; the same paths give them the same label), and the project, agent and node VMs' checks again, with `TestE2EExecQuoting` in `internal/backend/docker`; at the end everything with the sandbox's label is deleted.
- `from`: run with a `docker` that isn't running, so nothing comes from Docker. A node+python+docker+claude VM from `node:24` (Debian 12, in a home of its own) boots, with its base keyed by the registry's digest, its node, python and claude at the locked versions, and the docker layer's Docker from Docker's Debian repository; a node+docker VM from `alpine:3` runs node and pnpm from musl tool images and the docker layer's Docker on OpenRC, with no crashed services; base builds from `ubuntu:26.04`; and the build VM and its temporary folders are gone.

base, the `--from` bases and the shared tool images stay cached between runs, except what a rebuild or gc deletes: every stale image in the sandbox (other selections' bases, whose features differ, among them) and every unused tool image (an earlier `from` run's musl ones); every other VM a run makes is deleted at its end, and its project folders are unlocked and deleted. Each check prints `[n/N]`, how long it took and the time left as the last complete run measured it, and a heartbeat every minute shows the latest output. A check a run has nothing for is counted as `SKIP`: base's features, with base cached. A run with fewer checks than the last complete one fails, as a section ended early. Checks look for devvm's messages with `devvm _msg`, never by their wording.

[Testing on other hosts](testing.md) covers Linux hosts, x86_64 and the checks to run by hand.

## Messages

Every message devvm prints is a table in `locales/en.toml`, and Go code calls it through a function generated from it. `internal/msg` renders messages but never prints them; the run's `ui.UI` does. Only `cmd/devvm` and `internal/cli` hold it: other packages print through `Info`/`Warn` fields and the like, which cli sets from it.

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

It writes `internal/msg/messages_gen.go`, with one function per message. The parameters are the template's fields in sorted order, in camelCase, and `count int` for plural messages: `[no_vm]` becomes `msg.NoVM(dir string) msg.M`, and a value that's missing or extra is a compile error. `msg.M` is also an `error`: one made from another error carries it with `.Because(err)`, so `errors.Is` and `errors.As` see through it, while the text stays the message's own.

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
