# AGENTS.md

devvm: one isolated VM per project folder, cloned from a base image, with toolchains and agents shared read-only between VMs. It's a Go CLI that runs its VMs on Lima, behind a backend interface another backend could implement. Read [README.md](README.md) for what it is, and the [docs site](docs/content/docs/) for how each part works; [Contributing](docs/content/docs/contributing/_index.md) covers building, the tools, testscript and writing docs.

## Layout

| Path | What |
| --- | --- |
| `cmd/devvm/` | the entry point: `ui.Main` makes the run's UI and runs the root command with it; the only `os.Exit` |
| `internal/cli/` | Cobra commands, one file each (root = up, `build`/`rebuild`, `images`, `gc`, `space`, `retention`, `layer` (its new, add and rm), `setup`, `restart`, `tools`, `adopt`, `rm`, `stop`, `update`, `history`, `ls`, `network` (its rules and log too), `doctor`, `secret`, `link` (its add, rm, publish and unpublish too), `group` (join, leave), `ssh-agent`, `host`, `code`, `desktop`, `limactl`, `ports`, `size`, `git-protect`, `clone`, `completion`, hidden `version`, `_msg`, `_qemu`, `_proxy`, `_ports` (`forwarder.go`), `_links`, `_refresh-tools`); `args.go` checks arguments and flags (usage errors), `flags.go` the flags several commands share, `word.go` refuses devvm WORD and help's unknown words (a part's name points to its flag; did-you-mean over commands, `msg.Closest`); `help.go` the help's groups, `Example`s and docs links (`msg.CommandDocURL`); `json.go`: `--json` (`printJSON`, `"version": 1`), its shapes in `json_test.go`; `script_test.go` runs testscript; `docgen/` writes the CLI reference |
| `internal/vm/` | `Env` (`env.go`: shared dependencies, finding and starting the folder's VM); `Launch`: a new VM (`up.go`; `parts.go`: its toolchains, agents and layers, the picker, setup's agents and layers; `detect.go`: what the folder's files call for), an existing one (`existing.go`), `ssh.go`, agents' mounts; `Lifecycle` (`lifecycle.go`; `remake.go`: the one remake, a VM made again from another image, devvm layer add, devvm layer rm, devvm update and devvm clone; `home.go`: its home folder's copy); `Tools` (`tools.go`: a VM's shared tools); `Settings` (`settings.go`, `size.go`, `ports.go`, `sshagent.go`); `Net` (`network.go`, `secret.go`); `Links` (`links.go`: links between VMs, published ports), `Groups` (`groups.go`: the links a group makes); `--clone` (`clone.go`: the clone, rm's check, devvm clone's switch); a devcontainer.json's mapping and commands (`devcontainer.go`); the setup it sends (`sync.go`) |
| `internal/shell/`, `internal/vmname/` | stdlib leaves: quoting for a POSIX shell (`Quote`, `QuoteJoin`) and its variables' names (`EnvName`); a project VM's name grammar (`Prefix`, `HashLen`, `IsVM`, `HashOf`, `SlugOf`, a remake's `Aside`) |
| `internal/project/`, `internal/gitprotect/`, `internal/hostsafety/`, `internal/hostgit/` | a project folder (VM name, link targets, unsafe folders, parents); `.git` protection; the folder's other host checks (set-id files, skipping `.git`, once stopped; an SSH origin); every host git command devvm runs (no fsmonitor, hooks or filters, ssh only, no `GIT_*` of the environment, no prompts) |
| `internal/sshconfig/`, `internal/sshagent/` | the ssh config for running VMs by name, the `~/.ssh/config` Include line; each VM's SSH agent choice, devvm's own agent and its keys |
| `internal/build/`, `internal/image/` | building and rebuilding images, their records; image keys and names (pure) |
| `internal/fetch/` | every HTTP client (stdlib): `NewClient` (timeout, `HTTPSOnly`/`SameScheme`), `Retry`/`Transient`, `ReadCapped`, `Get`, `Download` (checks the SHA-256) |
| `internal/from/` | `--from`: references, registry digests (anonymous token), Docker's IDs, the recorded digests |
| `internal/redact/` | passwords out of logs (stdlib): `String` (the run log's lines: proxy keys, URL passwords), `File` (another program's log, in place, its length kept) |
| `internal/devcontainer/`, `internal/jsonc/` | a project's devcontainer.json (pure): finding it, mapping it to a new VM's plan, refusals; JSON with comments (`jsonc.Strip`, stdlib) |
| `internal/tools/` | the shared tools: the lock, host mise, the daily check, tool image builds, the share |
| `internal/netpolicy/`, `internal/secrets/` | network policy (the choice, the VM's proxy, the self-check); tokens on the host and the VM's bindings |
| `internal/trust/` | the user's CAs (`~/.config/devvm/ca`): read and checked, sent to VMs, build VMs' `Trust` scripts, the guest environment; each VM's own CA and its next (`InitCA`, in `vms/<hash>/run/ca/`) |
| `internal/agentlogin/` | agents' login and settings files on the host: the VM's copies with placeholders, bindings, values and OAuth refresh (`File`), `Redact` |
| `internal/proxy/` | the proxy, run as `devvm _proxy`; see its `README.md` |
| `internal/daemon/` | devvm's background processes (stdlib only): detached starts, pid files checked against the process's arguments, stopping (TERM, then KILL), readiness by status file, the lifeline; the only place a process is signalled by pid (forbidigo) |
| `internal/desktop/` | devvm desktop's relay on the host: VNC's password checked here, then an ssh channel to the VM's socket, the viewer's clipboard dropped; passwords |
| `internal/backend/` | `Backend` and its roles, `Caps`, `Require`, `Check`, the registry and its guard (`As`); `backendtest/` (fake), `registry/` (every backend's), `lima/` |
| `internal/vmssh/` | devvm's own over a VM's ssh, on every backend: the port forwarder and its one owner (`Forwards`: lock, stop, start, its ports from vm.json, the backend's `SSH` checked by `FromTarget`), links' sessions, each VM's one host-key record per start (`PinHostKeys`, `vms/<hash>/run/known_hosts`) that every ssh of devvm's checks |
| `internal/ui/` | the only package that prints: `ui.UI` (messages, prompts, status line, run log, cleanups), `ui.Main` (signals); no package state |
| `internal/msg/`, `locales/` | messages: `en.toml`, the generated functions, doc links (`docs.go`), the generator (`gen/`) |
| `internal/ports/` | a VM's ports section (forwards asked for, host ports chosen) and the one allocator of host ports, under the claims lock |
| `internal/paths/`, `internal/state/` | every folder and file devvm uses, a VM's in its folder (`Dirs.VM`: `vms/<hash>/`), host paths resolved and compared (`Physical`, `PhysicalParent`, `Within`, `Below`: `resolve.go` says which for what); a VM's state, `vm.json` (core fields, one section per package, changed only in `VM.Update` under the VM's lock); the layout version (`CheckLayout`, `migrations`; `testdata/script/layout.txtar` lists every file); every file written whole (`WriteFile`, `WriteFileMode`: forbidigo bans `os.WriteFile`, `os.Create` and `O_TRUNC` elsewhere but the proxy), private folders (`PrivateDir`) |
| `internal/settings/` | the defaults (`Defaults`, `Read`, `Save`): `config.toml` (yours, strict) over `settings.json` (devvm's), each value's check (`ParsePorts`, `ParseCPUs`, `ParseGiB`, the retention settings) |
| `internal/platform/`, `internal/ubuntu/`, `internal/testbuild/` | build-tagged OS code; finding and verifying Ubuntu's cloud image (never downloads); whether this is a test build (ldflags: only it reads the test overrides, `cli`'s `readOverrides`) |
| `assets/` | the files devvm ships, embedded (`assets.FS`, passed to constructors by `cli`): `base.yaml`, `prelude.sh` (`$devvm_arch`, `$devvm_libc`, `devvm_sha`: first in each build script run with bash, read through `image.OwnScript`), `layers/` (base's Lima template and scripts; the built-in layers, `desktop`, `docker` and `nix`, each with its policy), `features/` (base's optional ones), `shared-tools/` (`mise.toml`, `build.sh`, `*.policy`), `guest/` (in-VM scripts sent on every launch, and the port forwarder's `ports.sh`), `oci/` (`--from`'s conversion; see its `README.md`) |
| `internal/structure/` | the structure test (tests only) |
| `internal/stubtest/` | the test stubs (tests only): `stub/`, one Go binary for every host command, dispatched on `argv[0]` (each stub's comment lists its switches; `guest.go`, the VM behind limactl shell and ssh; `stub`, scripts' commands on its state), built once into `$TMPDIR/devvm-stubs-<hash>`; `stubstate/`, their typed JSON state under a lock; `stubtest` puts them on `PATH`: their folder, calls, files and state, `ExecRunner`; `msg.Recorder`, `msg.IDOf` and `msg.NamesOf` are tests' too, `backendtest.Stored` a fake with disks |
| `testdata/` | `script/*.txtar` (CLI tests), `golden/<os>/` (what the real tools printed, `tests/e2e.sh --golden`; the stubs and devvm's parsers are checked against it), `mise.lock` (fixture), `coverage/<os>.txt` (each package's coverage baseline, per system), `lint/` (a line breaking each forbidigo rule, which `lint:code` checks) |
| `tests/e2e.sh` | end-to-end tests on real VMs (bash 3.2); `e2e-mise.lock`, the shared tools' versions every section but `tools` uses (`scripts/bump.sh e2e-tools`) |
| `scripts/`, `packaging/`, `.goreleaser.yaml`, `VERSION`, `CHANGELOG.md` | `bump.sh` (pins), `release.sh` (signed-tag checks around GoReleaser, then minisign), `test.sh` and `coverage.sh` (mise's test tasks); the Homebrew formula; `VERSION` is read by the scripts only |
| `install.sh` | `curl \| bash`: the host's archive (the binary, README, changelog, license), checked, into `~/.local/lib/devvm/<version>`, then doctor's fixes, asking first; tests source it (`DEVVM_INSTALL_TEST`) |
| `docs/` | the docs site (Hugo + Hextra); `content/docs/cli/` is generated, never edit it |
| `mise.toml`, `.golangci.yml`, `.vale*`, `.markdownlint-cli2.yaml`, `.github/` | tools, tasks, linters, CI and Pages |
| `.agents/skills/security-audit/` | the security-audit skill, for backend reviews |
| `.agents/skills/cli/` | the cli skill: devvm's command line rules, the checklist for adding a command, the tests behind each rule, `judgment.md` for the rest |
| `.agents/skills/docs-voice/` | the docs-voice skill: the voice of the prose and messages, terms, tropes, `scan.sh` and `facts.sh` |

Still to come: the release keys (`packaging/minisign.pub`, `packaging/release-signer`).

## Tasks

`mise run <task>`: `fmt`; `lint` (`lint:code`: Go, shellcheck, workflows with actionlint and zizmor; `lint:docs`: docs, `TestDocLinks`, `TestCommandLines`); `test` (`scripts/test.sh`: `-race`, in a throwaway `HOME`; needs a C compiler; then each package's coverage, unit and CLI tests merged, against `testdata/coverage/<os>.txt`); `coverage` (the same, then raises the baselines it beat: commit them with the change); `vuln`; `deadcode` (lists only `backendtest`, `stubtest`, `msg.IDOf`, `msg.NamesOf` and `msg.Recorder`, which tests use; anything else gets a caller or goes); `cli-docs` (after changing any command's `Use`, `Short`, `Long` or flags; commit the pages); `docs` (serves `http://localhost:1313/`); `e2e`.

## Rules

- **The gate:** `mise run fmt && mise run lint && mise run test`, all passing, before starting a task and before marking it done; a gate failing at the start gets fixed first. Anything touching VMs also passes `tests/e2e.sh` in the background: the sections for what changed (`--only`).
- **Nothing in project folders:** state goes in `~/.local/share/devvm` and `~/.config/devvm`.
- **devvm's own files never get a shared copy on disk:** the proxy reads them from the binary, base.yaml and scripts go on stdin, `--from`'s build writes its scripts into its own temp folder.
- **Credentials never go into images.** Host agent history never goes into VMs.
- **Never auto-delete project VMs:** they may hold unsaved work. Warn instead.
- **The startup check only reports.** Never install host software without asking.
- **Image keys hash everything that goes into an image**, the backend included. A tool image's key is its lock entry, `assets/shared-tools/build.sh` after the prelude, `shared-tools/mise.toml`'s `[settings]`, the mise `layers/base.sh` pins (not rust's), the libc and the base it builds in: devvm's own Ubuntu base for gnu, the only one gnu ones build in, and for musl its `--from` base's digest (`Share.Build` checks the base's record).
- **Pin every download** in `assets/`'s `layers/`, `features/`, `shared-tools/build.sh`, `oci/build.sh`: `<tool>_version` and `<tool>_sha256_arm64`/`_amd64` at the top, picked with the prelude's `devvm_sha`, fetched with `devvm-fetch` (`oci/build.sh`: `sha256sum`). No `curl | sh`, no "latest". Upstream-checked exceptions: shared tools (`MISE_LOCKED=1`), Ubuntu's signed `SHA256SUMS`, Lima in `install.sh`. Go checks a download's SHA-256 before use.
- **Shared tools are read-only in VMs:** whatever writes into a tool's folder (global installs, self-updates, `corepack enable`, `rustup component add`) is redirected (`~/.npm-global`, `go_set_gobin = false`), done in `assets/shared-tools/build.sh`, or off.
- **In-VM scripts are bash 3.2-safe and shellcheck-clean** (`assets/`, `install.sh`, `tests/e2e.sh`): guard empty arrays (`${a[@]+"${a[@]}"}`), no associative arrays. Everything else is Go.
- **Docs:** an easy-to-follow guide in full, joined sentences, every fact kept; write and edit prose with the docs-voice skill (Vale's `devvm.AIWords` fails on chatbot vocabulary). Every behavior change updates its page in `docs/content/docs/` in the same task. The README stays high level.
- **Adding or changing a command** follows the cli skill (`.agents/skills/cli/`): its checklist, then its review.
- **Adding or changing a shared tool** updates its page: `using/toolchains.md` for a toolchain, `using/coding-agents.md`'s Supported agents for an agent.
- **Docs by reader:** user pages (`getting-started/`, `using/`, `safety/`, `backends/`) say what devvm does and what you type, without repo paths, Go identifiers or message names; How it works (`internals/`) explains mechanisms; Contributing (`contributing/`) has repo paths and Go identifiers.
- **Extend the owner, don't copy it:** before adding a helper, parser, path, file reader, message or table, find its owner and extend it; a new owner gets a forbidigo rule or a structure-test check only when a second copy would be a bug ([Contributing](docs/content/docs/contributing/_index.md#one-owner-for-each-concern)).
- **Edit files with the editor tools,** not `sed`, `awk`, `python`, heredocs or `cat >`.
- **Ask first** before anything outward-facing (publishing, releases, repos, pushing) and before deleting anything you didn't create.
- **A task that turns out wrong or incomplete** gets a proposed change (`dex edit`), not quietly something else.

## Go conventions

- **Only `internal/ui` prints.** forbidigo bans `fmt.Print*`, `os.Stdout`/`os.Stderr` elsewhere and `os.Exit` outside `cmd/devvm`. Data goes to the UI's `Stdout()`, non-message log lines to its `Stderr()`.
- **Functions take their caller's `ctx`:** forbidigo bans `context.Background`/`TODO` outside `internal/ui` and `cmd/devvm`; one that must outlive its caller's gets `//nolint:forbidigo // why`.
- **Every `//nolint` names its linters and says why** (nolintlint). Functions stay within revive's `function-length` and `cognitive-complexity` limits: split one that grows past them.
- **Only `cmd/devvm` and `internal/cli` import `internal/ui`** (depguard): cli passes the run's `*ui.UI` to each command; other packages take what they need as fields (`Info`/`Warn func(msg.M)`, `LoggedCmd`, `AddCleanup`, `Interrupted`…), set from a small `UI` interface of their own that their constructors take, or cli asks first and passes the answer in.
- **Fail with `u.Die(msg.X(...))`:** it prints and panics; `ui.Main` recovers, runs the cleanups, returns 1. Commands end with `fail` (`DieLogged` for a `LoggedError`). INT/TERM/HUP cancel the root context, wait for the logged command, run the cleanups and return 130; a second signal is ignored; after one, `Die` prints nothing. A logged command a signal ended (or 128+) waits up to 1 s for devvm's own interrupt, as Ctrl+C can end it first.
- **Arguments and flag values are checked before anything runs**, in `Args` (`noArgs`, `exactArgs`… from `args.go`, never Cobra's; values with `cobra.MatchAll`), `checkFlagValue` and `exclusive`: a usage error exits 2 and names the command's --help. Exit codes: 0, 1, 2, 130.
- **Cleanups:** whatever a failure would leave (a build VM, a temp file) registers with the UI's `AddCleanup` and is removed once done; one only for interrupts checks `Interrupted()`. Cleanups get a fresh context.
- **External commands run with the UI's `LoggedCmd(ctx, step, cmd)`** (a backend's `Runner`): output to the run log, failure via `DieLogged`. `WithSteps(ctx, n)` numbers steps.
- **OS-specific code goes behind build tags** in `internal/platform` (`*_darwin.go`, `*_linux.go`, `*_other.go`).
- **Dependencies:** the depguard allowlist (cobra, pflag, cobra/doc, go-i18n, BurntSushi/toml, x/text, x/term, x/sys; go-internal in tests; cobra/doc's own deps never imported directly). `internal/proxy` and `internal/daemon` are stdlib only. Adding one: ask.
- **External tools are commands, never linked** (`limactl`, `docker`, `security`, `git`, `chflags`…); prefer the standard library to shelling out. Strings into a shell are quoted with `shell.Quote`.
- **Test seams are fields** tests set (`Host`, `Exec`, `Runner`, `OS`, `Warn`…), never package vars (reassign: no package sets another's); `internal/proxy`'s are its `server`'s, which each test makes.
- **The structure test** (`internal/structure`, in `mise run test`) holds types to 24 methods, bans `func init`, allows only constants' kin as package vars, checks the package layers and the owners (host git only in `internal/hostgit`; `DEVVM_` variables only in `readOverrides` and the listed settings; `HOME` only in `internal/paths`, as `Dirs.Home`; shell quoting only in `internal/shell`, the VM-name grammar only in `internal/vmname`): see [Contributing](docs/content/docs/contributing/_index.md#the-structure-test) before raising a limit or listing an exception.
- **The CLI reference is generated:** `TestReferenceIsCurrent` fails on a stale one (`mise run cli-docs`).
- `CGO_ENABLED=0`. `internal/proxy` and `internal/daemon` are unix-only for now (`syscall.Kill`, `Setsid`, `Flock`).

## Backends

- Every backend implements `backend.Backend` and declares `Caps` per feature: native, emulated (reviewed) or none. A new `Feature` goes in `backend.Features` and the table in `docs/content/docs/internals/backends.md`.
- **`backend.Backend` is `Name`, `Caps` and four roles**, `Host`, `Images`, `Machines`, `Housekeeping`, all required; a backend's type splits the same way (Lima's `host`, `images`, `machines`, `housekeeping`). The one optional role, `Forgetter`, is found with `backend.As`.
- **A missing feature fails** in one line naming the VM and what to do; never fall back silently. Callers check the registry's `Review.Require(b, vm, spec.Features()...)` first, and its guard refuses the rest (it has every role's methods); backends don't decide what a VM needs.
- **Every emulation is refused until reviewed:** a design note in `docs/content/docs/internals/design.md`, a threat check with the security-audit skill, and e2e checks proving the guarantee; then it's marked reviewed in `Caps`.
- Backend-specific code stays in its package; what isn't (`.git` protection, the proxy) stays out.
- **Backends return errors, they don't die:** a `msg.M`, or `backend.Logged(m)` after a logged command failed. A stopped VM changes only at `Start`, which applies what its `Want` asks where it differs (`backend.Validate` first); a running one is left as it is; restart warnings are the callers'. Ports are never the backend's: `vmssh.Forwards` forwards over its `SSH`, which `vmssh.FromTarget` checks.
- **Lima edits only what differs:** any `limactl edit` gives the VM new ssh host keys at its next boot, so `Start` compares first and makes one edit. `Start` owns `env:`, `propagateProxyEnv`, `dns` and `hostResolver`, and a project VM's size, agent mounts and `forwardAgent`.

## Messages

- **Messages use the docs voice**, only shorter: the docs-voice skill's Messages and Terms rules.
- **Every message lives in `locales/en.toml`**, called through its generated function (`msg.NoVM(dir)`); `go generate ./internal/msg` after editing. Only go-i18n's fields (`other`, `one`, `description`); snake_case plain template fields (`{{.vm_name}}`); `one` makes it plural (`count int`). Doc links go in `internal/msg/docs.go`. A message without a caller fails the tests. `msg.M` is an `error`. Never build prose with `fmt`. Message values render on one line without control or bidi characters (`msg.OneLine`, at render); a VM's text shown as data goes through `msg.Printable`. A message made from an error carries it (`.Because(err)`), not only `err.Error()`.
- An error or warning is at most two sentences; a prompt gets at most two lines of context.
- **The mechanism goes in the doc link**, not the message: every warning or error the user can act on has one.
- Status steps stay a few words, lowercase. Commands are written as devvm x, not quoted. No "please", "note that" or "simply". `[y/N]`/`[Y/n]` stay literal.
- Not messages: data output and text in scripts that run elsewhere.
- **Tests check which message and with what, never its wording:** `DEVVM_MESSAGES=names` prints `no_vm dir=/x`; `devvm _msg NAME k=v…` gives e2e the text.

## Testing

- **Unit tests** are Go table tests next to the code; **CLI tests** are testscript against the built binary in a `/tmp` sandbox in names mode, stubs first on `PATH` (`stub_calls`, `stub_reset`), with `project`, `vm_name`, `catg`, `vm_state`, `json_set` and `$DATA`/`$VMS`/`$RUN`/`$CFG`; the binary has `assets/` embedded (no override: scripts read `$DEVVM_REPO/assets/`; rebuild before a manual run after editing them). New logic needs a test. Neither touches the Keychain, real VMs or the user's folders; state-writing tests point the XDG folders at `t.TempDir()`. Go tests never run up: scripts do.
- **Never touch VMs, images or containers devvm's tests didn't make:** your own stay as they are. Manual runs: `DEVVM_LIMA_HOME=~/.lt LIMA_HOME=~/.lt XDG_DATA_HOME=/tmp/x/data XDG_CONFIG_HOME=/tmp/x/config DEVVM_SECRET_SERVICE=devvm-sandbox-test` (devvm's Lima home is `$XDG_DATA_HOME/devvm/lima` unless `DEVVM_LIMA_HOME`; `LIMA_HOME` is for your `limactl`). Add a `config.toml` with `features = []` and pass `--network-policy`/`--no-network-policy` so nothing prompts. Delete test Keychain items. Never use your own tokens or logins: a network-policy VM reads its agent's login file from `HOME`, so give it its own (the Keychain needs the real `HOME`: test it apart).
- **End-to-end:** `tests/e2e.sh` builds devvm from the checkout and drives real VMs in its sandbox (`~/.lt-e2e-go`, `/tmp/devvm-go-e2e`, service `devvm-go-e2e`), deleting only what it made; images stay cached (`--clean`). `--slots N` spreads the sections over N sandboxes at once (slot K's paths end `-K`, its ports +100·K, its images cloned from slot 0's). Only `tools` resolves the newest tool versions; the rest use the pinned lock, and `--from` images go by pinned digest but one tag (`scripts/bump.sh e2e-tools e2e-from`). Run it in the background. Sections: `images`, `project`, `clone` (a throwaway repo, its own `HOME`), `gc` (an `img-*` folder's VM, a stale fixture), `tools` (a node VM), `network` (a VM with Claude Code and Codex), `desktop` (a base VM refused, then the desktop layer, devvm desktop), `nix` (a node VM with the nix layer and a flake), `from` (Debian 12, Alpine and Ubuntu bases, a docker that isn't running), `link` (two targets and a client, then the three in a group, the design note's threat check). Tiers: none (all but `desktop` and `from`, no features); `--from` (everything, every feature; before merging `assets/` or `--from` changes); `--only a,b` (with `from`: every feature). Hosted CI can't run it on macOS.- **Linux hosts:** `LIMA_HOME=~/.lt-linux-go`, VM `linuxhost` (nested virtualization, the repo read-only, copied to `~/devvm`), set up as [testing.md](docs/content/docs/contributing/testing.md) says, apart from any Lima home of your own.
- **Unreviewed emulations** are checked through `DEVVM_UNREVIEWED=backend:feature`, which only test builds (e2e's, the CLI tests', by ldflags) read.
- **Delete test VMs and temp files afterwards.** Long builds run in the background. Tests that `chflags` real files unlock them (`chflags -R nouchg`), only in their own temp folders.
- **The testscript runner stops the sandbox's proxies** (`pkill -f` on `_proxy -config` in the sandbox): the stub hostagent is gone before they look, so they'd wait 15 minutes.
- **`limactl shell` reads stdin:** give it none unless the input is meant for it.
- **Boot problems:** read `<devvm's Lima home>/<vm>/serialv.log` before restarting; every start overwrites it.

## Gotchas learned

- **Lima `vz`:** raw disks only; clones are APFS copy-on-write (`du` overcounts). Direct kernel boot needs an uncompressed arm64 `Image`. Stopping sends a GPIO power key, so guests need `gpio_keys`, or Lima hard-kills them after 30 s. Lima can't shrink disks. virtiofs tags are `lima-<hash>`. After root touches a directory on a virtiofs mount, the guest sees it as root's for a few seconds (git: "dubious ownership"). `mounts: []` doesn't remove the home mount from `base: template:…`: pass `--mount-none` (or `--mount-only`) to `limactl create`/`clone`.
- **Lima can report a failed start for a VM that is running but degraded.** Starting tolerates it.
- **`limactl list --json` fills in the config:** mounts get `mountPoint`, port forwards a `guestIP` (`127.0.0.1`) and host side; compare only what devvm set. `--set` applies after `--mount-none`. `limactl create` downloads the image at once (no dry run).
- **`limactl shell` quotes each argument for the guest's shell**, so `Exec` passes `sh -c` and one `shell.QuoteJoin` string. QEMU's guestfwd command goes through `/bin/sh -c` too, commas doubled.
- **The ssh socket path limit (104 characters)** bounds VM names (`NameMax`).
- **Linux hosts (QEMU):** mounts are 9p, and a 9p share mounts once per guest. Debian's tmpfs `/tmp` hides anything mounted below it during boot. Project VMs' qcow2 disks are overlays of their image's, so never start an image VM or write its disk after it's built.
- **Shared tools:** mise rewrites `latest`/`24`/`lts` links in a tool's folder after any install, so those folders are local in the VM; only version folders are mounted.
- **Linux's `pgrep -f` matches the shell running it** (procps), so e2e's patterns are written `[_]proxy`. mise skips minisign on linux/arm64 (no build).
- **Lima writes `lima.REJECTED.yaml` into the current folder** on an invalid config: run `limactl` outside the repo (devvm runs it from its data folder).

## Commits and tasks

- Tasks are in dex (`.dex/`, git-ignored).
- **One commit when each task is complete**, its Docs and Review subtasks included. Conventional Commits: `type(scope): summary`, types `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`. Then `dex complete <id> --commit <sha>`.
- **Don't commit or `dex complete` until the gate passes**, with whatever `mise run fmt` changed in the commit.
- **Dex task IDs never go in commit messages, code comments or docs.**
- **AGENTS.md stays short:** one line per path or rule. Inventories (functions, messages, test names) go in the commit message and the `dex complete` result, not here.
