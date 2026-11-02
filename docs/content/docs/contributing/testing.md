---
title: Testing on other hosts
weight: 1
---

For contributors. The everyday tests, CLI tests and end-to-end tests are in [Contributing](_index.md#end-to-end-tests); this page is about running them on a host other than your Mac.

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
| Tokens | the Keychain | files only you can read, in `~/.config/devvm/secrets/` |
| The CA | kept out of Time Machine | no backup to keep it out of |

`tests/e2e.sh` checks each side where it runs: on Linux, the `.git` protection checks become one that it's off and devvm says so, and the Keychain check is left out.

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

   Run it from your checkout of the repo, which the VM mounts at the same path. `--mount` alone keeps the template's home mount, hence the edit. Lima gives the VM's user your Mac's uid unless `user.uid` sets another, and a Mac's first user has uid 501. Most Linux hosts' first user has uid 1000, and so does the user that some container images ship, such as the `ubuntu` user of Ubuntu's images. With the VM's user at uid 1000, the `from` section of `tests/e2e.sh` meets that clash, as devvm does on most Linux hosts.

2. Inside it, install the packages, then Lima and mise from their release tarballs, each checked against its release's checksums (`SHA256SUMS`, `SHASUMS256.txt`) with `sha256sum -c`, and add yourself to the `kvm` and `docker` groups:

   ```sh
   sudo apt-get install -y qemu-system-arm qemu-utils qemu-efi-aarch64 virtiofsd docker.io docker-buildx build-essential git curl jq rsync
   sudo usermod -aG kvm,docker "$USER"
   ```

   Then restart the VM, so a fresh login has the groups.

3. Copy the repo into the VM's home, so the read-only mount stays untouched, install the pinned tools and run the gate, then the end-to-end tests:

   ```sh
   rsync -a --delete /path/to/your/checkout/ ~/devvm/ && cd ~/devvm
   mise trust && mise install
   mise run fmt && mise run lint && mise run test
   tests/e2e.sh --from
   ```

The first `--from` run builds every image and takes about half an hour on an M3 Max; later ones reuse them. Stop the VM when done (`limactl stop linuxhost`); it keeps its images for the next run.

Gotchas of this setup:

- The Linux host's home and its VMs' homes are both `/home/<user>.guest`, so a file under `$HOME` in a VM isn't proof of a mount: `tests/e2e.sh` checks the VM's mounts list instead.
- A VM booting while another one works boots degraded and its downloads time out: `tests/e2e.sh` stops a VM it's done with before starting the next.
- minisign, for signing releases, has no linux/arm64 build, so mise skips it there.

## Docker engines

`tests/e2e.sh --backend docker` checks the [Docker backend](../backends/docker.md) on two engines, which differ in what a container reaches on the host:

- **A Linux engine**, in a Lima VM e2e makes the first time and keeps: Lima home `~/.lt-e2e-go-docker` (or `$DEVVM_E2E_DOCKER_LIMA_HOME`), VM `dockerhost`, Ubuntu 26.04 with Docker's engine from Ubuntu's archive and you in its `docker` group, and only this checkout mounted, read-only. e2e builds a test build of `devvm` for it here, copies it in, and runs its own Docker checks there (`DEVVM_E2E_BIN` names the binary, as the VM has no Go). Its images stay cached in the VM's Docker; `--clean` deletes them. `limactl stop dockerhost` (with that `LIMA_HOME`) when done; `limactl delete` to start over. The checkout's path is the VM's mount: run e2e from the same one.
- **Docker Desktop**, yours: e2e touches only what has its sandbox's label, checks that everything else is the same at the end, and deletes everything with the label then. The images bases start from, which devvm pulls by digest (Ubuntu's pinned one, `alpine:3`'s), are the exception: Docker keeps them, as it does any image devvm pulls. It builds its own images, through a proxy of devvm's, with devvm's Linux build beside the test build, as a macOS release has it. Network policy's DNS check captures DNS on the engine's own network: a container of the sandbox's, from `alpine:3`, on Desktop's host network (its VM's), `tcpdump` added from Alpine's packages, deleted at the end of the section.

On Linux, `--backend docker` checks the machine's own engine, which must be rootful and its own.

The CLI tests stand in a stub for Docker's engine (`testdata/stubs/docker`). A test build reads `DEVVM_DOCKER_KIND` (`linux`, `desktop`, `other`) as the engine's kind, so they build on a stand-in Linux engine on any host; releases ignore it.

## x86_64 hosts

These need real x86_64 hardware. An emulated x86_64 VM on a Mac (`--arch=x86_64`, QEMU TCG; Lima needs its `lima-additional-guestagents`) runs the unit tests, but VMs nested in it crash in their firmware.

## Test builds

