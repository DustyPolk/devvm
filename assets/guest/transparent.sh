#!/bin/bash
# devvm-transparent: transparent mode, for tools that ignore the proxy settings on a confined VM.
# dnsmasq answers every name with the stand-in address (proxy.SentinelAddr, on lo) and nothing else,
# with no upstream, so no DNS query leaves the VM. nftables sends a connection to one of the
# stand-in address's ports (80, 443 and the ports the VM's allow rules name, from the proxy's
# /transparent/ports) to a socat listener on that address, which makes it an ordinary CONNECT to the
# VM's proxy with the VM's password, for the reserved target (proxy.TransparentHost) at that port.
# The proxy reads the host from the connection's first bytes and judges it as any CONNECT. Every
# other port of the address is refused at once, and DNS to a public server goes to dnsmasq too.
# None of it is a boundary: the host confines the VM to its proxy, whatever root here changes.
#   devvm-transparent setup ADDR HOST < PROXY_URL  root, sync.sh: install it all, start the service
#   devvm-transparent teardown                     root, sync.sh: stop the service, remove it all
#   devvm-transparent run                          the service: start and watch what it runs
#   devvm-transparent down                         the service's stop: the table, the address, the children
#   devvm-transparent status                       devvm-why, as root: whether it runs, refusals, names
#   devvm-transparent render PORT=LISTEN...        the ruleset for those ports and their listeners, and
#                                                  DEVVM_TRANSPARENT_RESOLVERS's name servers
#   devvm-transparent slots PORT...                each port's listener, keeping the ports that stay
#   devvm-transparent valid FILE                   whether FILE is a port list the ruleset takes
# Exit 3 from setup: dnsmasq, socat or nft is missing (an image from before transparent mode).
set -u
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
tp=${DEVVM_TRANSPARENT_DIR:-/etc/devvm/transparent}
run_dir=/run/devvm-transparent
self=/usr/local/sbin/devvm-transparent
user=devvm-transparent
table=devvm_transparent
# The listeners' ports: 80's, 443's and 32 more, above Linux's ephemeral range (32768-60999), so no
# connection out or bind(0) is given one; dead has no listener, so what goes there is refused.
listen80=61080 listen443=61443 first_slot=61100 last_slot=61131 dead=61079
# The VM's own name servers the loaded ruleset sends to dnsmasq (apply).
resolvers=''

# load_env: addr, host, proxy_host and proxy_port from the env file setup wrote.
load_env() {
  addr='' host='' proxy_host='' proxy_port=''
  # shellcheck source=/dev/null
  [[ -f $tp/env ]] && . "$tp/env"
  [[ -n $addr && -n $host && -n $proxy_host && -n $proxy_port ]]
}

# valid_ports FILE: 2 to 34 lines, each a port from 1 to 65535 in digits, once, never 22 or 53, with
# 80 and 443 among them. Anything else from the proxy is kept out of the ruleset.
valid_ports() {
  local p n=0 seen=' '
  while IFS= read -r p; do
    n=$((n + 1))
    [[ $p =~ ^[1-9][0-9]{0,4}$ ]] && ((p <= 65535)) && [[ $p != 22 && $p != 53 && $seen != *" $p "* ]] || return 1
    seen+="$p "
  done <"$1"
  ((n >= 2 && n <= 34)) && [[ $seen == *' 80 '* && $seen == *' 443 '* ]]
}

