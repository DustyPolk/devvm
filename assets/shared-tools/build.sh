#!/bin/bash
# Builds shared tool images in a throwaway VM (a clone of base) that has an output folder of its own
# mounted writable at /mnt/devvm-tools (never the share itself), with mise.toml and the host's lock
# (~/.local/share/devvm/mise.lock) in its .build folder.
#   build.sh TOOL VERSION FILE [TOOL VERSION FILE ...]
# Each tool is installed at exactly the locked version (checked against the lock's checksums), then
# packed into one compressed read-only EROFS image, FILE in that folder; devvm moves it into the share
# and records its hash. Project VMs mount it at /opt/devvm/tools/<tool>/<version>, where mise finds it
# as a shared install.
# shellcheck disable=SC2034  # the checksums are read through sha()
set -euxo pipefail
# Pinned downloads (scripts/bump.sh updates them). rustup checks the toolchain against Rust's signed
# channel manifest.
rustup_version=1.29.1
rustup_sha256_arm64=15f6e4ce9f583b929c996c91562bad6d4454f3281de858b02cdfdef615fac433
rustup_sha256_amd64=dda7234360b7f578ca8b0ddcb80145646fa61a67c1720a5abc7051b35c9fcb71
rustup_sha256_arm64_musl=1ddf36182ac5d1782dbeefcb9bea7f5f9412d88f4a0d5047b1564a223624e8c6
rustup_sha256_amd64_musl=331228566cc931f32cd684f9bebc5956a5e5d7394c8d5de595f196a658ab98e6

share=/mnt/devvm-tools
# Where VMs mount the tools: installs record absolute paths (mise's bin links, scripts' shebangs).
work=/opt/devvm/tools
case $(uname -m) in aarch64) a=arm64 ;; x86_64) a=amd64 ;; esac
# On Alpine (musl), the musl builds: mise picks them, and rustup gets the musl toolchain.
libc=gnu musl=""
if command -v apk >/dev/null; then libc=musl musl=_musl; fi
triple=$(uname -m)-unknown-linux-$libc
sha() { local v="${1}_sha256_$a$musl"; echo "${!v}"; }
# The output folder, once it's there: Lima mounts it at boot (cloud-init, nofail), so a mount that
# failed or came late leaves an empty folder. Waits up to 60 s for the lock devvm put in it,
# mounting it again from fstab while it isn't mounted, then fails naming it and mount's last error.
wait_share() {
  local i err=""
  for (( i = 0; i < 60; i++ )); do
    [[ -f $share/.build/mise.lock ]] && return 0
    mountpoint -q "$share" || err=$(sudo mount "$share" 2>&1) || true
    sleep 1
  done
  echo "build.sh: devvm's output folder isn't mounted at $share (no .build/mise.lock after 60 s)${err:+: $err}" >&2
  return 1
}
wait_share
sudo rm -rf "$work"; sudo mkdir -p "$work"; sudo chown "$USER" "$work"
cd "$share/.build"
export MISE_LOCKED=1 MISE_TRUSTED_CONFIG_PATHS="$share/.build" MISE_YES=1

while (( $# >= 3 )); do
  tool=$1 version=$2 file=$3; shift 3
  if [[ $tool == rust ]]; then
    # The toolchain as rustup lays it out, plus rustup itself for the VM to install (it links the
    # toolchain in). rust-src and rust-analyzer now: components can't be added to a read-only toolchain.
    dir=$work/rust/$version; mkdir -p "$dir"
    devvm-fetch "https://static.rust-lang.org/rustup/archive/$rustup_version/$triple/rustup-init" \
      "$(sha rustup)" "$dir/rustup-init"
    chmod 755 "$dir/rustup-init"
    RUSTUP_HOME=$work/rustup CARGO_HOME=$work/cargo "$dir/rustup-init" -y -q --no-modify-path \
      --profile default --default-toolchain "$version" -c rust-src -c rust-analyzer
    mv "$work/rustup/toolchains/$version-$triple" "$dir/toolchain"
  else
    # By name: mise resolves mise.toml's version through the lock (an explicit tool@version isn't
    # matched to the lock's entry).
    mise install --shared "$work" "$tool"
    dir=$work/$tool/$version
    [[ -d $dir ]] || { echo "build.sh: mise installed another $tool version than $version: $(ls "$work/$tool")" >&2; exit 1; }
    # Corepack writes its shims next to node: now, since VMs can't.
    [[ $tool != node ]] || PATH=$dir/bin:$PATH corepack enable yarn
  fi
  # --all-root: the VM's user only reads them. lz4hc: about half the size, fast to read.
  mkfs.erofs -zlz4hc --all-root "$share/$file.tmp" "$dir" >/dev/null
  mv "$share/$file.tmp" "$share/$file"
done
sudo rm -rf "$work"
