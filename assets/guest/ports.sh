#!/bin/sh
# Run as the VM's user by its port forwarder on the host (devvm _lima-ports), over
# devvm's ssh, for forwarding its ports: the TCP sockets listening in the VM, as /proc/net/tcp and tcp6 have
# their local addresses (ADDRESS:PORT, in hex), one per line, each list ended by a line ".". A list
# goes out when it changes, and every 10 s all the same, so the script ends soon after the
# connection does. The forwarder decides what it forwards: this only reports.
set -u
last=x n=0
while :; do
  now=$(cat /proc/net/tcp /proc/net/tcp6 2>/dev/null | awk '$4 == "0A" { print $2 }' | sort -u)
  if [ "$now" != "$last" ] || [ "$n" -ge 10 ]; then
    printf '%s\n.\n' "$now" || exit 0
    last=$now n=0
  fi
  n=$((n + 1))
  sleep 1
done
