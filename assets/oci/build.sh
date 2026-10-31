#!/bin/bash
# Turns a container image into the three files Lima boots directly, in a throwaway VM booted from
# Ubuntu's cloud image, as root. No Docker: crane pulls the image and flattens its
# layers, bootify.sh adds what a container lacks in a chroot, and mke2fs writes the disk.
#   build.sh OCI_DIR OUT IMAGE NAME USER_NAME USER_UID USER_HOME SIZE [CA_SH]
# IMAGE is a registry reference, pulled for this VM's architecture, or - for OUT/image.tar (from
# `docker save`, for an image only Docker has); NAME is what the user called it, for messages.
# CA_SH is devvm's guest/ca.sh when this VM trusts the user's CAs: the image's chroot trusts them
# too while bootify.sh runs, and keeps none of them.
# Writes, in OUT:
#   disk.raw  ext4 filesystem of the image's root (the whole disk, no partition table)
#   kernel    uncompressed kernel (Virtualization.framework won't take a compressed one)
#   initrd    the image's initramfs
#   cmdline   the kernel command line bootify.sh chose
#   libc      gnu, or musl (Alpine): which builds of the shared tools its VMs run
# No apt here: the cloud image's first boot may be running it (unattended-upgrades holds its lock).
# shellcheck disable=SC2034  # the checksums are read through crane_sha
set -euo pipefail
# Pinned downloads (scripts/bump.sh updates them).
crane_version=0.22.1
crane_sha256_arm64=898c0cff975f898a33e8c4580bdafb0e7c02c7faa33374e946762f97c4ab7110
crane_sha256_amd64=0ab7a1d6932a213aed964ce97666c3077fe691c8606413674a8b3e0b9ec4cda0

die() { echo "devvm: $*" >&2; exit 1; }
arch() { case $(uname -m) in aarch64|arm64) echo arm64 ;; x86_64) echo amd64 ;; *) die "unsupported architecture $(uname -m)" ;; esac; }

# crane, checked against its pinned checksum, into ./crane.
fetch_crane() {
  local a f sha
  a=$(arch); f=$a; [[ $a != amd64 ]] || f=x86_64
  sha=crane_sha256_$a
  curl -fsSL --retry 3 -o crane.tar.gz \
    "https://github.com/google/go-containerregistry/releases/download/v$crane_version/go-containerregistry_Linux_$f.tar.gz"
  echo "${!sha}  crane.tar.gz" | sha256sum -c --quiet - >/dev/null || die "crane's download doesn't match its pinned checksum"
  tar -xzf crane.tar.gz crane
  rm crane.tar.gz
}

# export_image IMAGE TAR: the image's flattened filesystem (layers applied, deletions included).
export_image() {
  if [[ $1 == - ]]; then ./crane export - "$2" < "$out/image.tar"; return; fi
  ./crane export --platform "linux/$(arch)" "$1" "$2" 2>crane.err && return 0
  cat crane.err >&2
  if grep -q 'no child with platform' crane.err; then
    die "'$name' has no linux/$(arch) build (it has: $(platforms "$1"))"
  fi
  die "couldn't download '$name'"
}
# platforms IMAGE: the architectures an image's index lists (attestations' "unknown" left out).
platforms() {
  ./crane manifest "$1" 2>/dev/null | grep -oE '"architecture" *: *"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/' |
    grep -vx unknown | sort -u | paste -sd' ' - || true
}

# libc_for "ID ID_LIKE": gnu or musl for a distribution /etc/os-release names; fails for the rest.
libc_for() {
  case " $1 " in
    *" debian "*|*" ubuntu "*) echo gnu ;;
    *" alpine "*)              echo musl ;;
    *) return 1 ;;
  esac
}

# chroot_run ROOT CMD...: run CMD in ROOT as root, with /proc, /sys, /dev and the VM's DNS, and
# without starting services (Debian's policy-rc.d). Everything is undone afterwards.
chroot_run() {
  local r=$1 m; shift
  mkdir -p "$r/proc" "$r/sys" "$r/dev" "$r/tmp" "$r/etc"
  chmod 1777 "$r/tmp"
  mount -t proc proc "$r/proc"
  mount --bind /sys "$r/sys"
  mount --bind /dev "$r/dev"
  mkdir -p "$r/dev/pts"; mount --bind /dev/pts "$r/dev/pts"
  rm -f "$r/etc/resolv.conf"
  cp -L "$(if [[ -f /run/systemd/resolve/resolv.conf ]]; then echo /run/systemd/resolve/resolv.conf; else echo /etc/resolv.conf; fi)" "$r/etc/resolv.conf"
  # An image's own policy-rc.d (Debian's Docker images have one) stays as it was.
  local policy=""
  if [[ -x $r/usr/bin/dpkg && ! -e $r/usr/sbin/policy-rc.d ]]; then
    policy=$r/usr/sbin/policy-rc.d
    mkdir -p "$r/usr/sbin"; printf '#!/bin/sh\nexit 101\n' > "$policy"; chmod 755 "$policy"
  fi
  # Of this script's environment, only the build's proxy, which devvm made for this build alone
  # (never the company proxy's own settings), for the downloads behind a company proxy.
  local rc=0 v proxy=()
  for v in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
    [[ -z ${!v:-} ]] || proxy+=("$v=${!v}")
  done
  chroot "$r" /usr/bin/env -i PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin HOME=/root \
    LC_ALL=C.UTF-8 ${proxy[@]+"${proxy[@]}"} "$@" || rc=$?
  [[ -z $policy ]] || rm -f "$policy"
  for m in dev/pts dev sys proc; do umount -l "$r/$m"; done
  return $rc
}

