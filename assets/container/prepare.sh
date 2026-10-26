#!/bin/sh
# Run as root in base's build machine or container, before assets/layers/base.sh: a container image
# has what Lima's cloud images bring (sudo, CA certificates, bash) and the VM's user, as the host's:
# its name and uid (virtiofs and bind mounts show the project folder as the host user's), and with
# GID its group too (a Linux host's bind mounts keep it); home /home/NAME.guest. The user's password
# is * (none matches), not locked: sshd without PAM refuses a locked account's keys too. sshd comes
# after base's scripts (sshd.sh).
#   prepare.sh NAME UID [GID]
set -eu
name=$1 uid=$2 gid=${3:-} home=/home/$1.guest
# group: the group with GID, made (named NAME) when there's none; "" without GID.
group() {
  [ -n "$gid" ] || return 0
  g=$(getent group "$gid" | cut -d: -f1 || true)
  if [ -z "$g" ]; then
    if getent group "$name" >/dev/null; then groupmod -g "$gid" "$name"; else groupadd -g "$gid" "$name"; fi
    g=$name
  fi
  echo "$g"
}
# owned: the group with GID that was the removed user's own (Ubuntu's ubuntu) is renamed NAME.
owned() {
  [ -n "$gid" ] && [ -n "$other" ] && [ "$other" != "$name" ] || return 0
  if [ "$(getent group "$gid" | cut -d: -f1)" = "$other" ] && ! getent group "$name" >/dev/null; then
    groupmod -n "$name" "$other"
  fi
}
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
    owned
    g=$(group)
    if ! id "$name" >/dev/null 2>&1; then
      if [ -n "$g" ]; then
        useradd --create-home --home-dir "$home" --uid "$uid" --gid "$g" --shell /bin/bash "$name"
      else
        useradd --create-home --home-dir "$home" --uid "$uid" --shell /bin/bash "$name"
      fi
    fi
    ;;
  *" alpine "*)
    # OpenRC and ssh's client too, which Alpine's container image lacks: base.sh adds an OpenRC
    # service (enabled only: no init system runs it) and writes /etc/ssh/ssh_known_hosts.
    apk add --no-cache ca-certificates sudo bash shadow openrc openssh-client
    other=$(getent passwd "$uid" | cut -d: -f1 || true)
    if [ -n "$other" ] && [ "$other" != "$name" ]; then deluser --remove-home "$other"; fi
    owned
    g=$(group)
    if ! id "$name" >/dev/null 2>&1; then
      if [ -n "$g" ]; then
        adduser -D -h "$home" -u "$uid" -G "$g" -s /bin/bash "$name"
      else
        adduser -D -h "$home" -u "$uid" -s /bin/bash "$name"
      fi
    fi
    ;;
  *) echo "prepare.sh: unsupported distro '$ID' (supported: Debian/Ubuntu and Alpine families)" >&2; exit 1 ;;
esac
usermod -p '*' "$name"
echo "$name ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-devvm
chmod 440 /etc/sudoers.d/90-devvm
