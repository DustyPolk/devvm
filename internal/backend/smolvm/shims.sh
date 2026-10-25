#!/bin/sh
# Run as root in a build machine: around devvm's scripts, which enable services the way Lima's
# guests boot them (systemctl, rc-update). A smolvm machine runs no init system, so while they run,
# those commands are stand-ins that do nothing, and devvm starts the services itself after each
# start. Only a command the image lacks gets one, and only devvm's stand-ins are removed.
#   shims.sh on|off
set -eu
mark="# devvm build stand-in"
for c in systemctl rc-update; do
  f=/usr/local/sbin/$c
  case $1 in
    on)
      command -v "$c" >/dev/null 2>&1 && continue
      mkdir -p /usr/local/sbin
      printf '#!/bin/sh\n%s: smolvm machines run no init system\nexit 0\n' "$mark" > "$f"
      chmod 755 "$f"
      ;;
    off)
      if [ -f "$f" ] && grep -qx "$mark: smolvm machines run no init system" "$f"; then rm -f "$f"; fi
      ;;
  esac
done
