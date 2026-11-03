#!/bin/sh
# Run as root in a machine or container after each start, for what base's services do at boot on
# Lima: an image run as a container (smolvm's machines) runs no init system. Its /dev has no loop
# devices, which the shared tools' images are mounted with, so they're made first; then the tools
# are mounted (devvm-tools, once assets/guest/sync.sh installed it; one that fails is the launch's
# to report), the VM's sshd started when it's asked for, Nix's daemon started in the background
# where the VM has it (the nix layer), and Docker's where the VM has it (the docker layer).
# A pid file whose process isn't dockerd (one an image kept, or a start before this one) doesn't
# count, and Docker's run folder goes before dockerd starts, as a boot would leave it.
#   services.sh [ssh USER KEY [VAR...]]
# ssh: devvm's sshd for ssh and editors, on a config of its own: new host keys (ed25519) at each
# start, so no two VMs made from one image share them; USER alone may log in, with KEY (the VM's
# own, an authorized_keys line) alone; local forwarding, to IPv4 or IPv6, and remote forwards of TCP
# ports only on 127.77.0.0/16, the addresses of the VM's links, as layers/base.sh allows them on
# Lima; no remote socket forwards, tunnels or X11 (a safeguard for an honest VM: root here can change
# it, and the boundary is the host's ssh, which asks for no other remote forward and accepts a
# channel only for a forward it asked for); its sessions get /etc/environment and each
# VAR from this script's environment (the proxy's under network policy), as devvm's own commands
# do. The config holds them, so it's root's alone. devvm asks for it only once it found sshd there.
set -eu
i=0
while [ "$i" -lt 64 ]; do
  [ -e "/dev/loop$i" ] || mknod -m 660 "/dev/loop$i" b 7 "$i"
  i=$((i + 1))
done
[ -e /dev/loop-control ] || mknod -m 660 /dev/loop-control c 10 237
# A tool it can't mount doesn't stop the start: the launch's setup runs devvm-tools again and fails.
if [ -x /usr/local/sbin/devvm-tools ] && [ -d /mnt/devvm-tools ]; then /usr/local/sbin/devvm-tools || true; fi
if [ "${1:-}" = ssh ]; then
  user=$2 key=$3
  shift 3
  sshd=$(command -v sshd || echo /usr/sbin/sshd)
  [ -x "$sshd" ] || { echo "services.sh: no sshd" >&2; exit 1; }
  rm -f /etc/ssh/ssh_host_*
  mkdir -p /etc/ssh
  ssh-keygen -q -t ed25519 -N '' -C '' -f /etc/ssh/ssh_host_ed25519_key
  umask 022
  mkdir -p /etc/devvm/ssh /run/sshd
  chmod 755 /etc/devvm/ssh /run/sshd
  printf '%s\n' "$key" > /etc/devvm/ssh/authorized_keys
  # SetEnv's entries, each once, quoted: values with a quote, a backslash or a newline are left out.
  nl='
'
  setenv="" seen=" "
  add() {
    case " $seen " in *" $1 "*) return 0 ;; esac
    case $2 in *'"'*|*\\*|*"$nl"*) return 0 ;; esac
    setenv="$setenv \"$1=$2\"" seen="$seen$1 "
  }
  if [ -r /etc/environment ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      k=${line%%=*} v=${line#*=}
      case $k in ''|*[!A-Za-z0-9_]*|[0-9]*) continue ;; esac
      [ "$k" != "$line" ] || continue
      v=${v#\"} && v=${v%\"}
      add "$k" "$v"
    done < /etc/environment
  fi
  for k in "$@"; do
    case $k in ''|*[!A-Za-z0-9_]*|[0-9]*) continue ;; esac
    v=$(printenv "$k") || continue
    add "$k" "$v"
  done
  conf=/etc/devvm/ssh/sshd_config
  umask 077
  {
    echo "# devvm's sshd for ssh and editors, written at each start (services.sh)."
    # IPv4 alone to listen on; both to connect to, so forwarding every port reaches servers on ::1.
    echo "ListenAddress 0.0.0.0:22"
    echo "AddressFamily any"
    echo "HostKey /etc/ssh/ssh_host_ed25519_key"
    echo "PidFile /run/devvm-sshd.pid"
    echo "AllowUsers $user"
    echo "PermitRootLogin no"
    echo "AuthenticationMethods publickey"
    echo "PubkeyAuthentication yes"
    echo "PasswordAuthentication no"
    echo "KbdInteractiveAuthentication no"
    echo "HostbasedAuthentication no"
    echo "AuthorizedKeysFile /etc/devvm/ssh/authorized_keys"
    echo "AuthorizedKeysCommand none"
    echo "AuthorizedPrincipalsFile none"
    echo "PermitUserEnvironment no"
    # Remote forwards bind the address asked for, which PermitListen matches as text: 0.0.0.0,
    # 127.0.0.1, ::1 and localhost are refused.
    echo "AllowTcpForwarding all"
    echo "AllowStreamLocalForwarding local"
    echo "GatewayPorts clientspecified"
    echo "PermitListen 127.77.*:*"
    echo "X11Forwarding no"
    echo "PermitTunnel no"
    echo "AllowAgentForwarding yes"
    echo "PrintMotd no"
    echo "Subsystem sftp internal-sftp"
    [ -z "$setenv" ] || echo "SetEnv$setenv"
  } > "$conf"
  umask 022
  "$sshd" -t -f "$conf"
  "$sshd" -f "$conf" -E /var/log/devvm-sshd.log
fi
# Nix's daemon where the VM has it (the nix layer), with its settings (/etc/conf.d/nix-daemon:
# guest/sync.sh's proxy under network policy), unless it runs; its pid file counts as Docker's does.
nixd=/nix/var/nix/profiles/default/bin/nix-daemon
pid=$(cat /run/nix-daemon.pid 2>/dev/null || true)
if [ -x "$nixd" ] && ! { [ -n "$pid" ] && [ "$(cat "/proc/$pid/comm" 2>/dev/null)" = nix-daemon ]; }; then
  (
    # shellcheck source=/dev/null
    [ ! -f /etc/conf.d/nix-daemon ] || . /etc/conf.d/nix-daemon
    mkdir -p /var/log
    nohup "$nixd" --daemon >/var/log/nix-daemon.log 2>&1 </dev/null &
    echo $! > /run/nix-daemon.pid
  )
fi
pid=$(cat /run/docker.pid 2>/dev/null || true)
if command -v dockerd >/dev/null 2>&1 &&
  ! { [ -n "$pid" ] && [ "$(cat "/proc/$pid/comm" 2>/dev/null)" = dockerd ]; }; then
  # /run outlives a stop here, unlike a boot's: its pid files name processes long gone, whose pids
  # this start's may have, and dockerd refuses to start its containerd over one.
  rm -rf /run/docker.pid /run/docker 2>/dev/null || true
  mkdir -p /var/log
  nohup dockerd -p /run/docker.pid >/var/log/dockerd.log 2>&1 </dev/null &
fi
