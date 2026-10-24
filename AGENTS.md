# AGENTS.md

devvm: one isolated VM per project folder, cloned from a base image, with toolchains and agents shared read-only between VMs. It's a Go CLI with swappable VM backends (Lima first). Read [README.md](README.md) for what it is, and the [docs site](docs/content/docs/) for how each part works; [Contributing](docs/content/docs/contributing.md) covers building, the tools, testscript and writing docs.

## Layout

| Path | What |
| --- | --- |
| `cmd/devvm/main.go` | the entry point: runs the root command and exits with its code. The only place that calls `os.Exit` |
| `internal/cli/` | Cobra commands, one file each (the empty root and the hidden `_msg` so far), and `script_test.go`, the testscript runner |
| `internal/msg/` | messages: `messages_gen.go` (one function per message, generated), `M` (text in the user's language, names mode, doc links, `FromArgs` for `_msg`), the go-i18n bundle. `gen/` is the generator (`go generate ./internal/msg`); the tests check the generated file is current, every message renders, every one is called and every doc link exists |
| `locales/` | `en.toml`, every message (other languages join as `<lang>.toml`), embedded by `locales.go` |
| `internal/paths/` | names every folder and file devvm uses: the folder holding the binary (after following links), and `Dirs` (XDG data and config folders, devvm's Lima home, `DEVVM_LIMA_HOME`) with a method per state folder and config file |
| `internal/state/` | reads and writes state files atomically, with `CorruptError` for unreadable ones: VM state (key=value, `backend=` included), image records and `--from` digests, and the config files (`size`, `ports`, `base-features`, `agent-keys`). Its errors aren't messages yet |
| `testdata/script/*.txtar` | CLI tests (testscript) |
| `testdata/stubs/` | stand-ins for host commands, first on `PATH` in tests; they log calls to `$STUB_LOG`. So far `uname` and `sw_vers` |
| `docs/` | the docs site (Hugo + Hextra): `hugo.yaml`, `content/_index.md`, `content/docs/*.md`. Pages arrive with the behavior they describe |
| `.vale.ini`, `.vale/styles/devvm/`, `.markdownlint-cli2.yaml` | the docs linters' config |
| `mise.toml`, `mise.lock` | every tool, pinned and locked, and the tasks |
| `.golangci.yml` | linters, formatters, the dependency allowlist (depguard) and the printing and exit bans (forbidigo) |
| `.github/workflows/` | `ci.yml` (lint and test on macOS and Linux), `docs.yml` (the site on GitHub Pages) |
| `.agents/skills/security-audit/`, `skills-lock.json` | the security-audit skill, used by the backend reviews |

Not there yet, and where it goes when its task arrives: `internal/ui` (say/info/note/warn/die/ask/confirm, tty, spinner, run logs, interrupts, the cleanup stack), `internal/{project,image,tools,gitprotect,netpolicy,secrets,vm}`, `internal/backend/` (interface, `Caps`, registry; `lima/`, `smolvm/`, `docker/`), `internal/proxy` (stdlib only), `internal/platform` (build-tagged OS code). The in-VM scripts (`base.yaml`, `layers/`, `features/`, `tools/`, `guest/`, `oci/`) sit next to the binary; `tests/e2e.sh` drives real VMs; `install.sh`, `scripts/` and `packaging/` handle releases.

## Tasks

`mise run <task>`:
- `fmt`: gofumpt and goimports, through golangci-lint.
- `lint`: golangci-lint, shellcheck on every shell script, then markdownlint-cli2, Vale and `lychee --offline` on `docs/content`, then the message-link check (`TestDocLinks` in `internal/msg`: every `doc =` in `locales/en.toml` is a page and heading).
- `test`: `gotestsum -- -race -cover ./...`, including the testscript suites. Needs a C compiler for `-race`.
- `vuln`: govulncheck.
- `docs`: serves the site (`http://localhost:1313/devvm/`). Regenerating the CLI reference joins it once commands exist.
- `e2e`: `tests/e2e.sh`, once it exists.

## Rules

- **The gate:** `mise run fmt && mise run lint && mise run test`, all passing, before starting a task and before marking it done. A gate that fails at the start gets fixed first. Anything touching VMs also passes `tests/e2e.sh --quick` in the sandbox (see Testing), in the background.
- **Nothing in project folders:** state goes in `~/.local/share/devvm` and `~/.config/devvm`.
- **Credentials never go into images.** Host agent history never goes into VMs.
- **Never auto-delete project VMs:** they may hold unsaved work. Warn instead.
- **The startup check only reports.** Never install host software without asking.
- **Image keys hash everything that goes into an image**, the backend included: add any new input file to the image key. A tool image's key is its lock entry, `tools/build.sh` and the libc it's built for.
- **Pin every download** in `layers/`, `features/`, `tools/build.sh` and `oci/build.sh`: a `<tool>_version` and `<tool>_sha256_arm64`/`_amd64` at the top of the script, fetched with `devvm-fetch` (`oci/build.sh` checks with `sha256sum`: its VM has no `devvm-fetch`). No `curl | sh`, no "latest". The exceptions follow upstream, still checked: the shared tools (resolved per user into `~/.local/share/devvm/mise.lock` with upstream's checksums, installed with `MISE_LOCKED=1`), base's Ubuntu cloud image (its `SHA256SUMS`, signature-checked) and Lima in `install.sh`. In Go, downloads check their SHA-256 before use.
- **Shared tools are read-only in VMs:** whatever writes into a tool's own folder (global installs, self-updates, `corepack enable`, `rustup component add`) must be redirected (`~/.npm-global`, `go_set_gobin = false`), done in `tools/build.sh`, or turned off.
- **In-VM scripts are bash 3.2-safe and shellcheck-clean** (`layers/`, `features/`, `tools/build.sh`, `guest/`, `oci/`, `install.sh`, `tests/e2e.sh`): guard empty arrays (`${a[@]+"${a[@]}"}`), and don't use associative arrays. Everything else is Go.
- **Docs:** succinct and explicit. Every behavior change updates its page in `docs/content/docs/` in the same task. The README stays high level and links to the site.
- **Edit files with the editor tools** (read, write, edit), not shell text tools (`sed`, `awk`, `python`, heredocs, `cat >`).
- **Ask first** before anything outward-facing (publishing, releases, creating repos, pushing) and before deleting anything you didn't create.
- **If a task turns out wrong or incomplete,** propose a change to it (`dex edit`) instead of quietly doing something else.

## Go conventions

`internal/ui` doesn't exist yet; these are the rules it's built to.

- **Only `internal/ui` prints.** forbidigo bans `fmt.Print*` and `os.Stdout`/`os.Stderr` elsewhere, and `os.Exit` outside `cmd/devvm`.
- **Fail with `ui.Die(msg.X(...))`:** it panics with an exit error; `main` recovers it, runs the cleanup stack and exits 1. Ctrl+C (INT/TERM/HUP) cancels the root context and runs the same cleanups. Anything that leaves something behind on failure (a build VM, a temp file) registers its cleanup.
- **OS-specific code goes behind build tags** in `internal/platform` (`*_darwin.go`, `*_linux.go`, `*_other.go`), so Windows can follow.
- **Dependencies:** the depguard allowlist in `.golangci.yml` (cobra, pflag, go-i18n and its TOML parser BurntSushi/toml, x/text, x/term, x/sys; go-internal in tests). `internal/proxy` is stdlib only. Adding one is a decision: ask.
- **External tools are called as commands, never linked** (`limactl`, `smolvm`, `docker`, `security`, `git`, `chflags`, `sandbox-exec`, `qemu-img`, `ssh-keygen`). Prefer the standard library to shelling out (`net/http`, `crypto/sha256`, `encoding/json`, `text/template`, `net.Dial`). Strings sent into a backend's shell are quoted with `shq()`.
- **A command exists only once it's ported.** Cobra's docs generator writes `docs/content/docs/cli/`; a test fails when it's stale.
- `CGO_ENABLED=0` for builds.

## Backends

- Every backend implements one interface and declares its capabilities (`Caps`) per feature: native, emulated (reviewed) or none. Each backend's column is in `docs/content/docs/backends.md`.
- **A feature a backend lacks fails** in one line naming the VM and what to do. Never fall back silently to something weaker.
- **Every emulation is refused until its review passes:** a design note in `docs/design.md`, a threat check with the security-audit skill, and e2e checks proving the guarantee. Then it's marked reviewed in `Caps`.
- Backend-specific code stays in its package. What doesn't depend on the backend (`.git` protection with host `chflags`, the network policy proxy) stays out of it.

## Messages

- **Succinct and direct.** Say what is being done, or what failed and what to do. No explanations; link a docs page when more is needed.
- **Every message lives in `locales/en.toml`** and is called through its generated function (`msg.NoVM(dir)`, parameters in sorted field order); run `go generate ./internal/msg` after editing it. Template fields are snake_case plain fields (`{{.vm_name}}`, no logic); `one` next to `other` makes it plural (`count int`); a `doc = "page#anchor"` adds the link. A message with no caller fails the tests: add it with its first caller. `msg.M` is an `error`, so a function can return one. Never build prose with `fmt` outside `msg`.
- An error is one line. Explain why only when the user has to decide something; a prompt gets at most two lines of context. Status steps are a few words, lowercase. Commands are written as devvm x, not quoted. No "please", "note that" or "simply".
- Not messages: data output (tables, files devvm writes) and text in scripts that run elsewhere.
- **Tests check which message was printed and with what, never its wording:** `DEVVM_MESSAGES=names` prints `no_vm dir=/x`, and `devvm _msg NAME k=v…` gives e2e the text.

## Testing

- **Unit tests** are Go table tests next to the code. **CLI tests** are testscript scripts in `testdata/script/` against the built binary, in a sandbox under `/tmp`, with `testdata/stubs/` standing in for host commands (`stub_calls` checks what they were called with). New logic needs a test. Unit and CLI tests never touch the Keychain or real VMs: stubs stand in for `security`, `limactl` and the rest.
- **Don't touch the user's own VMs or images.** For manual runs and e2e, use a sandbox: `DEVVM_LIMA_HOME=~/.lt LIMA_HOME=~/.lt XDG_DATA_HOME=/tmp/x/data XDG_CONFIG_HOME=/tmp/x/config DEVVM_SECRET_SERVICE=devvm-sandbox-test` (devvm uses its own Lima home, `$XDG_DATA_HOME/devvm/lima`, unless `DEVVM_LIMA_HOME` moves it; `LIMA_HOME` is for your own `limactl` calls). Put a `base-features` file in the config dir so the picker doesn't prompt, and pass `--network-policy` or `--no-network-policy` so devvm doesn't ask. `DEVVM_SECRET_SERVICE` keeps test tokens out of the user's devvm Keychain items (delete them afterwards). Never use the user's real tokens in tests. This applies once the commands exist.
- **End-to-end**, once `tests/e2e.sh` exists: it drives real VMs in its own sandbox (`~/.lt-e2e`, `/tmp/devvm-e2e`), deleting only the VMs it made, with `--backend lima|smolvm|docker`. Sections: `images project clone size gc network tools from`. Every run first resolves the shared tools' newest versions. Run it in the background. Pick the tier by what changed:
  - `--quick`: project (mounts, agent folder mounts, `.git` protection), `--clone`, size, gc safety and network policy, from `base`. A few minutes with cached images. After most changes.
  - (no option): adds `images` and `tools` (every shared tool in VMs).
  - `--from`: everything, with `--from` images and every optional feature. Before merging changes to `layers/`, `tools/`, `features/`, `oci/`, or the `--from` code.
  - `--only network,size`: just those sections, while working on one area. Hosted CI can't run it on macOS (no nested VMs).
- **Delete test VMs and temp files afterwards.** Long builds (minutes) run in the background.
- **`limactl shell` reads stdin:** give it none unless the input is meant for it.
- **Boot problems:** read `<devvm's Lima home>/<vm>/serialv.log` before restarting; every start overwrites it.

## Gotchas learned

- **Lima `vz`:** raw disks only; clones are APFS copy-on-write (`du` overcounts). Direct kernel boot needs an uncompressed arm64 `Image`. Stopping sends a GPIO power key, so guests need `gpio_keys`, or Lima hard-kills them after 30 s. Lima can't shrink disks. virtiofs tags are `lima-<hash>`. After root touches a directory on a virtiofs mount, the guest sees it as root's for a few seconds (git says "dubious ownership"). `mounts: []` doesn't remove the home mount from `base: template:…`: pass `--mount-none` (or `--mount-only`) to `limactl create`/`clone`.
- **Lima can report a failed start for a VM that is running but degraded.** Starting tolerates it.
- **Linux hosts (QEMU):** mounts are 9p, and a 9p share mounts once per guest. Docker writes bind-mounted files as root, so containers that write host files `chown` them back. Ubuntu's `docker.io` has no BuildKit (`RUN --mount` needs `docker-buildx`). Debian's tmpfs `/tmp` hides anything mounted below it during boot. The disk is one qcow2 file and project VMs are overlays of their image's, so never start an image VM or write its disk after it's built.
- **Shared tools:** mise rewrites `latest`/`24`/`lts` links in a tool's folder after any install, so those folders are local in the VM and only the version folders are mounted read-only.
- **Lima writes `lima.REJECTED.yaml` into the current folder** when a config is invalid: run test `limactl` commands outside the repo.

## Commits and tasks

- Tasks are in dex (`.dex/`, git-ignored).
- **One commit when each task is complete**, its Docs and Review subtasks included. Conventional Commits: `type(scope): summary`, with types `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`. Then `dex complete <id> --commit <sha>`.
- **Don't commit or `dex complete` until the gate passes**, with whatever `mise run fmt` changed in the commit.
- **Dex task IDs never go in commit messages, code comments or docs.**

## Porting

The only place in the repo that mentions the bash devvm. It lives at `/Users/me/dev/devvm`, and its commit `5efc65f` is the spec: its bats tests and `tests/e2e.sh` define the behavior. Never change it.

- Copy only what a task needs (`git -C /Users/me/dev/devvm show 5efc65f:<path>`), and list each copied file below.
- Bash bats cases (`tests/unit/*.bats`) become testscript or Go table tests in the task that ports them. `tests/e2e.sh` sections come over with the task that needs them.
- Bash fixes copied later are listed below with their bash commit.
- Nothing else in the repo (docs, comments, messages, tests) mentions the bash devvm or the port. This section goes when the port is done.

Copied:
- `LICENSE`
- `.agents/skills/security-audit/` and `skills-lock.json`: not from `5efc65f`; they were untracked files in the bash repo's working tree.
- `docs/images.md` → `docs/content/docs/images.md`, partly: "Where state lives", rewritten.

Ported from `devvm` (rewritten in Go, not copied):
- paths and state (`internal/paths`, `internal/state`): the `*_dir`/`*_file` definitions, XDG and `DEVVM_LIMA_HOME`, `vm_get`/`vm_set`/`new_vm_state`, image records, `from-digests`, and reading and writing the `size`, `ports`, `base-features` and `agent-keys` files. Go's own formats; their callers come with their tasks. Bats cases converted to Go table tests: `cli.bats` "vm_set/vm_get: keys replace, values may contain '='" and the Lima home half of "devvm's own Lima home" (the `devvm limactl` half goes with that command); the file side of `from.bats` "refresh_from_digest records the registry's digest per tag" and of `build.bats`'s libc record. `images.bats` "the chosen features move from the old features file" is dropped: no migration from the bash files.
- messages (`internal/msg`, `locales/en.toml`, `devvm _msg`): `messages.sh`'s `msg` helper and the unit tests' names-mode `msg` (values now in sorted key order), and `tests/lint-messages.sh`, whose checks are now the compiler, the generator and the msg tests. No bulk conversion of `messages.sh`: each task writes its own messages. Bats cases converted: `messages.bats` "msg passes values by name, as they are" (`TestValuesAsGiven`, `msg.txtar`) and "every message prints with the names devvm passes" (`TestEveryMessageRenders`); `lint.bats` "a missing, extra or repeated name, an argument that isn't name=value" and the unknown half of "unknown and unused messages" (`TestFromArgs`, `msg.txtar`; in Go code the compiler checks calls), its unused half (`TestEveryMessageIsUsed`), "messages use names, not positional arguments" (the generator's snake_case plain-field check, `TestGenerateRefuses`) and "devvm's calls match messages.sh" (`TestGeneratedFileIsCurrent` plus the compiler). Left to `internal/ui`: "each helper's stream and prefix", "warn and die prefix every line", "die prints the message and exits 1", "confirm" and "ask". Dropped as bash-specific: `lint.bats` "calls that pass exactly what each message uses" and the "helper with no message" case (the compiler), "helper names in strings, comments, heredocs and arguments aren't calls" (no text parsing in Go), "plain text passes until --strict" and "--strict: prose outside messages.sh" (forbidigo bans printing outside `internal/ui`; prose in `fmt` is a review rule).

Bash fixes copied: none yet.
