# AGENTS.md

devvm: one isolated VM per project folder, cloned from a base image, with toolchains and agents shared read-only between VMs. It's a Go CLI with swappable VM backends (Lima first). Read [README.md](README.md) for what it is, and the [docs site](docs/content/docs/) for how each part works; [Contributing](docs/content/docs/contributing.md) covers building, the tools, testscript and writing docs.

## Layout

| Path | What |
| --- | --- |
| `cmd/devvm/` | the entry point: the root command through `ui.Main`; the only `os.Exit` |
| `internal/cli/` | Cobra commands, one file each (root = up, `build`/`rebuild`, `images`, `gc`, `layer`, `setup`, `restart`, `update`, `remove`, `adopt`, `rm`, `ls`, `log`, `doctor`, `secret`, `auth`, `ssh-agent`, `ssh`, `code`, `limactl`, `ports`, `size`, `git-protect`, `completion`, hidden `version`, `_msg`, `_qemu`, `_proxy`, `_refresh-tools`); `script_test.go` runs testscript; `docgen/` writes the CLI reference |
| `internal/vm/` | up for a new VM (`up.go`) and an existing one (`existing.go`), the commands on a folder's VM (`commands.go`); `--clone` (`clone.go`); agents' mounts; the setup it sends (`sync.go`) |
| `internal/project/`, `internal/gitprotect/` | a project folder (VM name, unsafe folders, detection, set-id files); `.git` protection |
| `internal/sshconfig/`, `internal/sshagent/` | the ssh config for running VMs by name, their pinned host keys, the `~/.ssh/config` Include line; each VM's SSH agent choice, devvm's own agent and its keys |
| `internal/build/`, `internal/image/` | building and rebuilding images, their records; image keys and names (pure) |
| `internal/from/` | `--from`: references, registry digests (anonymous token), Docker's IDs, the recorded digests |
| `internal/tools/` | the shared tools: the lock, host mise, the daily check, tool image builds, the share |
| `internal/netpolicy/`, `internal/secrets/` | network policy (the choice, the VM's proxy, the self-check); tokens on the host and the VM's bindings |
| `internal/proxy/` | the proxy, run as `devvm _proxy`; see its `README.md` |
| `internal/backend/` | `Backend`, `Caps`, `Require`, the registry; `backendtest/` (fake), `backends/`, `lima/` |
| `internal/ui/` | the only package that prints: messages, prompts, status line, run log, cleanups, signals |
| `internal/msg/`, `locales/` | messages: `en.toml`, the generated functions, doc links (`docs.go`), the generator (`gen/`) |
| `internal/paths/`, `internal/state/` | every folder and file devvm uses; atomic state files (errors aren't messages yet) |
| `internal/platform/`, `internal/ubuntu/` | build-tagged OS code; finding and verifying Ubuntu's cloud image (never downloads) |
| `base.yaml`, `layers/`, `features/`, `tools/`, `guest/`, `oci/` | base's Lima template and scripts; base's optional features; shared tools (`mise.toml`, `build.sh`, `*.policy`); in-VM scripts sent on every launch; `--from`'s conversion (see its `README.md`) |
| `testdata/` | `script/*.txtar` (CLI tests), `stubs/` (host commands; see each header), `mise.lock` (fixture) |
| `tests/e2e.sh` | end-to-end tests on real VMs (bash 3.2) |
| `docs/` | the docs site (Hugo + Hextra); `content/docs/cli/` is generated, never edit it |
| `mise.toml`, `.golangci.yml`, `.vale*`, `.markdownlint-cli2.yaml`, `.github/` | tools, tasks, linters, CI and Pages |
| `.agents/skills/security-audit/` | the security-audit skill, for backend reviews |

Still to come: smolvm and docker backends; `install.sh`, `scripts/`, `packaging/`.

## Tasks

`mise run <task>`: `fmt`; `lint` (Go, shellcheck, docs, `TestDocLinks`); `test` (`-race`, in a throwaway `HOME`; needs a C compiler); `vuln`; `deadcode` (what it lists waits for a caller in Pending, or goes); `cli-docs` (after changing any command's `Use`, `Short`, `Long` or flags; commit the pages); `docs` (serves `http://localhost:1313/devvm/`); `e2e`.

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
- **Fail with `ui.Die(msg.X(...))`:** it prints and panics; `ui.Main` recovers, runs the cleanups, returns 1. Commands end with `fail` (`ui.DieLogged` for a `LoggedError`). INT/TERM/HUP cancel the root context, wait for the logged command, run the cleanups and return 130; a second signal is ignored; after one, `Die` prints nothing. A logged command a signal ended (or 128+) waits up to 1 s for devvm's own interrupt, as Ctrl+C can end it first.
- **Cleanups:** whatever a failure would leave (a build VM, a temp file) registers `ui.AddCleanup` and is removed once done; one only for interrupts checks `ui.Interrupted()`. Cleanups get a fresh context.
- **External commands run with `ui.LoggedCmd(ctx, step, cmd)`:** output to the run log, failure via `ui.DieLogged`. `ui.WithSteps(ctx, n)` numbers steps.
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
- **End-to-end:** `tests/e2e.sh` builds devvm from the checkout and drives real VMs in its sandbox (`~/.lt-e2e-go`, `/tmp/devvm-go-e2e`, service `devvm-go-e2e`), deleting only what it made; images stay cached (`--clean`). `tools` first resolves the newest tool versions. Run it in the background. Sections: `images`, `project`, `clone` (a throwaway repo, its own `HOME`), `gc` (an `img-*` folder's VM, a stale fixture), `tools` (a node VM), `network` (a claude VM), `from` (Debian 12, Alpine and Ubuntu bases, a docker that isn't running). Tiers: none (all but `from`, no features); `--from` (everything, every feature; before merging `layers/`, `tools/`, `features/`, `oci/` or `--from` changes); `--only a,b` (with `from`: every feature); to come: `--quick` (project, `--clone`, size, gc, network; after most changes). Hosted CI can't run it on macOS.
- **Linux hosts:** `LIMA_HOME=~/.lt-linux-go`, VM `linuxhost` (nested virtualization, the repo read-only, copied to `~/devvm`), set up as [testing.md](docs/content/docs/testing.md) says; never `~/.lt-linux`, which is the user's.
- **e2e to come:** `--backend`; network: git over SSH, opencode's login; tools: python, uv, claude, codex, rust, go, adding in place, gc.
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
- `testdata/stubs/{limactl,qemu-img,security}` ← `tests/stubs/`, adapted
- `testdata/stubs/docker` ← `tests/stubs/`
- `oci/{bootify,kernel,try}.sh` ← same; `oci/build.sh`, `oci/README.md` ← same, edited: comment, docs link
- `internal/proxy/` ← `devvm-proxy/` (`main.go` → `proxy.go`), adapted: `Main(args, IO)`, no `-connect`
- `tests/e2e.sh` ← same, adapted: framework, `images`, `project` (relaunch, restart, rm, `.git` reports, agent folders), `clone`, `tools` (node half, `update`, `remove`), `network` (self-check on relaunch), `from` (no image only Docker has: it would build one in the user's Docker)
- `features/*` ← same paths

Docs (page ← bash page, all rewritten, partly):
- `images.md`, `commands.md`, `security.md`, `git-protection.md`, `coding-agents.md`, `ssh.md`, `clone.md`, `from.md` ← same names
- `lima.md` ← `network.md`, `security.md`, `design.md`, `limits.md`
- `network.md` ← `network.md`, `devvm-proxy/README.md`
- `customizing.md` ← `customizing.md`
- `testing.md` ← same name (with the platform table, the Linux host as verified, manual checks)
- README "Quickstart" ← README usage
- `install.md` ← same name (requirements and checking them; installing waits for `install.sh`)

Bats files (pending cases, by task):

| File | Pending |
| --- | --- |
| `bump.bats`, `release.bats` | all (release.sh, bump.sh) |
| `install.bats` | all (install.sh) |
| `build`, `cli`, `delete`, `from`, `guest`, `images`, `layers`, `lint`, `messages`, `network`, `oci`, `ports`, `project`, `size`, `tools` | none |

Dropped: `build_proxy` (two), `json_str` (`encoding/json`), the claude-token, features-file, pre-folder-check and pre-git-protection migrations.

Deliberate changes from the bash:
- A failed network self-check stops the VM, existing VMs included; a failed start stops the proxy it started.
- A failed build keeps its stopped VM for inspection; a failed create or clone deletes the partial instance. `script_failed` suggests `devvm limactl shell --start` (no `lima_home=`).
- Rebuild warns about and keeps an image in use (`image_in_use`). Build VMs are always confined (no `.confined`, `build_vm_unconfined`, `unconfine`).
- The proxy is in the binary (no `build_proxy`, `tool_intact`); `devvm _qemu connect` replaces `-connect`; log prefix `devvm proxy: `; `dial` refuses ports outside 1–65535; placeholders keep `devvm-proxy-managed`; its password and the login salt are in no process's arguments.
- New image keys, no parity: length-prefixed fields, the backend in base's key, a missing script hashed as missing.
- VM state is written before the clone, so a failed clone leaves nothing that starts. Slugs are cut per character; a too-long Lima home is `vm_name_no_room`, before anything.
- New VMs on Linux are checked for set-id files. Unsafe-folder paths resolve as far as they exist. VM-made values are Go-quoted, not stripped.
- The QEMU wrapper also refuses `-nic`, `-net` other than none, `-readconfig`, and an option as `-netdev`'s value.
- Sync stages in memory (no temp folder). Agent folder changes are `agent_dirs_added`/`_removed`; `mounts_next_start` replaces `mounts_on_restart`. Placeholders, tokens and host settings go in single-quoted, not `%q`.
- Host keys are kept only as a key type and base64; a linked `~/.ssh/config` is written through its link. A VM whose host keys are read is shown at most 64 KiB, for 30 s.
- An existing VM without state or `net=` is `vm_unfinished` before anything; rm deletes it without an export. `--backend` on one warns. `size_next_start`/`ports_next_start` replace `*_on_restart`. A VM whose `.git` protection was never set up gets it as new. `adopt` clears `git_inode`, so the next launch locks `.git` as appeared.
- `--clone`: host git pushes and fetches through devvm's ssh config (pinned host keys), not Lima's; rm fetches unfetched branches into new local `saved/<remote>[-N]/*` branches, keeps the remote's fetched-only work there too, then `git remote remove`s it; a failed copy is retried on the next launch (`clone_seed_failed`); `fetch_failed`, `clone_check_failed`, `remote_add_failed` keep the VM.
- `--from`: `from_lookup_failed`/`from_cached` are `from_unreachable`, `from_not_found` and `from_cached_*` (no nested message); a registry still failing (5xx, 429) after retries is unreachable; `building_image_from` replaces `building_image`'s `from=`; Lima sets the converted disk with `--set .images` on `base.yaml` (no `lima.yaml`) and the build VM's mounts with `--set .mounts`; `from_ignored` on restart, update and remove too; a missing `--from` value is cobra's `usage_error`.
- `ls`: a BACKEND column, VMs from every backend sorted by name, `-` for what a VM's state lacks (an unset GIT isn't `on`), a tab-aligned table; a backend that can't list fails it (`ls_failed`, or its check's message) only when a VM's state names it.
- `log`: `--follow`/`--lines` long names; `--blocked` refuses `-f`/`-n` (not ignored); `net_off_log` on stderr; an empty log is `log_empty` too (`-f` waits on it); lines that aren't events are skipped; `--blocked` is a tab-aligned table, `-` for refused requests, IPv6 hosts bracketed.
- `policy`: `allow`/`deny`/`rm` subcommands with `-g`/`--global`; rules are the proxy's own syntax (`proxy.ValidRule`), all checked before any is added, `allow !RULE` refused; duplicates and `rm` match in any case, comments aside; files rewritten whole, through a link; `rule_added*`/`rule_removed*`/`rule_present` replace `policy_changed`/`removed`; `rm` on a missing file warns per rule (no `no_rules`); `net_off_policy` on edits too; `state.VMNames` skips `<vm>.policy`.
- `net_direct_route` doesn't name the Lima config. `DEVVM_SECRET_SERVICE` is checked for reads too. A failed tools resolve is `tools_need_resolve` with the log's end. e2e's sandbox and service differ from the bash's: its rebuilds and gc delete every stale image in its Lima home, and gc every unused tool image.
- Features: each runs as a `running_script` step (no `adding_feature`); none chosen is `base_features_none`; feature names that aren't layer-like are skipped in the listing and make `base-features` unreadable.
- `help` is Cobra's (`devvm help COMMAND`, per-command help); `devvm completion bash|zsh|fish` is new; `--version` has no `-v`; the version is set at build time, else Go's build info (no `VERSION` file read).
- Host checks are each backend's `Problems`: a command stops at the first (none listed under it); `doctor` checks the new VMs' backend (`--backend`), adds a features table, Lima's global configs (`lima-config`) and `qemu-img`, `*_fix` messages carry the fix, `doctor_ok` names the backend.
- `network`: no argument prints `ls`'s value (`unenforced` too); an unchanged one is `net_already`; `on` needs the backend's egress proxy first; a running VM turned off stays on its proxy until it next starts (`net_off_next_start`); `Confined` means started with the proxy, and `net_unenforced*` say "wasn't started confined"; a bad argument is cobra's `usage_error`.
- `secret`: `ls` (and `devvm secret`) is a table of every secret, SAVED and USED BY; set, rm, use and unuse rewrite running proxies' bindings (no touch); `secret_saved`/`_keychain`, `secret_removed`, `secret_used`/`_unused` replace `secret_saved keychain=`, `removed`, `secret_bound`; `unuse claude` on an image with claude is `secret_with_image`; control characters are refused too; `use` warns when no token is saved; bad names are cobra's `usage_error`.
- `ssh-agent`: an unchanged mode is `agent_already`; a stopped VM's forwarding changes at its next launch (`agent_set_*`, no `agent_off`), a running one's warns `agent_next_start` (was `agent_on_restart`); a new VM without forwarding carries no agent from the start; `agent_key_unpinned` names no older devvm; a missing key is `agent_key_not_found`, a bad lifetime `agent_lifetime_invalid` (no usage); `keys` shows the default lifetime beside other options.
- `layer`: `devvm layer` alone is its help, a missing or extra NAME cobra's `usage_error` (no `usage_layer`); `layer_created` is one line; the script is linked into place whole, never over one that appeared.
- `ports`: `add`/`rm` subcommands with `-g`/`--global` anywhere; a running VM's pending ports are `ports_pending` on stderr (no `next_start` on stdout); `add`/`rm` of all or none is `ports_list_needed`, other bad usage cobra's `usage_error` (no `usage_ports`); `-g` refuses a hand-edited ports file too; a change checks the backend's port forwarding first.
- `size`: its own `--cpus`/`--memory`/`--disk` and `-g`/`--global`; a running VM's pending size is `size_pending` on stderr (no `next_start` on stdout); a running VM's unchanged size is `vm_size` too; `size_defaults` shows sizes as `vm_size` does; `-g` refuses a hand-edited size file too; a change checks the backend's features first; `no_vm_size`/`size_failed` (no `usage_size`, `usage_size_flag`, `size_change_failed`: cobra's `usage_error`, `lima_edit_failed`).
- `git-protect`: a `--clone` VM shows `clone` and refuses changes (`git_protect_clone`); a never-set-up VM shows what its next launch sets; strict counts as on for launches and rm; strict that can't set `diff.ignoreSubmodules` fails (`git_config_failed`); `git_locked*`/`git_unlocked`/`git_lock_later` name the VM; a bad mode is cobra's `usage_error` (no `usage_git_protect`).
- `images`: two tab-aligned tables, `-` for what's missing: images on every backend (a BACKEND column; FROM a column, not `image_from`; STATE `unfinished` for `(unfinished)`/`(unknown)`), then tool images (NAME, TOOL, VERSION, SIZE, VMS counted per image from VMs' `tools=`, STATE; the lock's unbuilt version a `not built` row, not `newer:`); never resolves the lock; no custom layer rows, share total or section messages; `images rm` resolves every name before deleting, an unfinished one is `deleted_unfinished_build`, `deleted_image` names the key, `no_image_named`/`_from` point to devvm images; bad usage is cobra's `usage_error`.
- `gc`: deletes what `images` lists as stale or unfinished (every backend) and as `unused`: tool images any VM's state lists stay (a VM deleted outside devvm's too), none without a lock (never resolves); a stale image an overlay uses is listed, then kept with `image_in_use` (no `keeping_stale`); no `.tmp` files (tool builds write outside the share); `gc_images`/`gc_image`/`gc_unfinished`/`gc_tools` replace `stale_images`/`unused_tools`; `images` adds `images_stale_note`/`images_unused` on stderr.
- `setup`: `--features LIST` chooses without asking (in the features' run order; an unknown name is `no_feature`, nothing saved) and `--backend NAME` saves the default backend (`default_backend`; alone, features untouched); the picker starts from the current choice, an unreadable `base-features` is `setup_failed`; `base_will_rebuild` compares the chosen lists, only in setup (not a first build's choice); `gc_old_hint` when any image record exists.
- `export`: needs a finished VM made for this folder (`vm_unfinished`, `vm_other_folder`); the tarball is written beside its file (mode 600) and renamed in once complete, so a failure leaves none and replaces nothing; `history_not_exported`/`export_failed` for export, `rm_export_failed` for rm's.
- `stop`: needs a finished VM made for this folder (`no_vm`, `vm_other_folder`, `vm_unfinished`); stops its proxy at once; `stopped`, or `already_stopped` (checked all the same, not an error); a `--clone` VM's folder isn't checked; `git_protect_running` names it.
- `ssh`/`code`: a launch without its shell (`launch`, then setup: a stopped VM's pending size and ports applied, the user's agent checked, `sync` run so VS Code's Machine settings are set again, a `--clone` VM's clone made); they need a finished VM made for this folder and the backend's `ssh`; `code` gets the environment devvm started with (its `SSH_AUTH_SOCK`); `ssh_host_info` is two lines naming the host and folder; `ssh_failed`/`code_failed`.
- `limactl`: runs in devvm's place (exec), every argument passed on (`devvm help limactl` is devvm's); no `limactl` is `lima_missing`, a failed exec `limactl_failed`.

Pending in code:
- deadcode's list: `backendtest` (tests'), `backend.Emulated` (an emulating backend).
- `devvm setup` asks for the default backend once there's a second one. `log_ref`, `see_log`/`see_output` come with their callers.
- Release builds set the version with `-ldflags "-X github.com/w3cj/devvm/internal/cli.version=…"` (`.goreleaser.yaml` comes with releases); `doctor_ok` shows it.

Bash fixes copied: none yet.
