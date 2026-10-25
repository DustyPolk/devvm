#!/bin/bash
# Installed in project VMs as /usr/local/sbin/devvm-tools, run as root at boot (devvm-tools.service)
# and by guest/sync.sh on every launch. Mounts the VM's shared tool images, read-only, from the share
# at /mnt/devvm-tools: each line of /etc/devvm/tools is "FILE TOOL VERSION", mounted at
# /opt/devvm/tools/TOOL/VERSION, where mise finds it as a shared install. Tool images no longer
# listed are unmounted. The tool folders themselves stay writable, so mise can keep its version
# links (latest, 24, lts, ...) there.
set -euo pipefail
share=/mnt/devvm-tools root=/opt/devvm/tools list=/etc/devvm/tools
mkdir -p "$root"
wanted=" "
while read -r file tool version; do
  [[ -n $file ]] || continue
  m=$root/$tool/$version
  wanted+="$m "
  mountpoint -q "$m" && continue
  [[ -f $share/$file ]] || { echo "devvm-tools: $file is missing from the share" >&2; continue; }
  mkdir -p "$m"
  # Direct I/O: the guest caches the files inside the image, not the image file as well.
  dev=$(losetup --find --show --read-only --direct-io=on "$share/$file")
  mount -t erofs -o ro "$dev" "$m" || { echo "devvm-tools: can't mount $file" >&2; losetup -d "$dev"; }
done < <(cat "$list" 2>/dev/null || true)
# Unmount what's no longer listed, and free its loop device.
findmnt -rn -o TARGET,SOURCE | { grep "^$root/" || true; } | while read -r m dev; do
  [[ $wanted != *" $m "* ]] || continue
  umount "$m" && { losetup -d "$dev" 2>/dev/null || true; rmdir "$m" 2>/dev/null || true; }
done
# Hand the tool folders to the VM's user (mise writes its links there).
user=$(stat -c %U /home/*.guest 2>/dev/null | head -1 || true)
[[ -z $user ]] || find "$root" -maxdepth 1 -type d ! -user "$user" -exec chown "$user:" {} +
