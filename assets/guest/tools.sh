#!/bin/bash
# Installed in project VMs as /usr/local/sbin/devvm-tools, run as root at boot (devvm-tools.service)
# and by guest/sync.sh on every launch. Mounts the VM's shared tool images, read-only, from the share
# at /mnt/devvm-tools: each line of /etc/devvm/tools is "FILE TOOL VERSION", mounted at
# /opt/devvm/tools/TOOL/VERSION, where mise finds it as a shared install. A FILE that's a folder in
# the share (unpacked, on Docker) is linked there instead. Tool images no longer listed are
# unmounted, or unlinked. The tool folders themselves stay writable, so mise can keep its version
# links (latest, 24, lts, ...) there.
# Every listed image is tried. One that's missing from the share, or can't be mounted or linked, is
# left out (its empty mount point too, so mise doesn't take it for an install) and said on stderr.
# At the end they're listed, a FILE per line, in /run/devvm/tools-unmounted, which devvm reads when
# the setup fails, and the script fails.
set -euo pipefail
share=/mnt/devvm-tools root=/opt/devvm/tools list=/etc/devvm/tools report=/run/devvm/tools-unmounted
rm -f "$report"
mkdir -p "$root"
wanted=" " failed=()
while read -r file tool version; do
  [[ -n $file ]] || continue
  m=$root/$tool/$version
  wanted+="$m "
  if [[ -d $share/$file && ! -L $share/$file ]]; then
    # Unpacked in the share (Docker's containers, which mount no images): linked, not mounted.
    [[ "$(readlink "$m" 2>/dev/null)" != "$share/$file" ]] || continue
    if ! { mkdir -p "$root/$tool" && { [[ -L $m || ! -d $m ]] || rmdir "$m"; } && ln -sfn "$share/$file" "$m"; }; then
      echo "devvm-tools: can't link $file" >&2
      failed+=("$file")
    fi
    continue
  fi
  mountpoint -q "$m" && continue
  if [[ ! -f $share/$file ]]; then
    echo "devvm-tools: $file is missing from the share" >&2
    rmdir "$m" 2>/dev/null || true
    failed+=("$file")
    continue
  fi
  mkdir -p "$m"
  # Direct I/O: the guest caches the files inside the image, not the image file as well.
  if ! dev=$(losetup --find --show --read-only --direct-io=on "$share/$file"); then
    echo "devvm-tools: can't mount $file" >&2
    rmdir "$m" 2>/dev/null || true
    failed+=("$file")
  elif ! mount -t erofs -o ro "$dev" "$m"; then
    echo "devvm-tools: can't mount $file" >&2
    losetup -d "$dev" || true
    rmdir "$m" 2>/dev/null || true
    failed+=("$file")
  fi
done < <(cat "$list" 2>/dev/null || true)
# Unmount what's no longer listed, and free its loop device.
findmnt -rn -o TARGET,SOURCE | { grep "^$root/" || true; } | while read -r m dev; do
  [[ $wanted != *" $m "* ]] || continue
  umount "$m" && { losetup -d "$dev" 2>/dev/null || true; rmdir "$m" 2>/dev/null || true; }
done
# And the links into the share to what's no longer listed (not mise's own version links).
for m in "$root"/*/*; do
  [[ -L $m && $wanted != *" $m "* && "$(readlink "$m")" == "$share/"* ]] || continue
  rm -f "$m"
done
# Hand the tool folders to the VM's user (mise writes its links there).
user=$(stat -c %U /home/*.guest 2>/dev/null | head -1 || true)
[[ -z $user ]] || find "$root" -maxdepth 1 -type d ! -user "$user" -exec chown "$user:" {} +
[[ -n ${failed[*]+x} ]] || exit 0
mkdir -p "$(dirname "$report")"
for file in ${failed[@]+"${failed[@]}"}; do echo "devvm-tools: unmounted $file" >&2; echo "$file"; done > "$report"
exit 1
