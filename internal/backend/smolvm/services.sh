#!/bin/sh
# Run as root in a machine after each start, for what base's services do at boot on Lima: a smolvm
# machine runs no init system. Its /dev has no loop devices, which the shared tools' images are
# mounted with, so they're made first; then the tools are mounted (devvm-tools, once guest/sync.sh
# installed it) and Docker's daemon started in the background, where base has them.
set -eu
i=0
while [ "$i" -lt 64 ]; do
  [ -e "/dev/loop$i" ] || mknod -m 660 "/dev/loop$i" b 7 "$i"
  i=$((i + 1))
done
[ -e /dev/loop-control ] || mknod -m 660 /dev/loop-control c 10 237
if [ -x /usr/local/sbin/devvm-tools ] && [ -d /mnt/devvm-tools ]; then /usr/local/sbin/devvm-tools; fi
pid=$(cat /run/docker.pid 2>/dev/null || true)
if command -v dockerd >/dev/null 2>&1 && ! { [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; }; then
  mkdir -p /var/log
  nohup dockerd -p /run/docker.pid >/var/log/dockerd.log 2>&1 </dev/null &
fi
