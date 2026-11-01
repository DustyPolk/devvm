# AGENTS.md

devvm: one isolated VM per project folder, cloned from a base image, with toolchains and agents shared read-only between VMs. It's a Go CLI with swappable VM backends: Lima, the default, and smolvm and Docker, both experimental. Read [README.md](README.md) for what it is, and the [docs site](docs/content/docs/) for how each part works; [Contributing](docs/content/docs/contributing/_index.md) covers building, the tools, testscript and writing docs.

## Layout

| Path | What |
| --- | --- |
| `cmd/devvm/` | the entry point: `ui.Main` makes the run's UI and runs the root command with it; the only `os.Exit` |
| `internal/cli/` | Cobra commands, one file each (root = up, `build`/`rebuild`, `images`, `gc`, `space`, `retention`, `layer`, `setup`, `restart`, `tools`, `adopt`, `rm`, `stop`, `history`, `ls`, `network` (its rules and log too), `doctor`, `secret`, `ssh-agent`, `host`, `code`, `desktop`, `limactl`, `ports`, `size`, `git-protect`, `completion`, hidden `version`, `_msg`, `_qemu`, `_proxy`, `_lima-ports`, `_smolvm-ports`, `_docker-ports`, `_docker-connect`, `_refresh-tools`); `script_test.go` runs testscript; `docgen/` writes the CLI reference |
| `internal/vm/` | `Env` (`env.go`: shared dependencies, finding and starting the folder's VM); `Launch`: a new VM (`up.go`), an existing one (`existing.go`), `ssh.go`, agents' mounts; `Lifecycle` (`lifecycle.go`); `Tools` (`tools.go`: a VM's shared tools); `Settings` (`settings.go`, `size.go`, `ports.go`, `sshagent.go`); `Net` (`network.go`, `secret.go`); `--clone` (`clone.go`); a devcontainer.json's mapping and commands (`devcontainer.go`); the setup it sends (`sync.go`) |
| `internal/project/`, `internal/gitprotect/` | a project folder (VM name, unsafe folders, detection, set-id files); `.git` protection |
| `internal/sshconfig/`, `internal/sshagent/` | the ssh config for running VMs by name, their pinned host keys, the `~/.ssh/config` Include line; each VM's SSH agent choice, devvm's own agent and its keys |
| `internal/build/`, `internal/image/` | building and rebuilding images, their records; image keys and names (pure) |
| `internal/from/` | `--from`: references, registry digests (anonymous token), Docker's IDs, the recorded digests |
| `internal/devcontainer/` | a project's devcontainer.json (pure): finding it, JSONC, mapping it to a new VM's plan, refusals |
| `internal/tools/` | the shared tools: the lock, host mise, the daily check, tool image builds, the share |
| `internal/netpolicy/`, `internal/secrets/` | network policy (the choice, the VM's proxy, the self-check); tokens on the host and the VM's bindings |
| `internal/trust/` | the user's CAs (`~/.config/devvm/ca`): read and checked, sent to VMs, build VMs' `Trust` scripts, the guest environment |
| `internal/proxy/` | the proxy, run as `devvm _proxy`; see its `README.md` |
| `internal/desktop/` | devvm desktop's relay on the host: VNC's password checked here, then an ssh channel to the VM's socket, the viewer's clipboard dropped; passwords |
| `internal/backend/` | `Backend`, `Caps`, `Require`, the registry; `backendtest/` (fake), `registry/` (every backend's), `lima/`, `smolvm/`, `docker/` (both experimental), `vmssh/` (their ssh: VMs' keys, the port forwarder) |
| `internal/ui/` | the only package that prints: `ui.UI` (messages, prompts, status line, run log, cleanups), `ui.Main` (signals); no package state |
| `internal/msg/`, `locales/` | messages: `en.toml`, the generated functions, doc links (`docs.go`), the generator (`gen/`) |
| `internal/paths/`, `internal/state/` | every folder and file devvm uses; atomic state files, `assets.FS` written out where a path is needed |
| `internal/platform/`, `internal/ubuntu/` | build-tagged OS code; finding and verifying Ubuntu's cloud image (never downloads) |
| `assets/` | the files devvm ships, embedded (`assets.FS`, passed to constructors by `cli`): `base.yaml`, `layers/` (base's Lima template and scripts; the built-in layers, `docker` and `nix`, each with its policy), `features/` (base's optional ones), `shared-tools/` (`mise.toml`, `build.sh`, `*.policy`), `guest/` (in-VM scripts sent on every launch), `oci/` (`--from`'s conversion; see its `README.md`), `container/` (smolvm's and Docker's scripts in containers), `smolvm/`, `docker/` (each one's own) |
| `internal/structure/` | the structure test (tests only) |
| `testdata/` | `script/*.txtar` (CLI tests), `stubs/` (host commands; see each header), `mise.lock` (fixture) |
| `tests/e2e.sh` | end-to-end tests on real VMs (bash 3.2) |
| `scripts/`, `packaging/`, `.goreleaser.yaml`, `VERSION`, `CHANGELOG.md` | `bump.sh` (pins), `release.sh` (signed-tag checks around GoReleaser, then minisign); the Homebrew formula; `VERSION` is read by the scripts only |
| `install.sh` | `curl \| bash`: the host's archive (the binary, README, changelog, license), checked, into `~/.local/lib/devvm/<version>`, then doctor's fixes, asking first; tests source it (`DEVVM_INSTALL_TEST`) |
| `docs/` | the docs site (Hugo + Hextra); `content/docs/cli/` is generated, never edit it |
| `mise.toml`, `.golangci.yml`, `.vale*`, `.markdownlint-cli2.yaml`, `.github/` | tools, tasks, linters, CI and Pages |
| `.agents/skills/security-audit/` | the security-audit skill, for backend reviews |
| `.agents/skills/docs-voice/` | the docs-voice skill: the voice of the prose and messages, terms, tropes, `scan.sh` and `facts.sh` |

Still to come: the release keys (`packaging/minisign.pub`, `packaging/release-signer`).

## Tasks

`mise run <task>`: `fmt`; `lint` (`lint:code`: Go, shellcheck, workflows with actionlint and zizmor; `lint:docs`: docs, `TestDocLinks`); `test` (`-race`, in a throwaway `HOME`; needs a C compiler); `vuln`; `deadcode` (lists only `backendtest`, which tests use; anything else gets a caller or goes); `cli-docs` (after changing any command's `Use`, `Short`, `Long` or flags; commit the pages); `docs` (serves `http://localhost:1313/`); `e2e`.

## Rules

- **The gate:** `mise run fmt && mise run lint && mise run test`, all passing, before starting a task and before marking it done; a gate failing at the start gets fixed first. Anything touching VMs also passes `tests/e2e.sh` in the background: the sections for what changed (`--only`, `--backend`).
- **Nothing in project folders:** state goes in `~/.local/share/devvm` and `~/.config/devvm`.
- **Credentials never go into images.** Host agent history never goes into VMs.
- **Never auto-delete project VMs:** they may hold unsaved work. Warn instead.
- **The startup check only reports.** Never install host software without asking.
- **Image keys hash everything that goes into an image**, the backend included. A tool image's key is its lock entry, `assets/shared-tools/build.sh` and the libc.
- **Pin every download** in `assets/`'s `layers/`, `features/`, `shared-tools/build.sh`, `oci/build.sh`: `<tool>_version` and `<tool>_sha256_arm64`/`_amd64` at the top, fetched with `devvm-fetch` (`oci/build.sh`: `sha256sum`). No `curl | sh`, no "latest". Upstream-checked exceptions: shared tools (`MISE_LOCKED=1`), Ubuntu's signed `SHA256SUMS`, Lima in `install.sh`. Go checks a download's SHA-256 before use.
- **Shared tools are read-only in VMs:** whatever writes into a tool's folder (global installs, self-updates, `corepack enable`, `rustup component add`) is redirected (`~/.npm-global`, `go_set_gobin = false`), done in `assets/shared-tools/build.sh`, or off.
- **In-VM scripts are bash 3.2-safe and shellcheck-clean** (`assets/`, `install.sh`, `tests/e2e.sh`): guard empty arrays (`${a[@]+"${a[@]}"}`), no associative arrays. Everything else is Go.
- **Docs:** an easy-to-follow guide in full, joined sentences, every fact kept; write and edit prose with the docs-voice skill (Vale's `devvm.AIWords` fails on chatbot vocabulary). Every behavior change updates its page in `docs/content/docs/` in the same task. The README stays high level.
- **Adding or changing a shared tool** updates its page: `using/toolchains.md` for a toolchain, `using/coding-agents.md`'s Supported agents for an agent.
- **Docs by reader:** user pages (`getting-started/`, `using/`, `safety/`, `backends/`) say what devvm does and what you type, without repo paths, Go identifiers or message names; How it works (`internals/`) explains mechanisms; Contributing (`contributing/`) has repo paths and Go identifiers.
- **Edit files with the editor tools,** not `sed`, `awk`, `python`, heredocs or `cat >`.
- **Ask first** before anything outward-facing (publishing, releases, repos, pushing) and before deleting anything you didn't create.
- **A task that turns out wrong or incomplete** gets a proposed change (`dex edit`), not quietly something else.

## Go conventions

- **Only `internal/ui` prints.** forbidigo bans `fmt.Print*`, `os.Stdout`/`os.Stderr` elsewhere and `os.Exit` outside `cmd/devvm`. Data goes to the UI's `Stdout()`, non-message log lines to its `Stderr()`.
- **Functions take their caller's `ctx`:** forbidigo bans `context.Background`/`TODO` outside `internal/ui` and `cmd/devvm`; one that must outlive its caller's gets `//nolint:forbidigo // why`.
- **Every `//nolint` names its linters and says why** (nolintlint). Functions stay within revive's `function-length` and `cognitive-complexity` limits: split one that grows past them.
- **Only `cmd/devvm` and `internal/cli` import `internal/ui`** (depguard): cli passes the run's `*ui.UI` to each command; other packages take what they need as fields (`Info`/`Warn func(msg.M)`, `LoggedCmd`, `AddCleanup`, `Interrupted`…), set from a small `UI` interface of their own that their constructors take, or cli asks first and passes the answer in.
- **Fail with `u.Die(msg.X(...))`:** it prints and panics; `ui.Main` recovers, runs the cleanups, returns 1. Commands end with `fail` (`DieLogged` for a `LoggedError`). INT/TERM/HUP cancel the root context, wait for the logged command, run the cleanups and return 130; a second signal is ignored; after one, `Die` prints nothing. A logged command a signal ended (or 128+) waits up to 1 s for devvm's own interrupt, as Ctrl+C can end it first.
- **Cleanups:** whatever a failure would leave (a build VM, a temp file) registers with the UI's `AddCleanup` and is removed once done; one only for interrupts checks `Interrupted()`. Cleanups get a fresh context.
- **External commands run with the UI's `LoggedCmd(ctx, step, cmd)`** (a backend's `Runner`): output to the run log, failure via `DieLogged`. `WithSteps(ctx, n)` numbers steps.
- **OS-specific code goes behind build tags** in `internal/platform` (`*_darwin.go`, `*_linux.go`, `*_other.go`).
- **Dependencies:** the depguard allowlist (cobra, pflag, cobra/doc, go-i18n, BurntSushi/toml, x/text, x/term, x/sys; go-internal in tests; cobra/doc's own deps never imported directly). `internal/proxy` is stdlib only. Adding one: ask.
- **External tools are commands, never linked** (`limactl`, `docker`, `security`, `git`, `chflags`…); prefer the standard library to shelling out. Strings into a backend's shell are quoted with `shq()`.
- **Test seams are fields** tests set (`Host`, `Exec`, `Runner`, `OS`, `Warn`…), never package vars (reassign: no package sets another's); the one exception is `internal/proxy`'s globals (set by `Main`).
- **The structure test** (`internal/structure`, in `mise run test`) holds types to 24 methods, bans `func init`, allows only constants' kin as package vars and checks the package layers: see [Contributing](docs/content/docs/contributing/_index.md#the-structure-test) before raising a limit or listing an exception.
- **The CLI reference is generated:** `TestReferenceIsCurrent` fails on a stale one (`mise run cli-docs`).
- `CGO_ENABLED=0`. `internal/proxy` is unix-only for now (`syscall.Kill`, `Flock`).

## Backends

- Every backend implements `backend.Backend` and declares `Caps` per feature: native, emulated (reviewed) or none. A new `Feature` goes in `backend.Features` and the table in `docs/content/docs/internals/backends.md`.
- **A missing feature fails** in one line naming the VM and what to do; never fall back silently. Callers check the registry's `Review.Require(b, vm, spec.Features()...)` first; backends don't decide what a VM needs.
- **Every emulation is refused until reviewed:** a design note in `docs/content/docs/internals/design.md`, a threat check with the security-audit skill, and e2e checks proving the guarantee; then it's marked reviewed in `Caps`.
- Backend-specific code stays in its package; what isn't (`.git` protection, the proxy) stays out.
- **Backends return errors, they don't die:** a `msg.M`, or `backend.Logged(m)` after a logged command failed. `SetPorts` accepts a running VM (starts its forwarder again); other Set calls on one get `vm_not_stopped`; restart warnings are the callers'.
- **Lima edits only what differs:** any `limactl edit` gives the VM new ssh host keys at its next boot, so `Start` compares first. `Start` owns `env:`, `propagateProxyEnv`, `dns` and `hostResolver`.

## Messages

- **Messages use the docs voice**, only shorter: the docs-voice skill's Messages and Terms rules.
- **Every message lives in `locales/en.toml`**, called through its generated function (`msg.NoVM(dir)`); `go generate ./internal/msg` after editing. Only go-i18n's fields (`other`, `one`, `description`); snake_case plain template fields (`{{.vm_name}}`); `one` makes it plural (`count int`). Doc links go in `internal/msg/docs.go`. A message without a caller fails the tests. `msg.M` is an `error`. Never build prose with `fmt`. VM-made values go through `msg.Shown`. A message made from an error carries it (`.Because(err)`), not only `err.Error()`.
- An error or warning is at most two sentences; a prompt gets at most two lines of context.
- **The mechanism goes in the doc link**, not the message: every warning or error the user can act on has one.
- Status steps stay a few words, lowercase. Commands are written as devvm x, not quoted. No "please", "note that" or "simply". `[y/N]`/`[Y/n]` stay literal.
- Not messages: data output and text in scripts that run elsewhere.
- **Tests check which message and with what, never its wording:** `DEVVM_MESSAGES=names` prints `no_vm dir=/x`; `devvm _msg NAME k=v…` gives e2e the text.

## Testing

- **Unit tests** are Go table tests next to the code; **CLI tests** are testscript against the built binary in a `/tmp` sandbox, stubs first on `PATH` (`stub_calls`); the binary has `assets/` embedded (no override: scripts read `$DEVVM_REPO/assets/`; rebuild before a manual run after editing them). New logic needs a test. Neither touches the Keychain, real VMs or the user's folders; state-writing tests point the XDG folders at `t.TempDir()`. Go tests never run up: scripts do.
- **Never touch VMs, images or containers devvm's tests didn't make:** your own stay as they are. Manual runs: `DEVVM_LIMA_HOME=~/.lt LIMA_HOME=~/.lt XDG_DATA_HOME=/tmp/x/data XDG_CONFIG_HOME=/tmp/x/config DEVVM_SECRET_SERVICE=devvm-sandbox-test` (devvm's Lima home is `$XDG_DATA_HOME/devvm/lima` unless `DEVVM_LIMA_HOME`; `LIMA_HOME` is for your `limactl`). Add a `base-features` file and pass `--network-policy`/`--no-network-policy` so nothing prompts. Delete test Keychain items. Never use your own tokens or logins: a network-policy VM reads its agent's login file from `HOME`, so give it its own (the Keychain needs the real `HOME`: test it apart).
- **End-to-end:** `tests/e2e.sh` builds devvm from the checkout and drives real VMs in its sandbox (`~/.lt-e2e-go`, `/tmp/devvm-go-e2e`, service `devvm-go-e2e`), deleting only what it made; images stay cached (`--clean`). `tools` first resolves the newest tool versions. Run it in the background. Sections: `images`, `project`, `clone` (a throwaway repo, its own `HOME`), `gc` (an `img-*` folder's VM, a stale fixture), `tools` (a node VM), `network` (a claude VM), `desktop` (base with the desktop feature, devvm desktop), `nix` (a node+nix flake VM), `from` (Debian 12, Alpine and Ubuntu bases, a docker that isn't running). Tiers: none (all but `desktop` and `from`, no features); `--from` (everything, every feature; before merging `assets/` or `--from` changes); `--only a,b` (with `from`: every feature). Hosted CI can't run it on macOS.- **Linux hosts:** `LIMA_HOME=~/.lt-linux-go`, VM `linuxhost` (nested virtualization, the repo read-only, copied to `~/devvm`), set up as [testing.md](docs/content/docs/contributing/testing.md) says, apart from any Lima home of your own.
- **`tests/e2e.sh --backend smolvm`** runs only the `smolvm` section (after touching `internal/backend/smolvm` or shared code); it acts only on machines with its sandbox's label. Unreviewed emulations are checked through `DEVVM_UNREVIEWED=backend:feature`, which only test builds (e2e's, the CLI tests', by ldflags) read.
- **`tests/e2e.sh --backend docker`** (after touching `internal/backend/docker` or shared code): on macOS a Linux engine in a VM it keeps (`~/.lt-e2e-go-docker`, `dockerhost`), then Docker Desktop, which holds your own containers: it acts only on what has its sandbox's label, deletes all that at the end, and checks the rest is unchanged. Never an unfiltered prune or list-and-delete on Desktop, in tests or by hand.
- **Delete test VMs and temp files afterwards.** Long builds run in the background. Tests that `chflags` real files unlock them (`chflags -R nouchg`), only in their own temp folders.
- **The testscript runner stops the sandbox's proxies** (`pkill -f` on `-lifeline`): the stub hostagent is gone before they look, so they'd wait 15 minutes.
- **`limactl shell` reads stdin:** give it none unless the input is meant for it.
- **Boot problems:** read `<devvm's Lima home>/<vm>/serialv.log` before restarting; every start overwrites it.

## Gotchas learned

- **Lima `vz`:** raw disks only; clones are APFS copy-on-write (`du` overcounts). Direct kernel boot needs an uncompressed arm64 `Image`. Stopping sends a GPIO power key, so guests need `gpio_keys`, or Lima hard-kills them after 30 s. Lima can't shrink disks. virtiofs tags are `lima-<hash>`. After root touches a directory on a virtiofs mount, the guest sees it as root's for a few seconds (git: "dubious ownership"). `mounts: []` doesn't remove the home mount from `base: template:…`: pass `--mount-none` (or `--mount-only`) to `limactl create`/`clone`.
- **Lima can report a failed start for a VM that is running but degraded.** Starting tolerates it.
- **`limactl list --json` fills in the config:** mounts get `mountPoint`, port forwards a `guestIP` (`127.0.0.1`) and host side; compare only what devvm set. `--set` applies after `--mount-none`. `limactl create` downloads the image at once (no dry run).
- **`limactl shell` quotes each argument for the guest's shell**, so `Exec` passes `sh -c` and one `shq` string. QEMU's guestfwd command goes through `/bin/sh -c` too, commas doubled.
- **The ssh socket path limit (104 characters)** bounds VM names (`NameMax`).
- **Linux hosts (QEMU):** mounts are 9p, and a 9p share mounts once per guest. Docker writes bind-mounted files as root, so containers that write host files `chown` them back. Ubuntu's `docker.io` has no BuildKit (`RUN --mount` needs `docker-buildx`). Debian's tmpfs `/tmp` hides anything mounted below it during boot. Project VMs' qcow2 disks are overlays of their image's, so never start an image VM or write its disk after it's built.
- **Shared tools:** mise rewrites `latest`/`24`/`lts` links in a tool's folder after any install, so those folders are local in the VM; only version folders are mounted.
- **Linux's `pgrep -f` matches the shell running it** (procps), so e2e's patterns are written `[_]proxy`. mise skips minisign on linux/arm64 (no build).
- **smolvm runs an image as a container in an Alpine microVM:** no init system, `exec` enters the container as root with only `HOME` set (no `USER`, no `/etc/environment`), `/dev` has no loop nodes (`mknod`), its EROFS mounts only uncompressed images, Docker needs `-v /var/lib/docker` and installs `systemctl` (tell systemd by `/run/systemd/system`). virtiofs shows host files as the host's uid, so the VM's user has it.
- **smolvm packs carry their build VM's network, CPUs and memory;** `--block-private` can't be removed. `update --storage` breaks a registry-image machine, and a pack machine whose pack (stub and `.smolmachine`) is gone. `--state-dir` is Linux only: run it with `SMOLVM_USAGE=0`. On macOS it shares the user's machines (kept under `HOME`: a devvm run with another `HOME` sees others): touch only those with devvm's label.
- **Docker:** a container's mounts and network are fixed when it's made (devvm makes it again from a commit), and one it names that doesn't exist Docker makes unlabelled (make devvm's volumes first). `docker inspect` shows Desktop's bind sources as `/host_mnt/…`: compare destinations. Desktop's containers reach the host's loopback, so devvm builds no images on it.
- **smolvm's network:** allow lists are per address (the gateway `100.96.0.1` is every host loopback port), DNS answers any name under them; its own forwards (devvm uses one, for the VM's sshd) reach only servers on `0.0.0.0` and take a host port another program has on `*` (`SO_REUSEADDR`, as Go's `net.Listen`: devvm's forwarder uses `platform.ListenExclusive`).
- **smolvm reads `SSH_AUTH_SOCK` only at `machine start`,** and a machine made with `--ssh-agent` won't start without one (a missing path is accepted): e2e's direct starts pass `/dev/null/...`.
- **smolvm base's packages before `assets/layers/base.sh`** come from Ubuntu's live archive, then base.sh pins a snapshot: anything pulling systemd libraries (openssh-server) goes after base's scripts (`sshd.sh`), or Docker's packages can't be installed. Under mediated egress a refused connection still opens in the guest, then resets (test with data, not a connect); TCP and UDP to any port 53 go to its resolver; `--dns` is create-only; a machine once started mediated won't start without a decider until `--no-egress-interceptor`.
- **Docker's networks:** Desktop's containers reach the host's loopback (`host.docker.internal`); on Linux the gateway reaches the host's servers on all addresses, an `--internal` network's too unless `inhibit_ipv4`; internal networks publish no ports and resolve no outside names.
- **Docker's mounts:** a bind-mounted host socket works alone (`-v`: Desktop's `--mount` says the source doesn't exist), not inside a mounted folder on Desktop (`Not supported`); Desktop shows it as root's 0660 and follows the path (none there: connects hang), Linux binds the inode (a socket in a mounted folder works, `chmod` in the container changes the host's).
- **Docker's containers:** systemd needs a read-write `/sys/fs/cgroup` (the host's whole tree), `dockerd` and loop devices `--privileged`; `-p`, mounts and the network are fixed at create (`docker update`: limits only); `--storage-opt size` is refused on Desktop and ignored by the containerd store; Alpine's image lacks OpenRC and ssh, which base.sh writes into.
- **Sysbox:** on Ubuntu 25.04+ fusermount3's AppArmor profile must allow mounts under `/var/lib/sysboxfs` (`/etc/apparmor.d/local/fusermount3`) or containers don't start; root gets every capability in its namespace (`--cap-drop` ignored), writable `/proc/sys`, and writes as the host's root on ID-mapped bind mounts; only the bridge's `route_localnet` 0 keeps its routed `127.0.0.1` off the host (Docker sets 1 with `"userland-proxy": false`).
- **Lima writes `lima.REJECTED.yaml` into the current folder** on an invalid config: run `limactl` outside the repo (devvm runs it from its data folder).

## Commits and tasks

- Tasks are in dex (`.dex/`, git-ignored).
- **One commit when each task is complete**, its Docs and Review subtasks included. Conventional Commits: `type(scope): summary`, types `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`. Then `dex complete <id> --commit <sha>`.
- **Don't commit or `dex complete` until the gate passes**, with whatever `mise run fmt` changed in the commit.
- **Dex task IDs never go in commit messages, code comments or docs.**
- **AGENTS.md stays short:** one line per path or rule. Inventories (functions, messages, test names) go in the commit message and the `dex complete` result, not here.
