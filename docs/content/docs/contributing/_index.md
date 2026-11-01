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
| actionlint, zizmor | linting the GitHub workflows, and checking them for security problems |
| minisign | signing releases |

Run the tasks with `mise run <task>`:

| Task | Does |
| --- | --- |
| `fmt` | formats Go code |
| `lint` | runs `lint:code` and `lint:docs` |
| `lint:code` | runs golangci-lint, shellcheck, actionlint and zizmor |
| `lint:docs` | runs the docs linters and the message link check |
| `test` | runs the tests with the race detector and coverage |
| `vuln` | runs govulncheck |
| `deadcode` | lists functions nothing calls outside the tests |
| `cli-docs` | generates the CLI reference |
| `docs` | generates the CLI reference, then serves the docs site |
| `e2e` | runs the end-to-end tests against real VMs |

`mise run test` needs a C compiler for the race detector: Xcode Command Line Tools on macOS, gcc on Linux.

Before and after every change, `mise run fmt && mise run lint && mise run test` must pass.

CI runs `lint` and `test` on macOS and Linux. A change that touches only the docs and Markdown files skips them: the Docs workflow runs `lint:docs` and builds the site instead, and deploys it on every push to `main`.

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
- `assets`, `locales`, `internal/platform` and `internal/msg/gen` import no devvm package; `internal/msg` only `locales`; `internal/paths` only `internal/msg`; `internal/state` only `internal/msg` and `internal/paths`; `internal/image` only `assets`, `internal/msg` and `internal/paths`; `internal/devcontainer` only `internal/image` and `internal/msg`.

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
| `DEVVM_UBUNTU_URL` | `ubuntu/`, with the pinned dated release |
| `DEVVM_KVM` | `stub/kvm`, a file standing in for Linux's `/dev/kvm` |
| `DEVVM_TEST_CPUS`, `DEVVM_TEST_MEMORY_GIB`, `DEVVM_TEST_FREE_GIB` | 8, 16 and 500: a large host, whatever runs the tests ([details](testing.md#a-host-of-any-size)) |
| `DEVVM_HOST_MISE` | the `mise` stub, which resolves the shared tools in place of devvm's own mise |
| `MISE_STUB_LOCK` | `testdata/mise.lock`, the lock the `mise` stub writes |
| `DEVVM_BIN` | the folder of the built `devvm`, alone |
| `DEVVM_REPO` | the checkout, for the scripts in `scripts/`, which tests copy into a throwaway repo before running, and the scripts in `assets/`, which tests source to check their pieces |

The built `devvm` has the checkout's `assets/` embedded, as a release has, and no test build reads them from elsewhere: a script that needs other files for devvm (layers, features, policies) puts them in the user's folders in the sandbox, as a user would.

`$WORK`, the script's own folder, is the current folder.

Tests never touch the network. Downloads are served from a test server (`net/http/httptest`) or read from a `file://` folder. `DEVVM_UBUNTU_URL` replaces `https://cloud-images.ubuntu.com/releases/resolute`, where devvm finds Ubuntu's pinned dated release: point it at a folder with a `release-YYYYMMDD/` folder for the pin, with its `SHA256SUMS` (listing the pinned checksums) and `SHA256SUMS.gpg`, as `file:///path/to/folder`. Container registries (`--from`) are a TLS test server in Go tests; scripts use pinned digests, which need no lookup, or set `HTTPS_PROXY` to a closed port, so lookups fail as offline ones do.

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
- `project`: devvm in a new project folder (under `/tmp/devvm-go-e2e/p`) makes a VM from base and opens a shell there, which reads its commands from devvm's input. The folder and the shared tools are mounted and nothing else is, there's no SSH agent, no Docker (no docker layer) and the hostname is the folder's. The ssh config lists it as `devvm-web` with its host keys pinned; through an `~/.ssh/config` of the run's own that includes it under a `Host *` forwarding the agent, `ssh devvm-web` works, forwards no agent (a throwaway one), and refuses a changed host key. `.git` is locked (macOS) and git in the VM can still commit. `devvm stop` stops it and drops its entry, and on macOS reports and locks a submodule git folder the VM added. `TestE2EExecQuoting` in `internal/backend/lima`, which runs only there (`DEVVM_E2E_VM` names the VM), checks that arguments with spaces, quotes, `$`, newlines and globs, a folder and input reach the guest as given. Then a VM from a `devcontainer.json`, with no ports by default: its `forwardPorts` said and not forwarded, none reaching this machine, its `containerEnv` in the shell and its commands, its `postCreateCommand` run once in the folder, its `postStartCommand` at each start (again after `devvm restart`), nothing written to the folder, and a changed file warned about without changing the VM. Then a claude VM with a fake agent config, history, git identity and token in a home of its own: the agent's folder is mounted read-only and linked into the VM's home, its settings are copied and its history isn't, the VM gets the git identity, the timezone and its hostname, and the token itself. Then SSH agents, with a home of their own and throwaway keys: a `--ssh-agent` VM uses a throwaway agent standing for yours; after `devvm ssh-agent add` and `devvm ssh-agent devvm` it keeps yours, in its shell and by name, until a restart, then uses only devvm's own agent's key, in its shells and by name; after `devvm ssh-agent off`, none.
- `gc`: a VM from a folder named `img-keep`, which is named like an image, and a fixture layer edited after its build: `devvm gc` deletes the stale image and its record and an old tool image no VM lists (once there's a tools lock), and keeps base and the VM; a second gc finds nothing.
- `tools`: first, the shared tools resolve to upstream's newest versions, with devvm's pinned mise (downloaded once), each download with its checksum; the rest of the run doesn't check again (`DEVVM_NO_TOOL_CHECK`). Then a node project's VM: node, npm, pnpm, bun and yarn at the locked versions, each a read-only EROFS image, the share read-only even for root, `npm i -g` into the VM's home, a non-login shell over SSH, an `.nvmrc`'s own version, the tools mounted at boot without devvm, and a second node VM that builds nothing.
- `network`: a claude+codex VM with `--network-policy`, a fake token in devvm's environment and a fake `~/.claude.json` and Codex login in a home of its own. Its proxy runs and the self-check passed; its password was in no host process's arguments while devvm launched it (`ps`, sampled), and the guest has it; `ssh devvm-net` reaches it; direct connections fail, even as root, to an address and to other ports; an allowed host works and others, or other ports, are blocked; DNS fails at once; the host's loopback services are out of reach, directly or through the proxy; the proxy refuses requests without the VM's password; the token is nowhere in the VM, which has devvm's CA, its own placeholder and logins' copies with placeholders only; the proxy puts the token in for its host (for the VM's own placeholder only), an MCP server's header and Codex's key for theirs, over TLS the VM verifies; the host's login files are unchanged. `devvm secret set github --stdin` saves a fake token (in the Keychain on macOS, under the run's service, found through a link to `~/Library/Keychains` in the VM's home) and `secret get` reads it back; `secret use github` binds it to the running proxy at once, and on the next launch the VM has its own placeholder in `GH_TOKEN`, never the token, which the proxy puts in for `api.github.com`; `secret unuse` and `rm` take the binding and the token away. `TestE2ESelfCheck` in `internal/netpolicy` runs the self-check on the VM: it passes, then, once the VM is started outside devvm, it fails and stops the VM. `devvm stop` stops the VM and its proxy at once. `devvm network off` makes its next start direct, without a proxy; `on` while it runs shows it `unenforced` until `devvm restart` starts it confined; `off` while it runs confined keeps it on its proxy until it next starts.
- `nix`: the nix layer. A copy of `assets/layers/nix.sh` as a layer of the run's own, built behind a company proxy of the run's own, reaches Nix's releases through it and leaves no proxy settings for `nix-daemon` (and not the company proxy's password) anywhere on its image's disk. Then a folder with `package.json`, `flake.nix` and `.envrc` gets a node+nix VM with network policy: Nix and direnv at their pins, a store that's root's, `sandbox` on with no fallback, `trusted-users` root alone and an untrusted user's `--option substituters` ignored, `nix-daemon` with the VM's own proxy in devvm's drop-in alone, `nix run nixpkgs#hello` from Nix's cache, a build in Nix's sandbox, `sudo nix` and `nix` over ssh, the flake's dev shell (nixpkgs at a pinned revision) loaded by direnv in an interactive shell, its `node` ahead of the shared node prompt after prompt and gone outside the folder, nix-direnv's cache in the VM's home with no `.direnv` in the folder, a fixed-output download from a host that isn't allowed refused with `403` and named by `devvm-why`, and after `devvm network off` and `devvm restart`, no drop-in and no proxy for the daemon.
- `smolvm` (`--backend smolvm`): the [smolvm backend](../backends/smolvm.md) on smolvm's own machines, which on macOS share yours: the run looks only at machines with its sandbox's label, deletes only those it made, and checks every other machine is as it was. base and a fixture layer built as packs, a rebuild, `images rm`, and a failed script's machine kept stopped; a project VM with its size, the folder read-write and files the VM makes yours, the tools read-only even for root, its hostname, `TestE2EExecQuoting` in `internal/backend/smolvm` (arguments, folder, input, the VM's user and login environment), no Docker (no docker layer), a listed port on the host's loopback, a server on the VM's loopback alone reaching it, an unlisted port not forwarded, smolvm publishing none of the ports, a port added to a running VM reaching this machine at once and gone once removed, the VM never restarted, a new size applied on restart and new ports at once, a listed port a host program has on every address left to it until it's freed, stop, start and rm; a `--ssh-agent` VM using a throwaway agent, and none once turned off and restarted; ssh for editors' emulation ([its checks](../internals/design.md#ssh-for-editors-on-smolvm)): pinned host keys of each VM's own, its own key alone, no forwarding out even past a hostile sshd, `devvm host` and `code`, a VM `--from alpine:3` reached over ssh, a `--clone` VM's push and fetch; network policy's emulation, on a docker VM (no agent's login read): the VM behind its proxy and decider, the proxy's password in no host process's arguments while devvm launched it and in the guest's environment from its proxy file and Docker's daemon, Docker pulling through the proxy, its allow list and closed DNS, allowed and blocked hosts, direct connections failing even as root (to the host's loopback too), DNS failing at once and every probe query (TXT, ANY, AAAA, HTTPS, EDNS0, under `devvm.invalid`) refused in smolvm's decision log with no flow beyond the host's loopback (nettop, macOS), `devvm.invalid` as the log's control, UDP and ICMP getting nowhere, the decider's token nowhere in the VM or a process's arguments, `devvm network log`, `network allow` and `rm` at once, an allow list widened while it runs making it `unenforced`, nothing out with the proxy gone or a decider that answers garbage or nothing, no start without a decider, a start with someone else's decider stopped, and `network off` and `on`, where the direct, DNS and UDP probes get out (their controls); a docker VM `--from alpine:3` with network policy, its Docker restarted by devvm's setup and pulling through the proxy; a node VM's uncompressed tool images; a nix VM with network policy, whose `nix-daemon` devvm starts with the VM's proxy in `/etc/conf.d/nix-daemon`, running `nix run nixpkgs#hello` through the proxy and a build in Nix's sandbox, then after `devvm network off` and `devvm restart` a daemon without the proxy.
- `docker` (`--backend docker`): the [Docker backend](../backends/docker.md), with state under `/tmp/devvm-go-e2e-docker`, on an engine where only what has its sandbox's label is touched, and what hasn't must be as it was at the end. On a Linux engine of the machine's own (on Linux; on macOS in the docker host VM below, from a `devvm` built here for it, its results counted here): doctor, base and a fixture layer built by committing containers, the layer's build reaching the internet and the host's server on every address but not its loopback-only one, a rebuild, `images rm`, a failed script's container kept stopped; a project VM with its CPUs and memory, the folder read-write and what the VM writes yours (uid and gid), its home a volume of its own, the tools' volume read-only even for root, its hostname, no Docker, `devvm size`'s `disk_used`, stop and start keeping its disk; a disk size and the nix layer refused in one line, nothing made; the docker layer ([its checks](../internals/design.md#the-docker-layer-on-docker)): on the Linux engine (Sysbox installed in the docker host VM, pinned) under Sysbox, on its own network, and refused for a VM that exists; refused with network policy, and on Docker Desktop; ssh for editors', ports' and agent forwarding's emulations ([their checks](../internals/design.md#ssh-for-editors-on-docker)): no port published and no sshd listening, pinned host keys of each VM's own, its own key alone, no forwarding out even past a hostile sshd, `devvm host` and `code`, `devvm _docker-connect` refusing another devvm's container, a VM started outside devvm without ssh, every port and listed ports through devvm's forwarder, never a port of this machine's programs (on its `127.0.0.1`, `::1` or every address) or another VM's, the agent in ssh sessions alone (a throwaway one standing for yours, devvm's, none), a `--clone` VM's push and fetch; an agent VM in a home of its own whose added and removed folders make its container again from a commit, its disk and home kept, the commit before deleted, rm deleting its commit and volumes; a node VM whose tools are linked from Docker's tools volume, read-only even for root. Then on Docker Desktop: doctor naming it and its file sharing, a build refused in one line with nothing made, base and the shared tools copied from the Linux engine's run (Desktop can't build yet; the same paths give them the same label), and the project, agent and node VMs' checks again, with `TestE2EExecQuoting` in `internal/backend/docker`; at the end everything with the sandbox's label is deleted.
- `from`: run with a `docker` that isn't running, so nothing comes from Docker. A node+python+docker+nix+claude VM from `node:24` (Debian 12, in a home of its own) boots, with its base keyed by the registry's digest, its node, python and claude at the locked versions, the docker layer's Docker from Docker's Debian repository, and the nix layer's Nix under systemd running `nix run nixpkgs#hello` and a build in Nix's sandbox; a node+docker+nix VM from `alpine:3` runs node and pnpm from musl tool images, the docker layer's Docker on OpenRC, with no crashed services, and the nix layer's Nix as an OpenRC service, with `nix run nixpkgs#hello` and a sandboxed build, then after `devvm network on` and `devvm restart` a download only its daemon makes from a host that isn't allowed refused with the proxy's `403`; base builds from `ubuntu:26.04`; and the build VM and its temporary folders are gone.

base, the `--from` bases and the shared tool images stay cached between runs, except what a rebuild or gc deletes: every stale image in the sandbox (other selections' bases, whose features differ, among them) and every unused tool image (an earlier `from` run's musl ones); every other VM a run makes is deleted at its end, and its project folders are unlocked and deleted. Each check prints `[n/N]`, how long it took and the time left as the last complete run measured it, and a heartbeat every minute shows the latest output. A check a run has nothing for is counted as `SKIP`: base's features, with base cached. A run with fewer checks than the last complete one of the same selection (and Docker engines) fails, as a section ended early. Checks look for devvm's messages with `devvm _msg`, never by their wording.

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

A message links a docs page through the `docs` map in `internal/msg/docs.go`, from message ID to a page in `docs/content/docs` and a heading on it. The page is its path without `.md`, and a section's own page, its `_index.md`, is the section's folder:

```go
var docs = map[string]string{
    "vm_outdated":   "using/images#outdated-vms",
    "backend_lacks": "backends#when-a-backend-lacks-something",
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

### Colors

Only `internal/ui` adds color, through a `style` for each stream that `onTerminal` makes, and `colorOn` decides each one: `NO_COLOR` or `DEVVM_MESSAGES=names` turns it off, then `FORCE_COLOR` or `CLICOLOR_FORCE` (but `0`) turns it on, then `TERM=dumb` turns it off, and otherwise the stream gets color only when it's a terminal. Names mode stays plain in a terminal too, so end-to-end checks that run devvm under `script` read the names as they are. A `UI` from `New`, as tests make, has no terminal, so it's plain unless `FORCE_COLOR` or `CLICOLOR_FORCE` is set, which is how cli's tests get a colored one.

The colors are SGR codes from the 16 base colors only, which the theme maps to its own shades. devvm uses no bright colors (90–97), because Solarized maps them to greys and bright black is Solarized Dark's background, and it uses no backgrounds, 256 colors or truecolor. The codes go on labels, links, log text and a message's values, never on the template's own words:

- A message's values are bold (`1`), on the status line and in prompts too. `msg.M.Segments` renders the template with each value between two private-use runes, `U+E000` and `U+E001`, after dropping those runes from the values, so a value can't start or end a marking of its own. It returns the text cut into segments, each one the template's words or a value, beside the doc link. Joined, the segments are `Text`, and `String`, `Line` and names mode don't change. Values a VM made still go through `msg.Shown` first, so their control characters are quoted, bold or not. The log end that `DieLogged` prints uses `Line`, because bold inside dim would cancel it.
- `Die`'s `devvm:` label is bold red (`1;31`), and `Warn`'s is bold yellow (`1;33`).
- A doc link is underlined blue (`4;34`). `msg.M.Segments` gives the doc link apart from the text, so `ui` colors the link alone.
- `Info`'s `==>` is bold (`1`).
- The run log's end that `DieLogged` prints, with its `---` lines, and the log text on the status line are dim (`2`).
- The done mark that a slow step leaves is green (`32`).

The codes are added after `printable` and `msg.Shown`, so they're the only ones devvm prints. The status line cuts and counts its plain text with `style.fit` before painting it, so it still fits the terminal's width and clears the right number of rows. The run log is never colored.

Tables on `Stdout()` are colored as stdout is. A command gets a `ui.Table` from `u.Table()` and adds a header, rows of `ui.Cell`s and a totals row, and cli never reads the environment itself. Each cell has a `ui.Role`: `Good` is green (`32`), `Attention` yellow (`33`), `Bad` red (`31`), `Secondary` dim (`2`), and `Plain` stays in the default color, while the header and totals rows are bold. `Table` pads each cell on its plain text before painting it, because `text/tabwriter` counts the codes' bytes as width, and a plain table is byte for byte what `text/tabwriter` writes with a padding of 2 (`TestTableLikeTabwriter`). The roles for each table's values are next to the code that writes it:

- devvm images: `current` is good, `stale`, `unfinished`, `unused` and `not built` need attention, and digests and keys are secondary.
- devvm ls: `Running` and the `.git` protections `on`, `strict` and `clone` are good, `Stopped` needs attention, and any other status, such as `Broken`, is bad, as is the network policy `unenforced`, while `on` is good.
- devvm space: `only copy` and `leftover` in KEPT need attention.
- devvm doctor: `native` is good, `emulated` needs attention, and `unreviewed` and `none` are bad.

`doctor --fixes` lines and devvm space `--json` are for scripts and stay plain.

`mise run test` unsets `NO_COLOR`, `FORCE_COLOR` and `CLICOLOR_FORCE`, and `tests/e2e.sh` unsets the last two, so your own settings never change what they check.

### Translations

A translation is `locales/<language>.toml` (`de.toml`) with the same tables, `one` and `other` translated (and the language's other plural forms, such as `few`), loaded by go-i18n the same way. Doc links are the same in every language: the docs site is English. devvm picks the language from the first of `LC_ALL`, `LC_MESSAGES` and `LANG` that is set (`de_DE.UTF-8` is German); `C` and `POSIX` are English. A message missing from a translation is shown in English.

## Writing docs

The site is [Hugo](https://gohugo.io) with the [Hextra](https://imfing.github.io/hextra/) theme, built from `docs/`. Hextra is a Hugo module pinned in `docs/go.mod`; update it with `hugo mod get github.com/imfing/hextra@<version>` in `docs/`.

- **Pages** are Markdown files in `docs/content/docs/`, one per topic, in a folder per section of the sidebar. The sections run in the order a new user needs them: `getting-started/`, `using/`, `safety/`, `backends/`, `cli/`, `internals/`, then `contributing/`. Each folder's `_index.md` is the section's own page, which lists its pages. Update the page for any behavior you change.
- **What goes where:** the user pages, in `getting-started/`, `using/`, `safety/` and `backends/`, say what devvm does, what you type and what you see, and they name no repo paths, Go identifiers or message names. How it works, in `internals/`, explains how devvm does it, for readers who want to check a claim, and it holds the mechanisms and the message names that come from them. Contributing, in `contributing/`, covers changing devvm, so repo paths, Go identifiers, version bumps, releases and tests go there. A user page links once to its How it works page where there is one.
- **Front matter** sets `title`, shown as the page's heading, and `weight`, the page's place in its section, or a section's place in the sidebar for an `_index.md`. Start the body at `##`.
- **Links** to other pages point at their files (`[Contributing](_index.md)`), so the linters can check them.
- **Diagrams** are Mermaid code blocks, fenced with `mermaid`, which the browser draws when the page loads. `docs/layouts/_partials/scripts/mermaid.html` takes the place of Hextra's partial of the same name. It loads the Mermaid version that `hugo.yaml` pins in `params.mermaid.base`, which Hugo fetches when it builds the site, and it gives every diagram the same theme, so a diagram never sets its own colors or font. The theme uses the site's palette: panels on the surface color with blue borders, lime arrows, and text in Departure Mono at 14px, larger than the home page's art so labels read easily. It uses Mermaid's classic look, because its newer neo look draws a decision's hexagon 70px taller than its label. A diagram can set its own layout in front matter, as Staying safe's three columns set `minNodeWidth`, but never its own colors or font. Mermaid draws each diagram at its own size and never scales it to fit, and a diagram wider than the page scrolls sideways. Keep labels short, and break a long one with `<br>`. In a label, write `<` as `#lt;` and `>` as `#gt;`. A diagram goes where it helps the reader follow the text, with at most one sentence to introduce it, and the text keeps every fact the diagram shows.
- **Style:** succinct and direct. Commands are written as devvm x or in backticks, never in quotes. No `please`, `simply` or `note that`.
- **The home page** is `docs/content/_index.md`, on Hextra's `hextra-home` layout: its hero shortcodes, then plain Markdown. `custom.css` styles it through the one `div` that the layout wraps its content in. After the hero, the page has a quick start, what's supported, why to use devvm with its feature cards, a short FAQ, and a closing call to action with the hero's buttons. It keeps a marketing tone, but each of its claims matches the docs page it links to, it uses the same terms, and it says each thing once.
- **The quick start** shows the three commands to run first, each after a comment short enough to fit a phone's width, because a comment after its command would scroll out of view there.
- **Feature cards** on the home page come from two shortcodes in `docs/layouts/_shortcodes/`, which make a `{{</* features */>}}` grid of `{{</* feature title="Ready in seconds" icon="high-voltage" art="copies" */>}}` cards, each closed with `{{</* /feature */>}}`. A card shows its title as a heading, after the pixel icon its `icon` names, drawn in the title's own color at 24px, then the Markdown inside it, then its illustration, which is optional. The cards are in one centered column on smaller screens. From 1024px wide, they go into two columns, the first, third and so on on the left and the rest on the right, and each card is as tall as its content, so no card leaves a gap under it. `features` places them: `feature` renders its card through `docs/layouts/_partials/feature-card.html` and hands it over, and each card's `order` style keeps the page's order when the columns stack. What's supported uses the same cards with smaller illustrations. `{{</* features columns="3" */>}}` puts three in a row from 1024px wide, and `level="3"` on a card makes its title an `h3`, under the section's own heading.
- **Art beside a section's text** comes from `{{</* beside art="blast-radius" */>}}`, closed with `{{</* /beside */>}}`, in `docs/layouts/_shortcodes/beside.html`. It renders the Markdown inside it, then the illustration it names. From 1024px wide, the text and the drawing each take half the column, with the drawing on the right, centered on the text. On narrower screens, the drawing goes below the text. Why use devvm? uses it.
- **Install commands** come from `{{</* install */>}}` in `docs/layouts/_shortcodes/install.html`, which the home page and Installing use. The commands are written in one place, `docs/layouts/_partials/install-tabs.html`, which the tabs and the pages' Markdown copies both read. It renders Hextra's tabs with a macOS tab, which has the Homebrew command, and a Linux tab, which has the install script. `{{</* install step="1" */>}}` numbers each tab's comment as a list's first step, so the commands after it go in a code block of their own, starting at step 2. The macOS tab is selected in the HTML, so it shows when JavaScript is off. A small script in `docs/layouts/_partials/install-os.html` selects the Linux tab as the page loads when the browser's platform or user agent string says Linux, apart from Android. To check the Linux tab in headless Chrome, pass `--user-agent=` with a Linux user agent string. `custom.css` styles the tabs on a blue line, with the selected tab in lime.
- **Other content per operating system** goes in `{{</* os-tabs */>}}`, with a `{{</* os-tab name="macOS" */>}}` and then a `{{</* os-tab name="Linux" */>}}` inside it, each holding Markdown, as Installing's requirements tables do. It uses the same tabs, styles and script as the install commands, so the visitor's system is selected, and the build fails unless the tabs are macOS's, then Linux's.
- **Colors:** the site is dark only (`hugo.yaml`'s `params.theme`). Its palette is in `docs/assets/css/custom.css`: a black background and one lime accent, `#7fff00`, which sets Hextra's primary color. The home page's illustrations also use `#0086b3`, the blue that Hextra's syntax highlighting gives commands like `cd`, and `#ff5370` (`--devvm-red`), the JustBlack theme's red, which marks only what an attack has compromised. The red's contrast is 6.7:1 on the black and 6.0:1 on the surface color, and it's much darker than the lime, so the two stay apart for readers who can't tell red from green.
- **Fonts:** Instrument Sans for text and Martian Mono for code, from Google Fonts (`docs/layouts/_partials/custom/head-end.html`); Departure Mono, a pixel font in `docs/static/fonts/` with its license, for the logo, the home page's headings, badge and buttons. Departure Mono is sharp at multiples of 11px and has one weight.
- **The logo** in the navbar is the branding page's `devvm-logo-on-dark.svg` at its own size, which `hugo.yaml`'s `params.navbar.logo` names, with the site's title as its alt text. Right-clicking it opens the branding page instead of the browser's menu, through a script in `docs/layouts/_partials/custom/footer.html`. A left click, or Enter on the focused logo, still goes to the home page.
- **The branding page** is `docs/content/branding.md`, at `/branding/`, on its own layout, `docs/layouts/branding.html`, which is Hextra's home layout with the home page's look and the script that copies a swatch's hex. Its front matter keeps it out of the sidebar. It has the logo as box-drawing text in a code block, the logo's SVGs and PNGs, the icons, the social card, the palette, the fonts, how to use the logo and the credits.
  - **Its downloads** are in `docs/static/branding/`, and `{{</* brand-asset */>}}` tiles inside `{{</* brand-assets */>}}` show each one on its background, with a link to each file. The build fails when a file doesn't exist, and it reads each PNG's size for its link. `devvm-logo-on-dark.svg` and `devvm-logo-on-light.svg` are the logo, drawn on a pixel grid as the favicon is, made from Departure Mono's 11px glyphs. The prompt and devvm are at twice the glyphs' size, the box is in 1-pixel lines, and the lock, a block with a keyhole under a 2-pixel arch, sits on the box's bottom-right corner, with the box's lines stopping short of it so that the arch stands clear. The version for light backgrounds is black, with its prompt in the dark lime, `#408000` (`--devvm-accent-dark`), which is the lime's hue at half its lightness, with a contrast of 4.9:1 on white, where the lime has 1.2:1. Each logo's PNG is its SVG at four times its size on a transparent background, and `devvm-icon-64.png` to `devvm-icon-1024.png` are `docs/static/favicon.svg` at 4 to 64 times its size. After changing an SVG, render its PNGs again with headless Chrome's `--screenshot` at the exact size, passing `--default-background-color=00000000` for the logos' transparency, and look at each one.
  - **The palette** comes from `{{</* palette */>}}`, which reads the colors that `docs/data/palette.yaml` lists, with each one's name and use, and takes each hex from its variable in `custom.css`, so the page always shows the site's colors. The build fails when a variable isn't there. A new color goes in both files.
  - **The fonts** each come from `{{</* font-sample */>}}`, which shows a line of text in the font, with a link to the license the site serves for Departure Mono.
- **The favicon** is the same mark drawn on a 16-pixel grid in `docs/static/favicon.svg`: the box, a lime prompt in its middle and a small lock centered on its bottom-right corner, overhanging it by 2 pixels each way, its shackle a closed arch, with the box's lines stopping a pixel short of the lock, on black, with a pixel of black around the whole mark. Hextra links its icons by name, and the ones in `docs/static/` take the place of its own: `favicon-16x16.png` and `favicon-32x32.png`, `favicon.ico`, which holds those two PNGs, `apple-touch-icon.png`, which is the grid at 160px with 10px of black around it, and the two `android-chrome-*.png` icons that `site.webmanifest` lists. Each PNG is the SVG at a whole multiple of its size, so its pixels stay sharp. **The social card** in `docs/static/images/social-card.png` is the 1200x630 image shown with a link to the site when someone shares it, and `hugo.yaml`'s `params.images` names it, so every page gets it as its `og:image` and `twitter:image`. It has `devvm-logo-on-dark.svg` at twice its size, the home page's headline and tagline in Departure Mono, and the site's colors. After changing the SVG or the card's text, render the PNGs again with headless Chrome's `--screenshot` at the image's exact size, and look at each one.
- **The illustrations** on the home page are SVGs in `docs/assets/home/`, and `{{</* art "name" */>}}` inlines one into the page, so that it gets the page's fonts and colors. A feature card's `art` parameter inlines one the same way, through `docs/layouts/_partials/art.html`. Each one is drawn on Departure Mono's 11px cell, 7 pixels wide and 14 tall, so its text and shapes are sharpest at its own size or at twice its size. The `art` shortcode shows it at twice its size, 728px wide. A card shows it as wide as the card, at whatever scale that takes.
  - **Each card's drawing shows what its text says, and nothing more.** The big cards' drawings are 364 by 140 pixels, or 168 for `isolated.svg`: `zero-config.svg` shows a dev server's forwarded port, `devvm code` and a commit authored by you, `copies.svg` the base image and its copies, `isolated.svg` the mount and the proxy, `agents.svg` the agents' mounts, the history that stays and the token, and `make-it-yours.svg` the stack of base, your layer and the VM, with `--from`. The What's supported cards' drawings are 224 by 70 pixels: `toolchains.svg`, `coding-agents.svg` and `backends.svg`. Why use devvm?'s drawing, `blast-radius.svg`, is 364 by 210 pixels and shows the same four attacks as its text on two machines. Without devvm, they light up on your machine, and the project, `~/.aws`, `~/.ssh` and another project turn red one after another. With devvm, they light up in the VM, each one's path toward your machine ends at the VM's border with a cross, and your machine's folders stay as they are.
  - **Every text element holds one run of characters at its own `x`,** because Hugo's `--minify` removes the spaces between `tspan` elements. A gap of a cell is 7 pixels added to `x`.
  - **Animations** are CSS keyframes in `custom.css`, which classes in the SVGs turn on. `pulse` dims and brightens a lime arrow every 3 seconds, and `pulse-2` and `pulse-3` start a part of the arrow 0.3 and 0.6 seconds later, so the pulse runs along it. `pulse-slow` does the same every 4 seconds. `lit` lights a text's fill and `lit-chip` a chip's opacity for most of a 4-second loop, and `lit-2`, `lit-3` and `lit-4` start them 0.5, 1 and 1.5 seconds later, so they light one after another. `blink` turns a cursor on and off every 2 seconds. `mover` marks a block that travels a path, `mover-token` along the token's line, under the proxy and down to the API in 3 seconds, and `mover-egress` under the proxy, down to the allowlist and back, then out to the internet in 4 seconds. A block is drawn before the shapes it passes under. `attack-1` to `attack-4`, `path-1` to `path-4` and `reach-1` to `reach-4` tell `blast-radius.svg`'s story in an 8-second loop, and every part of the drawing stays in place all along, so the story only changes colours and draws lines. Each attack's chip turns from gray to lime at 0.4, 1.6, 2.8 and 4 seconds. In the VM, its path then draws itself down to the VM's border, and when it gets there, the cross at the border turns lime and, on the machine without devvm, the folder it reaches turns red. Everything holds from 5.2 seconds, and from 6.6 seconds the colours go back and the paths draw back up, so the drawing rests from 7.2 seconds until the story starts again. Each step has its own keyframes, so that all of them end together. The SVG draws each part in its end colour, and `--rest` in `custom.css` gives the colour it has before its step, so a drawing without animation shows the end of the story. When the visitor's system asks for reduced motion, `prefers-reduced-motion` turns every animation off and hides the blocks.
  - **To see an animation's state in a screenshot,** pause the animations with a stylesheet of your own and set each class's `animation-delay` to minus the time you want, because headless Chrome's `--virtual-time-budget` barely moves CSS animations.
- **Icons** are pixel icons in `docs/assets/icons/`, one SVG per icon. Most come from [Dinkie Icons](https://github.com/atelier-anchor/dinkie-icons), which is MIT licensed, and the folder keeps its license and a README that names the version they were copied from. The penguin, prompt, warning and light bulb icons are drawn for devvm in the same style. `{{</* icon "rocket" */>}}` inlines one through `docs/layouts/_partials/icon.html`, so it takes the text's color, and the build fails when no icon has that name. An icon is 0.75em square, which makes each of its pixels one screen pixel in 16px text, and it sits on the text's baseline. Screen readers skip it, unless a second argument gives it a label, as in `{{</* icon "warning" "Warning" */>}}`, for an icon that says something the text doesn't. To add a Dinkie icon, copy its path data from the same version of the `@dinkie-icons/react` npm package into a new file named after the icon in kebab case, with the same `svg` attributes as the others. A new icon of your own uses the same 12-pixel grid and 1-pixel lines, and the README lists it.
  - **Each section's icon** is chosen once, in `docs/data/sections.yaml`, which maps a section's folder under `docs/` to an icon's name, as in `getting-started: rocket`. A new section gets a line there. The section cards and the sidebar both read it, so they always show the same icon, and the build fails when the list names a section or an icon that doesn't exist.
  - **Section cards** on the docs' first page are Hextra's `{{</* cards */>}}` holding a `{{</* section-card path="getting-started" */>}}` for each section, with its one-line description inside, closed with `{{</* /section-card */>}}`. The shortcode takes the section's title and link from its page under `docs/`, and its icon from `sections.yaml`, and the build fails when there's no such page. Hextra's card takes an icon only from Hextra's own set, so the shortcode puts ours in the card's title, at 24px. It's dimmed as Hextra dims its own cards' icons, and it turns lime on hover.
  - **The sidebar's top-level items** show their section's icon at 12px, in the link's color, so it's gray, and lime for the section you're in. Hextra's sidebar has no icons, so `docs/layouts/_partials/custom/head-end.html` writes a CSS rule for each section when the site builds, keyed to the section page's address, with the icon from `docs/assets/icons/` as a mask. Nothing is generated into the repo, so there's no step to rerun.
  - **The OS tabs** of `install` and `os-tabs` show the Apple icon before macOS and the penguin before Linux, which `docs/layouts/_partials/os-tab-name.html` adds to each tab's name.
  - **The experimental badge**, `{{</* experimental */>}}`, is the flask icon and the word experimental in Departure Mono at 11px, in the site's blue, in a rounded box. It reads as the word, so it can take the word's place in a sentence, as on the home page's Backends card. The smolvm and Docker pages show it above their first line, and the backends' comparison table beside their names.
  - **✓ and ✗** go before a table cell's word: `{{</* yes */>}} yes` puts a lime check before it, and `{{</* no */>}} doesn't` a gray cross. They go only in cells that are a plain yes or no, or what stands for one, such as the install script's offers it, never in cells that are sentences, and the word stays, because screen readers skip the icon. Icons in tables are 12px, because a table's text is smaller than 16px.
  - **Buttons** take an icon before their text: `docs/layouts/_shortcodes/hextra/hero-button.html` is Hextra's `hero-button` with an `icon` parameter, as in `{{</* hextra/hero-button text="Install" link="docs/getting-started/install/" icon="download" */>}}`, drawn at 24px like the headings' icons. The 404 page's two buttons have their icons in its layout, a rocket for Getting started and the open book for the docs, as the home page's Read the docs has it.
  - **The navbar's GitHub link** uses Dinkie's GitHub icon, which `docs/data/icons.yaml` adds to Hextra's own icons as `dinkie-github`, and `hugo.yaml`'s menu names. Hextra's icon partial reads only that data, so an icon for the navbar goes there as its SVG, not in `docs/assets/icons/`.
- **Heading icons:** a heading gets an icon before its text with an `icon` attribute, as in `## Quick start {icon="rocket"}`, which the home page's section headings use. `docs/layouts/_markup/render-heading.html` takes the place of Hextra's heading render hook and adds the icon after Hugo makes the heading's anchor, so the anchor comes from the text alone, such as `quick-start`. On the home page, `custom.css` draws these icons in lime at 24px, so each icon pixel is two screen pixels, as each of Departure Mono's pixels is at 22px.
- **Callouts** are GitHub's alerts: a blockquote whose first line is `[!NOTE]`, `[!TIP]` or `[!WARNING]`, which GitHub shows the same way. `docs/layouts/_markup/render-blockquote-alert.html` takes the place of Hextra's alerts and draws each one on the surface color with a line down its left side, and a title with its icon: information-circled in blue for a note, the light bulb in the soft lime for a tip, and warning in lime for a warning. Any other type fails the build. Hextra's own `callout` shortcode isn't used. Use a callout only for a real warning, such as Docker's isolation, or a tip that saves the reader trouble, such as keeping network policy on, and move the sentence into it instead of repeating it.
- **For agents and other tools that read the site,** `hugo.yaml` turns on three things that Hextra has. The site's root has `llms.txt`, which lists every section and its pages. Each page has a Markdown copy beside its HTML, such as `/docs/using/clone.md` beside `/docs/using/clone/`, and a section's copy is `index.md` in its folder, such as `/docs/using/index.md`. Each docs page has a Copy page menu beside its title, which copies the page's Markdown or opens it. `hugo.yaml` lists every output for the home page, the sections and the pages, so the home page and the sections keep their RSS feeds, and the search index is built apart from these outputs.
  - **`llms.txt`** comes from `docs/layouts/llms.txt`, which takes the place of Hextra's. It has the site's title and description, then each section under its title, with the section's own page first and its pages after it. Each one is a link to its Markdown copy, as the llms.txt convention prefers, which `docs/layouts/_partials/llms-link.txt` writes, and a line of plain text, which `docs/layouts/_partials/llms-summary.txt` writes. A page without a copy gets a link to its HTML page instead. The line is the page's `description` from its front matter when it has one, and otherwise its first paragraph, cut at 200 characters. A `description` is also the page's description for search engines and link previews, so give a page one when its first paragraph doesn't say what the page covers, as Coding agents has. `llms.txt` also lists the CLI reference, with each command's one-line summary.
  - **The Markdown copies** come from `docs/layouts/page.markdown.md` and `section.markdown.md`, which take the place of Hextra's and both use `docs/layouts/_partials/markdown-copy.md`. A copy is the page's title as a heading, then its Markdown with its shortcodes rendered. In a copy, a link to a section's `_index.md` points at `index.md`, the section copy's name, so the links work between the copies, and a link written in code stays as it is.
  - **Each shortcode that docs pages use has a Markdown version** beside its HTML one in `docs/layouts/_shortcodes/`, named after it with `.markdown.md`, which renders it in the copies. `install` writes each system's name in bold, then its commands, which come from `docs/layouts/_partials/install-tabs.html`, the same partial its tabs use. `os-tabs` writes its tabs the same way, through `docs/layouts/_partials/os-sections.md`. `section-card` makes a list item that links the section's copy, and Hextra's `cards` keeps only what's inside it. `yes` and `no` become ✓ and ✗, and `experimental` becomes the word in parentheses. A shortcode without a Markdown version puts its HTML into the copy.
  - **The home page and the branding page** are mostly shortcodes, and neither layout shows the menu. `hugo.yaml` gives the home page no Markdown copy, and its front matter sets `contextMenu: false`. The branding page's front matter sets `outputs: [html]`, `contextMenu: false` and `llms: false`, so it has no copy, no menu and no line in `llms.txt`. Any other page can set `llms: false` to stay out of `llms.txt`, or `contextMenu: false` to have no menu.
  - **After you add a page or a shortcode,** build the site and read the page's line in `llms.txt` and its Markdown copy. The line should say what the page covers, and the copy should have no shortcode left in it, such as `{{</* os-tabs */>}}`, and no HTML from one. A shortcode that docs pages use gets its Markdown version. To try the menu, serve the site with `mise run docs` and open a docs page. `custom.css` gives the menu the site's border and secondary text, lime on hover, and its list the surface color.
- **The 404 page** is `docs/layouts/404.html`, which takes the place of Hextra's and uses the site's navbar and footer. It shows a VM's terminal answering `cd ./this-page` with "no such page", drawn like the home page's art in `docs/assets/home/not-found.svg` with the `blink` cursor, then Hextra's search box and links to Getting started and the docs. Build the site and open `/404.html` to see it.

`mise run lint:docs`, which `mise run lint` includes, checks `docs/` with:

| Linter | Checks | Config |
| --- | --- | --- |
| markdownlint-cli2 | Markdown | `.markdownlint-cli2.yaml` |
| Vale | the style rules | `.vale.ini`, `.vale/styles/devvm/` |
| lychee (`--offline`) | links to pages and files | |

### The CLI reference

The [CLI reference](../cli/_index.md) in `docs/content/docs/cli/` is generated from devvm's commands with Cobra's docs generator, and checked in. Don't edit it: change the command's `Use`, `Short`, `Long` or flags, then run:

```sh
mise run cli-docs
```

It runs `internal/cli/docgen`, which replaces every page in that folder: `_index.md`, listing the commands, and a page per command named after its path (`devvm_build.md`), with `title` and `weight` front matter and links to the other pages as files. Hidden commands (`_msg`, `_qemu`) and Cobra's help command are left out. The pages carry no dates, so the same commands give the same files.

The tests fail when the checked-in pages are stale: `TestReferenceIsCurrent` in `internal/cli/docgen` generates the reference into a temporary folder and compares it with `docs/content/docs/cli/`. `mise run docs` regenerates it before serving the site.

### Building the site

`mise run docs` serves the site at `http://localhost:1313/` and reloads on every change. `mise exec -- hugo -s docs --minify` builds it into `docs/public/`, as the Pages workflow does on every push to `main`.
