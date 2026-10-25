#!/bin/sh
# Run as root in base's build machine, before layers/base.sh: a container image has what Lima's
# cloud images bring (sudo, CA certificates, bash) and the VM's user, as the host's: its name and
# uid (virtiofs shows the project folder as the host user's), home /home/NAME.guest. The user's
# password is * (none matches), not locked: sshd without PAM refuses a locked account's keys too.
# sshd comes after base's scripts (sshd.sh).
#   prepare.sh NAME UID
set -eu
name=$1 uid=$2 home=/home/$1.guest
# shellcheck source=/dev/null
. /etc/os-release
case " $ID ${ID_LIKE:-} " in
  *" ubuntu "*|*" debian "*)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates sudo bash passwd
    rm -rf /var/lib/apt/lists/*
    # Another user with the uid (Ubuntu's own ubuntu, 1000) goes: files are told apart by uid.
    other=$(getent passwd "$uid" | cut -d: -f1 || true)
    if [ -n "$other" ] && [ "$other" != "$name" ]; then userdel -r "$other" 2>/dev/null || userdel "$other"; fi
    id "$name" >/dev/null 2>&1 || useradd --create-home --home-dir "$home" --uid "$uid" --shell /bin/bash "$name"
    ;;
  *" alpine "*)
    apk add --no-cache ca-certificates sudo bash shadow
    other=$(getent passwd "$uid" | cut -d: -f1 || true)
    if [ -n "$other" ] && [ "$other" != "$name" ]; then deluser --remove-home "$other"; fi
    id "$name" >/dev/null 2>&1 || adduser -D -h "$home" -u "$uid" -s /bin/bash "$name"
    ;;
  *) echo "prepare.sh: unsupported distro '$ID' (supported: Debian/Ubuntu and Alpine families)" >&2; exit 1 ;;
esac
usermod -p '*' "$name"
echo "$name ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-devvm
chmod 440 /etc/sudoers.d/90-devvm