# slots PORT...: "PORT LISTEN" for each port, 80 and 443 on their own listeners, the others each on
# the one the slots file gave it, while they stay, else on the lowest that's free.
slots() {
  local p l used=' ' pending='' out=''
  for p in "$@"; do
    case $p in
    80) out+="80 $listen80"$'\n' ;;
    443) out+="443 $listen443"$'\n' ;;
    *)
      l=$(awk -v p="$p" '$1 == p { print $2; exit }' "$tp/slots" 2>/dev/null)
      if [[ $l =~ ^[0-9]+$ ]] && ((l >= first_slot && l <= last_slot)) && [[ $used != *" $l "* ]]; then
        used+="$l " out+="$p $l"$'\n'
      else
        pending+="$p "
      fi
      ;;
    esac
  done
  for p in $pending; do
    for ((l = first_slot; l <= last_slot; l++)); do
      [[ $used == *" $l "* ]] && continue
      used+="$l " out+="$p $l"$'\n'
      break
    done
  done
  printf '%s' "$out"
}

# render PORT=LISTEN...: the ruleset, one transaction that replaces the table whole. Both NAT chains
# run before Docker's (dstnat, -100: priorities are numbers, which Debian 12's nft needs here) and
# give every TCP and UDP port of the stand-in address a
# mapping, so Docker's chains, which would send a published port's traffic to a container, never
# see one: a redirected port goes to its listener, DNS to dnsmasq, and every other port to the dead
# port, which refuses it at once. DNS to any public server goes to dnsmasq, and so does DNS to the
# VM's own name servers ($resolvers, own_resolvers), which Docker gives containers; loopback's other
# addresses, private, link-local and CGNAT ones are left as they are. 127.0.0.1:53 is where the
# VM's resolver asks (Lima's DNS setting): it goes to dnsmasq, which listens on the stand-in
# address alone, so nothing of transparent mode listens on loopback.
render() {
  local pl r redirects='' own='' a=${addr:-198.18.0.1}
  for pl in "$@"; do
    redirects+="    ip daddr $a tcp dport ${pl%%=*} dnat ip to $a:${pl#*=}"$'\n'
  done
  for r in ${resolvers:-}; do own+="${own:+, }$r"; done
  if [[ -n $own ]]; then
    redirects+="    ip daddr { $own } udp dport 53 dnat ip to $a:53"$'\n'
    redirects+="    ip daddr { $own } tcp dport 53 dnat ip to $a:53"$'\n'
  fi
  cat <<EOF
table inet $table
delete table inet $table
table inet $table {
  set kept {
    type ipv4_addr; flags interval
    elements = { 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.168.0.0/16, $a }
  }
  # Before every NAT chain (-150): the stand-in address takes only the VM's own and Docker's bridges,
  # and only TCP, UDP and ICMP, so another bridge (Podman's, LXD's) or a protocol the NAT chains don't
  # map (SCTP) never reaches Docker's DNAT of a published port through it.
  chain early_in {
    type filter hook prerouting priority -150; policy accept;
    ip daddr $a iifname != "lo" iifname != "docker0" iifname != "br-*" drop
    ip daddr $a meta l4proto != { tcp, udp, icmp } drop
  }
  chain early_out {
    type filter hook output priority -150; policy accept;
    ip daddr $a meta l4proto != { tcp, udp, icmp } drop
  }
  chain output {
    type nat hook output priority -110; policy accept;
${redirects}    jump sentinel
    ip daddr 127.0.0.1 udp dport 53 dnat ip to $a:53
    ip daddr 127.0.0.1 tcp dport 53 dnat ip to $a:53
    ip daddr != @kept udp dport 53 dnat ip to $a:53
    ip daddr != @kept tcp dport 53 dnat ip to $a:53
  }
  chain prerouting {
    type nat hook prerouting priority -110; policy accept;
    iifname "docker0" jump from_containers
    iifname "br-*" jump from_containers
  }
  chain from_containers {
${redirects}    jump sentinel
    ip daddr != @kept udp dport 53 dnat ip to $a:53
    ip daddr != @kept tcp dport 53 dnat ip to $a:53
  }
  # IPv6 returns first: in an inet table, ip daddr matches IPv4 alone, so an IPv6 packet would skip
  # the next return and be logged as refused. The stand-in address is IPv4, and a connection to it
  # from an IPv6 socket (::ffff:198.18.0.1) leaves as IPv4, so this opens no way to it.
  chain sentinel {
    meta nfproto != ipv4 return
    ip daddr != $a return
    udp dport 53 dnat ip to $a:53
    tcp dport 53 dnat ip to $a:53
    meta l4proto tcp limit rate 5/second log prefix "devvm-transparent refused: " level info
    meta l4proto tcp dnat ip to $a:$dead
    meta l4proto udp dnat ip to $a:$dead
  }
  chain input {
    type filter hook input priority -10; policy accept;
    ip daddr $a iifname != "lo" iifname != "docker0" iifname != "br-*" drop
    ip daddr $a udp dport 53 accept
    ip daddr $a tcp dport 53 accept
    ip daddr $a ct status dnat accept
    ip daddr $a meta l4proto tcp reject with tcp reset
    ip daddr $a meta l4proto udp reject
  }
}
EOF
}

