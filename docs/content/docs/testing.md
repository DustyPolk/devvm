---
title: Testing on other hosts
weight: 101
---

For contributors. The everyday tests, CLI tests and end-to-end tests are in [Contributing](contributing.md#end-to-end-tests); this page is about running them on a host other than your Mac.

## What's platform-specific

The same code and tests run on macOS and Linux; what differs is behind build tags in `internal/platform` and the backend's own checks:

| | macOS | Linux |
| --- | --- | --- |
| VMs | Lima on Apple's Virtualization framework (`vz`) | Lima on QEMU with KVM |
| Mounts | virtiofs | 9p, one mount per share per guest |
| Project VM disks | APFS copy-on-write clones of the image's | qcow2 overlays of the image's |
| Network policy | the VM's Lima process under a macOS sandbox | QEMU's `restrict=on` and a `guestfwd` to the proxy |
| `.git` protection | locked with `chflags` | none: devvm says so ([details](git-protection.md#linux-hosts)) |
| Set-id files | dropped by virtiofs | reach the host: new VMs check for them |
| Tokens | the Keychain | files only you can read, in `~/.config/devvm/secrets/` |
| The CA | kept out of Time Machine | no backup to keep it out of |

`tests/e2e.sh` checks each side where it runs: on Linux, the `.git` protection checks become one that it's off and devvm says so, and the Keychain check is left out.

## Linux hosts

Test in a Lima VM with nested virtualization, which needs an M3 or newer and macOS 15 or newer. Give it a Lima home of its own, so its VMs stay apart from yours:

1. Make it from Lima's Ubuntu template, with nested virtualization, room for nested VMs, and only the repo mounted, read-only:

   ```sh
   cd /tmp
   export LIMA_HOME=~/.lt-linux-go
   limactl create --name=linuxhost --tty=false --vm-type=vz --cpus=8 --memory=16 --disk=150 \
     --containerd=none --set '.nestedVirtualization=true' --mount ~/dev/devvm-go template:ubuntu-24.04
   limactl edit --tty=false --set '.mounts=[{"location":"'"$HOME"'/dev/devvm-go","writable":false}]' linuxhost
   limactl start --tty=false linuxhost
   ```

   `--mount` alone keeps the template's home mount, hence the edit.

2. Inside it, install the packages, then Lima and mise from their release tarballs, each checked against its release's checksums (`SHA256SUMS`, `SHASUMS256.txt`) with `sha256sum -c`, and add yourself to the `kvm` and `docker` groups:

   ```sh
   sudo apt-get install -y qemu-system-arm qemu-utils qemu-efi-aarch64 virtiofsd docker.io docker-buildx build-essential git curl jq rsync
   sudo usermod -aG kvm,docker "$USER"
   ```

   Then restart the VM, so a fresh login has the groups.

3. Copy the repo into the VM's home, so the read-only mount stays untouched, install the pinned tools and run the gate, then the end-to-end tests:

   ```sh
   rsync -a --delete --exclude=.dex ~/dev/devvm-go/ ~/devvm/ && cd ~/devvm
   mise trust && mise install
   mise run fmt && mise run lint && mise run test
   tests/e2e.sh --from
   ```

The first `--from` run builds every image and takes about half an hour on an M3 Max; later ones reuse them. Stop the VM when done (`limactl stop linuxhost`); it keeps its images for the next run.

Gotchas of this setup:

- The Linux host's home and its VMs' homes are both `/home/<user>.guest`, so a file under `$HOME` in a VM isn't proof of a mount: `tests/e2e.sh` checks the VM's mounts list instead.
- A VM booting while another one works boots degraded and its downloads time out: `tests/e2e.sh` stops a VM it's done with before starting the next.
- minisign, for signing releases, has no linux/arm64 build, so mise skips it there.

## x86_64 hosts

These need real x86_64 hardware. An emulated x86_64 VM on a Mac (`--arch=x86_64`, QEMU TCG; Lima needs its `lima-additional-guestagents`) runs the unit tests, but VMs nested in it crash in their firmware.

## Unreviewed emulations

devvm refuses an emulated feature until its review passes, yet the review needs end-to-end checks of it. In a test build, `DEVVM_UNREVIEWED` lets the ones it names through, as `backend:feature` pairs, comma-separated, each time with a warning (`backend_unreviewed_used`). A test build is one made with `-ldflags "-X github.com/w3cj/devvm/internal/backend/backends.unreviewed=allowed"`, as `tests/e2e.sh` and the CLI tests make theirs; releases (GoReleaser, the Homebrew formula) set only the version, so they ignore it:

```sh
DEVVM_UNREVIEWED=smolvm:ssh devvm --backend smolvm --ports none base
```

It's for tests only, of an emulation before its review: none needs it now (smolvm's network policy and ssh for editors were checked this way, then reviewed). It never makes a feature a backend lacks usable, and `devvm doctor` still shows the feature `unreviewed`.

## Manual checks

What `tests/e2e.sh` leaves out, as it would change something of yours or needs something it doesn't have:

- **smolvm's DNS, from outside:** e2e watches smolvm's own decision log and this machine's flows, not the network. With a domain whose authoritative nameserver's queries you can watch (a canary zone), resolve a new name under it from a smolvm VM without network policy (`getent hosts <new>.canary.example`): the nameserver sees it. Then from one with network policy, through its resolver and straight to `1.1.1.1`, a new name under the zone, also as TXT: the nameserver sees none of them.

- **`--from` an image only Docker has** (built locally, never pushed): it would build an image in your Docker. Build a small one (`docker build -t devvm-local-test .` from a `Dockerfile` with `FROM alpine:3`), run `devvm --from devvm-local-test` in a new folder, check the VM boots from it, then `devvm rm` and `docker image rm devvm-local-test`.
