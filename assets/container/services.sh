#!/bin/sh
# Run as root in a machine or container after each start, for what base's services do at boot on
# Lima: an image run as a container (smolvm's machines, Docker's containers) runs no init system.
# Its /dev has no loop devices, which the shared tools' images are mounted with, so they're made
# first; then the tools are mounted or linked (devvm-tools, once assets/guest/sync.sh installed it),
# the VM's sshd started when it's asked for, and Docker's daemon started in the background where the
# VM has it (the docker layer), unless DEVVM_DOCKERD is off (Docker's containers can't run it).
#   services.sh [ssh USER KEY [VAR...]]
# ssh: devvm's sshd for ssh and editors, on a config of its own: new host keys (ed25519) at each
# start, so no two VMs made from one image share them; USER alone may log in, with KEY (the VM's
# own, an authorized_keys line) alone; local forwarding only, no remote forwarding, tunnels or X11
# (a defence for an honest VM: root here can change it), to IPv4 or IPv6; its sessions get /etc/environment and each
# VAR from this script's environment (the proxy's under network policy), as devvm's own commands
# do. The config holds them, so it's root's alone. devvm asks for it only once it found sshd there.
# With DEVVM_SSHD=inetd (Docker's containers) no sshd daemon starts and nothing listens: each
# connection runs sshd -i on this config, through docker exec (devvm _docker-connect).
set -eu
i=0
while [ "$i" -lt 64 ]; do
  [ -e "/dev/loop$i" ] || mknod -m 660 "/dev/loop$i" b 7 "$i"
  i=$((i + 1))
done
[ -e /dev/loop-control ] || mknod -m 660 /dev/loop-control c 10 237
if [ -x /usr/local/sbin/devvm-tools ] && [ -d /mnt/devvm-tools ]; then /usr/local/sbin/devvm-tools; fi
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
    echo "AllowTcpForwarding local"
    echo "AllowStreamLocalForwarding local"
    echo "PermitListen none"
    echo "GatewayPorts no"
    echo "X11Forwarding no"
    echo "PermitTunnel no"
    echo "AllowAgentForwarding yes"
    echo "PrintMotd no"
    echo "Subsystem sftp internal-sftp"
    [ -z "$setenv" ] || echo "SetEnv$setenv"
  } > "$conf"
  umask 022
  "$sshd" -t -f "$conf"
  [ "${DEVVM_SSHD:-}" = inetd ] || "$sshd" -f "$conf" -E /var/log/devvm-sshd.log
fi
pid=$(cat /run/docker.pid 2>/dev/null || true)
if [ "${DEVVM_DOCKERD:-}" != off ] && command -v dockerd >/dev/null 2>&1 && ! { [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; }; then
  mkdir -p /var/log
  nohup dockerd -p /run/docker.pid >/var/log/dockerd.log 2>&1 </dev/null &
fi
