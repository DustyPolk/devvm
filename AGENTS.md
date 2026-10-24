# AGENTS.md

devvm: one isolated VM per project folder, cloned from a base image, with toolchains and agents shared read-only between VMs. It's a Go CLI with swappable VM backends (Lima first). Read [README.md](README.md) for what it is, and the [docs site](docs/content/docs/) for how each part works; [Contributing](docs/content/docs/contributing.md) covers building, the tools, testscript and writing docs.

## Layout

| Path | What |
| --- | --- |
| `cmd/devvm/` | the entry point: the root command through `ui.Main`; the only `os.Exit` |
| `internal/cli/` | Cobra commands, one file each (root = up, `build`/`rebuild`, `secret`, hidden `_msg`, `_qemu`, `_proxy`, `_refresh-tools`); `script_test.go` runs testscript; `docgen/` writes the CLI reference |
| `internal/vm/` | up for a new VM, from the folder checks to the shell; agents' mounts; the setup it sends (`sync.go`) |
| `internal/project/`, `internal/gitprotect/` | a project folder (VM name, unsafe folders, detection, set-id files); `.git` protection |
| `internal/sshconfig/` | the ssh config for running VMs by name, their pinned host keys, the `~/.ssh/config` Include line |
| `internal/build/`, `internal/image/` | building and rebuilding images, their records; image keys and names (pure) |
| `internal/tools/` | the shared tools: the lock, host mise, the daily check, tool image builds, the share |
| `internal/netpolicy/`, `internal/secrets/` | network policy (the choice, the VM's proxy, the self-check); tokens on the host and the VM's bindings |
| `internal/proxy/` | the proxy, run as `devvm _proxy`; see its `README.md` |
| `internal/backend/` | `Backend`, `Caps`, `Require`, the registry; `backendtest/` (fake), `backends/`, `lima/` |
| `internal/ui/` | the only package that prints: messages, prompts, status line, run log, cleanups, signals |
| `internal/msg/`, `locales/` | messages: `en.toml`, the generated functions, doc links (`docs.go`), the generator (`gen/`) |
| `internal/paths/`, `internal/state/` | every folder and file devvm uses; atomic state files (errors aren't messages yet) |
| `internal/platform/`, `internal/ubuntu/` | build-tagged OS code; finding and verifying Ubuntu's cloud image (never downloads) |
| `base.yaml`, `layers/`, `tools/`, `guest/` | base's Lima template and scripts; shared tools (`mise.toml`, `build.sh`, `*.policy`); in-VM scripts sent on every launch |
| `testdata/` | `script/*.txtar` (CLI tests), `stubs/` (host commands; see each header), `mise.lock` (fixture) |
| `tests/e2e.sh` | end-to-end tests on real VMs (bash 3.2) |
| `docs/` | the docs site (Hugo + Hextra); `content/docs/cli/` is generated, never edit it |
| `mise.toml`, `.golangci.yml`, `.vale*`, `.markdownlint-cli2.yaml`, `.github/` | tools, tasks, linters, CI and Pages |
| `.agents/skills/security-audit/` | the security-audit skill, for backend reviews |

Still to come: smolvm and docker backends; `features/` and `oci/` (next to the binary, like `base.yaml`); `install.sh`, `scripts/`, `packaging/`.

## Tasks

`mise run <task>`: `fmt`; `lint` (Go, shellcheck, docs, `TestDocLinks`); `test` (`-race`, in a throwaway `HOME`; needs a C compiler); `vuln`; `cli-docs` (after changing any command's `Use`, `Short`, `Long` or flags; commit the pages); `docs` (serves `http://localhost:1313/devvm/`); `e2e`.

## Rules

- **The gate:** `mise run fmt && mise run lint && mise run test`, all passing, before starting a task and before marking it done; a gate failing at the start gets fixed first. Anything touching VMs also passes `tests/e2e.sh` in the background: `--quick` once it exists, until then the sections for what changed.
- **Nothing in project folders:** state goes in `~/.local/share/devvm` and `~/.config/devvm`.
- **Credentials never go into images.** Host agent history never goes into VMs.
- **Never auto-delete project VMs:** they may hold unsaved work. Warn instead.
- **The startup check only reports.** Never install host software without asking.
- **Image keys hash everything that goes into an image**, the backend included. A tool image's key is its lock entry, `tools/build.sh` and the libc.
- **Pin every download** in `layers/`, `features/`, `tools/build.sh`, `oci/build.sh`: `<tool>_version` and `<tool>_sha256_arm64`/`_amd64` at the top, fetched with `devvm-fetch` (`oci/build.sh`: `sha256sum`). No `curl | sh`, no "latest". Upstream-checked exceptions: shared tools (`MISE_LOCKED=1`), Ubuntu's signed `SHA256SUMS`, Lima in `install.sh`. Go checks a download's SHA-256 before use.
- **Shared tools are read-only in VMs:** whatever writes into a tool's folder (global installs, self-updates, `corepack enable`, `rustup component add`) is redirected (`~/.npm-global`, `go_set_gobin = false`), done in `tools/build.sh`, or off.
- **In-VM scripts are bash 3.2-safe and shellcheck-clean** (`layers/`, `features/`, `tools/build.sh`, `guest/`, `oci/`, `install.sh`, `tests/e2e.sh`): guard empty arrays (`${a[@]+"${a[@]}"}`), no associative arrays. Everything else is Go.
- **Docs:** succinct and explicit. Every behavior change updates its page in `docs/content/docs/` in the same task; a page's parts not ported yet come with their tasks. The README stays high level.
- **Edit files with the editor tools,** not `sed`, `awk`, `python`, heredocs or `cat >`.
- **Ask first** before anything outward-facing (publishing, releases, repos, pushing) and before deleting anything you didn't create.
- **A task that turns out wrong or incomplete** gets a proposed change (`dex edit`), not quietly something else.

## Go conventions

- **Only `internal/ui` prints.** forbidigo bans `fmt.Print*`, `os.Stdout`/`os.Stderr` elsewhere and `os.Exit` outside `cmd/devvm`. Data goes to `ui.Stdout()`, non-message log lines to `ui.Stderr()`.
- **Fail with `ui.Die(msg.X(...))`:** it prints and panics; `ui.Main` recovers, runs the cleanups, returns 1. Commands end with `fail` (`ui.DieLogged` for a `LoggedError`). INT/TERM/HUP cancel the root context, wait for the logged command, run the cleanups and return 130; a second signal is ignored; after one, `Die` prints nothing.
- **Cleanups:** whatever a failure would leave (a build VM, a temp file) registers `ui.AddCleanup` and is removed once done; one only for interrupts checks `ui.Interrupted()`. Cleanups get a fresh context.
- **External commands run with `ui.Logged(ctx, step, argv...)`** (`LoggedCmd` for input, env or folder): output to the run log, failure via `ui.DieLogged`. `ui.WithSteps(ctx, n)` numbers steps.
- **OS-specific code goes behind build tags** in `internal/platform` (`*_darwin.go`, `*_linux.go`, `*_other.go`).
- **Dependencies:** the depguard allowlist (cobra, pflag, cobra/doc, go-i18n, BurntSushi/toml, x/text, x/term, x/sys; go-internal in tests; cobra/doc's own deps never imported directly). `internal/proxy` is stdlib only. Adding one: ask.
- **External tools are commands, never linked** (`limactl`, `docker`, `security`, `git`, `chflags`…); prefer the standard library to shelling out. Strings into a backend's shell are quoted with `shq()`.
- **Test seams are fields or package vars** tests swap (`Host`, `Exec`, `Runner`, `OS`, `Warn`…). The proxy's state is globals set by `Main`; tests set them.
- **A command exists only once it's ported;** `TestReferenceIsCurrent` fails on a stale CLI reference.
- `CGO_ENABLED=0`. `internal/proxy` is unix-only for now (`syscall.Kill`, `Flock`).

## Backends

- Every backend implements `backend.Backend` and declares `Caps` per feature: native, emulated (reviewed) or none. A new `Feature` goes in `backend.Features` and the table in `docs/content/docs/backends.md`.
- **A missing feature fails** in one line naming the VM and what to do; never fall back silently. Callers check `backend.Require(b, vm, spec.Features()...)` first; backends don't decide what a VM needs.
- **Every emulation is refused until reviewed:** a design note in `docs/design.md`, a threat check with the security-audit skill, and e2e checks proving the guarantee; then it's marked reviewed in `Caps`.
- Backend-specific code stays in its package; what isn't (`.git` protection, the proxy) stays out.
- **Backends return errors, they don't die:** a `msg.M`, or `backend.Logged(m)` after a logged command failed. Warnings go through a `Warn` field. Set calls on a running VM get `vm_not_stopped`; restart warnings are the callers'.
- **Lima edits only what differs:** any `limactl edit` gives the VM new ssh host keys at its next boot, so `Start` compares first. `Start` owns `env:`, `propagateProxyEnv`, `dns` and `hostResolver`.

## Messages

- **Succinct and direct.** Say what is being done, or what failed and what to do. No explanations; link a docs page when more is needed.
- **Every message lives in `locales/en.toml`**, called through its generated function (`msg.NoVM(dir)`); `go generate ./internal/msg` after editing. Only go-i18n's fields (`other`, `one`, `description`); snake_case plain template fields (`{{.vm_name}}`); `one` makes it plural (`count int`). Doc links go in `internal/msg/docs.go`. A message without a caller fails the tests. `msg.M` is an `error`. Never build prose with `fmt`. VM-made values go through `msg.Shown`.
- An error is one line. Explain why only when the user must decide; a prompt gets at most two lines of context. Status steps are a few words, lowercase. Commands are written as devvm x, not quoted. No "please", "note that" or "simply". `[y/N]`/`[Y/n]` stay literal.
- Not messages: data output and text in scripts that run elsewhere.
- **Tests check which message and with what, never its wording:** `DEVVM_MESSAGES=names` prints `no_vm dir=/x`; `devvm _msg NAME k=v…` gives e2e the text.

## Testing

- **Unit tests** are Go table tests next to the code; **CLI tests** are testscript against the built binary in a `/tmp` sandbox, stubs first on `PATH` (`stub_calls`). New logic needs a test. Neither touches the Keychain, real VMs or the user's folders; state-writing tests point the XDG folders at `t.TempDir()`. Go tests never run up: scripts do.
- **Don't touch the user's own VMs or images.** Manual runs: `DEVVM_LIMA_HOME=~/.lt LIMA_HOME=~/.lt XDG_DATA_HOME=/tmp/x/data XDG_CONFIG_HOME=/tmp/x/config DEVVM_SECRET_SERVICE=devvm-sandbox-test` (devvm's Lima home is `$XDG_DATA_HOME/devvm/lima` unless `DEVVM_LIMA_HOME`; `LIMA_HOME` is for your `limactl`). Add a `base-features` file and pass `--network-policy`/`--no-network-policy` so nothing prompts. Delete test Keychain items. Never use the user's tokens or logins: a network-policy VM reads its agent's login file from `HOME`, so give it its own (the Keychain needs the real `HOME`: test it apart).
- **End-to-end:** `tests/e2e.sh` builds devvm from the checkout and drives real VMs in its sandbox (`~/.lt-e2e-go`, `/tmp/devvm-go-e2e`, service `devvm-go-e2e`), deleting only what it made; images stay cached (`--clean`). `tools` first resolves the newest tool versions. Run it in the background. Sections: `images`, `project`, `tools` (a node VM), `network` (a claude VM). Tiers, once they exist: `--quick` (project, `--clone`, size, gc, network; after most changes); none (adds `images`, `tools`); `--from` (everything, every feature; before merging `layers/`, `tools/`, `features/`, `oci/` or `--from` changes); `--only a,b`. Hosted CI can't run it on macOS.
- **e2e to come:** `--backend`; `clone size gc from`; network: `policy allow` at once, `log`, git over SSH, `secret use github`, `unenforced` in `ls`/relaunch, opencode's login; project: agent folders on relaunch and restart, SSH agent, host keys read again after a start, stop, relaunch, restart, rm, `.git` per-run reports, `git-protect`; tools: python, uv, claude, codex, rust, go, adding in place, `update`, `remove`, gc.
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
- **Lima writes `lima.REJECTED.yaml` into the current folder** on an invalid config: run `limactl` outside the repo (devvm runs it from its data folder).

## Commits and tasks

- Tasks are in dex (`.dex/`, git-ignored).
- **One commit when each task is complete**, its Docs and Review subtasks included. Conventional Commits: `type(scope): summary`, types `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`. Then `dex complete <id> --commit <sha>`.
- **Don't commit or `dex complete` until the gate passes**, with whatever `mise run fmt` changed in the commit.
- **Dex task IDs never go in commit messages, code comments or docs.**
- **AGENTS.md stays short:** one line per path, rule or porting entry. Inventories (functions, messages, test names, converted cases) go in the commit message and the `dex complete` result, not here.

## Porting

The only place in the repo that mentions the bash devvm. It lives at `/Users/me/dev/devvm`; commit `5efc65f` is the spec (its bats tests and `tests/e2e.sh` define the behavior). Never change it.

- Copy only what a task needs (`git -C /Users/me/dev/devvm show 5efc65f:<path>`) and list it below. Rewritten code isn't listed: each task's commit and dex result say what it ported.
- Bats cases become testscript or Go table tests in the task that ports them; update the table. e2e sections come with the task that needs them.
- Bash fixes copied later are listed with their bash commit. Nothing else in the repo mentions the bash devvm or the port. This section goes when the port is done.
- No migrations from the bash's files or older state.

Copied (here ← there):
- `LICENSE`, `.shellcheckrc`, `layers/*`, `tools/*.policy`, `guest/tools.sh` ← same paths
- `base.yaml`, `tools/mise.toml`, `tools/build.sh`, `guest/sync.sh` ← same; edited: comment
- `.agents/skills/security-audit/`, `skills-lock.json` ← the bash repo's untracked files
- `testdata/mise.lock`, `testdata/stubs/tmutil` ← `tests/fixtures/`, `tests/stubs/`
- `testdata/stubs/{gpgv,mise}` ← `tests/stubs/`; edited: comment
- `testdata/stubs/{limactl,qemu-img,security}` ← `tests/stubs/`, adapted; `limactl`'s `oci/build.sh` and Ctrl+C parts to come
- `internal/proxy/` ← `devvm-proxy/` (`main.go` → `proxy.go`), adapted: `Main(args, IO)`, no `-connect`
- `tests/e2e.sh` ← same, adapted: framework, `images`, `project`, `tools` (node half), `network`

Docs (page ← bash page, all rewritten, partly):
- `images.md`, `commands.md`, `security.md`, `git-protection.md`, `coding-agents.md`, `ssh.md` ← same names
- `lima.md` ← `network.md`, `security.md`, `design.md`, `limits.md`
- `network.md` ← `network.md`, `devvm-proxy/README.md`
- README "Quickstart" ← README usage

Bats files (pending cases, by task):

| File | Pending |
| --- | --- |
| `build.bats` | features (step counts) |
| `bump.bats`, `release.bats` | all (release.sh, bump.sh) |
| `cli.bats` | `--from` (parse_args), secret/auth (auth, set/rm/ls, use, quoted values, `json_str`), ssh-agent (five), ssh config (`rm` forgets host keys), doctor (preflight, doctor), version, limactl |
| `delete.bats` | gc (two, overlay half, `delete_image_key`), ls (VM named like an image), existing VM (`vm_unfinished`, `rm` two), allowlist of a VM deleted outside devvm |
| `from.bats`, `oci.bats` | `--from` (all but the digest record and key cases) |
| `guest.bats` | ssh/code (VS Code settings) |
| `images.bats` | layer, features (listing), gc (three), images (three, `images rm`) |
| `install.bats` | all (install.sh) |
| `network.bats` | policy/network (five), log (three), ls, secret (`unuse`), ssh/code (direct route) |
| `ports.bats` | existing VM (applied record, restart warning), ports (five) |
| `project.bats` | `--from` (input checks), existing VM (folder made again, `update`, `remove`, five `.git` per-run reports), git-protect (strict) |
| `size.bats` | existing VM (restart warning), size (three) |
| `layers`, `lint`, `messages`, `tools` | none |

Dropped: `build_proxy` (two), the claude-token, features-file and pre-folder-check migrations.

Deliberate changes from the bash:
- A failed network self-check stops the VM, existing VMs included; a failed start stops the proxy it started.
- A failed build keeps its stopped VM for inspection; a failed create or clone deletes the partial instance. `script_failed` suggests `LIMA_HOME=… limactl shell --start` (no `devvm limactl`, no gc yet).
- Rebuild warns about and keeps an image in use (`image_in_use`). Build VMs are always confined (no `.confined`, `build_vm_unconfined`, `unconfine`).
- The proxy is in the binary (no `build_proxy`, `tool_intact`); `devvm _qemu connect` replaces `-connect`; log prefix `devvm proxy: `; `dial` refuses ports outside 1–65535; placeholders keep `devvm-proxy-managed`; its password and the login salt are in no process's arguments.
- New image keys, no parity: length-prefixed fields, the backend in base's key, a missing script hashed as missing.
- VM state is written before the clone, so a failed clone leaves nothing that starts. Slugs are cut per character; a too-long Lima home is `vm_name_no_room`, before anything.
- New VMs on Linux are checked for set-id files. Unsafe-folder paths resolve as far as they exist. VM-made values are Go-quoted, not stripped.
- The QEMU wrapper also refuses `-nic`, `-net` other than none, `-readconfig`, and an option as `-netdev`'s value.
- Sync stages in memory (no temp folder). Agent folder changes are `agent_dirs_added`/`_removed`; `mounts_next_start` replaces `mounts_on_restart`. Placeholders, tokens and host settings go in single-quoted, not `%q`.
- Host keys are kept only as a key type and base64; a linked `~/.ssh/config` is written through its link. A VM whose host keys are read is shown at most 64 KiB, for 30 s.
- `net_direct_route` doesn't name the Lima config. `DEVVM_SECRET_SERVICE` is checked for reads too. A failed tools resolve is `tools_need_resolve` with the log's end. e2e's sandbox and service differ from the bash's: its rebuilds delete every stale image in its Lima home.

Pending in code:
- up refuses an existing VM (`vm_exists`) until up for an existing VM brings callers for `WarnIfOutdated`, `WarnNewer`, `RefreshDaily`, plus `vm_unfinished`, `net_unset`'s `stop_proxy`, `net_flag_ignored`, the self-check on relaunch, gitprotect's per-run checks and keeping a running VM's proxy. `tools.Remove`, `Update`, `Garbage` wait for remove, update, gc.
- `ssh_remote_hint` doesn't name `devvm ssh-agent`. `EnsureInclude` (asks, then adds the Include line) waits for its callers, `--clone` and `devvm ssh`/`code`; `checkAgent` runs only once `--ssh-agent` exists; ssh config entries forward devvm's agent with `devvm ssh-agent`. Base features aren't run or hashed. Lima's `BuildImage` refuses `--from`. `log_ref`, `see_log`/`see_output` come with their callers; `tools_not_images_rm` and the tools listing with `devvm images`.
- `UpdateAgentDirs` waits for up for an existing VM. Secret brings `secret_store`, `secret_delete`, `proxies_forget_values`, Keychain writes, and `devvm secret set` in `no_claude_token`/`no_github_token` (they name `CLAUDE_CODE_OAUTH_TOKEN`, or nothing, until then).

Bash fixes copied: none yet.
