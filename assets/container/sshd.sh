#!/bin/sh
# Run as root in base's build machine or container after its scripts: sshd, for devvm's ssh for editors
# (services.sh starts it). After them, as it comes from the archive they left (Ubuntu's, pinned to a
# snapshot), so it fits what they installed: from the live archive, before them, its systemd
# libraries were newer than the snapshot's, which packages layers install (Docker's) then couldn't
# match. The package lists stay, the snapshot's, for layers' apt-get install. The host keys its
# package made go: each VM makes its own at each start.
set -eu
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends openssh-server
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache openssh-server openssh-keygen
else
  echo "sshd.sh: no apt-get or apk" >&2; exit 1
fi
rm -f /etc/ssh/ssh_host_*
