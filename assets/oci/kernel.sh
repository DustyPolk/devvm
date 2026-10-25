#!/bin/bash
# Turn a distro's vmlinuz into a kernel Lima can boot directly. Runs in the build VM (oci/build.sh).
# Usage: kernel.sh VMLINUZ OUT
# Virtualization.framework needs an uncompressed arm64 Image. Ubuntu ships it wrapped in an EFI
# executable with a zstd (or gzip) payload; try each candidate stream until one decodes to a
# kernel with the arm64 magic "ARMd" at offset 0x38. Other architectures use vmlinuz as-is (QEMU).
set -euo pipefail
in=$1 out=$2
is_arm64() { [[ "$(dd if="$out" bs=1 skip=56 count=4 2>/dev/null)" == ARMd ]]; }
cp "$in" "$out"   # already uncompressed on some distros
[[ "$(uname -m)" == aarch64 ]] || exit 0
is_arm64 || for fmt in "28b52ffd zstd -dc" "1f8b08 gunzip -c"; do
  read -r hex dec <<<"$fmt"
  pat=""; for ((j = 0; j < ${#hex}; j += 2)); do pat+="\\x${hex:j:2}"; done
  for off in $(LC_ALL=C grep -obUaP "$pat" "$in" | cut -d: -f1 | head -5); do
    # shellcheck disable=SC2086  # $dec is a command and its flag
    tail -c +$((off + 1)) "$in" | $dec > "$out" 2>/dev/null || true
    is_arm64 && break 2
  done
done
is_arm64 || { echo "could not extract an arm64 kernel from $in" >&2; exit 1; }
