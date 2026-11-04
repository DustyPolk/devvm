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
| Network policy | the VM's Lima process under a macOS sandbox | QEMU's `restrict=on` and a `guestfwd` to the proxy |
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

These need real x86_64 hardware. An emulated x86_64 VM on a Mac (`--arch=x86_64`, QEMU TCG; Lima needs its `lima-additional-guestagents`) runs the unit tests, but VMs nested in it crash in their firmware.

## Slots for each host

`tests/e2e.sh --slots N` runs the end-to-end sections in N sandboxes at once ([slots](_index.md#slots)), and each slot runs its own VMs, up to three at a time in the link section. How many slots a host can run depends on its cores and memory:

| Host | Slots |
| --- | --- |
| A bare-metal Linux host with 32 cores and 124 GB of memory | 3 or 4 |
| A Mac | 2 |
| A Linux host nested in a VM, such as [the Linux host above](#linux-hosts) | 1, the default: a VM booting while another one works boots degraded there |

A Mac clones the images it gives each slot, because `/tmp` and your home folder are on the same APFS volume. A Linux host reflinks the Lima instances in your home folder when its file system can, as XFS made with `reflink=1` and Btrfs can, but a `/tmp` on tmpfs can't reflink, so there each slot other than slot 0 builds its own tool images. Every slot keeps its images between runs, so only its first run builds them.

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

The settings that the [environment table](../using/settings.md#environment) lists, such as `DEVVM_LIMA_HOME`, `DEVVM_SECRET_SERVICE` and `DEVVM_NO_TOOL_CHECK`, are yours to set, so every build reads them.

## Tests on real Lima

Most Go tests run the `limactl` stub, which can't tell whether real Lima accepts what devvm gives it. The tests in `internal/backend/lima/real_test.go` run the real `limactl` instead, on dummy instances, as Lima's own tests do. A dummy instance's image is a small local file, so it can't boot, but `limactl` still creates, clones, edits, lists and deletes it. The tests run in a sandbox of their own, with a Lima home under `/tmp` and a `HOME` of their own, so they never touch your VMs or your Lima home, and they never start a VM. They run in `mise run test` wherever `limactl` is on `PATH`, and where it isn't, they skip and say why.

Each test walks through `scenario`, which makes every kind of Lima config devvm makes, through the same code devvm runs. It makes base from Ubuntu's image and from a `--from` conversion, a layer, the shared tools' build VM, the conversion's VM, and a project VM with its mounts, ports, size and environment. Then it makes the edits that a build's start makes, and those of the project VM's starts: its first start, a start with network policy and the proxy's password that changes its agents' folders, agent forwarding and size in the same edit, and starts without network policy and with no network. In the end it deletes everything. `TestLimaConfigsValid` runs `limactl validate` on every instance's `lima.yaml` after each step. That covers the proxy's password too, which devvm writes into `lima.yaml` itself, so `limactl edit` never checks it.

`TestLimactlStubContract` in `contract_test.go` keeps the stub in `testdata/stubs/limactl` honest, because the CLI tests and most Go tests trust it to act as Lima does. It runs `scenario` twice, once through the real `limactl` and once through the stub, and after each step it compares every instance as devvm reads it from `limactl list --json`. It compares the order of the instances, each one's status, size, mounts, agent forwarding, ssh port and networks, and the edits each kind of start would make to it. When Lima and the stub differ, the test names the step and the fields that differ, and the stub gets fixed to match Lima.

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

## Manual checks

What `tests/e2e.sh` leaves out, as it would change something of yours or needs something it doesn't have:

- **Screen Sharing on `devvm desktop`:** e2e speaks VNC itself (`TestE2EDesktop`), because it can't drive a window. In a VM made with `--layers desktop`, run `devvm desktop`. Screen Sharing opens and asks for the password devvm shows, and the Xfce desktop answers the mouse and keys. A wrong password gets refused. With Edit > Use Shared Clipboard on, text you copy on this machine doesn't paste in the VM, and with `devvm desktop --clipboard` it does. `~/.local/share/devvm/logs/runs/` has no line saying the relay doesn't know a message (`which the relay doesn't know`).

- **A VM's clock after this machine sleeps:** e2e can't sleep a Mac, and hosted CI can't either. With a VM without network policy and a VM with network policy running, compare each VM's `date +%s.%N` over ssh with this machine's clock, halfway through the ssh round trip. Then sleep the Mac for over 30 minutes and compare again right after it wakes and 2 minutes later. Each VM stays within about 0.1 s of this machine, the same as before the sleep, and the clock catches up without NTP: chrony in a VM with network policy has never synced, yet it stays right, while chrony in each VM logs `Forward time jump detected!` after the wake. Before you sleep the Mac, check that `pmset -g assertions` shows `PreventSystemSleep 0`. An app running `caffeinate -s` on AC power turns a sleep into a dark wake where the CPU keeps running, so the VMs never pause and the check proves nothing. In a real sleep, a measurement you run every few minutes in the background leaves a gap.

- **`--from` an image only Docker has** (built locally, never pushed): it would build an image in your Docker. Build a small one (`docker build -t devvm-local-test .` from a `Dockerfile` with `FROM alpine:3`), run `devvm --from devvm-local-test` in a new folder, check the VM boots from it, then `devvm rm` and `docker image rm devvm-local-test`.
