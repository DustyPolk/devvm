---
title: Testing on other hosts
weight: 1
---

For contributors. The everyday tests, CLI tests and end-to-end tests are in [Contributing](_index.md#end-to-end-tests), and how to raise a package's coverage baseline is in [Coverage](_index.md#coverage); this page is about running them on a host other than your Mac.

## What's platform-specific

The same code and tests run on macOS and Linux; what differs is behind build tags in `internal/platform` and the backend's own checks:

| | macOS | Linux |
| --- | --- | --- |
| VMs | Lima on Apple's Virtualization framework (`vz`) | Lima on QEMU with KVM |
| Mounts | virtiofs | 9p, one mount per share per guest |
| Project VM disks | APFS copy-on-write clones of the image's | qcow2 overlays of the image's |
| Confined tiers | the VM's Lima process under a macOS sandbox | QEMU's `restrict=on` and a `guestfwd` to the proxy |
| `.git` protection | locked with `chflags` | none: devvm says so ([details](../safety/git-protection.md#linux-hosts)) |
| Set-id files | dropped by virtiofs | reach the host: new VMs check for them |
| Tokens | the Keychain | files only you can read, in `~/.local/share/devvm/secrets/` |
| The CA | kept out of Time Machine | no backup to keep it out of |

`tests/e2e.sh` checks each side where it runs: on Linux, the `.git` protection checks become one that it's off and devvm says so, and the Keychain check is left out.

On macOS, `tests/e2e.sh` also needs OpenSSL as `openssl`, because its CA checks serve HTTPS with `openssl s_server`, and the LibreSSL that macOS ships as `/usr/bin/openssl` cuts the VM's OpenSSL client off mid-request. Install it with `brew install openssl@3`, then put `$(brew --prefix openssl@3)/bin` first on your `PATH`, because Homebrew doesn't link it into its own `bin`. On Linux, `openssl` is OpenSSL, and Ubuntu's images have it installed, as `ca-certificates` needs it.

Before it builds anything, `tests/e2e.sh` checks every command and perl module of this machine's that the chosen sections use, apart from those every macOS and Linux system has, such as `awk`, `sed`, `grep` and coreutils. It always checks `limactl`, `go` (or mise, which installs the repo's pinned Go), `git`, `pgrep`, `pkill` and `openssl`, which must be OpenSSL and not LibreSSL. For the sections that use them, it also checks `nc`, `lsof` on macOS or `ss` on Linux, `curl`, OpenSSH's `ssh`, `ssh-agent`, `ssh-add` and `ssh-keygen`, `tar`, `python3` on macOS, `sha256sum` or `shasum`, and perl with the two modules its helpers load, `File::Find` and `IO::Socket::INET`. When any of them is missing, e2e lists each one with the command that installs it, through Homebrew on macOS or `apt-get` or `dnf` on Linux, and stops. Fedora splits perl into a package for each module, so a Fedora host may need `perl-File-Find` and `perl-IO` as well as `perl`.

## Linux hosts

Test in a Lima VM with nested virtualization, which needs an M3 or newer and macOS 15 or newer. Give it a Lima home of its own, so its VMs stay apart from yours:

1. Make it from Lima's Ubuntu template, with nested virtualization, room for nested VMs, its user at uid 1000, and only the repo mounted, read-only:

   ```sh
   REPO=$(pwd) && cd /tmp
   export LIMA_HOME=~/.lt-linux-go
   limactl create --name=linuxhost --tty=false --vm-type=vz --cpus=8 --memory=16 --disk=150 \
     --containerd=none --set '.nestedVirtualization=true | .user.uid=1000' --mount "$REPO" template:ubuntu-24.04
   limactl edit --tty=false --set '.mounts=[{"location":"'"$REPO"'","writable":false}]' linuxhost
   limactl start --tty=false linuxhost
   ```

   Run it from the main checkout of the repo, which the VM mounts at the same path, and not from a git worktree. A worktree's `.git` is a file that points into the main checkout's `.git` folder, outside the mount, so git in the VM can't read it, and the VM's only mount goes when the worktree does. `--mount` alone keeps the template's home mount, hence the edit. Lima gives the VM's user your Mac's uid unless `user.uid` sets another, and a Mac's first user has uid 501. Most Linux hosts' first user has uid 1000, and so does the user that some container images ship, such as the `ubuntu` user of Ubuntu's images. With the VM's user at uid 1000, the `from` section of `tests/e2e.sh` meets that clash, as devvm does on most Linux hosts.

2. Inside it, install the packages, then Lima and mise from their release tarballs, each checked against its release's checksums (`SHA256SUMS`, `SHASUMS256.txt`) with `sha256sum -c`, and add yourself to the `kvm` group:

   ```sh
   sudo apt-get install -y qemu-system-arm qemu-utils qemu-efi-aarch64 virtiofsd build-essential git curl jq rsync
   sudo usermod -aG kvm "$USER"
   ```

   Then restart the VM, so a fresh login has the group.

3. Copy the repo into the VM's home, so the read-only mount stays untouched, install the pinned tools and run the gate, then the end-to-end tests:

   ```sh
   rsync -a --delete /path/to/your/checkout/ ~/devvm/ && cd ~/devvm
   mise trust && mise install
   mise run fmt && mise run lint && mise run test
   tests/e2e.sh --from
   ```

   The copy keeps the checkout's `.git` folder, which the gate needs, because `mise run lint` finds the shell scripts it checks with `git ls-files`, and the docs' build reads each page's git information (`enableGitInfo`). A copy without `.git`, such as one that `git archive` makes, fails the lint. To test a branch that a worktree has checked out, commit it, and clone that branch from the main checkout instead of copying the checkout, because a worktree's branches are the main checkout's too:

   ```sh
   rm -rf ~/devvm && git clone --branch BRANCH /path/to/your/checkout ~/devvm && cd ~/devvm
   ```

   A clone has only what's committed, so commit a change before you test it this way.

The first `--from` run builds every image and takes about half an hour on an M3 Max. Later ones reuse them, because every section but `tools` uses the pinned shared tools' versions and `--from` images, and those images stay valid until `scripts/bump.sh e2e-tools` or `scripts/bump.sh e2e-from` moves a pin ([end-to-end tests](_index.md#end-to-end-tests)). Stop the VM when done (`limactl stop linuxhost`); it keeps its images for the next run.

Gotchas of this setup:

- The Linux host's home and its VMs' homes are both `/home/<user>.guest`, so a file under `$HOME` in a VM isn't proof of a mount: `tests/e2e.sh` checks the VM's mounts list instead.
- A VM booting while another one works boots degraded and its downloads time out: `tests/e2e.sh` stops a VM it's done with before starting the next.
- minisign, for signing releases, has no linux/arm64 build, so mise skips it there.

## x86_64 hosts

These need real x86_64 hardware, or an x86_64 VM on such hardware with nested virtualization. An emulated x86_64 VM on a Mac (`--arch=x86_64`, QEMU TCG; Lima needs its `lima-additional-guestagents`) runs the unit tests, but VMs nested in it crash in their firmware.

A bare-metal x86_64 Linux host, such as Fedora 43 with SELinux enforcing, runs the gate and `tests/e2e.sh --from` as the [Linux host above](#linux-hosts) does, from a clone of the repo, and it can run `--slots 3` ([slots for each host](#slots-for-each-host)). Those runs check the x86_64 side of what devvm builds: the `_amd64` pins of base, every layer and feature, the shared tools' `linux-x64` lock entries and their checksums, `--from`'s conversion, whose VM reads its scripts over 9p, and the confined tiers through `devvm _qemu` and `qemu-system-x86_64`.

The tests don't cover the release itself, so check it by hand on that host. Build the archives with `scripts/release.sh --head`, with `DEVVM_MINISIGN_KEY` and `DEVVM_MINISIGN_PUB` naming a test key so the signature is checked too, and install `devvm-<version>-linux-amd64.tar.gz` from them in a throwaway `HOME`, as [maintaining](maintaining.md) shows, with `main --yes` in place of `main --no-deps`. With only the system's folders on `PATH`, the script finds no Lima, so it installs Lima's latest release into that `HOME`'s `~/.local`. When the host already has QEMU and access to `/dev/kvm`, nothing it runs needs sudo. Then run the installed `devvm --version` and `devvm doctor`, and check with `file` that the binary is statically linked.

devvm hasn't been tested on an Intel Mac. Its release has a `darwin-amd64` archive, and that binary runs under Rosetta on Apple silicon, but nobody has run the tests or a VM on Intel hardware.

GitHub's Linux runners are x86_64 with KVM, and the E2E workflow (`.github/workflows/e2e.yml`) runs `tests/e2e.sh --only project` on one as a trial. Start it from the repo's Actions tab, with Run workflow on the E2E workflow, because it has no other trigger yet. It installs QEMU, gives the runner's user `/dev/kvm` as Lima's own CI does, installs Lima at a pinned version checked against its checksum, and runs the section up to three times. It caches Ubuntu's cloud image under the release that `tests/e2e.sh` pins, and the built images under a hash of `assets/`, `tests/e2e-mise.lock`, that release and Lima's version, because devvm's image keys are only known once it builds. When the run fails, it uploads the e2e logs, devvm's run logs and each Lima instance's serial logs as the `e2e-logs` artifact.

## Slots for each host

`tests/e2e.sh --slots N` runs the end-to-end sections in N sandboxes at once ([slots](_index.md#slots)), and each slot runs its own VMs, up to three at a time in the link section. How many slots a host can run depends on its cores and memory:

| Host | Slots |
| --- | --- |
| A bare-metal Linux host with 32 cores and 124 GB of memory | 3 or 4 |
| A Mac | 2 |
| A Linux host nested in a VM, such as [the Linux host above](#linux-hosts) | 1, the default: a VM booting while another one works boots degraded there |

A Mac clones the images it gives each slot, because `/tmp` and your home folder are on the same APFS volume. A Linux host reflinks the Lima instances in your home folder when its file system can, as XFS made with `reflink=1` and Btrfs can, but a `/tmp` on tmpfs can't reflink, so there each slot other than slot 0 builds its own tool images. Every slot keeps its images between runs, so only its first run builds them.

## End-to-end tests in Go

The end-to-end tests are moving from `tests/e2e.sh` to Go, one section at a time, into `tests/e2e`. That package builds only with the `e2e` build tag, and its `TestE2E` is the run. The `images`, `clone`, `gc`, `tools`, `network` and `nix` sections are in Go so far, and `tests/e2e.sh` runs every other section. After its own sections, `tests/e2e.sh` hands the chosen sections that are Go's, the ones its `go_sections` lists, to `TestE2E`, in the same sandbox and under the same lock. It passes the Go run its own pid, which holds the lock, how many checks it has run and expects in all, and when it started, so the Go run's results count on from its own. The Go run writes its results to `summary.go` in the sandbox's state folder, and `tests/e2e.sh` adds them to its own. So `tests/e2e.sh`, `mise run e2e` and `tests/e2e.sh --only gc` still run every section they choose, and `--slots` places the Go sections as it places the others.

### Running the Go runner

To run the Go sections without `tests/e2e.sh`, run `mise run e2e:go` with the runner's flags after `--`, or run `go test` yourself from `tests/e2e`:

```sh
mise run e2e:go -- -e2e.only=gc
cd tests/e2e && go test -tags e2e -count=1 -timeout 0 -run '^TestE2E$' -args -e2e.only=gc
```

Run `go test` from `tests/e2e` without naming a package, because only then does `go test` show the run's lines as they come, and in a package list it holds them until the package ends. `TestE2E` refuses any `-timeout` but `0`, because `go test`'s default of 10 minutes would end a long section in the middle of a check, and `-count=1` keeps `go test` from showing a cached result. Its flags are `tests/e2e.sh`'s options with `e2e.` before them:

| Flag | `tests/e2e.sh`'s option |
| --- | --- |
| `-e2e.only=gc,…` | `--only gc,…` |
| `-e2e.from` | `--from` |
| `-e2e.clean` | `--clean` |
| `-e2e.every-feature` | `--every-feature` |
| `-e2e.slots=N` | `--slots N` |
| `-e2e.slot=N` | `--slot N` |
| `-e2e.newest-base` | `--newest-base` |
| `-e2e.golden` | `--golden` |

Without `-e2e.only`, the runner chooses the sections `tests/e2e.sh` would, but only the Go ones among them, and `-e2e.only` refuses a section that's still `tests/e2e.sh`'s, naming the `tests/e2e.sh --only` command that runs it. `-e2e.reuse` is the Go runner's own: it keeps the VMs that a section makes with `Reuse` for the next run, which takes each one as it finds it when it's still running, so iterating on one check doesn't launch its VM again. The flags that `tests/e2e.sh` passes when it hands its sections over, `-e2e.lock-holder`, `-e2e.count`, `-e2e.total`, `-e2e.started` and `-e2e.summary`, are only for it.

The Go runner keeps everything that `tests/e2e.sh` does around its checks. It uses the same sandbox for each slot, with the same fixed ports, it refuses your real Lima home, and it builds devvm from the checkout into `src-go` in the sandbox's state folder, as a test build with its files embedded. It writes the same `config.toml` and puts the pinned tool lock in place, after the same check of the lock's first line, and it reads the pinned Ubuntu release from `e2e_ubuntu_release` in `tests/e2e.sh`, its one copy. It takes the slot's lock, a symlink to its pid, and it deletes only the VMs, files and project folders that it made. Before anything else, it names each host command that the chosen sections need and this machine lacks, with what installs it. Its lines are `tests/e2e.sh`'s: each section's `== name` heading, then `PASS`, `FAIL`, `SKIP` or `RETRY` with `[n/N]`, the check's title, its time, the run's time and the time left. A heartbeat every minute shows the newest line of the check's output or devvm's run log, and the run ends with the same summary line. It keeps each section's times in the same timings file as `tests/e2e.sh`, under the same tier, so either runner estimates from and compares with the other's times. Beside them it keeps a checksum of `tests/e2e`'s Go files, in place of the script's, so a section's check count is compared only with a run of the same Go files. With `-e2e.slots`, it plans, seeds and runs the slots as `tests/e2e.sh` does, each slot a run of the same test binary, and it adds up their results. On macOS it seeds each slot with APFS clones (`clonefile`), and on Linux with reflinks.

Each check's time limit is the one `tests/e2e.sh` gives it, and so is the limit of the steps between two checks. Each command a check runs is in a process group of its own, and when the check's time runs out, the group gets TERM, then KILL 30 seconds later. The check then fails with the heartbeat's last line, and the run goes on to the next check. The runner's own tests, the `TestHarness` tests, need no VM, and `mise run test` runs them with the `e2e` tag.

### Writing a check

A section is a function that takes the running section, `*S`, and calls `s.Check` once for each check:

```go
s.Check("gc deletes a tool image no VM uses, with its record", func() error {
    if Exists(old) || Exists(old+".json") {
        return errors.New("the old tool image or its record is still there")
    }
    return nil
})
```

The check passes when its function returns nil. Its title is its key in the timings, so keep it word for word when you port a check, and give a new check a title that says what it proves. When a check fails, the runner prints its error, each line led by `failed:`, and then the last 5 lines that its commands printed, or as many as `Tail(n)` asks for. A check that the run has nothing for is `s.Skip(title, why)`, which counts it, so that runs compare their counts.

The commands a check runs go through the section, so that they run in the sandbox's environment and within the check's time limit, and their output goes into the check's log:

| Helper | What it runs or reads |
| --- | --- |
| `s.Devvm(dir, args…)`, `s.DevvmNames(dir, args…)` | the run's devvm in `dir`, with no input; `DevvmNames` in names mode (`DEVVM_MESSAGES=names`) |
| `s.Msg(name, "k=v"…)` | a message's text, from `devvm _msg`, so a check never repeats its wording |
| `s.Limactl(args…)`, `s.InVM(vm, script)`, `s.Status(vm)` | `limactl` in the sandbox's Lima home, a script in the VM's login shell with no input, and a VM's status |
| `s.Run(Cmd{…})` | any command, with its own folder, input or variables |
| `s.NewProject(name)`, `s.Launch(dir, input, args…)`, `s.Reuse(name, input, args…)` | a new project folder, deleted at the end, and devvm launched in a folder, its VM recorded for the cleanup |
| `s.Paths.State(vm)`, `s.Paths.VMOf(dir)`, `s.Paths.VMDir(vm)` | a VM's `vm.json`, flattened as `tests/e2e.sh`'s `vm_state` does, the VM a folder has, and its folder |
| `s.Paths.KeysOf(image)`, `s.Paths.Record(key, field)`, `s.Own(image)` | an image's keys and records, and its images made the run's to delete |
| `s.Layer(name, script)`, `s.Paths` | a layer of the run's (`e2e*`), deleted at the end, and the sandbox's folders |
| `s.Golden(name, args…)` | with `-e2e.golden`, a [golden fixture](#golden-fixtures) captured from what `args` printed |
| `s.GoE2E(test, pkg, "VAR=value"…)` | a Go test in `internal/pkg` that needs a real VM, such as `TestE2EExecQuoting`, with its variables in its environment and never in a process's arguments |
| `s.Until(timeout, fn)`, `Settle` | a wait for what the check looks for, trying each second, and the one fixed wait, before looking for something that must not have happened |
| `s.Port(p)`, `s.Defer(fn)` | one of slot 0's fixed ports as this slot has it, and a cleanup of the section's own |

Three options change how a check runs. `Flaky(reason)` marks a check that's known to flake: when it fails, the runner shows its output and runs it once more, and only a second failure fails it. Its result says `retried once`, and its timings line and the summary say so too, and every other check fails on its first failure. `Builds()` marks a check whose own command can build base or a `--from` base, so it gets at least an hour. `Needs(backend.FeatureLink)` names the backend features that a check proves: on a backend that has one of them neither natively nor emulated and reviewed, as `devvm doctor --json` shows its features, the check is skipped and counted, so a check is written once for every backend that has its features.

### Porting a section

Each section is a file of its own in `tests/e2e`, such as `gc_test.go`, so sections can be ported at the same time without touching each other's files. To port one:

1. Write `<section>Section(s *S)` in `tests/e2e/<section>_test.go`, with the same checks in the same order as its block in `tests/e2e.sh`, each one's title word for word, and the same skips. Keep its setup steps between the checks, as the block has them, because a check's time counts from the end of the one before it.
2. Set its `run` in `sections` (`tests/e2e/sections_test.go`), and list there the host commands it needs beyond `limactl` and `pkill`, each with what installs it.
3. Run it with `mise run e2e:go -- -e2e.only=<section>` on a Mac and on a Linux host, and check that each check passes and that the counts and titles match the section's last run of `tests/e2e.sh`.
4. Delete its block from `tests/e2e.sh` and add its name to `go_sections` there, then run `tests/e2e.sh --only <section>` to check that the hand-over runs it.

A helper that more than one section needs goes in the harness's files, beside those above, so that every section shares one copy, and a fixture that two sections share, such as an image, stays in the sandbox's cache, as it does for `tests/e2e.sh`. These are the ones the sections share so far:

| Helper | What it does | `tests/e2e.sh`'s |
| --- | --- | --- |
| `s.Start(Cmd{…})`, `Proc` | runs a command in the background, past the check that starts it, in a process group of its own, until `Stop` or the run's end, with its output in `Out` and never in the check's log | `cmd &` |
| `StartCorp(s)`, `Corp` | starts a company proxy, a `devvm _proxy` on `127.0.0.1` at slot 0's port 18766 as the slot has it, with a password of its own, allowing any host, and waits for it to listen; `Env` gives a command `HTTPS_PROXY` and `HTTP_PROXY` with its password, `Saw` looks for one of its events, and `Addr` is its address without the password | `corp_start`, `corp_stop` |
| `WriteProxyConfig(s, file, proxy.Config{…})` | writes a config for `devvm _proxy -config`, mode 600, with the proxy's own `Config` type | `proxy_config` |
| `NewCA(s)`, `CA` | makes the run's throwaway CA once, with a server certificate for `host.lima.internal`, `localhost` and `127.0.0.1`; `Trust` and `Untrust` put it in devvm's CA folder and take the folder away, `Serve(port)` serves HTTPS with it on every address until `StopServing`, `Sign` makes another server certificate, and `GoneFrom(s, vm)` checks that a VM keeps none of devvm's CA files | `e2e_ca`, `ca_serve`, `ca_stop`, `ca_gone` |
| `NoLeak(secrets, paths…)` | fails when a regular file under the paths holds one of the secrets, reading only where each file has data, as `SEEK_DATA` and `SEEK_HOLE` find it, so a VM's sparse disk takes seconds, and naming the file but never the secret | `no_leak` |
| `GuestHasNone(s, vm, secrets, dirs…)` | fails when a file under the folders in the VM holds one of the secrets, searched as root, the secrets on `grep`'s input | `sudo grep -rqsFf /dev/stdin` |
| `WatchProcs(s)`, `ProcWatch`, `ProcsHave(s, secret)` | samples every host process's command line every 0.1 s until `Stop`, into a file of the sandbox's, and `Saw` looks for a secret in the samples, while `ProcsHave` looks in one sample taken now; a check that reads either is `ProcsFlaky()`, as a listing can catch a short-lived process mid-start | `ps_watch`, `ps_unwatch` |
| `s.Paths.ToolVersion(tool)` | the tool's version in the shared tools' lock | `lockv` |
| `s.Paths.RunLogHas(line)` | whether one of devvm's run logs has the line, whole: what a build's script printed | `grep -qx … logs/runs/*.log` |
| `MountLine(s, vm)` | the host folders a VM's Lima config mounts, each with a space after it | `limactl list --format '{{range .Config.Mounts}}…'` |
| `HasLine`, `CaptureLine`, `ExitStatus` | whether text has a line, whole; the first group of the first line a regular expression matches; the exit status a command's error says | `grep -qx`, `sed -n 's/…/\1/p'`, `$?` |
| `launchWith(s, dir, env, args…)`, `devvmCmd(s, dir, input, env, args…)` | `Launch` and devvm with variables of their own in devvm's environment, such as a fake token or a `HOME` of the section's, never in its arguments | `HOME=… "$D"` |
| `vmHas(s, vm, login, script, secrets…)`, `vmGrep(s, vm, script, re)` | whether what a script prints in the VM holds one of the secrets, or matches `re`, looked for in the VM with the secrets on its shell's input, so neither they nor what the script printed reach the check's log | `in_vm … \| grep -qF` |
| `limaListens(s, proto, port)`, `loopbackOnly(s, port)` | whether Lima's own processes listen on a port of this machine, and whether what listens on one listens on loopback alone, with `lsof` on macOS and `ss` on Linux | `lima_listens`, `loopback_only` |

A secret that a check looks for is never in a process's arguments, where other users can see it. The company proxy's password is in its config file and in the environment of the commands that `Env` is given to, and each helper above takes a secret as a Go value, so it reaches `grep` on its input and is never on a command line.

## Test builds

The tests run a test build of devvm, which is one made with `-ldflags "-X github.com/w3cj/devvm/internal/testbuild.testBuild=yes"`, as `tests/e2e.sh` and the CLI tests make theirs. Only a test build reads the environment variables that stand in for what devvm would find on this machine. Releases (GoReleaser, the Homebrew formula) don't set the flag, so a release ignores every one of these variables, whatever its environment says. devvm reads them in one place, `readOverrides` in `internal/cli/overrides.go`, into an `Overrides` struct, and passes each one to the package that uses it:

| Variable | Stands in for |
| --- | --- |
| `DEVVM_HOST_MISE` | devvm's pinned and checked mise on the host |
| `DEVVM_KVM` | Linux's `/dev/kvm` |
| `DEVVM_UBUNTU_URL` | `https://cloud-images.ubuntu.com/releases/resolute`, where devvm finds Ubuntu's dated releases |
| `DEVVM_UBUNTU_RELEASE` | the newest dated release, which devvm pins base to: `tests/e2e.sh` pins its own, `e2e_ubuntu_release`, so its images stay cached between runs, and devvm checks it as it does the newest |
| `DEVVM_UNREVIEWED` | the refusal of unreviewed emulations ([unreviewed emulations](#unreviewed-emulations)) |
| `DEVVM_TEST_CPUS`, `DEVVM_TEST_MEMORY_GIB`, `DEVVM_TEST_FREE_GIB`, `DEVVM_TEST_LIST_TIMEOUT` | this machine's size and how long devvm waits for a backend's listings ([a host of any size](#a-host-of-any-size)) |
| `DEVVM_PIPED_ANSWERS` | a terminal to answer questions in: with `1`, each question reads its answer as a line of input that isn't a terminal, without its prompt, where devvm would refuse it |

The settings that the [environment table](../using/settings.md#environment) lists, such as `DEVVM_LIMA_HOME`, `DEVVM_SECRET_SERVICE` and `DEVVM_NO_TOOL_CHECK`, are yours to set, so every build reads them.

## Tests on real Lima

Most Go tests run the `limactl` stub, which can't tell whether real Lima accepts what devvm gives it. The tests in `internal/backend/lima/real_test.go` run the real `limactl` instead, on dummy instances, as Lima's own tests do. A dummy instance's image is a small local file, so it can't boot, but `limactl` still creates, clones, edits, lists and deletes it. The tests run in a sandbox of their own, with a Lima home under `/tmp` and a `HOME` of their own, so they never touch your VMs or your Lima home, and they never start a VM. They run in `mise run test` wherever `limactl` is on `PATH`, and where it isn't, they skip and say why.

Each test walks through `scenario`, which makes every kind of Lima config devvm makes, through the same code devvm runs. It makes base from Ubuntu's image and from a `--from` conversion, a layer, the shared tools' build VM, the conversion's VM, and a project VM with its mounts, ports, size and environment. Then it makes the edits that a build's start makes, and those of the project VM's starts: its first start, a confined start with the proxy's password that changes its agents' folders, agent forwarding and size in the same edit, and an open start and one with no network. In the end it deletes everything. `TestLimaConfigsValid` runs `limactl validate` on every instance's `lima.yaml` after each step. That covers the proxy's password too, which devvm writes into `lima.yaml` itself, so `limactl edit` never checks it.

`TestLimactlStubContract` in `contract_test.go` keeps the `limactl` stub in `internal/stubtest/stub` honest, because the CLI tests and most Go tests trust it to act as Lima does. It runs `scenario` twice, once through the real `limactl` and once through the stub, and after each step it compares every instance as devvm reads it from `limactl list --json`. It compares the order of the instances, each one's status, size, mounts, agent forwarding, ssh port and networks, and the edits each kind of start would make to it. When Lima and the stub differ, the test names the step and the fields that differ, and the stub gets fixed to match Lima.

## Golden fixtures

The golden fixtures are what the real tools printed, kept in `testdata/golden/darwin/` and `testdata/golden/linux/`, so the tests can check both devvm's parsers and the [stubs](_index.md#stubs) against real output on any machine. Each system's folder holds what its tools printed there:

| Fixture | The real output of | Captured from |
| --- | --- | --- |
| `limactl-version.txt` | `limactl --version` | `TestLimactlStubContract`'s run on real Lima |
| `limactl-list-stopped.json` | `limactl list --json` | the contract test's dummy instances, after the step that leaves the most |
| `limactl-list-running.json` | `limactl list --json` of a running VM | the `project` section's first VM |
| `qemu-img-info.json` | `qemu-img info -U --output=json` of base's disk | the `images` section, on Linux |
| `qemu-img-info-overlay.json` | the same, of a project VM's overlay disk | the `project` section, on Linux |
| `security-missing.txt` | `security find-generic-password` for an item that isn't there, and its exit status | the `images` section, on macOS |
| `tmutil-isexcluded.txt` | `tmutil isexcluded` | the `images` section, on macOS |

`tests/e2e.sh --golden` captures them on a real machine, into the checkout, and you commit what changed. It runs the contract test with `STUB_GOLDEN` set to the system's folder for the dummy instances' fixtures, and runs the other commands itself, so a run needs the `images` section, and the `project` section for a running VM. Each fixture has this machine's home folder replaced with `/home/me`, the user's name with `me`, and the user's full name, the timezone and a VM's environment made up, so it holds nothing of yours. A capture isn't a check, so a run with `--golden` runs as many checks as one without, and a capture that fails says so and keeps the fixture it had. Run it on a Mac and on a [Linux host](#linux-hosts) after Lima or QEMU changes what it prints.

`TestGoldenLimactlVersion`, `TestGoldenLimactlList` and `TestGoldenQemuImgInfo` in `internal/backend/lima/golden_test.go` read every system's fixtures with devvm's own parsers, so a change in what the tools print that devvm would misread fails them on any machine. Each also runs the stub and checks that its output has the same form, or the same shape for JSON: every field the stub prints is one that the real tool prints, with a value of the same kind. For `limactl list --json`, the stub's output is its listing after the contract test's scenario. `TestGoldenSecurityTmutil` in `internal/platform` checks the stub's `security` and `tmutil` against macOS's fixtures the same way, on macOS.

## Unreviewed emulations

devvm refuses an emulated feature until its review passes, yet the review needs end-to-end checks of it. In a [test build](#test-builds), `DEVVM_UNREVIEWED` lets the ones it names through, as `backend:feature` pairs, comma-separated, each time with a warning (`backend_unreviewed_used`). A release ignores it:

```sh
DEVVM_UNREVIEWED=lima:ports-all devvm
```

It's for tests only, of an emulation before its review: none needs it now, as every emulation is reviewed. It never makes a feature a backend lacks usable, and `devvm doctor` still shows the feature `unreviewed`.

## A host of any size

devvm reads this machine's CPUs, its memory and the free space on its disks when it fits base to the host, checks a size you give it, checks free space before a build or a new VM, and warns in `devvm doctor`. All of these reads go through `platform.Host`, so the CLI tests can stand in for a host of any size and pass the same way on a large Mac and on a small CI runner. In a test build, three environment variables replace what `platform.Host` reads:

| Variable | Replaces |
| --- | --- |
| `DEVVM_TEST_CPUS` | the number of CPUs |
| `DEVVM_TEST_MEMORY_GIB` | the memory, in GiB |
| `DEVVM_TEST_FREE_GIB` | the free space in GiB, on every disk |

Only a [test build](#test-builds) reads them, and a release ignores all three, so no variable can make devvm see more free space or more memory than this machine has. Each one that isn't set leaves the machine's own value. `internal/cli/script_test.go` sets a large host in every script: 8 CPUs, 16 GiB of memory and 500 GiB free, which is more than base asks for and more than any build checks for. A script that tests a small host sets its own values with `env`, as `small_host.txtar` does with 2 CPUs, 7 GiB and 1 GiB free:

```txtar
env DEVVM_TEST_CPUS=2
env DEVVM_TEST_MEMORY_GIB=7
! exec devvm size -g --cpus 3
stderr '^devvm: too_many_cpus have=2 want=3$'
```

To go back to the large host later in the same script, set the large values again, because an empty value leaves the machine's own. The fake host covers only size and free space. What differs between operating systems, such as Lima's disk floors on Linux, ports already in use, Docker on `PATH`, or tmpfs and 9p mounts, still differs, and the Linux CI job is what catches it.

## Any time zone

The tests pass in any time zone and on any date, because a test that compares times counts them from an instant it parsed or fixed, or works in UTC, and never sets `time.Local`. A testscript that needs an old file uses a date far in the past, such as `touch -t 202001010000`, and one that needs a file that stays new uses a date far in the future, such as an export named `20991231-235959`, so the scripts keep passing as the calendar moves on. CI runs `mise run test` in UTC on macOS and Linux, and a third job runs it on Linux with `TZ=America/Los_Angeles`, which has daylight saving time, so a test that only passes in one zone fails in CI and not only on a laptop. To check a change the same way, run the tests with another zone, including one with an odd offset or a half-hour DST change:

```sh
TZ=America/Los_Angeles mise run test
TZ=Pacific/Chatham mise run test
TZ=Australia/Lord_Howe mise run test
```

## Fuzzing

`go test` runs only the seed inputs of each fuzz target, so the Fuzz workflow in `.github/workflows/fuzz.yml` fuzzes every target for two minutes each Monday, and you can start it by hand from the Actions tab. It finds the targets by reading each `func Fuzz` in the tests, so a new target is fuzzed without any change to the workflow, and the proxy's targets run first. When a target fails, the workflow uploads the input that broke it as an artifact named `crash-<target>`. Put that file in the package's `testdata/fuzz/<target>/` folder, fix the code, and commit both, because `go test` then runs the input as a seed every time. To fuzz one target on your machine, run `go test -run '^$' -fuzz '^FuzzParseRule$' -fuzztime 1m ./internal/proxy`.

## Manual checks

What `tests/e2e.sh` leaves out, as it would change something of yours or needs something it doesn't have:

- **Screen Sharing on `devvm desktop`:** e2e speaks VNC itself (`TestE2EDesktop`), because it can't drive a window. In a VM made with `--layers desktop`, run `devvm desktop`. Screen Sharing opens and asks for the password devvm shows, and the Xfce desktop answers the mouse and keys. A wrong password gets refused. With Edit > Use Shared Clipboard on, text you copy on this machine doesn't paste in the VM, and with `devvm desktop --clipboard` it does. `~/.local/share/devvm/logs/runs/` has no line saying the relay doesn't know a message (`which the relay doesn't know`).

- **A VM's clock after this machine sleeps:** e2e can't sleep a Mac, and hosted CI can't either. With an open VM and an allowlist VM running, compare each VM's `date +%s.%N` over ssh with this machine's clock, halfway through the ssh round trip. Then sleep the Mac for over 30 minutes and compare again right after it wakes and 2 minutes later. Each VM stays within about 0.1 s of this machine, the same as before the sleep, and the clock catches up without NTP: chrony in the allowlist VM has never synced, yet it stays right, while chrony in each VM logs `Forward time jump detected!` after the wake. Before you sleep the Mac, check that `pmset -g assertions` shows `PreventSystemSleep 0`. An app running `caffeinate -s` on AC power turns a sleep into a dark wake where the CPU keeps running, so the VMs never pause and the check proves nothing. In a real sleep, a measurement you run every few minutes in the background leaves a gap.

- **`--from` an image only Docker has** (built locally, never pushed): it would build an image in your Docker. Build a small one (`docker build -t devvm-local-test .` from a `Dockerfile` with `FROM alpine:3`), run `devvm --from devvm-local-test` in a new folder, check the VM boots from it, then `devvm rm` and `docker image rm devvm-local-test`.