# pairs: the ports file's ports as render takes them (slots), and the slots file written to match.
pairs() {
  local p l list
  # shellcheck disable=SC2046 # one port a line
  list=$(slots $(cat "$tp/ports"))
  printf '%s\n' "$list" >"$tp/slots.tmp" && mv "$tp/slots.tmp" "$tp/slots"
  while read -r p l; do [[ -z $p ]] || printf '%s=%s ' "$p" "$l"; done <<<"$list"
}

# own_resolvers: the VM's own name servers but loopback's, which its resolver asks besides
# 127.0.0.1 (such as the one Lima's DHCP gives it) and which Docker gives containers: IPv4
# addresses from systemd-resolved's list and /etc/resolv.conf, each once, at most 8.
own_resolvers() {
  local r seen=' ' n=0
  while read -r r; do
    [[ $r =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ && $r != 127.* && $r != "${addr:-198.18.0.1}" && $seen != *" $r "* ]] || continue
    n=$((n + 1))
    ((n <= 8)) || break
    seen+="$r "
    echo "$r"
  done < <(awk '$1 == "nameserver" { print $2 }' /run/systemd/resolve/resolv.conf /etc/resolv.conf 2>/dev/null)
}

# apply: the ruleset for the ports file's ports and the VM's own name servers as they are now, loaded
# whole.
apply() {
  local f
  resolvers=$(own_resolvers | tr '\n' ' ')
  f=$(mktemp)
  # shellcheck disable=SC2046 # PORT=LISTEN words
  render $(pairs) >"$f"
  nft -f "$f" || echo "devvm-transparent: nft didn't load the ruleset" >&2
  rm -f "$f"
}

# alive PIDFILE NAME: the process in PIDFILE runs, and is NAME.
alive() {
  local pid
  pid=$(cat "$1" 2>/dev/null) || return 1
  [[ -n $pid && "$(cat "/proc/$pid/comm" 2>/dev/null)" == "$2" ]]
}

# stop PIDFILE NAME: end the process in PIDFILE when it's NAME, and forget it.
stop() {
  if alive "$1" "$2"; then kill "$(cat "$1")" 2>/dev/null; fi
  rm -f "$1"
}

# search_domains: the domains the VM's resolver appends to a name without a dot (/etc/resolv.conf's
# search line, which Lima fills from this machine's network), each a DNS name, at most 6.
search_domains() {
  local d n=0
  while read -r d; do
    d=${d%.}
    [[ $d =~ ^[A-Za-z0-9]([A-Za-z0-9.-]{0,252}[A-Za-z0-9])?$ ]] || continue
    n=$((n + 1))
    ((n <= 6)) || break
    echo "$d"
  done < <(awk '$1 == "search" { for (i = 2; i <= NF; i++) print $i }' /etc/resolv.conf 2>/dev/null)
}