The tests run a test build of devvm, which is one made with `-ldflags "-X github.com/w3cj/devvm/internal/testbuild.testBuild=yes"`, as `tests/e2e.sh` and the CLI tests make theirs. Only a test build reads the environment variables that stand in for what devvm would find on this machine. Releases (GoReleaser, the Homebrew formula) don't set the flag, so a release ignores every one of these variables, whatever its environment says. devvm reads them in one place, `readOverrides` in `internal/cli/overrides.go`, into an `Overrides` struct, and passes each one to the package that uses it:

| Variable | Stands in for |
| --- | --- |
| `DEVVM_HOST_MISE` | devvm's pinned and checked mise on the host |
| `DEVVM_KVM` | Linux's `/dev/kvm` |
| `DEVVM_UBUNTU_URL` | `https://cloud-images.ubuntu.com/releases/resolute`, where devvm finds Ubuntu's pinned dated release |
| `DEVVM_UNREVIEWED` | the refusal of unreviewed emulations ([unreviewed emulations](#unreviewed-emulations)) |
| `DEVVM_DOCKER_KIND`, `DEVVM_DOCKER_PROC_SYS` | the kind of Docker's engine and its host's `/proc/sys` |
| `DEVVM_LINUX_BUILD_URL`, `DEVVM_LINUX_BUILD_SHA256` | a release's Linux archive and its devvm's SHA-256 |
| `DEVVM_TEST_CPUS`, `DEVVM_TEST_MEMORY_GIB`, `DEVVM_TEST_FREE_GIB`, `DEVVM_TEST_LIST_TIMEOUT` | this machine's size and how long devvm waits for a backend's listings ([a host of any size](#a-host-of-any-size)) |

The settings that the [environment table](../using/settings.md#environment) lists, such as `DEVVM_LIMA_HOME`, `DEVVM_SECRET_SERVICE` and `DEVVM_NO_TOOL_CHECK`, are yours to set, so every build reads them.

## Unreviewed emulations

devvm refuses an emulated feature until its review passes, yet the review needs end-to-end checks of it. In a [test build](#test-builds), `DEVVM_UNREVIEWED` lets the ones it names through, as `backend:feature` pairs, comma-separated, each time with a warning (`backend_unreviewed_used`). A release ignores it:

```sh
DEVVM_UNREVIEWED=smolvm:ports-all devvm --backend smolvm base
```

It's for tests only, of an emulation before its review: none needs it now, as every emulation is reviewed. It never makes a feature a backend lacks usable, and `devvm doctor` still shows the feature `unreviewed`.

A test build also reads `DEVVM_LINUX_BUILD_URL` and `DEVVM_LINUX_BUILD_SHA256` in place of a release's Linux archive and its devvm's SHA-256, and then fetches devvm's [Linux build](../backends/docker.md#requirements) as a release on macOS does, whatever the host: the CLI tests serve one on a loopback port, with a matching SHA-256 and a mismatched one.

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

- **smolvm's DNS, from outside:** e2e watches smolvm's own decision log and this machine's flows, not the network. With a domain whose authoritative nameserver's queries you can watch (a canary zone), resolve a new name under it from a smolvm VM without network policy (`getent hosts <new>.canary.example`): the nameserver sees it. Then from one with network policy, through its resolver and straight to `1.1.1.1`, a new name under the zone, also as TXT: the nameserver sees none of them.

- **Screen Sharing on `devvm desktop`:** e2e speaks VNC itself (`TestE2EDesktop`), because it can't drive a window. In a VM whose base has the `desktop` feature, run `devvm desktop`. Screen Sharing opens and asks for the password devvm shows, and the Xfce desktop answers the mouse and keys. A wrong password gets refused. With Edit > Use Shared Clipboard on, text you copy on this machine doesn't paste in the VM, and with `devvm desktop --clipboard` it does. `~/.local/share/devvm/logs/runs/` has no line saying the relay doesn't know a message (`which the relay doesn't know`).

- **A VM's clock after this machine sleeps:** e2e can't sleep a Mac, and hosted CI can't either. With a Lima VM without network policy, a Lima VM with network policy and a smolvm VM running, compare each VM's `date +%s.%N` over ssh with this machine's clock, halfway through the ssh round trip. Then sleep the Mac for over 30 minutes and compare again right after it wakes and 2 minutes later. Each VM stays within about 0.1 s of this machine, the same as before the sleep, and the clock catches up without NTP: chrony in a VM with network policy has never synced, and smolvm runs no time sync, yet both stay right, while chrony in Lima's VMs logs `Forward time jump detected!` after the wake. Before you sleep the Mac, check that `pmset -g assertions` shows `PreventSystemSleep 0`. An app running `caffeinate -s` on AC power turns a sleep into a dark wake where the CPU keeps running, so the VMs never pause and the check proves nothing. In a real sleep, a measurement you run every few minutes in the background leaves a gap.

- **`--from` an image only Docker has** (built locally, never pushed): it would build an image in your Docker. Build a small one (`docker build -t devvm-local-test .` from a `Dockerfile` with `FROM alpine:3`), run `devvm --from devvm-local-test` in a new folder, check the VM boots from it, then `devvm rm` and `docker image rm devvm-local-test`.