# make_disk ROOT: the kernel, initrd, command line and disk, in $out.
make_disk() {
  local r=$1 k i
  k=$(find "$r/boot" -maxdepth 1 -name 'vmlinuz-*' -printf '%f\n' 2>/dev/null | sort -V | tail -1)
  i=$(find "$r/boot" -maxdepth 1 \( -name 'initrd.img-*' -o -name 'initramfs-*' \) -printf '%f\n' 2>/dev/null | sort -V | tail -1)
  [[ -n $k && -n $i ]] || die "bootify.sh left no kernel or initrd in /boot"
  bash "$oci/kernel.sh" "$r/boot/$k" "$out/kernel"
  cp "$r/boot/$i" "$out/initrd"
  cp "$r/etc/devvm/cmdline" "$out/cmdline"
  # Container leftovers: /.dockerenv makes systemd think it runs in a container (cloud-init then
  # skips Lima's cidata disk), and hostname, hosts and resolv.conf are placeholders (or the builder's).
  rm -f "$r/.dockerenv"
  # What a container runtime adds to /dev (and `docker export` kept): without /dev/console, Debian
  # 12's initramfs drops to its shell after mounting the root.
  mkdir -p "$r/dev/pts" "$r/dev/shm"
  [[ -e $r/dev/console ]] || mknod -m 600 "$r/dev/console" c 5 1
  echo localhost > "$r/etc/hostname"
  printf '127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n' > "$r/etc/hosts"
  ln -sf ../run/systemd/resolve/stub-resolv.conf "$r/etc/resolv.conf"
  rm -f "$out/disk.raw"
  truncate -s "$size" "$out/disk.raw"
  mke2fs -q -t ext4 -L devvm-root -d "$r" "$out/disk.raw"
}

main() {
  oci=$1 out=$2 image=$3 name=$4 user_name=$5 user_uid=$6 user_home=$7 size=$8 ca_sh=${9:-}
  local work=/var/tmp/devvm-oci os libc
  for c in curl sha256sum tar chroot mount mke2fs truncate; do
    command -v "$c" >/dev/null || die "the build VM has no $c"
  done
  rm -rf "$work"; mkdir -p "$work/root"; cd "$work"
  echo "==> downloading $name"
  fetch_crane
  export_image "$image" rootfs.tar
  # On this VM's own disk, as root: Linux names, case and ownership survive.
  tar -xpf rootfs.tar -C root --numeric-owner
  rm rootfs.tar
  # The image's own shell, with nothing of this script's environment.
  os=$(env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin chroot root /bin/sh -c '. /etc/os-release && echo "$ID ${ID_LIKE:-}"' 2>/dev/null) ||
    die "'$name' has no shell or /etc/os-release, so it can't become a VM"
  libc=$(libc_for "$os") || die "'$name' is based on '${os%% *}'; devvm needs a Debian, Ubuntu or Alpine based image"
  echo "==> making $name bootable"
  mkdir -p root/tmp/devvm; cp "$oci/bootify.sh" root/tmp/devvm/
  # The user's CAs, which this VM trusts, for bootify.sh's downloads, then gone from the image again.
  if [[ -n $ca_sh ]]; then
    mkdir -p root/tmp/devvm/ca
    cp /usr/local/share/ca-certificates/devvm-*.crt root/tmp/devvm/ca/
    cp "$ca_sh" root/tmp/devvm/ca.sh
    chroot_run root sh /tmp/devvm/ca.sh /tmp/devvm/ca
  fi
  chroot_run root USER_NAME="$user_name" USER_UID="$user_uid" USER_HOME="$user_home" sh /tmp/devvm/bootify.sh
  [[ -z $ca_sh ]] || chroot_run root sh /tmp/devvm/ca.sh
  rm -rf root/tmp/devvm
  echo "==> writing disk, kernel and initrd"
  make_disk root
  echo "$libc" > "$out/libc"
  cd /; rm -rf "$work"
}

# Sourced by the unit tests for its functions; run with arguments in the build VM.
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