# start_dnsmasq: dnsmasq, with no answer for the names under the search domains: so a name without
# a dot that isn't in /etc/hosts doesn't resolve, as it doesn't to the stand-in address either
# (address=//), rather than turning into one of this machine's network's names.
start_dnsmasq() {
  local d args=()
  for d in $(search_domains); do args+=("--address=/$d/"); done
  dnsmasq --keep-in-foreground --conf-file="$tp/dnsmasq.conf" ${args[@]+"${args[@]}"} &
  echo "$!" >"$run_dir/dnsmasq.pid"
}

# start_socat PORT LISTEN: the listener for PORT on the stand-in address's LISTEN.
start_socat() {
  socat "TCP4-LISTEN:$2,bind=$addr,reuseaddr,fork,max-children=256,backlog=256,su=$user" \
    "PROXY:$proxy_host:$host:$1,proxyport=$proxy_port,proxyauthfile=$tp/auth,connect-timeout=10" &
  echo "$!" >"$run_dir/socat-$1.pid"
}

# listeners: start what isn't running, the listeners of the slots file's ports, and stop those whose
# port is gone.
listeners() {
  local f p l
  alive "$run_dir/dnsmasq.pid" dnsmasq || start_dnsmasq
  for f in "$run_dir"/socat-*.pid; do
    [[ -e $f ]] || continue
    p=${f##*/socat-} p=${p%.pid}
    grep -q "^$p " "$tp/slots" || stop "$f" socat
  done
  while read -r p l; do
    [[ -z $p ]] || alive "$run_dir/socat-$p.pid" socat || start_socat "$p" "$l"
  done <"$tp/slots"
}

# fetch: the proxy's port list, into the ports file when it's valid and has changed (0), else 1.
fetch() {
  local f
  f=$(mktemp)
  if curl -sSf --max-time 5 -K "$tp/curlrc" -o "$f" http://devvm.proxy/transparent/ports 2>/dev/null &&
    valid_ports "$f" && ! cmp -s "$f" "$tp/ports"; then
    mv "$f" "$tp/ports"
    return 0
  fi
  rm -f "$f"
  return 1
}

run() {
  load_env || { echo "devvm-transparent: no $tp/env" >&2; exit 1; }
  mkdir -p "$run_dir" && chmod 700 "$run_dir"
  [[ -s $tp/ports ]] && valid_ports "$tp/ports" || printf '80\n443\n' >"$tp/ports"
  down_children
  ip -4 addr show dev lo | grep -q " $addr/32 " || ip addr add "$addr/32" dev lo
  apply
  trap 'down; exit 0' TERM INT
  local tick=0 hosts_at
  hosts_at=$(stat -c %Y /etc/hosts 2>/dev/null)
  listeners
  touch "$run_dir/ready"
  while :; do
    sleep 1 &
    wait $!
    listeners
    if [[ "$(stat -c %Y /etc/hosts 2>/dev/null)" != "$hosts_at" ]]; then
      hosts_at=$(stat -c %Y /etc/hosts 2>/dev/null)
      [[ ! -f $run_dir/dnsmasq.pid ]] || kill -HUP "$(cat "$run_dir/dnsmasq.pid")" 2>/dev/null
    fi
    # A launch's setup fetched new ports (reload); every tenth second, the proxy's ports, the VM's
    # own name servers, and the table back if something removed it (nft flush ruleset).
    tick=$(((tick + 1) % 10))
    if [[ -e $run_dir/reload ]] || { ((tick == 0)) && { fetch || [[ "$(own_resolvers | tr '\n' ' ')" != "$resolvers" ]] ||
      ! nft list table inet "$table" >/dev/null 2>&1; }; }; then
      rm -f "$run_dir/reload"
      apply
      listeners
    fi
  done
}

# down_children: stop every child a supervisor recorded, this one's or one that died.
down_children() {
  local f
  for f in "$run_dir"/socat-*.pid; do [[ ! -e $f ]] || stop "$f" socat; done
  stop "$run_dir/dnsmasq.pid" dnsmasq
}

down() {
  load_env || addr=198.18.0.1
  down_children
  rm -f "$run_dir/ready"
  nft delete table inet "$table" 2>/dev/null
  ip addr del "$addr/32" dev lo 2>/dev/null
  return 0
}

systemd=false
[[ -d /run/systemd/system ]] && systemd=true

# put FILE MODE OWNER: FILE from stdin, written only when it differs; 0 when it changed.
put() {
  local tmp
  tmp=$(mktemp)
  cat >"$tmp"
  if cmp -s "$tmp" "$1"; then
    rm -f "$tmp"
    return 1
  fi
  install -m "$2" -o root -g "$3" "$tmp" "$1"
  rm -f "$tmp"
}

# dnsmasq_conf: dnsmasq's settings, read with --conf-file, so /etc/dnsmasq.conf and /etc/dnsmasq.d
# aren't. Every name's A record is the stand-in address, with TTL 0, and every other type is an empty
# answer, never REFUSED, which musl waits 5 s on. /etc/hosts comes first: localhost, the VM's
# hostname, host.lima.internal and the links' names keep their own addresses. A name without a dot,
# and any other name under devvm.internal, doesn't exist.
dnsmasq_conf() {
  cat <<EOF
# devvm: names for tools that skip the proxy (devvm-transparent). No upstream: nothing leaves the VM.
port=53
listen-address=$addr
bind-interfaces
no-resolv
no-poll
pid-file=
user=$user
group=$user
address=/#/$addr
local=/#/
address=/localhost/127.0.0.1
address=/localhost/::1
address=/devvm.internal/
address=//
log-queries
EOF
  # systemd's journal keeps stderr; OpenRC's service doesn't, so there it's syslog.
  ! $systemd || echo 'log-facility=-'
}

unit() {
  cat <<'EOF'
[Unit]
Description=devvm: names and connections for tools that skip the proxy
After=network.target systemd-resolved.service
ConditionPathExists=/etc/devvm/transparent/env
[Service]
Type=simple
ExecStart=/usr/local/sbin/devvm-transparent run
ExecStopPost=/usr/local/sbin/devvm-transparent down
Restart=always
RestartSec=1
KillMode=control-group
[Install]
WantedBy=multi-user.target
EOF
}

openrc() {
  cat <<'EOF'
#!/sbin/openrc-run
description="devvm: names and connections for tools that skip the proxy"
supervisor=supervise-daemon
command=/usr/local/sbin/devvm-transparent
command_args=run
respawn_delay=1
respawn_max=0
depend() { need net localmount; after firewall; }
start_pre() { [ -f /etc/devvm/transparent/env ]; }
stop_post() { /usr/local/sbin/devvm-transparent down; }
EOF
}

setup() {
  addr=$1 host=$2
  local url hp auth changed=false svc
  for p in dnsmasq socat nft; do command -v "$p" >/dev/null || exit 3; done
  IFS= read -r url
  hp=${url#http://} auth=''
  if [[ $hp == *@* ]]; then auth=${hp%%@*} hp=${hp#*@}; fi
  proxy_host=${hp%:*} proxy_port=${hp##*:}
  proxy_port=${proxy_port%%/*}
  # The user dnsmasq and socat's connections run as: shadow's tools, else busybox's.
  grep -q "^$user:" /etc/group || groupadd --system "$user" 2>/dev/null || addgroup -S "$user"
  grep -q "^$user:" /etc/passwd || useradd --system --no-create-home --home-dir /nonexistent --shell /sbin/nologin -g "$user" "$user" 2>/dev/null ||
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -G "$user" "$user"
  install -d -m 755 "$tp"
  printf 'addr=%s\nhost=%s\nproxy_host=%s\nproxy_port=%s\n' "$addr" "$host" "$proxy_host" "$proxy_port" | put "$tp/env" 644 root && changed=true
  printf '%s' "$auth" | put "$tp/auth" 640 "$user" && changed=true
  printf 'proxy = "http://%s:%s"\nproxy-user = "%s"\nnoproxy = ""\n' "$proxy_host" "$proxy_port" "$auth" | put "$tp/curlrc" 600 root || true
  dnsmasq_conf | put "$tp/dnsmasq.conf" 644 root && changed=true
  put "$self" 755 root <"$0" && changed=true
  if $systemd; then
    svc=/etc/systemd/system/devvm-transparent.service
    unit | put "$svc" 644 root && { changed=true; systemctl daemon-reload; }
    systemctl enable devvm-transparent.service >/dev/null 2>&1
  else
    openrc | put /etc/init.d/devvm-transparent 755 root && changed=true
    rc-update add devvm-transparent default >/dev/null 2>&1
  fi
  # The ports now, so the first shell has them, which a running service takes at once (reload); else
  # the last ones, else 80 and 443.
  if fetch; then
    mkdir -p "$run_dir" && chmod 700 "$run_dir" && touch "$run_dir/reload"
  fi
  [[ -s $tp/ports ]] || printf '80\n443\n' >"$tp/ports"
  if $changed; then
    rm -f "$run_dir/ready"
    if $systemd; then systemctl restart devvm-transparent; else rc-service devvm-transparent restart >/dev/null; fi
  elif $systemd; then
    systemctl start devvm-transparent
  else
    rc-service devvm-transparent status >/dev/null 2>&1 || rc-service devvm-transparent start >/dev/null
  fi
  local i=0
  until [[ -e $run_dir/ready && ! -e $run_dir/reload ]]; do
    i=$((i + 1))
    ((i <= 100)) || { echo "devvm-transparent: the service didn't start" >&2; return 1; }
    sleep 0.1
  done
}

teardown() {
  [[ -e $self || -e $tp ]] || return 0
  if $systemd; then
    systemctl disable --now devvm-transparent.service >/dev/null 2>&1
    rm -f /etc/systemd/system/devvm-transparent.service
    systemctl daemon-reload
  else
    rc-service devvm-transparent stop >/dev/null 2>&1
    rc-update del devvm-transparent default >/dev/null 2>&1
    rm -f /etc/init.d/devvm-transparent
  fi
  down
  rm -rf "$tp" "$run_dir" "$self"
  ! $systemd || resolvectl flush-caches 2>/dev/null
  return 0
}

status() {
  local ports
  if [[ -e $run_dir/ready ]]; then
    ports=$(tr '\n' ' ' <"$tp/ports" 2>/dev/null)
    echo "Transparent mode: on, for ports ${ports% }; other ports are refused."
  else
    echo "Transparent mode: not running."
    return 0
  fi
  echo "Refused lately (a port devvm doesn't send to the proxy):"
  if $systemd; then
    journalctl -k -n 1000 -o short-iso --no-pager 2>/dev/null | grep 'devvm-transparent refused: ' | tail -10 |
      sed -E 's/^([^ ]+).* DPT=([0-9]+).*/  \2 at \1/'
  else
    dmesg 2>/dev/null | grep 'devvm-transparent refused: ' | tail -10 | sed -E 's/.* DPT=([0-9]+).*/  \1/'
  fi
  echo "Names looked up lately:"
  { if $systemd; then journalctl -u devvm-transparent -n 1000 -o cat --no-pager 2>/dev/null; else cat /var/log/messages 2>/dev/null; fi; } |
    grep -oE 'query\[A\] [^ ]+' | awk '{ print "  " $2 }' | tail -10
}

case ${1:-} in
setup) shift; setup "$@" ;;
teardown) teardown ;;
run) run ;;
down) down ;;
status) status ;;
render) shift; resolvers=${DEVVM_TRANSPARENT_RESOLVERS:-}; render "$@" ;;
slots) shift; slots "$@" ;;
valid) valid_ports "${2:?}" ;;
*) grep '^#   devvm-transparent ' "$0" >&2; exit 2 ;;
esac
