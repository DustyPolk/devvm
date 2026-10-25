#!/bin/bash
# Boot a converted image (an OUT folder oci/build.sh wrote) as a throwaway Lima VM and run the checks
# devvm relies on: init system, cloud-init, user, sudo, a writable host mount and port forwarding.
# Usage: try.sh OUTDIR NAME     (the VM is left running; remove with: limactl delete -f NAME)
set -euo pipefail
out=$(cd "$1" && pwd -P) name=$2
disk=$out/disk.raw cmdline=$(cat "$out/cmdline" 2>/dev/null || true)
mnt=$(mktemp -d /tmp/devvm-try.XXXXXX)
echo hello-from-host > "$mnt/hello.txt"

cat > "$out/lima.yaml" <<EOF
vmType: vz
images:
  - location: $disk
    arch: $(uname -m | sed 's/arm64/aarch64/')
    kernel:
      location: $out/kernel
      cmdline: "$cmdline"
    initrd:
      location: $out/initrd
cpus: 4
memory: 4GiB
disk: 30GiB
mounts:
  - location: $mnt
    writable: true
containerd:
  system: false
  user: false
EOF

limactl delete -f "$name" >/dev/null 2>&1 || true
limactl create --tty=false --name="$name" "$out/lima.yaml" >/dev/null
s=$SECONDS
if limactl start --tty=false "$name" > "$out/start.log" 2>&1; then
  echo "start: ok in $((SECONDS - s))s"
else
  echo "start: FAILED after $((SECONDS - s))s (see $out/start.log and ${LIMA_HOME:-~/.lima}/$name/serialv.log)"
  grep -E 'level=(error|fatal)' "$out/start.log" | tail -3
  exit 1
fi

limactl shell --workdir "$mnt" "$name" sh -c '
  . /etc/os-release; echo "os: $PRETTY_NAME, kernel $(uname -r)"
  if command -v systemctl >/dev/null; then echo "init: systemd $(systemctl is-system-running)"
  else echo "init: openrc, failed services: $(rc-status --crashed 2>/dev/null | wc -l)"; fi
  echo "cloud-init: $(cloud-init status 2>/dev/null | tail -1)"
  echo "root: $(findmnt -no FSTYPE /), $(df -h / | awk "NR==2 {print \$4}") free"
  command -v docker >/dev/null && echo "docker: $(sudo docker run --rm hello-world 2>&1 | grep -m1 Hello || echo FAILED)"
  echo "user: $(id -un) uid=$(id -u) home=$HOME, sudo: $(sudo -n true && echo ok)"
  echo "mount: $(cat hello.txt), cwd=$(pwd)"; echo written-in-vm > from-vm.txt
  (python3 -m http.server 8765 >/dev/null 2>&1 || busybox httpd -f -p 8765) >/dev/null 2>&1 &
  sleep 1' 2>&1 | grep -v 'level='
echo "mount from host: $(cat "$mnt/from-vm.txt" 2>/dev/null || echo MISSING)"
for _ in 1 2 3 4 5 6 7 8; do
  code=$(curl -s -o /dev/null -w '%{http_code}' localhost:8765 || true)
  [[ $code != 000 ]] && break; sleep 1
done
echo "port forward: HTTP $code"
