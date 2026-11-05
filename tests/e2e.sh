#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]] [--every-feature] [--slots N | --slot N | --newest-base] [--golden]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images, and
#                builds base again from its recorded packages with the mirror unreachable;
#                project makes a VM from base in a project folder, opens it from a subfolder in that
#                subfolder, and checks its mounts, a file watcher in it (the VM's edits seen, none
#                of this machine's: Lima's mounts pass on no file events), no Docker, every
#                port through devvm's forwarder (never a port a program here has, nor a server on a
#                link's address), ssh by
#                name with pinned host keys, .git protection and shell, then devvm stop, opening it
#                again, restart, devvm size, devvm ports (a running VM's at once, never a listed port a
#                program here has, a bad list a usage error), a mistyped command refused with a suggestion,
#                devvm git-protect and rm, a VM from a devcontainer.json (its ports, variables, commands
#                once and at each start, a changed file, an image on this machine's registry refused), an agent VM's folders and history (devvm history export, rm's export), and SSH agent forwarding
#                (a throwaway agent's, and devvm's own with a throwaway key); clone makes a --clone
#                VM of a throwaway repo and checks its clone (pushed past an insteadOf), the remote, fetching, and rm's checks
#                for unfetched commits and uncommitted changes and the work it keeps, then a git worktree's clone; gc deletes a stale
#                image and an unused tool image, keeping base and a VM named like an image; tools resolves
#                the shared tools' newest versions (every other section has tests/e2e-mise.lock's), makes a node
#                VM and checks its shared tools, devvm tools, tools update, rm and add, devvm node an
#                unknown command and --tools ignored on it; network makes a VM with Claude Code and Codex, with
#                network policy and checks it reaches only its allowlist, devvm network, its log and rules, its tokens
#                stay on the host (devvm secret too), and the self-check, on relaunch too; devvm stop
#                stops its proxy; nix builds the nix layer behind a company proxy (no proxy left in its
#                image), makes a node VM with the nix layer of a flake project with network policy and checks Nix's
#                pins, store and trust, nix-daemon's proxy, nix run nixpkgs#hello through the shared Nix
#                cache (fetched by the host, a VM's writes and other requests refused, a second VM's
#                hits), sudo nix, the flake's dev shell through direnv before the shared node, a blocked
#                download named, and network off (no shared cache); link makes two small targets and
#                a client that forwards every port, and checks links between them, each check a row
#                of the design note's threat check: names alone (never the client's localhost, never
#                forwarded to this machine), two targets on one port, port 80, publish, unpublish,
#                add and rm on a running client, the sockets, the 256-connection cap, a program
#                holding a link's address, root loosening the client's sshd, a stopped and restarted
#                target, the target's rm and a VM made again, the three in a group (joined, a port
#                published to it, leaving, a VM refused whose hostname is a member's name), network policy on both and the policy rule's refusals,
#                a group's all-or-none refusals, the log, and nothing left behind (in the --from tier, an
#                Alpine client's link to port 80 too)
#     --only desktop
#                a base VM without a desktop (devvm desktop refused), then a VM with the desktop
#                layer and network policy whose desktop devvm
#                desktop serves on this machine's loopback alone: a wrong password refused, the
#                password let in, the VM's server on its user's own socket and no TCP port, the
#                clipboard off, the password in no file, log or process's arguments, no way out of the
#                VM; root's own server in its place (no password) still behind devvm's, the viewer's
#                clipboard never reaching it; --clipboard
#     --from     every section, with from, and every base image feature: the longest. from makes VMs
#                from node:24 (Debian 12, with node, python, the docker and nix layers and claude) and
#                alpine:3 (musl tools, the docker layer on OpenRC, the nix layer's daemon an OpenRC service,
#                through the proxy once network policy is on, the desktop layer's programs), each with every
#                feature, and builds base from ubuntu:26.04, with a
#                docker that isn't running; each image by its pinned digest
#     --only     just these sections; SECTIONS below
#     --clean    delete the sandbox's cached images and tool images first (default: reuse them
#                between runs)
#     --every-feature
#                base with every feature, as a run with from builds it (--slots passes it to each
#                slot of a run with from)
#     --slots N  the chosen sections spread over N slots (1-9; default 1), which run at once, each a
#                tests/e2e.sh --slot run of its own; their results and times are added up at the end
#     --slot N   run in slot N's sandbox (0-9; default 0, the sandbox below), its images as it has them
#     --newest-base
#                base as a user's first build makes it: from Ubuntu's newest cloud image (no
#                e2e_ubuntu_release) and the mirrors' newest packages, built from nothing in a sandbox
#                of its own (slot 0's paths with -newest, its ports +1000), whose images and package
#                cache it deletes first; images, project and tools unless --only says. A failure is
#                upstream drift, which its summary names with the release
#     --golden   also capture the golden fixtures the test stubs are checked against, into this
#                checkout's testdata/golden/<os>/, this machine's home, user name and timezone
#                replaced: images, real Lima's dummy instances (TestLimactlStubContract), limactl
#                --version, qemu-img info of base's disk (Linux), security's answer for an item
#                that isn't there and tmutil isexcluded (macOS); project, a running VM's limactl
#                list --json, and its overlay disk's qemu-img info (Linux)
# The sandbox: Lima home ~/.lt-e2e-go (or $DEVVM_E2E_LIMA_HOME), state under /tmp/devvm-go-e2e, Keychain
# service devvm-go-e2e and the lock /tmp/devvm-go-e2e.lock: slot 0's. Slot N's add -N to each, and
# 100 × N to each fixed port. It refuses to run against ~/.lima or devvm's own Lima home, deletes only
# the VMs it created, and uses no real tokens. devvm is built from this checkout into the sandbox,
# with its files embedded.
set -uo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sections_all="images project clone gc tools network desktop nix from link"
usage() { echo "usage: tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]] [--every-feature] [--slots N | --slot N | --newest-base] [--golden]  (sections: $sections_all)" >&2; exit 2; }
clean=false sel="images project clone gc tools network nix link" every=false slot=0 slots=1 newest=false chosen=false golden=false
while (( $# )); do
  case $1 in
    --clean) clean=true ;;
    --from)  sel=$sections_all chosen=true ;;
    --only)  [[ -n "${2:-}" ]] || usage; sel=$(tr ',' ' ' <<<"$2") chosen=true; shift
             for s in $sel; do [[ " $sections_all " == *" $s "* ]] || usage; done ;;
    --every-feature) every=true ;;
    --slots) [[ ${2:-} =~ ^[1-9]$ ]] || usage; slots=$2; shift ;;
    --slot)  [[ ${2:-} =~ ^[0-9]$ ]] || usage; slot=$2; shift ;;
    --newest-base) newest=true ;;
    --golden) golden=true ;;
    *) usage ;;
  esac
  shift
done
(( slots == 1 || slot == 0 )) || usage
# --newest-base: its own sandbox, never a slot's, and base built from nothing.
if $newest; then
  (( slots == 1 && slot == 0 )) || usage
  clean=true
  $chosen || sel="images project tools"
fi
want() { [[ " $sel " == *" $1 "* ]]; }
# A run with from builds base with every feature, which from checks, so its other sections run on that
# base too: their checks of base differ, and so do their timings ($tier).
features_on=false tier=plain
if want from || $every; then features_on=true tier=features; fi
# A newest base's run builds everything, so its times are kept apart from the cached runs'.
! $newest || tier=$tier-newest
mins() { printf '%dm%02ds' $(($1 / 60)) $(($1 % 60)); }
# A slot's sandbox: slot_work N, slot_lima N and slot_service N are its state folder, Lima home and
# Keychain service; slot 0's are the sandbox above, and slot N's add -N. Its lock is slot_work's
# with .lock: a symlink to the pid of the run holding it.
slot_work() { if (( $1 == 0 )); then echo /tmp/devvm-go-e2e; else echo "/tmp/devvm-go-e2e-$1"; fi; }
slot_lima() { local h=${DEVVM_E2E_LIMA_HOME:-$HOME/.lt-e2e-go}; if (( $1 == 0 )); then echo "$h"; else echo "$h-$1"; fi; }
slot_service() { if (( $1 == 0 )); then echo devvm-go-e2e; else echo "devvm-go-e2e-$1"; fi; }
# lock_take FILE: the lock FILE is this process's (the run's), else it says whose it is and fails. A
# lock whose process has ended is taken over.
lock_take() { local holder
  ln -s "$$" "$1" 2>/dev/null && return 0
  holder=$(readlink "$1" 2>/dev/null)
  if [[ -n $holder ]] && kill -0 "$holder" 2>/dev/null; then echo "e2e: $1 is held by process $holder, another run in that slot" >&2; return 1; fi
  rm -f "$1"; ln -s "$$" "$1" 2>/dev/null || { echo "e2e: couldn't take $1" >&2; return 1; }
}
# lock_drop FILE: the lock FILE, released when it's this process's.
lock_drop() { [[ $(readlink "$1" 2>/dev/null) != "$$" ]] || rm -f "$1"; }

# Every host command and perl module the chosen sections use, checked before anything is built:
# each missing one is named with what installs it (Homebrew on macOS, apt-get or dnf on Linux), then
# e2e stops. Commands every macOS and Linux system has (bash, awk, sed, grep, find, coreutils) aren't
# checked.
missing=()
host_os=$(uname -s)
# pkg BREW APT DNF: the command that installs a package, with this system's package manager.
pkg() {
  if [[ $host_os == Darwin ]]; then echo "brew install $1"
  elif command -v apt-get >/dev/null 2>&1; then echo "sudo apt-get install $2"
  elif command -v dnf >/dev/null 2>&1; then echo "sudo dnf install $3"
  else echo "your distribution's package for it ($2 on Debian, $3 on Fedora)"; fi
}
# need CMD BREW APT DNF: CMD is on PATH, else it's missing, with what installs it.
need() { command -v "$1" >/dev/null 2>&1 || missing+=("$1: $(pkg "$2" "$3" "$4")"); }
# need_pm MODULE BREW APT DNF: perl loads MODULE (Fedora splits perl into a package per module).
need_pm() { perl -M"$1" -e 1 >/dev/null 2>&1 || missing+=("perl's $1: $(pkg "$2" "$3" "$4")"); }
# The VM's limactl and devvm's build (go, or mise, which installs the repo's pinned go).
if [[ $host_os == Darwin ]]; then need limactl lima - -
else command -v limactl >/dev/null 2>&1 || missing+=("limactl: Lima's release tarball (https://lima-vm.io/docs/installation/)"); fi
command -v go >/dev/null 2>&1 || command -v mise >/dev/null 2>&1 ||
  missing+=("go: mise (https://mise.jdx.dev), then mise install in the repo")
need git git git git
need pgrep proctools procps procps-ng
need pkill proctools procps procps-ng
# OpenSSL first on PATH: the CA checks' HTTPS server is openssl s_server, and LibreSSL's (macOS's
# /usr/bin/openssl) cuts the VM's OpenSSL client off mid-request.
ossl=$(openssl version 2>/dev/null)
if [[ $ossl != OpenSSL* ]]; then
  fix='brew install openssl@3, then put $(brew --prefix openssl@3)/bin first on PATH'
  [[ $host_os == Darwin ]] || fix=$(pkg openssl@3 openssl openssl)
  missing+=("openssl${ossl:+ (OpenSSL, not $ossl)}: $fix")
fi
# Ports and listeners on this machine: nc, and lsof on macOS or ss on Linux.
if want project || want network || want desktop || want link; then need nc netcat netcat-openbsd nmap-ncat; fi
if want project || want network || want link; then
  if [[ $host_os == Darwin ]]; then need lsof lsof - -; else need ss - iproute2 iproute; fi
fi
if want network || want link; then need curl curl curl curl; fi
if want project || want clone || want link; then need ssh openssh openssh-client openssh-clients; fi
if want project; then
  for c in ssh-agent ssh-add ssh-keygen; do need "$c" openssh openssh-client openssh-clients; done
  need tar gnu-tar tar tar
fi
# The sandbox check binds ports with python3 (macOS only).
if want network && [[ $host_os == Darwin ]]; then need python3 python - -; fi
if want link && ! command -v shasum >/dev/null 2>&1; then need sha256sum coreutils coreutils coreutils; fi
# perl: every section reads VMs' vm.json with JSON::PP (vm_state); no_leak reads disks with
# File::Find (images, nix, from), and a program on this machine holds a VM's port with
# IO::Socket::INET (project).
need perl perl perl perl
if command -v perl >/dev/null 2>&1; then need_pm JSON::PP perl perl perl-JSON-PP; fi
if want images || want project || want nix || want from; then
  if command -v perl >/dev/null 2>&1; then
    if want images || want nix || want from; then need_pm File::Find perl perl perl-File-Find; fi
    if want project; then need_pm IO::Socket::INET perl perl-base perl-IO; fi
  fi
fi
if (( ${#missing[@]} )); then
  echo "e2e: the chosen sections need these, which this machine lacks:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi

# The times of each section's checks in its last complete run, one file for each, kept in slot 0's
# state for every slot ("label<TAB>seconds[<TAB>retried]", and SECTION.sum, the checksum of the
# e2e.sh that ran it). A run with every feature checks base's features too, so it keeps its own.
tdir=$(slot_work 0)/timings/$tier
# secs_of SECTION: the seconds its checks took in its last complete run, else a guess.
secs_of() { local t
  t=$(awk -F'\t' '{ s += $2 } END { if (NR) print s }' "$tdir/$1" 2>/dev/null)
  if [[ -n $t ]]; then echo "$t"; return; fi
  case $1 in from) echo 1800 ;; project|nix|link) echo 900 ;; images|tools|network) echo 600 ;; desktop) echo 300 ;; *) echo 120 ;; esac
}

# Slots (--slots N): the chosen sections spread over N slots, each a tests/e2e.sh --slot run of its
# own, all at once. plan_slots: $plan, each slot's sections, one line each ("SLOT SECTION..."):
# longest first (secs_of), each to the slot with the least time so far, the lowest on a tie; link
# goes with from when both run, as link's Alpine client in the --from tier starts from from's image.
# A slot runs its sections in the usual order; a slot without one isn't started.
# plan_units: what plan_slots places, one line each: "SECONDS SECTION", or "SECONDS from+link".
plan_units() { local s
  for s in $sel; do
    case $s in
      link) want from || echo "$(secs_of link) link" ;;
      from) if want link; then echo "$(( $(secs_of from) + $(secs_of link) )) from+link"; else echo "$(secs_of from) from"; fi ;;
      *) echo "$(secs_of "$s") $s" ;;
    esac
  done
}
plan_slots() { local unit w k best load=() mine=() s line
  for ((k = 0; k < slots; k++)); do load[k]=0 mine[k]=""; done
  while read -r w unit; do
    best=0; for ((k = 1; k < slots; k++)); do (( load[k] >= load[best] )) || best=$k; done
    load[best]=$((load[best] + w)) mine[best]="${mine[best]} ${unit//+/ }"
  done < <(plan_units | sort -k1,1nr -s)
  plan=""
  for ((k = 0; k < slots; k++)); do
    line=""; for s in $sections_all; do [[ " ${mine[k]} " != *" $s "* ]] || line="$line $s"; done
    [[ -z $line ]] || plan="$plan$k$line"$'\n'
  done
}
# clone SRC DST: DST a clone of SRC (a file or a folder) that shares its blocks: on macOS an APFS
# clone, on one APFS volume alone (cp -c makes a whole copy anywhere else); on Linux a reflink (cp
# --reflink=always, which fails where it can't). Where neither can be made it fails, leaving no DST.
clone() {
  if [[ $host_os == Darwin ]]; then
    apfs_same "$1" "$(dirname "$2")" && cp -c -R -p "$1" "$2" 2>/dev/null && return 0
  else
    cp -a --reflink=always "$1" "$2" 2>/dev/null && return 0
  fi
  rm -rf "$2"; return 1
}
# apfs_same A B: A and B are on one APFS volume.
apfs_same() { local a b
  a=$(df "$1" 2>/dev/null | awk 'NR == 2 { print $1 }') b=$(df "$2" 2>/dev/null | awk 'NR == 2 { print $1 }')
  [[ -n $a && $a == "$b" ]] && mount | awk -v d="$a" '$1 == d && /\(apfs/ { f = 1 } END { exit !f }'
}
# seed_packages SRC DST: base's package cache, each <sha256>.deb SRC has and DST lacks cloned
# (clone), its sets/ (small files, the packages each base key recorded) copied. It fails at the
# first .deb that can't be cloned.
seed_packages() { local f
  [[ -d $1 ]] || return 0
  for f in "$2" "$2/sets"; do [[ -d $f ]] || mkdir -m 700 "$f"; done
  for f in "$1"/*.deb; do
    [[ -f $f && ! -e $2/${f##*/} ]] || continue
    clone "$f" "$2/${f##*/}" || return 1
  done
  for f in "$1"/sets/*.json; do
    [[ -f $f && ! -e $2/sets/${f##*/} ]] || continue
    cp -p "$f" "$2/sets/"
  done
}
# seed K: what slot 0 has and slot K lacks, cloned into slot K's sandbox (clone), never copied whole:
# each image's Lima instance (then its record, a small file, copied), each tool image (then its
# record), devvm's caches (the --from digests, a small file, copied) and base's package cache
# (seed_packages). What can't be cloned is said, and slot K builds its own.
seed() {
  local k=$1 l0 lk d0 dk r key f n=0 t=0 cold=""
  l0=$(slot_lima 0) lk=$(slot_lima "$k") d0=$(slot_work 0)/data/devvm dk=$(slot_work "$k")/data/devvm
  mkdir -p "$lk" "${dk%/devvm}"
  for f in "$dk" "$dk/images" "$dk/tools" "$dk/cache"; do [[ -d $f ]] || mkdir -m 700 "$f"; done
  for r in "$d0"/images/*/record.json; do
    [[ -f $r ]] || continue
    key=${r%/record.json}; key=${key##*/}
    [[ -d $l0/devvm-img-$key && ! -e $lk/devvm-img-$key && ! -e $dk/images/$key ]] || continue
    if ! clone "$l0/devvm-img-$key" "$lk/devvm-img-$key"; then cold="$cold images"; break; fi
    cp -R -p "$d0/images/$key" "$dk/images/$key" && n=$((n + 1))
  done
  for f in "$d0"/tools/*; do
    [[ -f $f && -f $f.json && ! -e $dk/tools/${f##*/} ]] || continue
    if ! clone "$f" "$dk/tools/${f##*/}"; then cold="$cold tool-images"; break; fi
    cp -p "$f.json" "$dk/tools/" && t=$((t + 1))
  done
  [[ ! -f $d0/cache/from-digests.json || -e $dk/cache/from-digests.json ]] || cp -p "$d0/cache/from-digests.json" "$dk/cache/"
  for f in "$d0"/cache/*/; do
    f=${f%/}; [[ -d $f && ! -e $dk/cache/${f##*/} ]] || continue
    clone "$f" "$dk/cache/${f##*/}" || { cold="$cold caches"; break; }
  done
  seed_packages "$d0/packages" "$dk/packages" || cold="$cold packages"
  echo "e2e: slot $k: $n images and $t tool images cloned from slot 0"
  [[ -z $cold ]] || echo "e2e: slot $k: these can't be cloned here, so it builds its own:$cold"
}
# run_slots: the --slots run. It takes every slot's lock, seeds each other slot from slot 0's (seed;
# with --clean, each slot deletes its own instead), lets the locks go and starts the slots, each
# taking its own lock. Their output is shown as it comes, each line led by its slot ("[1] "), and
# kept in the slot's e2e.log; their results ($work/summary) are added up at the end. TERM or INT
# stops every slot with TERM, as a slot's own run ignores INT (it runs in the background).
run_slots() {
  local k s line pids=() logs=() parts=() seen=() rcs=() extra=() started=$SECONDS stopping=false signalled=false alive n
  local pass=0 fail=0 skipped=0 retried=() failed=() short=() key val p f sk r used=0 ok=true
  plan_slots
  for ((k = 0; k < slots; k++)); do mkdir -p "$(slot_work "$k")"; lock_take "$(slot_work "$k").lock" || exit 1; done
  trap 'for ((k = 0; k < slots; k++)); do lock_drop "$(slot_work "$k").lock"; done' EXIT
  if ! $clean; then
    for ((k = 1; k < slots; k++)); do [[ $plan != *$'\n'"$k "* ]] || seed "$k"; done
  fi
  for ((k = 0; k < slots; k++)); do lock_drop "$(slot_work "$k").lock"; done
  ! $clean || extra+=(--clean)
  ! $features_on || extra+=(--every-feature)
  ! $golden || extra+=(--golden)
  # Go found once, its folder first on PATH: slots that each ran mise exec at once would each have
  # mise install the repo's tools, over one another.
  if ! command -v go >/dev/null; then
    f=$(cd "$root" && mise exec -- go version >/dev/null 2>&1; mise which go 2>/dev/null) && [[ -x $f ]] || { echo "e2e: couldn't find go through mise" >&2; exit 1; }
    PATH=$(dirname "$f"):$PATH
  fi
  trap 'stopping=true' INT TERM
  while read -r k line; do
    [[ -n $k ]] || continue
    parts[k]=$line logs[k]=$(slot_work "$k")/e2e.log seen[k]=0
    rm -f "$(slot_work "$k")/summary"
    echo "e2e: slot $k runs ${line// /, }"
    "$root/tests/e2e.sh" --slot "$k" --only "${line// /,}" ${extra[@]+"${extra[@]}"} </dev/null >"${logs[k]}" 2>&1 &
    pids[k]=$!; used=$((used + 1))
  done <<<"$plan"
  # show K [all]: slot K's new lines, whole ones (all: the rest too, once it has ended).
  show() { local n
    if [[ ${2:-} == all ]]; then n='$'; else n=$(wc -l < "${logs[$1]}" | tr -d ' '); (( n > seen[$1] )) || return 0; fi
    sed -n "$((seen[$1] + 1)),${n}p" "${logs[$1]}" | awk -v k="$1" '{ print "[" k "] " $0 }'
    [[ $n == '$' ]] || seen[$1]=$n
  }
  while :; do
    alive=false
    for k in "${!pids[@]}"; do kill -0 "${pids[k]}" 2>/dev/null && alive=true; show "$k"; done
    $alive || break
    if $stopping && ! $signalled; then for k in "${!pids[@]}"; do kill -TERM "${pids[k]}" 2>/dev/null; done; signalled=true; fi
    sleep 1
  done
  for k in "${!pids[@]}"; do wait "${pids[k]}"; rcs[k]=$?; show "$k" all; done
  echo
  for k in "${!pids[@]}"; do
    f=$(slot_work "$k")/summary
    if [[ ! -f $f ]]; then ok=false; echo "e2e: slot $k (${parts[k]// /, }) ended without its results (exit ${rcs[k]}): look at its lines above, or ${logs[k]}"; continue; fi
    p=0 r=0 sk=0 n=0
    while IFS=$'\t' read -r key val; do
      case $key in
        pass) p=$val ;; fail) r=$val ;; skipped) sk=$val ;; elapsed) n=$val ;;
        failed) failed+=("$val") ;; retried) retried+=("$val") ;; short) short+=("slot $k: $val") ;;
      esac
    done < "$f"
    pass=$((pass + p)) fail=$((fail + r)) skipped=$((skipped + sk))
    (( rcs[k] == 0 )) || ok=false
    echo "e2e: slot $k (${parts[k]// /, }): $p passed, $r failed$( (( sk == 0 )) || echo ", $sk skipped") in $(mins "$n")"
  done
  echo "e2e: $pass passed, $fail failed$( (( skipped == 0 )) || echo ", $skipped skipped")$( (( ${#retried[@]} == 0 )) || echo ", ${#retried[@]} retried") in $(mins $((SECONDS - started))) on $used slots${failed[*]+: ${failed[*]}}"
  for line in ${retried[@]+"${retried[@]}"}; do echo "e2e: retried once: $line"; done
  for line in ${short[@]+"${short[@]}"}; do echo "e2e: FAILED: $line"; done
  ! $signalled || exit 130
  $ok && (( fail == 0 ))
}
if (( slots > 1 )); then run_slots; exit; fi

# Its own Lima home (other sandboxes' images aren't its own), for devvm and for its own limactl calls.
export DEVVM_LIMA_HOME
DEVVM_LIMA_HOME=$(slot_lima "$slot")
work=$(slot_work "$slot")
export DEVVM_SECRET_SERVICE
DEVVM_SECRET_SERVICE=$(slot_service "$slot")   # its Keychain items: fake tokens only, deleted at the end
# --newest-base's sandbox: slot 0's paths with -newest, so the cached pinned images stay as they are.
if $newest; then DEVVM_LIMA_HOME=$DEVVM_LIMA_HOME-newest work=$work-newest DEVVM_SECRET_SERVICE=$DEVVM_SECRET_SERVICE-newest; fi
export LIMA_HOME=$DEVVM_LIMA_HOME
export XDG_DATA_HOME=$work/data XDG_CONFIG_HOME=$work/config
# Its lock, held until it ends: one run at a time in a slot.
mkdir -p "$work"
lock_take "$work.lock" || exit 1
trap 'lock_drop "$work.lock"' EXIT
rm -f "$work/summary"   # its results, for a --slots run, written at its end
# Its fixed ports: P<port> is slot 0's port plus 100 × the slot, so slots running at once never
# share one: slot 0's ranges (47160 to 47199, 47240 to 47249, 47613, 18765 and 18766) never meet
# another slot's. --newest-base's are slot 0's plus 1000, past slot 9's. Exported for the checks'
# own shells.
po=$((slot * 100))
! $newest || po=1000
P47160=$((47160 + po)) P47161=$((47161 + po)) P47162=$((47162 + po)) P47163=$((47163 + po)) P47164=$((47164 + po))
P47170=$((47170 + po)) P47171=$((47171 + po)) P47172=$((47172 + po)) P47173=$((47173 + po)) P47174=$((47174 + po))
P47175=$((47175 + po)) P47176=$((47176 + po)) P47177=$((47177 + po)) P47178=$((47178 + po))
P47180=$((47180 + po)) P47181=$((47181 + po)) P47182=$((47182 + po))
P47195=$((47195 + po)) P47196=$((47196 + po)) P47197=$((47197 + po))
P47240=$((47240 + po)) P47241=$((47241 + po)) P47242=$((47242 + po)) P47243=$((47243 + po)) P47244=$((47244 + po))
P47245=$((47245 + po)) P47246=$((47246 + po)) P47248=$((47248 + po)) P47249=$((47249 + po))
P47613=$((47613 + po)) P18765=$((18765 + po)) P18766=$((18766 + po))
export P47160 P47161 P47162 P47163 P47164 P47170 P47171 P47172 P47173 P47174 P47175 P47176 P47177 P47178 \
  P47180 P47181 P47182 P47195 P47196 P47197 P47240 P47241 P47242 P47243 P47244 P47245 P47246 P47248 P47249 \
  P47613 P18765 P18766
unset DEVVM_MESSAGES DEVVM_UBUNTU_URL DEVVM_HOST_MISE CLAUDE_CODE_OAUTH_TOKEN DEVVM_FROM
unset SSH_AUTH_SOCK   # your SSH agent is never reached: checks that need one start their own
unset FORCE_COLOR CLICOLOR_FORCE   # devvm's output stays plain where the checks read it
mkdir -p "$LIMA_HOME"
for real in "$HOME/.lima" "$HOME/.local/share/devvm/lima"; do
  real=$(cd "$real" 2>/dev/null && pwd -P || echo "$real")
  [[ "$(cd "$LIMA_HOME" && pwd -P)" != "$real" ]] || { echo "e2e: refusing to use your real Lima home ($real)" >&2; exit 1; }
done

# devvm is built from this checkout into the sandbox, its files embedded (so editing the repo mid-run
# is harmless).
src=$work/src
rm -rf "$src"; mkdir -p "$src" "$XDG_CONFIG_HOME/devvm/layers" "$XDG_DATA_HOME"
gocmd=(go); command -v go >/dev/null || gocmd=(mise exec -- go)
# go_e2e TEST PACKAGE [VAR=VALUE...]: the Go test TEST in internal/PACKAGE, run from the checkout
# with each VAR set (exported by the shell, so no value is in a process's arguments), its output
# printed; it passes when TEST does. Each Go e2e test skips unless its variables are set.
go_e2e() {
  local test=$1 pkg=$2 kv; shift 2
  (cd "$root" && for kv in "$@"; do export "${kv?}"; done
   CGO_ENABLED=0 "${gocmd[@]}" test -count=1 -v -run "^$test\$" "./internal/$pkg") >"$work/go-e2e.log" 2>&1
  cat "$work/go-e2e.log"; grep -q -- "^--- PASS: $test (" "$work/go-e2e.log"
}
# golden NAME CMD...: with --golden, CMD's output and errors as the golden fixture NAME, in
# testdata/golden/<os>/, this machine's home, user name, full name and timezone replaced, and a
# VM's environment made up (internal/backend/lima's anonymous does the same). A capture isn't a
# check, so a run without --golden runs as many checks; one that fails says so and keeps the
# fixture it had.
golden_dir=$root/testdata/golden/$(uname -s | tr '[:upper:]' '[:lower:]')
golden() { local name=$1 f; shift
  $golden || return 0
  f=$golden_dir/$name
  mkdir -p "$golden_dir"
  if "$@" </dev/null 2>&1 | GOLDEN_HOME=$HOME GOLDEN_USER=$(id -un) perl -pe '
      s/\Q$ENV{GOLDEN_HOME}\E/\/home\/me/g; s/\b\Q$ENV{GOLDEN_USER}\E\b/me/g;
      s/"comment":"[^"]*"/"comment":"me"/g; s/"timezone":"[^"]*"/"timezone":"UTC"/g;
      s/"env":\{[^{}]*\}/"env":{"NAME":"value"}/g' >"$f.new"; then
    mv "$f.new" "$f"; echo "golden: $name"
  else
    rm -f "$f.new"; echo "golden: capturing $name failed"
  fi
}
# A test build: DEVVM_UNREVIEWED and the other test overrides count in it
# (never in a release).
testbuild=(build -ldflags "-X github.com/w3cj/devvm/internal/testbuild.testBuild=yes")
(cd "$root" && CGO_ENABLED=0 "${gocmd[@]}" "${testbuild[@]}" -o "$src/devvm" ./cmd/devvm) || { echo "e2e: couldn't build devvm" >&2; exit 1; }
# The features are in base's key, so only a run with from (which checks them) builds them all; every
# other selection shares one smaller base. config.toml chooses them, so nothing asks.
features=""
if $features_on; then for f in "$root"/assets/features/*.sh; do features="$features $(basename "$f" .sh)"; done; fi
toml=""
for f in $features; do toml="$toml${toml:+, }\"$f\""; done
printf 'features = [%s]\n' "$toml" > "$XDG_CONFIG_HOME/devvm/config.toml"
D=$src/devvm
# The dated release of Ubuntu's cloud image that base starts from here (scripts/bump.sh e2e-ubuntu),
# in place of the newest a user's devvm pins, so base stays cached between runs. devvm still checks
# its signed checksums.
e2e_ubuntu_release=release-20261016
export DEVVM_UBUNTU_RELEASE=$e2e_ubuntu_release
# base_release: the release base is to pin. --newest-base gives devvm no release, so it pins the
# newest, as a user's first build does: the newest Ubuntu lists now, read here as
# scripts/bump.sh e2e-ubuntu reads it, which the images section checks devvm pinned and booted.
base_release=$e2e_ubuntu_release
if $newest; then
  unset DEVVM_UBUNTU_RELEASE
  base_release=$(curl -fsSL --retry 3 https://cloud-images.ubuntu.com/releases/resolute/ | grep -o 'release-[0-9]\{8\}' | sort -ru | head -1)
  [[ -n $base_release ]] || { echo "e2e: couldn't read the newest release from Ubuntu's listing of its cloud images" >&2; exit 1; }
  echo "e2e: newest base: Ubuntu's cloud image $base_release (e2e pins $e2e_ubuntu_release) and the mirrors' newest packages, in $work"
fi
# The container images the --from checks start from, by digest, so the bases made from them stay
# cached until these pins move (scripts/bump.sh e2e-from). One check keeps a tag, so devvm's registry
# lookup is checked too: the company proxy's alpine:3.21, converted each run anyway.
from_node_digest=sha256:e7add2a490bb17d97b485b8641bd88455fff7839ade0f5eba493c3508124d413
from_alpine_digest=sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
from_ubuntu_digest=sha256:6a06111c3b0a8600feadb270aec771dc239bf407b25625f93f44ab00e11f4d3c
from_node=node:24@$from_node_digest from_alpine=alpine:3@$from_alpine_digest
from_ubuntu=ubuntu:26.04@$from_ubuntu_digest
images=$XDG_DATA_HOME/devvm/images   # the image records, one per key
layers=$XDG_CONFIG_HOME/devvm/layers
vms=$XDG_DATA_HOME/devvm/vms         # the VMs' folders, one per VM, named by its name's hash
# vmdir VM: the folder devvm keeps a VM's files in: its state, policy, run/ and logs/. A
# project VM's (devvm-FOLDER-HASH) is vms/HASH; a build VM's is run/builds/NAME.
vmdir() { if [[ $1 =~ ^devvm-.+-[0-9a-f]{14}$ ]]; then echo "$XDG_DATA_HOME/devvm/vms/${1: -14}"
  else echo "$XDG_DATA_HOME/devvm/run/builds/$1"; fi; }
export -f vmdir
# vm_state DIR_OR_FILE: a VM's state (DIR's vm.json, or FILE) as sorted KEY=VALUE lines, as the
# CLI tests' vm_state prints it: a section's fields dotted (net.on=true), a list of strings
# space-separated, a list of objects by index (links.links.0.name=db), strings as they are and
# anything else as JSON. Nothing for a VM without one.
vm_state() {
  local f=$1; [[ ! -d $f ]] || f=$f/vm.json
  [[ -f $f ]] || return 0
  perl -MJSON::PP -e '
    local $/; open my $fh, "<", $ARGV[0] or exit 1; my $v = JSON::PP->new->decode(<$fh>);
    my $js = JSON::PP->new->canonical->allow_nonref; my @l;
    sub flat { my ($p, $v) = @_;
      if (ref $v eq "HASH") { flat($p eq "" ? $_ : "$p.$_", $v->{$_}) for keys %$v; return }
      if (ref $v eq "ARRAY") { my @w; my $i = 0;
        for my $e (@$v) { if ($js->encode($e) =~ /^"/) { push @w, $e } else { flat("$p.$i", $e) } $i++ }
        push @l, "$p=" . join(" ", @w) if @w || !@$v; return }
      my $e = $js->encode($v); push @l, "$p=" . ($e =~ /^"/ ? $v : $e) }
    flat("", $v); print "$_\n" for sort @l;' "$f"
}
export -f vm_state
# st_get DIR_OR_FILE KEY: one KEY's value in a VM's state (vm_state), nothing when it has none.
st_get() { vm_state "$1" | sed -n "s/^$2=//p"; }
export -f st_get
# st_all: every VM's state, one vm_state after another.
st_all() { local f; for f in "$vms"/*/vm.json; do vm_state "$f"; done; }
# vm_set FILE KEY JSON: sets KEY (dotted: net.on) of a VM's vm.json to the JSON value, keeping the
# rest, and writes the file back whole, the user's alone.
vm_set() {
  perl -MJSON::PP -e '
    my ($f, $k, $j) = @ARGV; local $/; open my $fh, "<", $f or die "$f: $!\n"; my $js = JSON::PP->new->allow_nonref;
    my $v = $js->decode(<$fh>); close $fh; my @p = split /\./, $k; my $last = pop @p; my $o = $v;
    $o = ($o->{$_} //= {}) for @p; $o->{$last} = $js->decode($j);
    open $fh, ">", "$f.tmp" or die "$f: $!\n"; print $fh JSON::PP->new->pretty->canonical->encode($v); close $fh;
    chmod 0600, "$f.tmp"; rename "$f.tmp", $f or die "$f: $!\n";' "$1" "$2" "$3"
}
projects=$work/p                     # the project folders, this run's
share=$XDG_DATA_HOME/devvm/tools     # the shared tool images
lock=$XDG_DATA_HOME/devvm/mise.lock  # the shared tools' versions
export DEVVM_NO_TOOL_CHECK=1         # never checked for newer ones but by the tools section
# lockv TOOL: its version in the lock.
lockv() { awk -v t="[[tools.$1]]" '$0 == t { f = 1; next } f && /^version = / { gsub(/"/, "", $3); print $3; exit }' "$lock"; }
# The shared tools' versions are pinned (tests/e2e-mise.lock; scripts/bump.sh e2e-tools), so the tool
# images built for them stay cached between runs. Only the tools section resolves upstream's newest,
# and pin_lock puts the pins back after it. The pinned lock's first line is the SHA-256 of the
# shared-tools/mise.toml it was resolved from: for another, the run stops here.
e2e_lock=$root/tests/e2e-mise.lock
pin_lock() { cp "$e2e_lock" "$lock" && cp "$root/assets/shared-tools/mise.toml" "$lock.toml"; }
lock_for=$(sed -n '1s/^# devvm e2e: shared-tools\/mise\.toml sha256:\([0-9a-f]*\)$/\1/p' "$e2e_lock" 2>/dev/null)
if [[ -z $lock_for || $lock_for != "$({ sha256sum 2>/dev/null || shasum -a 256; } < "$root/assets/shared-tools/mise.toml" | cut -c1-64)" ]]; then
  echo "e2e: tests/e2e-mise.lock isn't resolved from this assets/shared-tools/mise.toml: run scripts/bump.sh e2e-tools" >&2; exit 1
fi
[[ -d $XDG_DATA_HOME/devvm ]] || mkdir -m 700 "$XDG_DATA_HOME/devvm"
pin_lock || { echo "e2e: couldn't put the pinned tool lock in place" >&2; exit 1; }
# m NAME [name=value...]: a message's text, from devvm, so checks never repeat its wording. Exported
# for the checks' own shells.
m() { "$D" _msg "$@"; }
export -f m; export D

created=()   # VMs this run made; the only ones it deletes
# ps_watch: every host process's command line, sampled every 0.1 s into $work/ps.log until
# ps_unwatch, for checking a secret was in none while devvm ran. ps_unwatch removes the watcher's
# $work/ps.on and waits for it to end after its current sample: killing it instead would leave that
# ps running, which could then record the next check's command line, the secret in its arguments.
watcher=""
ps_watch() {
  : > "$work/ps.log"; : > "$work/ps.on"
  (while [[ -e $work/ps.on ]]; do ps -axww -o command= >> "$work/ps.log" 2>/dev/null; sleep 0.1; done) &
  watcher=$!
}
ps_unwatch() { [[ -z $watcher ]] || { rm -f "$work/ps.on"; wait "$watcher" 2>/dev/null; watcher=""; }; }
cleanup() {
  local n
  for n in ${created[@]+"${created[@]}"}; do
    limactl delete -f "$n" >/dev/null 2>&1
    [[ $n != devvm-img-?* ]] || rm -rf "${images:?}/${n#devvm-img-}"
    rm -rf "$(vmdir "$n")"
  done
  rm -f "$layers"/e2e*.sh "$layers"/e2e*.policy
  [[ "$(uname -s)" != Darwin ]] || security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a github >/dev/null 2>&1
  rm -f "$XDG_DATA_HOME/devvm/secrets/github" "$work/net-home/Library/Keychains"   # the link, never the folder
  [[ -z ${listener:-} ]] || kill "$listener" 2>/dev/null
  [[ -z ${corp:-} ]] || kill "$corp" 2>/dev/null   # the company proxy's stand-in
  [[ -z ${hostprog:-} ]] || kill "$hostprog" 2>/dev/null
  [[ -z ${ca_server:-} ]] || kill "$ca_server" 2>/dev/null
  [[ -z ${cad:-} ]] || rm -rf "$cad" "$cafolder"   # the run's own CA, and devvm's CA folder in the sandbox
  ps_unwatch
  [[ -z ${desk:-} ]] || kill "$desk" 2>/dev/null   # a devvm desktop
  [[ -z ${agent_pid:-} ]] || kill "$agent_pid" 2>/dev/null
  [[ -z ${user_agent_pid:-} ]] || kill "$user_agent_pid" 2>/dev/null
  pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock" 2>/dev/null   # devvm's own, in the sandbox
  pkill -f "[_]ports -vm .* -lifeline $DEVVM_LIMA_HOME/" 2>/dev/null  # VMs' forwarders, the sandbox's
  rm -rf "$src" "$work/ssh" "$work/net-home" "$work/agent-home" "$work/agent-ssh" "$XDG_DATA_HOME/devvm/agent-keys.json" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/export.list" "$work/export" "$XDG_DATA_HOME/devvm/exports" "$work/nodocker" "$work/docker-calls" "$work/from-home" \
    "$work/listener.json" "$work/ps.log" "$work/lima-bypass" "$work/go-e2e.log" \
    "$work/corp.auth" "$work/corp.json" "$work/corp.policy" "$work/corp.jsonl" "$work/corp.log"
  # The project folders' .git is locked (macOS): unlocked to go.
  if [[ -d $projects ]]; then
    [[ "$(uname -s)" != Darwin ]] || chflags -R nouchg "$projects" 2>/dev/null
    rm -rf "$projects"
  fi
  lock_drop "$work.lock"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# Progress: each result shows [n/N], how long it took (since the previous check, so builds count),
# and the time left as the last complete run of each of its sections measured it ($tdir/SECTION). A
# heartbeat every minute says what's happening during long steps.
# Each section's times are a file of their own, so a run of any selection, in any slot, estimates
# from and compares with the sections it runs. SECTION.sum is the checksum of the e2e.sh that made
# it: only a run of that same script compares the section's count with it, as an edited one may have
# renamed, added or dropped checks. A run's times go to $work/timings.new/SECTION first.
mkdir -p "$tdir"; rm -rf "$work/timings.new"; mkdir -p "$work/timings.new"
total=0
for s in $sel; do
  n=$(wc -l 2>/dev/null < "$tdir/$s" | tr -d ' ')
  if [[ ${n:-0} -gt 0 ]]; then total=$((total + n)); else total="?"; break; fi
done
script_sum=$(cksum < "$root/tests/e2e.sh" | tr -s ' \t' ' ')
cur="" later=0 short=()
# begin SECTION: SECTION is chosen, and it begins, the one before it ending (section_end): its checks'
# times go to $work/timings.new/SECTION, and $later is what the chosen sections after it took.
begin() { local s after=false t
  want "$1" || return 1
  section_end
  cur=$1 later=0; : > "$work/timings.new/$cur"
  for s in $sections_all; do
    if $after && want "$s"; then t=$(awk -F'\t' '{ s += $2 } END { print s + 0 }' "$tdir/$s" 2>/dev/null); later=$((later + ${t:-0})); fi
    [[ $s != "$cur" ]] || after=true
  done
}
# section_end: the section running ends. Its times become its file, unless it ran fewer checks than
# the last complete run of this same e2e.sh: a script error ended it early, which $short says.
section_end() { local n m
  [[ -n $cur ]] || return 0
  n=$(wc -l < "$work/timings.new/$cur" | tr -d ' ') m=$(wc -l 2>/dev/null < "$tdir/$cur" | tr -d ' ')
  if [[ $(cat "$tdir/$cur.sum" 2>/dev/null) == "$script_sum" && ${m:-0} -gt 0 ]] && (( n < m )); then
    short+=("$cur ran $n checks, but its last complete run ran $m: it ended early (look for errors above)")
  else
    mv "$work/timings.new/$cur" "$tdir/$cur.new" && mv "$tdir/$cur.new" "$tdir/$cur" && echo "$script_sum" > "$tdir/$cur.sum"
  fi
  cur=""
}
started=$SECONDS ncheck=0
date +%s > "$work/mark"
# hb_line: the heartbeat's line: how long since the last check, and the newest output, a check's or
# devvm's own run log.
hb_line() {
  local esc=$'\033' s f
  s=$(( $(date +%s) - $(cat "$work/mark") ))
  # shellcheck disable=SC2012  # names without spaces
  f=$(ls -1t "$work/last.log" "$work/check.log" "$XDG_DATA_HOME"/devvm/logs/runs/*.log 2>/dev/null | head -1)
  printf '... %s since the last check: %s\n' "$(mins "$s")" \
    "$( [[ -f $f ]] && tail -1 "$f" | tr -d '\r' | sed "s/${esc}\[[0-9;?]*[A-Za-z]//g" | cut -c1-100)"
}
# Time limits: each check's command gets limit_of its label, from when it starts; the steps between
# two checks (launches, which may build images) get limit_build. window LIMIT KIND opens one (KIND
# check or setup): $work/deadline is "END LIMIT KIND", and $work/keep the processes already running
# (descendants), which the heartbeat leaves alone. Past END, the heartbeat writes $work/timedout (the
# deadline's line, then why, with its own last line), sends every other process of the run TERM, as
# Ctrl+C would, then after 30 s KILL each second, to whatever is left or new, until the next window.
limit_floor=120 limit_default=600 limit_build=3600
# builds: the checks whose own command can build base or a --from base (each run, or when its
# cached image is stale), so 3 times their last time, cached, isn't enough.
builds=(
  "build base"
  "base builds from ubuntu:26.04"
  "behind a company proxy, --from's conversion and the builds on it reach it through devvm's own proxies"
  "built behind a company proxy, the layer leaves no proxy settings for nix-daemon in its image"
)
# limit_of LABEL: 3 times the check's longest time in its section's last complete run, at least
# limit_floor; limit_default with none; limit_build at least for one in builds.
limit_of() { local t lim=$limit_default b
  t=$(awk -F'\t' -v l="$1" '$1 == l && (!f || $2 + 0 > m) { m = $2 + 0; f = 1 } END { if (f) print m }' "$tdir/$cur" 2>/dev/null)
  if [[ -n $t ]]; then lim=$((3 * t)); (( lim >= limit_floor )) || lim=$limit_floor; fi
  for b in "${builds[@]}"; do [[ $b != "$1" ]] || (( lim >= limit_build )) || lim=$limit_build; done
  echo "$lim"
}
# descendants KEEP: the run's processes (the main shell's descendants), but those listed in the file
# KEEP and theirs.
descendants() {
  ps -A -o pid= -o ppid= 2>/dev/null | awk -v root=$$ -v keepf="$1" '
    function walk(p,   n, a, i) { n = split(kids[p], a, " "); for (i = 1; i <= n; i++) if (!(a[i] in keep)) { print a[i]; walk(a[i]) } }
    BEGIN { while ((getline k < keepf) > 0) keep[k] = 1 }
    { kids[$2] = kids[$2] " " $1 }
    END { walk(root) }'
}
window() {
  descendants /dev/null > "$work/keep.new" && mv "$work/keep.new" "$work/keep"
  echo "$(( $(date +%s) + $1 )) $1 $2" > "$work/deadline.new" && mv "$work/deadline.new" "$work/deadline"
}
# timed_out: why the window open now ran out of time, when it did.
timed_out() { [[ -s $work/timedout && "$(head -1 "$work/timedout")" == "$(cat "$work/deadline" 2>/dev/null)" ]] && tail -n +2 "$work/timedout"; }
rm -f "$work/deadline" "$work/timedout" "$work/keep"
heartbeat() {
  local n=0 now d end lim kind fired="" since=0 sig p
  while sleep 1; do
    now=$(date +%s); d=$(cat "$work/deadline" 2>/dev/null)
    read -r end lim kind <<<"${d:-0 0 none}"
    if [[ $kind != none ]] && (( now >= end )); then
      if [[ $d != "$fired" ]]; then
        fired=$d; since=$now
        if [[ $kind == check ]]; then p="the check ran past its limit of $(mins "$lim"), and its processes were stopped"
        else p="the steps before this check ran past their limit of $(mins "$lim"), and their processes were stopped"; fi
        printf '%s\ntimed out: %s\nthe heartbeat'"'"'s last line: %s\n' "$d" "$p" "$(hb_line)" > "$work/timedout.new"
        mv "$work/timedout.new" "$work/timedout"
        echo "        ... time limit reached ($(mins "$lim")): stopping what the $kind started"
      fi
      sig=TERM; (( now - since < 30 )) || sig=KILL
      if [[ $sig == KILL ]] || (( now == since )); then
        for p in $(descendants "$work/keep"); do kill -s "$sig" "$p" 2>/dev/null; done
      fi
    fi
    n=$((n + 1)); (( n % 60 )) || echo "        $(hb_line)"
  done
}
heartbeat &
hb=$!
trap 'kill $hb 2>/dev/null; cleanup' EXIT
window "$limit_build" setup

pass=0 fail=0 failed=() retried=()
# retry: the only checks run a second time when they fail, by label, each for the reason above it.
# A check here that fails twice fails; every other check fails on its first failure.
retry=(
  # network: the process listings, sampled while devvm launched the VM and taken again here, can
  # catch a short-lived process mid-start.
  "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it"
  # link: 256 connections opened at once, and their relays ending within a minute, depend on how
  # busy this machine is.
  "256 connections at once, past the socket's backlog, all carried; a 257th is refused, and devvm network log shows link_limit"
  # link: the client's refusal reaches its event log within 5 s of the connection, which a busy
  # machine can miss.
  "a stopped target: the connection fails, devvm network log shows link_target_stopped, the launch warns, and the target stays stopped"
)
# retries LABEL: the check is on the retry list.
retries() { local l; for l in "${retry[@]}"; do [[ $l == "$1" ]] && return 0; done; return 1; }
# attempt LIMIT CMD...: run CMD in a window of LIMIT seconds, its output in $work/check.log; past
# it, CMD fails, the reason last in its output.
attempt() { local lim=$1 s t; shift
  window "$lim" check
  "$@" >"$work/check.log" 2>&1; s=$?
  t=$(timed_out) || return "$s"
  echo "$t" >> "$work/check.log"; return 1
}
# check LABEL CMD...: run CMD, count and print the result; on failure, CMD's lines that start with
# "failed: " (the step that failed), then its last other lines: 5, or $check_lines when the call
# sets it (check_lines=40 check ...). A check on the retry list that fails runs once more, its first
# output shown, and says so in its result, the timings ("label<TAB>seconds<TAB>retried") and the
# summary. When the steps before it ran out of time, it fails without running.
check() {
  local label=$1 r=PASS dur left again="" lim late; shift
  lim=$(limit_of "$label")
  if late=$(timed_out); then
    r=FAIL; echo "$late" > "$work/check.log"
  elif ! attempt "$lim" "$@"; then
    r=FAIL
    if retries "$label"; then
      again=retried; retried+=("$label")
      echo "  RETRY [$((ncheck + 1))/$total] $label: its first run failed"; check_tail
      ! attempt "$lim" "$@" || r=PASS
    fi
  fi
  window "$limit_build" setup
  if [[ $r == PASS ]]; then pass=$((pass + 1)); else fail=$((fail + 1)); failed+=("$label"); fi
  ncheck=$((ncheck + 1))
  dur=$(( $(date +%s) - $(cat "$work/mark") )); date +%s > "$work/mark"
  if [[ -n $again ]]; then printf '%s\t%s\t%s\n' "$label" "$dur" "$again"; else printf '%s\t%s\n' "$label" "$dur"; fi >> "$work/timings.new/$cur"
  left=$(awk -F'\t' -v l="$label" -v later="$later" 'found { s += $2 } $1 == l { found = 1 } END { if (found) print s + later }' "$tdir/$cur" 2>/dev/null)
  [[ $total == "?" ]] || (( ncheck <= total )) || total="?"   # more checks than last time
  echo "  $r  [$ncheck/$total] $label ($(mins "$dur"); $(mins $((SECONDS - started))) in${left:+, about $(mins "$left") left}${again:+; retried once})"
  [[ $r == PASS ]] || check_tail
}
# check_tail: a failed check's output, as check prints it.
check_tail() {
  { grep '^failed: ' "$work/check.log"; grep -v '^failed: ' "$work/check.log" | tail -n "${check_lines:-5}"; } | sed 's/^/        /'
}
# skip LABEL WHY: a check this run has nothing for (counted, so runs compare their counts).
skip() {
  ncheck=$((ncheck + 1)) skipped=$((skipped + 1))
  printf '%s\t0\n' "$1" >> "$work/timings.new/$cur"
  echo "  SKIP  [$ncheck/$total] $1 ($2)"
}
skipped=0
# rfwd_links HOST SSH...: the VM's sshd, reached as HOST by the command SSH..., binds a remote forward
# only on 127.77.0.0/16, the addresses of links: ssh's default (localhost), 127.0.0.1, 0.0.0.0, ::1,
# localhost and another range are refused, as is a remote socket forward, and 127.77.0.250 binds
# (the control). A safeguard for an honest VM: the boundary is the host's ssh.
rfwd_links() { local h=$1 a; shift
  for a in 0:127.0.0.1:9 $P47613:127.0.0.1:9 127.0.0.1:$P47613:127.0.0.1:9 0.0.0.0:$P47613:127.0.0.1:9 \
    "[::1]:$P47613:127.0.0.1:9" localhost:$P47613:127.0.0.1:9 127.78.0.2:$P47613:127.0.0.1:9 "/tmp/devvm-e2e-$$.sock:$sc/agent.sock"; do
    ! "$@" -o ExitOnForwardFailure=yes -R "$a" "$h" true </dev/null 2>/dev/null || { echo "-R $a bound"; return 1; }
  done
  "$@" -o ExitOnForwardFailure=yes -R 127.77.0.250:$P47613:127.0.0.1:9 "$h" true </dev/null || { echo "-R 127.77.0.250 refused"; return 1; }; }
export -f rfwd_links
section() { echo; echo "== $*"; }
quiet() { "$@" </dev/null >"$work/last.log" 2>&1; }
# rec KEY FIELD: a string field of the record of the image with that key, images/KEY/record.json,
# which devvm writes one field per line; nothing without one.
rec() { sed -n "s/^  \"$2\": \"\(.*\)\",\{0,1\}\$/\1/p" "$images/$1/record.json" 2>/dev/null; }
# keys_of IMAGE: the keys of the images recorded under that name, one per line.
keys_of() { local d; for d in "$images"/*/; do d=${d%/}; [[ $(rec "${d##*/}" name) != "$1" ]] || echo "${d##*/}"; done; }
# from_record REF: the key of a base built from that --from, whose record rec reads.
from_record() { local k; for k in $(keys_of base); do [[ $(rec "$k" from) != "$1" ]] || { echo "$k"; return; }; done; }
# owned IMAGE: the images by that name are this run's, deleted at the end.
owned() { local k; for k in $(keys_of "$1"); do created+=("devvm-img-$k"); done; }
status_of() { limactl list --format '{{.Status}}' "$1" 2>/dev/null; }
export -f rec keys_of from_record status_of; export images
# no_leak PATH... <STRINGS: none of STRINGS' lines is in a regular file under PATH..., read byte for
# byte like grep -raF but only where the file has data: a VM's disk is sparse, 100G of mostly holes
# that grep reads as zeros (over an hour on a slow Mac), and SEEK_DATA/SEEK_HOLE (3 and 4 on Linux,
# 4 and 3 on macOS) skip them. A file it finds one in, or can't read, fails it; a missing PATH has none.
no_leak() {
  perl -MFile::Find -e '
    use strict; use warnings;
    my ($data, $hole) = $^O eq "darwin" ? (4, 3) : (3, 4);
    my @s = grep { length } map { chomp; $_ } <STDIN>;
    @s or die "no_leak: no strings\n";
    my $keep = 0; for (@s) { $keep = length if length > $keep }
    $keep--;
    my $bad = 0;
    $SIG{__WARN__} = sub { print STDERR "no_leak: ", @_; $bad = 1 };
    sub extent { my ($fh, $f, $at, $end) = @_; my $tail = "";
      sysseek($fh, $at, 0) or die "no_leak: $f: $!\n";
      while ($at < $end) {
        my $want = $end - $at; $want = 8 << 20 if $want > 8 << 20;
        my $n = sysread($fh, my $buf, $want);
        defined $n or die "no_leak: $f: $!\n";
        last if $n == 0;
        $at += $n; $buf = $tail . $buf;
        for (@s) { die "no_leak: found in $f\n" if index($buf, $_) >= 0 }
        $tail = $keep > 0 && length($buf) > $keep ? substr($buf, -$keep) : "";
      } }
    sub scan { my $f = shift; my $fh;
      open($fh, "<:raw", $f) or do { warn "$f: $!\n"; return };
      my ($at, $size) = (0, -s $fh);
      while ($at < $size) {
        my $start = sysseek($fh, $at, $data);
        if (!defined $start) { last if $!{ENXIO}; extent($fh, $f, $at, $size); last }
        my $end = sysseek($fh, $start, $hole); $end = $size unless defined $end;
        extent($fh, $f, $start, $end); $at = $end;
      }
      close $fh; }
    find({ no_chdir => 1, wanted => sub { scan($_) if -f $_ && !-l $_ } }, grep { -e } @ARGV);
    exit $bad;
  ' "$@"
}
export -f no_leak
# newproj NAME: a new, empty project folder, as devvm sees it (its physical path).
newproj() {
  [[ "$(uname -s)" != Darwin || ! -e "$projects/$1" ]] || chflags -R nouchg "$projects/$1"
  rm -rf "${projects:?}/$1"; mkdir -p "$projects/$1"; (cd "$projects/$1" && pwd -P)
}
# vm_of DIR: the VM devvm made for that folder, from its state.
vm_of() { local f; for f in "$vms"/*/vm.json; do
    if [[ -f $f && $(st_get "$f" dir) == "$1" ]]; then st_get "$f" name; return; fi; done; }
# launch DIR INPUT ARGS...: devvm ARGS in DIR, with INPUT for the shell it opens (which then ends);
# sets $vm and records it for cleanup. Its output is in $work/last.log.
launch() {
  local d=$1 in=$2; shift 2
  (cd "$d" && printf '%s\n' "$in" | "$D" "$@" >"$work/last.log" 2>&1)
  vm=$(vm_of "$d"); [[ -z $vm ]] || created+=("$vm")
}
in_vm() { limactl shell --workdir / "$1" bash -lc "$2" </dev/null; }
export -f in_vm
# proxy_config FILE VM LISTEN PASSWORD [POLICY [LOG]]: a config of this run's own for devvm _proxy
# -config, the run's alone (mode 600): VM's proxy on LISTEN with PASSWORD, POLICY's rules (none
# without one) and its events in LOG (none: its standard output).
proxy_config() { local pol=""
  [[ -z ${5:-} ]] || pol="\"$5\""
  (umask 077 && printf '{"version": 1, "vm": "%s", "listen": "%s", "password": "%s", "policies": [%s], "log": "%s"}\n' \
    "$2" "$3" "$4" "$pol" "${6:-}" >"$1") && chmod 600 "$1"
}
# corp_start: a company proxy of this run's ($corp): a devvm _proxy on 127.0.0.1:$P18766 with a
# password of its own ($work/corp.auth, which the checks read it from), allowing any host, its
# events in $work/corp.jsonl; $corpurl is its address with the password. corp_stop ends it.
corp_start() {
  corppw=e2e-corp-pw-$$-$(date +%s)
  echo "$corppw" > "$work/corp.auth"; printf '**\n' > "$work/corp.policy"; : > "$work/corp.jsonl"
  proxy_config "$work/corp.json" e2e-corp 127.0.0.1:$P18766 "$corppw" "$work/corp.policy" "$work/corp.jsonl"
  "$D" _proxy -config "$work/corp.json" >"$work/corp.log" 2>&1 &
  corp=$!
  sleep 1
  corpurl="http://devvm:$corppw@127.0.0.1:$P18766"
}
corp_stop() { [[ -z ${corp:-} ]] || kill "$corp" 2>/dev/null; corp=""; }
# e2e_ca: a throwaway CA of this run's in $cad (ca.pem), and a server certificate it signed for
# host.lima.internal (srv.pem, srv.key): never a certificate of yours. ca_serve PORT: an HTTPS server
# on this machine with it, on every address ($ca_server). $cafolder is devvm's CA folder.
cad=$work/e2e-ca cafolder=$XDG_CONFIG_HOME/devvm/ca ca_server=""
e2e_ca() {
  [[ ! -f $cad/srv.pem ]] || return 0
  rm -rf "$cad"; mkdir -p "$cad"
  printf '%s\n' '[req]' 'distinguished_name = dn' 'prompt = no' '[dn]' 'CN = devvm e2e CA' \
    '[ca]' 'basicConstraints = critical,CA:TRUE' 'keyUsage = critical,keyCertSign,cRLSign' 'subjectKeyIdentifier = hash' \
    '[srv]' 'basicConstraints = CA:FALSE' 'subjectAltName = DNS:host.lima.internal,DNS:localhost,IP:127.0.0.1' \
    'extendedKeyUsage = serverAuth' 'subjectKeyIdentifier = hash' 'authorityKeyIdentifier = keyid' > "$cad/openssl.cnf"
  openssl req -x509 -newkey rsa:2048 -nodes -days 2 -config "$cad/openssl.cnf" -extensions ca \
    -keyout "$cad/ca.key" -out "$cad/ca.pem" >/dev/null 2>&1 &&
  openssl req -new -newkey rsa:2048 -nodes -config "$cad/openssl.cnf" -subj /CN=host.lima.internal \
    -keyout "$cad/srv.key" -out "$cad/srv.csr" >/dev/null 2>&1 &&
  openssl x509 -req -days 2 -in "$cad/srv.csr" -CA "$cad/ca.pem" -CAkey "$cad/ca.key" -CAcreateserial \
    -extfile "$cad/openssl.cnf" -extensions srv -out "$cad/srv.pem" >/dev/null 2>&1
}
ca_serve() {
  [[ -z $ca_server ]] || return 0
  # -www reads nothing from its input.
  openssl s_server -accept "$1" -cert "$cad/srv.pem" -key "$cad/srv.key" -www -quiet </dev/null >/dev/null 2>&1 &
  ca_server=$!; sleep 0.5
}
# ca_stop: ends ca_serve's server once a section is done with it, so its port is free for later
# sections' checks (the network section's VM server has $P47195 too).
ca_stop() { [[ -z $ca_server ]] || { kill "$ca_server" 2>/dev/null; wait "$ca_server" 2>/dev/null; }; ca_server=""; }
# ca_gone VM: the running VM has none of devvm's CA files, and the run's CA isn't in its system's
# bundle: what an image keeps.
ca_gone() {
  limactl shell --workdir / "$1" sh -c '! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 &&
    [ ! -e /etc/ssl/devvm.pem ] && [ ! -e /var/lib/devvm-ca ] && ! grep -qF "$1" /etc/ssl/certs/ca-certificates.crt' \
    _ "$(sed -n 4p "$cad/ca.pem")" </dev/null
}
export -f ca_gone; export cad
# host_port_kept DIR VM PORT CONTROL SERVE: a program on this machine has PORT, which VM lists, on
# every address (with SO_REUSEADDR, as most servers do, so its ended connections don't hold the port
# on Linux once it's gone): the VM's next start leaves it out, warned, and with servers in the
# VM on PORT and CONTROL (SERVE VM PORTS...), PORT's localhost connections still reach the program
# and CONTROL's the VM. Once the program is gone, the next start forwards PORT again.
host_port_kept() { local hp out r=1
  (cd "$1" && "$D" stop </dev/null >/dev/null 2>&1) || { echo "stop failed"; return 1; }
  perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalPort => shift, Listen => 5, ReuseAddr => 1) or die;
    while ($c = $s->accept) { print $c "host-program\n"; close $c }' "$3" &
  hp=$!; sleep 0.5
  (cd "$1" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1)
  if grep -qx "devvm: ports_taken count=1 ports=$3 vm=$2" "$work/last.log"; then
    "$5" "$2" "$3" "$4"
    for _ in $(seq 20); do nc -z 127.0.0.1 "$4" && break; sleep 1; done; sleep 3
    out=$(nc -w 2 127.0.0.1 "$3" </dev/null)
    echo "control $4: $(nc -z 127.0.0.1 "$4" && echo forwarded); $3 answered by: ${out:-nothing}"
    nc -z 127.0.0.1 "$4" && [[ $out == host-program ]] && r=0
  else cat "$work/last.log"; fi
  kill "$hp" 2>/dev/null; wait "$hp" 2>/dev/null
  (( r == 0 )) || return 1
  (cd "$1" && "$D" stop </dev/null >/dev/null 2>&1 && printf 'exit\n' | DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1)
  ! grep -q ports_taken "$work/last.log" || { cat "$work/last.log"; return 1; }
  "$5" "$2" "$3"
  for _ in $(seq 20); do nc -z 127.0.0.1 "$3" && return 0; sleep 1; done
  echo "$3 not forwarded again"; return 1; }
# lima_serve VM PORTS...: a server in the VM on each port, every address, left running.
lima_serve() { local p; for p in "${@:2}"; do in_vm "$1" "setsid -f python3 -m http.server -b 0.0.0.0 $p </dev/null >/dev/null 2>&1"; done; }
# vm_say VM ADDR PORT WORD: a server in a Lima VM on ADDR:PORT alone that answers each connection with
# WORD and a newline (pkill -f '[d]evvm-e2e-say' ends it).
vm_say() {
  in_vm "$1" "setsid -f python3 -c 'import socket,sys; s=socket.socket(); s.bind((sys.argv[1],int(sys.argv[2]))); s.listen(5); [(lambda c: (c.sendall(sys.argv[3].encode()+bytes([10])), c.close()))(s.accept()[0]) for _ in iter(int,1)]' $2 $3 $4 devvm-e2e-say </dev/null >/dev/null 2>&1"
}
# lima_boot VM: its boot's ID.
lima_boot() { in_vm "$1" 'cat /proc/sys/kernel/random/boot_id'; }
# ports_live DIR VM PORT SERVE BOOT: VM runs, listing ports without PORT, with a server in it on
# PORT (SERVE VM PORT), which doesn't reach this machine; devvm ports add PORT says the VM forwards
# its new ports and the server reaches this machine at once; devvm ports rm PORT closes it again.
# The VM is never restarted: BOOT VM (its boot or start) is the same after.
ports_live() { local b0 b1 now
  b0=$("$5" "$2"); "$4" "$2" "$3"; sleep 3
  ! nc -z 127.0.0.1 "$3" || { echo "$3 reached before it's listed"; return 1; }
  (cd "$1" && "$D" ports add "$3" </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  now=$(cd "$1" && "$D" ports 2>/dev/null)
  grep -qF "$(m ports_forwarding ports="$now" vm="$2")" "$work/last.log" || { cat "$work/last.log"; return 1; }
  for _ in $(seq 20); do nc -z 127.0.0.1 "$3" && break; sleep 1; done
  nc -z 127.0.0.1 "$3" || { echo "$3 not forwarded once added"; return 1; }
  (cd "$1" && "$D" ports rm "$3" </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  for _ in $(seq 15); do nc -z 127.0.0.1 "$3" || break; sleep 1; done
  ! nc -z 127.0.0.1 "$3" || { echo "$3 still forwarded once removed"; return 1; }
  b1=$("$5" "$2"); echo "boot or start: ${b0:-none} before, ${b1:-none} after"
  [[ -n $b0 && $b0 == "$b1" ]]; }
# lima_listens tcp|udp PORT: Lima's own processes (limactl's hostagent) have PORT on this machine.
lima_listens() {
  if [[ "$(uname -s)" == Darwin ]]; then
    if [[ $1 == tcp ]]; then lsof -a -nP -c limactl -iTCP:"$2" -sTCP:LISTEN >/dev/null 2>&1; else lsof -a -nP -c limactl -iUDP:"$2" >/dev/null 2>&1; fi
  else
    ss -Hlnp -"${1:0:1}" "sport = :$2" 2>/dev/null | grep -q '"limactl"'
  fi
}
export -f lima_listens
# loopback_only PORT: something on this machine listens on TCP PORT, and only on 127.0.0.1 or ::1.
loopback_only() { local addrs
  if [[ "$(uname -s)" == Darwin ]]; then
    addrs=$(lsof -nP -iTCP:"$1" -sTCP:LISTEN -F n 2>/dev/null | sed -n 's/^n//p')
  else
    addrs=$(ss -Hltn "sport = :$1" 2>/dev/null | awk '{print $4}')
  fi
  [[ -n $addrs ]] && ! grep -qvE "^(127\.0\.0\.1|\[::1\]):$1\$" <<<"$addrs"
}
export -f loopback_only
# desktop_serve DIR [ARGS...]: devvm desktop --no-open ARGS in DIR, in the background ($desk, its
# output in $work/desktop.log), until it says where the VM's desktop is: its address ($desk_addr,
# 127.0.0.1:PORT), port ($desk_port) and password ($desk_pw). desktop_end ends it with Ctrl+C.
desk=""
desktop_serve() {
  local d=$1 i; shift
  desk_addr="" desk_port="" desk_pw=""
  (cd "$d" && DEVVM_MESSAGES=names exec "$D" desktop --no-open "$@" </dev/null >"$work/desktop.log" 2>&1) &
  desk=$!
  for i in $(seq 240); do
    grep -q '^desktop_ready ' "$work/desktop.log" && break
    kill -0 "$desk" 2>/dev/null || break
    sleep 1
  done
  desk_addr=$(sed -n 's/^desktop_ready address=\([^ ]*\) .*/\1/p' "$work/desktop.log")
  desk_pw=$(sed -n 's/^desktop_ready .*password=\([a-z0-9]*\) .*/\1/p' "$work/desktop.log")
  desk_port=${desk_addr##*:}
  [[ -n $desk_addr && -n $desk_pw ]]
}
desktop_end() { [[ -z $desk ]] || { kill -INT "$desk" 2>/dev/null; wait "$desk" 2>/dev/null; desk=""; }; }
# desktop_view [VAR=VALUE...]: TestE2EDesktop on the desktop devvm desktop serves: a wrong password
# refused, the password let in (with DEVVM_E2E_CLIPBOARD, a clipboard and a key event sent).
desktop_view() { go_e2e TestE2EDesktop desktop DEVVM_E2E_DESKTOP="$desk_addr" DEVVM_E2E_PASSWORD="$desk_pw" "$@"; }
# desktop_in_vm: what's checked in a VM whose desktop runs, as its user: its Xvnc has its socket in a
# folder of the user's own (0700), the socket 0600, none of its sockets listening on TCP, and the
# clipboard off.
desktop_in_vm='set -e
d=/run/devvm-desktop pid=$(cat /run/devvm-desktop/xvnc.pid)
[ "$(cat "/proc/$pid/comm")" = Xvnc ]
[ "$(stat -c %a "$d")" = 700 ] && [ "$(stat -c %a "$d/vnc.sock")" = 600 ] && [ "$(stat -c %U "$d/vnc.sock")" = "$(id -un)" ]
for f in "/proc/$pid/fd/"*; do
  l=$(readlink "$f" 2>/dev/null) || continue
  case $l in socket:*) i=${l#socket:[}; i=${i%]}
    if awk -v i="$i" "\$4 == \"0A\" && \$10 == i { f = 1 } END { exit !f }" /proc/net/tcp /proc/net/tcp6 2>/dev/null; then
      echo "Xvnc listens on TCP"; exit 1; fi ;;
  esac
done
for p in AcceptCutText SendCutText; do [ "$(DISPLAY=:1 XAUTHORITY="$HOME/.Xauthority" vncconfig -get "$p")" = off ]; done'
# desktop_secret: the password, read from the input, is in no file of the VM's (run as root there),
# its fixed folders searched. Never on a command line: sudo would log it in the VM's journal.
desktop_secret='read -r pw && [ -n "$pw" ] && ! grep -rlsF -D skip -- "$pw" /home /root /etc /run /tmp /var/log /usr/local'
# watch_sees RUN VM DIR BY [NESTED]: a watcher in the VM on DIR and a subfolder, as a dev server's
# (RUN VM CMD runs CMD there: in_vm), sees a file changed in each, one made and one deleted, by this
# machine (BY host) or the VM (BY vm), each within 5 s; with NESTED no, never the subfolder's (9p's
# limit). The watcher is perl (every base has it) calling inotify itself, so the test VM needs
# nothing added. It prints its events, and writes them to a file in a folder of DIR's it doesn't
# watch too, as 9p shows this machine a file the VM writes only later; they're in $work/watch.out
# after. watch_misses RUN VM DIR: the watcher sees none of this machine's (Lima's limit: polling).
watch_pl='use strict; use IO::Handle;
my %nr = (x86_64 => [294, 254], aarch64 => [26, 27]);
chomp(my $m = qx(uname -m));
my ($init, $add) = @{$nr{$m} or die "no inotify syscalls for $m\n"};
my $fd = syscall($init, 0); $fd >= 0 or die "inotify_init1: $!\n";
my $log = shift @ARGV; open(my $lf, ">>", $log) or die "$log: $!\n";
sub out { print STDOUT @_; print $lf @_ }
my ($top, %rel) = ($ARGV[0]);
for my $dir (@ARGV) {
  my $wd = syscall($add, $fd, $dir, 0x2 | 0x4 | 0x8 | 0x40 | 0x80 | 0x100 | 0x200);
  $wd >= 0 or die "inotify_add_watch $dir: $!\n";
  $rel{$wd} = $dir eq $top ? "" : substr($dir, length($top) + 1) . "/";
}
open(my $in, "<&=", $fd) or die "inotify fd: $!\n";
STDOUT->autoflush(1); $lf->autoflush(1); alarm 120;
my %name = (0x2 => "MODIFY", 0x4 => "ATTRIB", 0x8 => "CLOSE_WRITE", 0x40 => "MOVED_FROM", 0x80 => "MOVED_TO", 0x100 => "CREATE", 0x200 => "DELETE");
out("ready\n");
while (sysread($in, my $buf, 65536)) {
  for (my $i = 0; $i + 16 <= length $buf; ) {
    my ($wd, $mask, undef, $len) = unpack("iIII", substr($buf, $i, 16));
    my $f = $rel{$wd} . unpack("Z*", substr($buf, $i + 16, $len));
    for my $bit (sort { $a <=> $b } keys %name) { out("$name{$bit} $f\n") if $mask & $bit }
    $i += 16 + $len;
  }
}'
# watch_events: what the watcher printed and wrote so far, each line once.
watch_events() { cat "$watch_log" "$work/watch.stream" 2>/dev/null | awk '!seen[$0]++'; }
watch_saw() { local end=$((SECONDS + 5))
  while (( SECONDS <= end )); do watch_events | grep -qE "^$1\$" && return 0; sleep 0.2; done
  echo "nothing for $1 within 5 s" >> "$work/watch.missed"; return 1; }
# watch_do RUN VM BY CMD: CMD on this machine (BY host) or in the VM.
watch_do() { if [[ $3 == host ]]; then bash -c "$4"; else "$1" "$2" "$4" >/dev/null; fi; }
# Each call's folders have names of their own, .e2e-watch-N (the script and its log) and e2e-sub-N:
# a folder deleted and made again under the same name can stay gone in the VM for longer than any
# wait (Lima's virtiofs keeps the deleted folder's entry), while a name the VM never looked up shows
# as soon as this machine makes it.
watch_n=0
watch_sees() { local w r=1 wd sub
  watch_n=$((watch_n + 1)); wd=$3/.e2e-watch-$watch_n sub=e2e-sub-$watch_n
  watch_log=$wd/events
  mkdir -p "$3/$sub" "$wd" && printf '%s\n' "$watch_pl" > "$wd/watch.pl" && : > "$watch_log" &&
    echo before > "$3/watched.txt" && echo before > "$3/$sub/nested.txt" || return 1
  : > "$work/watch.missed"
  # A bound, not the fix: the VM waits up to 20 s for the script to show, then runs it, which fails
  # naming the file if it's still missing.
  "$1" "$2" "for _ in \$(seq 100); do [ -s '$wd/watch.pl' ] && break; sleep 0.2; done
    LC_ALL=C exec perl '$wd/watch.pl' '$watch_log' '$3' '$3/$sub'" > "$work/watch.stream" 2>&1 &
  w=$!
  for _ in $(seq 60); do watch_events | grep -qx ready && break; sleep 0.5; done
  if watch_events | grep -qx ready; then
    r=0   # each one tried, so the output shows them all
    watch_do "$1" "$2" "$4" "echo after >> '$3/watched.txt'"; watch_saw "(MODIFY|CLOSE_WRITE|CREATE) watched.txt" || r=1
    watch_do "$1" "$2" "$4" "echo after >> '$3/$sub/nested.txt'"
    if [[ ${5:-yes} == yes ]]; then watch_saw "(MODIFY|CLOSE_WRITE|CREATE) $sub/nested.txt" || r=1; fi
    watch_do "$1" "$2" "$4" "echo made > '$3/made.txt'"; watch_saw '(CREATE|MOVED_TO) made.txt' || r=1
    watch_do "$1" "$2" "$4" "rm '$3/made.txt'"
    watch_saw '(DELETE|MOVED_FROM) made.txt' || r=1
    # NESTED no: the subfolder's change is never seen, even after the rest.
    if [[ ${5:-yes} == no ]] && watch_events | grep -q " $sub/"; then
      echo "the subfolder's change was seen" >> "$work/watch.missed"; r=1
    fi
  fi
  "$1" "$2" "pkill -f '[.]e2e-watch-$watch_n/watch[.]pl'" >/dev/null 2>&1
  kill "$w" 2>/dev/null; wait "$w" 2>/dev/null
  watch_events > "$work/watch.out"
  echo "the watcher saw:"; cat "$work/watch.out" "$work/watch.missed"   # what it missed last
  rm -rf "$wd" "$3/watched.txt" "$3/made.txt" "$3/${sub:?}"
  return "$r"; }
watch_misses() { watch_sees "$1" "$2" "$3" host; grep -qx ready "$work/watch.out" && ! grep -qvx ready "$work/watch.out"; }

# Links between VMs: link_section's checks, each the end-to-end proof of a row of the design note's
# threat check (internals/design.md, "Links between VMs").
# lk_pl: the VMs' helper, /var/tmp/e2e-link.pl (perl: every base has it). serve ADDR PORT WORD
# answers each connection with an HTTP response whose body is WORD; hold ADDR PORT keeps every
# connection it accepts open (each writes its pid to /var/tmp/e2e-link-ADDR-PORT.pid); conns HOST
# PORT N opens N connections, all at once but the last, 5 s after the others, and prints those the far
# end closed (a burst past the backlog of the link's socket on macOS, 128, which devvm _links accepts
# whole before it starts their relays);
# listeners prints each TCP listener as "PROCESS ADDR:PORT" (as root, to see every process's).
lk_pl='use strict; use warnings; use IO::Socket::INET; use IO::Select;
my $mode = shift // "";
sub v4 { join ".", reverse map { hex } $_[0] =~ /(..)/g }
sub addr { my $a = uc shift;
  return v4($a) if length $a == 8;
  return "[::]" if $a =~ /^0{32}$/;
  return "[::1]" if $a eq "00000000000000000000000001000000";
  return "[::ffff:" . v4($1) . "]" if $a =~ /^0{16}FFFF0000(.{8})$/;
  return "[$a]"; }
if ($mode eq "serve" or $mode eq "hold") {
  my ($host, $port, $word) = @ARGV;
  my $s = IO::Socket::INET->new(LocalAddr => $host, LocalPort => $port, Listen => 1024, ReuseAddr => 1) or die "$host:$port: $!\n";
  open(my $pf, ">", "/var/tmp/e2e-link-$host-$port.pid") or die "$!\n"; print $pf "$$\n"; close $pf;
  my @held;
  while (my $c = $s->accept) {
    if ($mode eq "hold") { push @held, $c; next }
    my $sel = IO::Select->new($c); my $buf = "";
    while ($buf !~ /\n\r?\n/ and $sel->can_read(2)) { sysread($c, $buf, 4096, length $buf) or last }
    my $body = "$word\n";
    print $c "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nContent-Length: " . length($body) . "\r\nConnection: close\r\n\r\n$body";
    close $c;
  }
} elsif ($mode eq "conns") {
  my ($host, $port, $n) = @ARGV; my @c;
  for my $i (1 .. $n) {
    sleep 5 if $i == $n;
    push @c, IO::Socket::INET->new(PeerAddr => $host, PeerPort => $port, Timeout => 10) || die "connection $i: $!\n";
  }
  sleep 5;
  my @closed;
  for my $i (0 .. $#c) { push @closed, $i + 1 if IO::Select->new($c[$i])->can_read(0) and !sysread($c[$i], my $x, 1) }
  print "closed: @closed\n";
} elsif ($mode eq "listeners") {
  my %own;
  for my $d (glob "/proc/[0-9]*") {
    open(my $f, "<", "$d/comm") or next; my $n = <$f> // "?"; chomp $n;
    opendir(my $fd, "$d/fd") or next;
    for my $e (readdir $fd) { my $l = readlink("$d/fd/$e") // next; $own{$1} = $n if $l =~ /^socket:\[(\d+)\]$/ }
  }
  for my $t ("tcp", "tcp6") {
    open(my $h, "<", "/proc/net/$t") or next; my $head = <$h>;
    while (my $line = <$h>) {
      my @f = split " ", $line; next unless $f[3] eq "0A";
      my ($a, $p) = split /:/, $f[1];
      printf "%s %s:%d\n", $own{$f[9]} // "?", addr($a), hex $p;
    }
  }
} else { die "e2e-link.pl: serve, hold, conns or listeners\n" }'
# lk_user VM CMD: CMD in a login shell as the VM's user; lk_root VM CMD: as root; lk_bg VM CMD: as
# root, left running; lk_put VM FILE: the input, as FILE in the VM.
lk_user() { in_vm "$1" "$2"; }
lk_root() { limactl shell --workdir / "$1" sudo sh -c "$2" </dev/null; }
lk_bg() { lk_root "$1" "setsid -f $2 </dev/null >/dev/null 2>&1"; }
lk_put() { limactl shell --workdir / "$1" sudo sh -c 'cat > "$1"' _ "$2"; }
# lk_running VM: it runs, as Lima says.
lk_running() { [[ $(status_of "$1") == Running ]]; }
# lk_until SECONDS CMD...: CMD, once a second until it passes, for up to SECONDS; lk_not CMD...: CMD fails.
lk_until() { local n=$1 i; shift; for ((i = 1; i < n; i++)); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done; "$@"; }
lk_not() { ! "$@"; }
# lk_start VM serve|hold ADDR PORT [WORD]: the helper's server in the VM, as root, once it listens;
# lk_end VM ADDR PORT ends it. lk_serve VM WORD PORTS...: the helper written to the VM, and a server
# on its 127.0.0.1 on each port, answering WORD-PORT.
# The wait ends once it listens (its pid file) or has ended (the wrapper's "exited N" in its log).
# On a failure: what it printed, and every TCP listener in the VM.
lk_start() { local f=/var/tmp/e2e-link-$3-$4.pid l=/var/tmp/e2e-link-$3-$4.log
  if ! { lk_root "$1" "rm -f $f $l" && lk_bg "$1" "sh -c 'perl /var/tmp/e2e-link.pl $2 $3 $4 ${5:-} >$l 2>&1; echo exited \$? >>$l'" &&
    lk_until 20 lk_root "$1" "test -s $f || grep -q '^exited ' $l" && lk_root "$1" "test -s $f"; }; then
    echo "failed: the helper's $2 on $3:$4 in $1 didn't start; the VM runs: $(lk_running "$1" && echo yes || echo no); its output:"
    lk_root "$1" "ls -l $f $l; tail -n 20 $l" 2>&1
    echo "the VM's listeners:"; lk_listens "$1"
    return 1
  fi; }
lk_end() { lk_root "$1" "f=/var/tmp/e2e-link-$2-$3.pid; [ ! -s \$f ] || kill \$(cat \$f); rm -f \$f"; }
lk_serve() { local vm=$1 w=$2 p; shift 2
  printf '%s\n' "$lk_pl" | lk_put "$vm" /var/tmp/e2e-link.pl || return 1
  for p in "$@"; do lk_start "$vm" serve 127.0.0.1 "$p" "$w-$p" || return 1; done; }
# lk_raw VM HOST PORT: what a raw TCP client in the VM (bash's /dev/tcp) gets back for a request: the
# body's line. lk_gets VM HOST PORT WORD: it's WORD. lk_curls VM URL WORD: curl gets WORD, through the
# VM's proxy settings when it has them. lk_resolves VM NAME: the name resolves in the VM.
lk_raw() { lk_user "$1" "exec 3<>/dev/tcp/$2/$3 && printf 'GET / HTTP/1.0\r\n\r\n' >&3 && timeout 10 cat <&3 | tail -n 1"; }
lk_gets() { local a; a=$(lk_raw "$1" "$2" "$3" 2>/dev/null); echo "$2:$3 answered: ${a:-nothing}"; [[ $a == "$4" ]]; }
lk_curls() { local a; a=$(lk_user "$1" "curl -fsS --max-time 10 $2" 2>/dev/null); echo "$2 answered: ${a:-nothing}"; [[ $a == "$3" ]]; }
lk_resolves() { lk_user "$1" "getent hosts $2" >/dev/null 2>&1; }
# lk_dv DIR ARGS...: devvm ARGS in DIR, messages by name, its output in $work/last.log; lk_up DIR
# [ARGS...]: the same with a shell that ends at once. lk_said LINE: the output has that line.
lk_dv() { local d=$1; shift; (cd "$d" && DEVVM_MESSAGES=names "$D" "$@" </dev/null >"$work/last.log" 2>&1); }
lk_up() { local d=$1; shift; (cd "$d" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "$@" >"$work/last.log" 2>&1); }
# lk_shows DIR REGEX ARGS...: devvm ARGS' output in DIR has a line matching REGEX (read whole first:
# grep -q ending a pipe early fails it, under pipefail).
lk_shows() { local o; o=$(cd "$1" && "$D" "${@:3}" 2>/dev/null); grep -Eq -- "$2" <<<"$o"; }
lk_said() { grep -qxF -- "$1" "$work/last.log" || { echo "not said: $1"; tail -5 "$work/last.log"; return 1; }; }
# lk_ssh ARGS...: ssh through devvm's ssh config, without an agent.
lk_ssh() { env -u SSH_AUTH_SOCK ssh -F "$XDG_DATA_HOME/devvm/ssh_config" -o BatchMode=yes "$@" </dev/null; }
# lk_addr VM NAME: the address its links named NAME listen on, from its state. lk_hash PARTS...: the
# hash devvm names a link session's files by. lk_sock VM ADDR PORT: a link's socket; lk_ctl VM: the
# session's control socket. lk_mode PATH: its mode, in octal.
lk_addr() { local i; i=$(vm_state "$(vmdir "$1")" | sed -n "s/^links\.links\.\([0-9]*\)\.name=$2\$/\1/p" | head -1)
  [[ -z $i ]] || st_get "$(vmdir "$1")" "links.links.$i.addr"; }
lk_hash() { local s=$1; shift
  { printf '%s' "$s"; for s in "$@"; do printf '\0%s' "$s"; done; } | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16; }
lk_sock() { echo "$XDG_DATA_HOME/devvm/run/link-$(lk_hash "$1" socket "$2" "$3").sock"; }
lk_ctl() { echo "$XDG_DATA_HOME/devvm/run/link-$(lk_hash "$1" session).ctl"; }
lk_mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
# lk_session VM: the pid of its devvm _links; lk_master VM: of that session's ssh master to it.
# lk_desc PIDS...: them and their descendants, comma-separated.
lk_session() { local p; p=$(pgrep -f -- "[_]links $1\$" | head -1); [[ -n $p ]] && echo "$p"; }
lk_master() { local p; p=$(pgrep -f -- "[-]M -S $(lk_ctl "$1") " | head -1); [[ -n $p ]] && echo "$p"; }
lk_desc() { local p c; for p in "$@"; do echo "$p"; for c in $(pgrep -P "$p" 2>/dev/null); do lk_desc "$c"; done; done; }
# lk_sshport VM: the host loopback port devvm's ssh reaches it on.
lk_sshport() { st_get "$(vmdir "$1")" ports.host.ssh; }
# lk_host_tcp PIDS: those processes' TCP sockets on this machine, "LOCAL (LISTEN)" or "LOCAL->PEER (STATE)".
lk_host_tcp() {
  if [[ "$(uname -s)" == Darwin ]]; then lsof -nP -a -p "$1" -iTCP 2>/dev/null | awk 'NR > 1 { print $9, $10 }'
  else ss -Htanp 2>/dev/null | grep -E "pid=($(tr , '|' <<<"$1"))," | awk '{ print ($1 == "LISTEN" ? $4 : $4 "->" $5), "(" $1 ")" }'; fi; }
# lk_host_clean CLIENT VMS...: CLIENT's link session and every process it started listen on no TCP
# port of this machine's, and connect only to the ssh ports of VMS on its loopback.
lk_host_clean() { local pid t peer v ok bad=""; pid=$(lk_session "$1") || { echo "no link session"; return 1; }; shift
  t=$(lk_host_tcp "$(lk_desc "$pid" | paste -sd, -)"); echo "the link session's TCP: $(paste -sd ' ' - <<<"${t:-none}")"
  ! grep -q LISTEN <<<"$t" || return 1
  while read -r peer; do
    [[ -n $peer ]] || continue
    ok=no; for v in "$@"; do [[ $peer != "127.0.0.1:$(lk_sshport "$v")" ]] || ok=yes; done; [[ $ok == yes ]] || bad+=" $peer"
  done < <(grep -- '->' <<<"$t" | awk '{ sub(/^.*->/, "", $1); print $1 }')
  [[ -z $bad ]] || { echo "connected to:$bad"; return 1; }; }
# lk_listens VM: its TCP listeners, as the helper prints them, read as root and as its user: root in a
# container may not read the processes of the user's (no CAP_SYS_PTRACE), whose sshd has the links'.
# lk_vm_clean VM PORTS...: its sshd listens only on its own port (22) and links' addresses, and on
# each PORT sshd listens on a link's address and nothing listens anywhere else (127.0.0.1, ::1, every
# address).
lk_listens() {
  { lk_root "$1" 'perl /var/tmp/e2e-link.pl listeners'; lk_user "$1" 'perl /var/tmp/e2e-link.pl listeners'; } 2>/dev/null |
    awk '!($2 in n) || n[$2] == "?" { n[$2] = $1 } END { for (a in n) print n[a], a }'; }
lk_vm_clean() { local vm=$1 l p bad; shift
  l=$(lk_listens "$vm") || { echo "no listeners read"; return 1; }
  bad=$(awk '$1 ~ /^sshd/ && $2 !~ /:22$/ && $2 !~ /^127\.77\./' <<<"$l")
  [[ -z $bad ]] || { echo "sshd also listens on: $bad"; return 1; }
  for p in "$@"; do
    grep -qE "^sshd[^ ]* 127\.77\.[0-9]+\.[0-9]+:$p\$" <<<"$l" || { echo "no link listens on $p: $(grep ":$p\$" <<<"$l")"; return 1; }
    bad=$(grep ":$p\$" <<<"$l" | grep -v ' 127\.77\.')
    [[ -z $bad ]] || { echo "on $p: $bad"; return 1; }
  done; }
# lk_event VM EVENT NAME PORT [TEXT]: its event log has that event for the link NAME:PORT, with TEXT.
lk_event() { grep -F "\"event\":\"$2\",\"host\":\"$3\",\"port\":$4," "$(vmdir "$1")/logs/network.jsonl" 2>/dev/null | grep -qF -- "${5:-}"; }
# lk_set_net VM on|off: its state's network policy set by hand, as nothing devvm runs would: the rule
# broken under devvm, which checks it again on every connection and at each launch.
lk_set_net() { local on=false; [[ $2 != on ]] || on=true; vm_set "$(vmdir "$1")/vm.json" net.on "$on"; }
# lk_hostile VM [gateway]: root in the client loosens its sshd (any listen address, all forwarding;
# with gateway, GatewayPorts yes too) and makes it read that; lk_honest VM puts it back.
lk_hostile_sh='if [ -f /etc/devvm/ssh/sshd_config ]; then c=/etc/devvm/ssh/sshd_config
  [ -f $c.e2e ] || cp $c $c.e2e
  sed -i -e "s/^PermitListen .*/PermitListen any/" -e "s/^AllowTcpForwarding .*/AllowTcpForwarding yes/" $c
  [ "$h" != gateway ] || sed -i "s/^GatewayPorts .*/GatewayPorts yes/" $c
  [ ! -s /run/devvm-sshd.pid ] || kill -HUP "$(cat /run/devvm-sshd.pid)"
else { echo "PermitListen any"; echo "AllowTcpForwarding yes"; [ "$h" != gateway ] || echo "GatewayPorts yes"; } > /etc/ssh/sshd_config.d/00-aaa-e2e.conf
  systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || rc-service sshd reload 2>/dev/null || true
fi'
lk_honest_sh='if [ -f /etc/devvm/ssh/sshd_config.e2e ]; then mv /etc/devvm/ssh/sshd_config.e2e /etc/devvm/ssh/sshd_config
  [ ! -s /run/devvm-sshd.pid ] || kill -HUP "$(cat /run/devvm-sshd.pid)"
else rm -f /etc/ssh/sshd_config.d/00-aaa-e2e.conf
  systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || rc-service sshd reload 2>/dev/null || true
fi'
lk_hostile() { lk_root "$1" "h=${2:-}; $lk_hostile_sh"; }
lk_honest() { lk_root "$1" "$lk_honest_sh"; }
# lk_reconnect VM NAME PORT WORD: its link session's connection ended (its master killed), so the
# session makes a new one, to sshd as it is now; the link NAME:PORT answers WORD again through it.
lk_reconnect() { local m; m=$(lk_master "$1"); [[ -n $m ]] || { echo "no master"; return 1; }
  kill "$m"; lk_until 30 lk_new_master "$1" "$m" && lk_until 30 lk_gets "$1" "$2" "$3" "$4"; }
lk_new_master() { local n; n=$(lk_master "$1"); [[ -n $n && $n != "$2" ]]; }

# link_section PREFIX: the link checks, the folders named from PREFIX: PREFIXdb and PREFIXapi the targets, PREFIXapp the client. Three small VMs without
# network policy, then network policy on the client and the first target; ports $P47240-$P47249, and 80.
# devvm runs with a soft limit of 256 open files, macOS's default (a shell of yours may have more),
# so the link sessions it starts get it too.
link_section() {
  local nofile; nofile=$(ulimit -S -n); ulimit -S -n 256
  lk_x=$1
  sc=${sc:-$work/ssh}   # rfwd_links's socket forward names a path there
  lk_p1=$(newproj "${lk_x}db") lk_p2=$(newproj "${lk_x}api") lk_pc=$(newproj "${lk_x}app")
  local small=(--no-network-policy --no-git-protect --cpus 2 --memory 2)
  launch "$lk_p1" exit "${small[@]}" --ports none; lk_v1=$vm
  launch "$lk_p2" exit "${small[@]}" --ports none; lk_v2=$vm
  launch "$lk_pc" exit "${small[@]}" --ports all; lk_vc=$vm
  lk_w1=${lk_x}db lk_w2=${lk_x}api lk_n2=${lk_x}api   # the targets' servers' words; the second's link name
  check "two targets that forward no port and a client that forwards every port, without network policy" lk_made
  lk_serve "$lk_v1" "$lk_w1" $P47240 $P47242 $P47243 $P47244 $P47249 80 >/dev/null 2>&1
  lk_serve "$lk_v2" "$lk_w2" $P47240 80 >/dev/null 2>&1
  printf '%s\n' "$lk_pl" | lk_put "$lk_vc" /var/tmp/e2e-link.pl >/dev/null 2>&1
  check "sshd -T in every VM: remote forwards on 127.77.*:* alone, at the address asked, no socket forwards, tunnels or X11" \
    lk_sshd_t "$lk_v1" "$lk_v2" "$lk_vc"
  check "Lima's own rules ignore every port of the client's" bash -c "
    [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $lk_vc) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]]"
  check "devvm link publish offers ports, and lists them, with \"version\": 1 in its --json" lk_publish
  check "a port the target doesn't publish is refused by devvm link add" lk_unpublished_add
  check "devvm link add on the running client applies within seconds: NAME:PORT and NAME.devvm.internal:PORT, by curl and a raw TCP client" lk_add_applies
  check "--as NAME names the link alone: the target's folder name doesn't resolve in the client" lk_as_alone
  check "two targets publishing the same port are each reached by their own name" lk_two
  check "a port below 1024 (80) is linked on the same port, sshd listening on it as the VM's user" lk_low
  check "devvm link lists each link, with its target, folder and status, and \"version\": 1 in its --json" lk_list
  check "devvm network on is refused for a client linked to a target without network policy" lk_on_refused
  check "a published port that isn't linked is refused: nothing listens for it in the client" lk_not_linked
  check "after devvm link unpublish, the link's next connection is refused and logged, and the clients are named" lk_unpublish
  check "devvm link rm on the running client applies within seconds: its listener goes" lk_rm_applies
  check "nothing from a link listens on the client's 127.0.0.1, ::1 or every address; its sshd listens only on its own port and the links' addresses" \
    lk_vm_clean "$lk_vc" $P47240 80
  check "the client forwards every port, and no link is forwarded to this machine" lk_not_forwarded
  check "a server of the client's own on its 127.0.0.1 with a link's port runs and reaches this machine as usual (the control); the link still reaches its target" lk_own_server
  check "each link's end on this machine is a socket of mode 600 in a folder of mode 700, and no TCP port of this machine's carries it" lk_sockets
  check "ssh -R into the client binds nothing on 0.0.0.0, 127.0.0.1, ::1, localhost, another range or a socket; on 127.77.0.0/16 it does (the control)" \
    rfwd_links "$lk_vc" lk_ssh
  check "a program in the client on a link's address and port first fails that link alone (link_port_taken); another link works" lk_port_taken
  check "256 connections at once, past the socket's backlog, all carried; a 257th is refused, and devvm network log shows link_limit" lk_cap
  check "root in the client lets sshd bind any address: the link session still asks only for its links' addresses, and the link works" lk_hostile_listen
  check "... with GatewayPorts yes too: the link session still listens on no TCP port here and connects only to the VMs' sshd, and the link works" lk_hostile_gateway
  echo "  recorded: $(cat "$work/link-gateway.log" 2>/dev/null)"; rm -f "$work/link-gateway.log"
  check "sshd put back, the session's next connection binds the links' addresses alone again" lk_honest_again
  check "devvm network log shows the links' connections and refusals without network policy" lk_log_off
  check "a stopped target: the connection fails, devvm network log shows link_target_stopped, the launch warns, and the target stays stopped" lk_stopped
  check "restarted, it works again, with its new host key" lk_restarted
  check "devvm rm of the target removes the client's link, names the client, and its sockets go; the client's session runs on" lk_rm_target
  check "a VM made again in that folder isn't linked" lk_made_again
  # Groups: the three VMs, none with network policy, in one group.
  lk_g=${lk_x}g
  check "three VMs join a group, the first making it, the trust said on each join: each reaches the ports the others publish by name" gp_join
  check "devvm group lists the three, each with its name, folder and published ports, and \"version\": 1 in its --json" gp_show
  check "a port a member publishes reaches the others' links at once" gp_publish
  check "in a group: devvm network on is refused, so is joining another group, and devvm link rm of the group's link" gp_refused
  check "devvm group leave removes only the group's links, in it and the others, naming them; the user's link stays; the last one's leaving removes the group" gp_leave
  check "a join is refused when a member's name is the joiner's own hostname, whatever name it joins as" gp_hostname
  check "devvm network on for a target while it runs, then for the client, restarted behind its proxy" lk_policy_on
  check "the target's network policy on but not yet enforced (devvm ls: unenforced): the link's next connection is refused and logged, and devvm link add and the client's launch warn" lk_policy_unenforced
  check "the target restarted behind its proxy" lk_policy_target
  check "with network policy, the client reaches NAME:PORT and NAME.devvm.internal:PORT by curl through its proxy settings and a raw TCP client; a direct connection fails (the control)" lk_policy_reach
  check "a link added while the client with network policy runs: curl through its proxy reaches it at once, by GET and CONNECT, no link name is in NO_PROXY, and nothing restarts" lk_proxy_added
  check "the client's proxy reaches only the links on its own list: not a name or port it hasn't linked, a name doubled or an address; the target's proxy doesn't reach the client's links" lk_proxy_own
  check "devvm network log shows the links' connections with network policy" lk_log_on
  check "a client with network policy is refused by devvm link add for a target without it" lk_mismatch_add
  check "devvm network off is refused for a target that a client with network policy links to" lk_off_refused
  check "the rule broken under devvm: the link's next connection is refused, and the client's launch warns and runs on without it" lk_policy_broken
  check "the rule restored, the client's next launch has the link again" lk_policy_back
  if want from; then
    check "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" lk_alpine
  else
    skip "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" "Alpine's image comes from the from section"
  fi
  check "a group is all or none: a VM without network policy is refused by a group with it, and one with it by a group without; two with it reach each other through their group's links" gp_policy
  check "devvm rm of the client leaves no link session, socket, pid file, log or link state" lk_rm_client
  (cd "$lk_p1" && quiet "$D" rm --force --no-export)
  (cd "$lk_p2" && quiet "$D" rm --force --no-export)
  ulimit -S -n "$nofile"
}
lk_made() { local v
  for v in "$lk_v1" "$lk_v2" "$lk_vc"; do
    if ! { [[ -n $v ]] && lk_running "$v" && vm_state "$(vmdir "$v")" | grep -qx net.on=false; }; then echo "${v:-a VM} wasn't made"; return 1; fi; done
  vm_state "$(vmdir "$lk_v1")" | grep -qx ports.forward=none && vm_state "$(vmdir "$lk_v2")" | grep -qx ports.forward=none &&
    ! vm_state "$(vmdir "$lk_vc")" | grep -q '^ports\.forward='; }
lk_sshd_t() { local vm t want
  for vm in "$@"; do
    t=$(lk_root "$vm" 's=$(command -v sshd || echo /usr/sbin/sshd); c=/etc/devvm/ssh/sshd_config; if [ -f $c ]; then $s -T -f $c; else $s -T; fi') ||
      { echo "$vm: no sshd -T"; return 1; }
    for want in 'allowtcpforwarding yes' 'gatewayports clientspecified' 'permitlisten 127.77.*:*' 'allowstreamlocalforwarding local' \
      'permittunnel no' 'x11forwarding no'; do
      grep -qxF "$want" <<<"$t" || { echo "$vm: not $want"; return 1; }
    done
  done; }
lk_publish() {
  # $P47243 on its own, as devvm link unpublish takes ports as they're listed.
  lk_dv "$lk_p1" link publish 80,$P47240-$P47242,$P47244 && lk_said "==> published ports=80,$P47240-$P47242,$P47244 vm=$lk_v1" &&
    lk_dv "$lk_p1" link publish $P47243 && lk_said "==> published ports=80,$P47240-$P47242,$P47244,$P47243 vm=$lk_v1" &&
    lk_dv "$lk_p2" link publish $P47240,80 && lk_said "==> published ports=$P47240,80 vm=$lk_v2" || return 1
  [[ $(cd "$lk_p1" && "$D" link publish 2>/dev/null) == 80,$P47240-$P47242,$P47244,$P47243 ]] &&
    lk_shows "$lk_p1" '^  "version": 1,$' link publish --json &&
    lk_shows "$lk_p1" "^  \"ports\": \"80,$P47240-$P47242,$P47244,$P47243\"\$" link publish --json; }
lk_unpublished_add() {
  ! lk_dv "$lk_pc" link add "${lk_x}db" $P47249 && lk_said "devvm: link_not_published port=$P47249 vm=$lk_v1" && ! vm_state "$(vmdir "$lk_vc")" | grep -q '^links\.links\.'; }
lk_add_applies() { local s=$SECONDS
  lk_dv "$lk_pc" link add "${lk_x}db" $P47240 --as db && lk_said "==> link_added name=db port=$P47240 target=$lk_v1 vm=$lk_vc" || return 1
  lk_until 15 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240" || return 1
  echo "reached $((SECONDS - s)) s after devvm link add began"
  lk_curls "$lk_vc" http://db.devvm.internal:$P47240/ "$lk_w1-$P47240" && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" &&
    lk_gets "$lk_vc" db.devvm.internal $P47240 "$lk_w1-$P47240" && [[ $(lk_addr "$lk_vc" db) == 127.77.0.1 ]]; }
lk_as_alone() { lk_resolves "$lk_vc" db && lk_resolves "$lk_vc" db.devvm.internal && ! lk_resolves "$lk_vc" "${lk_x}db" &&
  ! lk_resolves "$lk_vc" "${lk_x}db.devvm.internal"; }
lk_two() {
  lk_dv "$lk_pc" link add "$lk_n2" $P47240 && lk_said "==> link_added name=$lk_n2 port=$P47240 target=$lk_v2 vm=$lk_vc" || return 1
  lk_until 15 lk_gets "$lk_vc" "$lk_n2" $P47240 "$lk_w2-$P47240" && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" &&
    [[ $(lk_addr "$lk_vc" "$lk_n2") == 127.77.0.2 ]]; }
lk_low() {
  lk_dv "$lk_pc" link add "$lk_n2" 80 && lk_said "==> link_added name=$lk_n2 port=80 target=$lk_v2 vm=$lk_vc" || return 1
  echo "the client's ip_unprivileged_port_start: $(lk_root "$lk_vc" 'cat /proc/sys/net/ipv4/ip_unprivileged_port_start')"
  lk_until 15 lk_gets "$lk_vc" "$lk_n2" 80 "$lk_w2-80" && grep -qE '^sshd[^ ]* 127\.77\.0\.2:80$' <<<"$(lk_listens "$lk_vc")"; }
lk_list() { local l j
  l=$(cd "$lk_pc" && "$D" link 2>/dev/null); echo "$l"
  grep -Eq "^db +$P47240 +$lk_v1 +$lk_p1 +Running +yes\$" <<<"$l" && grep -Eq "^$lk_n2 +80 +$lk_v2 +$lk_p2 +Running +yes\$" <<<"$l" || return 1
  j=$(cd "$lk_pc" && "$D" link --json 2>/dev/null)
  grep -qxF '  "version": 1,' <<<"$j" && grep -qxF '      "name": "db",' <<<"$j" && grep -qxF "      \"target\": \"$lk_v2\"," <<<"$j"; }
lk_on_refused() { local r=0
  lk_dv "$lk_pc" network on && r=1
  cat "$work/last.log"
  (( r == 0 )) && grep -Eq "^devvm: network_on_linked targets=.*$lk_v1.* vm=$lk_vc\$" "$work/last.log" &&
    grep -Eq "^devvm: network_on_linked targets=.*$lk_v2.* vm=$lk_vc\$" "$work/last.log" && vm_state "$(vmdir "$lk_vc")" | grep -qx net.on=false; }
lk_not_linked() { local a; a=$(lk_raw "$lk_vc" db $P47244 2>&1); echo "db:$P47244: ${a:-nothing}"
  ! lk_raw "$lk_vc" db $P47244 >/dev/null 2>&1; }
lk_unpublish() {
  lk_dv "$lk_pc" link add "${lk_x}db" $P47243 || { tail -5 "$work/last.log"; return 1; }
  lk_until 15 lk_gets "$lk_vc" db $P47243 "$lk_w1-$P47243" || return 1
  lk_dv "$lk_p1" link unpublish $P47243 && lk_said "==> unpublished ports=$P47243 vm=$lk_v1" && lk_said "unpublished_linked vm=$lk_v1 vms=$lk_vc" || return 1
  lk_gets "$lk_vc" db $P47243 "" && lk_until 5 lk_event "$lk_vc" link_refused db $P47243 '"reason":"unpublished"' &&
    lk_shows "$lk_pc" "^db +$P47243 +$lk_v1 +$lk_p1 +Running +unpublished\$" link; }
lk_rm_applies() { local s=$SECONDS
  lk_dv "$lk_pc" link rm db $P47243 && lk_said "==> link_removed name=db port=$P47243 vm=$lk_vc" || return 1
  lk_until 15 lk_not lk_listening "$lk_vc" 127.77.0.1:$P47243 || return 1
  echo "gone $((SECONDS - s)) s after devvm link rm began"
  ! lk_raw "$lk_vc" db $P47243 >/dev/null 2>&1 && [[ ! -e $(lk_sock "$lk_vc" 127.77.0.1 $P47243) ]]; }
lk_listening() { grep -q " $2\$" <<<"$(lk_listens "$1")"; }
lk_not_forwarded() { sleep 3; lk_listening "$lk_vc" 127.77.0.1:$P47240 && ! nc -z 127.0.0.1 $P47240 && ! nc -z ::1 $P47240; }
lk_own_server() { local h
  lk_start "$lk_vc" serve 127.0.0.1 $P47240 own || return 1
  for _ in $(seq 20); do h=$(curl -s --max-time 5 http://127.0.0.1:$P47240/ 2>/dev/null); [[ $h != own ]] || break; sleep 1; done
  echo "this machine's 127.0.0.1:$P47240 answered: ${h:-nothing}"
  [[ $h == own ]] && lk_curls "$lk_vc" http://127.0.0.1:$P47240/ own && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" &&
    lk_end "$lk_vc" 127.0.0.1 $P47240; }
lk_sockets() { local s r=0 dir=$XDG_DATA_HOME/devvm/run
  for s in "$(lk_sock "$lk_vc" 127.77.0.1 $P47240)" "$(lk_sock "$lk_vc" 127.77.0.2 $P47240)" "$(lk_sock "$lk_vc" 127.77.0.2 80)"; do
    echo "$s: $(lk_mode "$s" 2>/dev/null)"; [[ -S $s && $(lk_mode "$s") == 600 ]] || r=1
  done
  echo "$dir: $(lk_mode "$dir")"; [[ $(lk_mode "$dir") == 700 ]] || r=1
  (( r == 0 )) && lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
lk_port_taken() {
  lk_start "$lk_vc" serve 127.77.0.1 $P47242 squatter || return 1
  lk_dv "$lk_pc" link add "${lk_x}db" $P47242 && lk_said "devvm: link_port_taken name=db port=$P47242 vm=$lk_vc" || return 1
  lk_gets "$lk_vc" db $P47242 squatter && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && lk_end "$lk_vc" 127.77.0.1 $P47242 &&
    lk_dv "$lk_pc" link rm db $P47242; }
lk_cap() { local out s r=0
  # A holding server of its own, started again on a retry: the first run's relays end with the old
  # one, before the 256 connections are opened.
  lk_end "$lk_v1" 127.0.0.1 $P47241 && lk_start "$lk_v1" hold 127.0.0.1 $P47241 || return 1
  if ! { s=$(lk_session "$lk_vc") && lk_until 60 lk_few "$s"; }; then echo "the link session's relays didn't end"; return 1; fi
  lk_dv "$lk_pc" link add "${lk_x}db" $P47241 || { cat "$work/last.log"; return 1; }
  out=$(lk_user "$lk_vc" "LC_ALL=C perl /var/tmp/e2e-link.pl conns db $P47241 257" 2>&1 | grep -E '^(closed:|connection )'); echo "$out"
  echo "events for db:$P47241: $(grep -F "\"host\":\"db\",\"port\":$P47241," "$(vmdir "$lk_vc")/logs/network.jsonl" | sed 's/.*"event":"\([a-z_]*\)".*/\1/' | sort | uniq -c | paste -sd ' ' -)"
  echo "their reasons: $(grep -F "\"host\":\"db\",\"port\":$P47241," "$(vmdir "$lk_vc")/logs/network.jsonl" | grep -v '"event":"link",' |
    sed -n 's/.*"event":"\([a-z_]*\)".*"reason":"\([^"]*\)".*/\1: \2/p' | sort | uniq -c | cut -c1-200 | paste -sd '|' -)"
  echo "the session's log: $(tail -3 "$(vmdir "$lk_vc")/logs/links.log" | paste -sd '|' -)"
  [[ $out == "closed: 257" ]] && lk_event "$lk_vc" link_limit db $P47241 '"reason":"256"' &&
    lk_shows "$lk_pc" "  REFUSED  db:$P47241  256 connections carried already\$" network log -n 100000 || r=1
  # Their relays end once the target's server lets them go. The link goes whatever came of the
  # checks: the group's links, later, would leave it out.
  lk_end "$lk_v1" 127.0.0.1 $P47241 && s=$(lk_session "$lk_vc") && lk_until 60 lk_few "$s" || r=1
  lk_dv "$lk_pc" link rm db $P47241 || r=1
  return "$r"; }
lk_few() { (( $(lk_desc "$1" | wc -l) < 20 )); }
lk_hostile_listen() {
  # Without the link to port 80, which GatewayPorts yes would put on the client's every address, and
  # its forwarder on this machine's port 80.
  lk_dv "$lk_pc" link rm "$lk_n2" 80 && lk_hostile "$lk_vc" || return 1
  # The control: sshd now binds an address it refused.
  lk_ssh -o ExitOnForwardFailure=yes -R 127.0.0.1:$P47248:127.0.0.1:9 "$lk_vc" true || { echo "sshd still refuses 127.0.0.1"; return 1; }
  lk_reconnect "$lk_vc" db $P47240 "$lk_w1-$P47240" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" $P47240 "$lk_w2-$P47240" &&
    lk_vm_clean "$lk_vc" $P47240 && lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
# With GatewayPorts yes, sshd binds every remote forward on the client's every address, whatever the
# host asked: db's on 0.0.0.0:$P47240, so the second target's on the same port can't bind. What it shows
# is printed; root in the client can put a relay of its own there as well.
lk_hostile_gateway() {
  lk_hostile "$lk_vc" gateway || return 1
  lk_reconnect "$lk_vc" db $P47240 "$lk_w1-$P47240" || return 1
  echo "the client's listeners on $P47240: $(lk_listens "$lk_vc" | grep -E ":$P47240\$" | paste -sd ' ' -)" | tee "$work/link-gateway.log"
  lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
lk_honest_again() {
  lk_honest "$lk_vc" && lk_reconnect "$lk_vc" db $P47240 "$lk_w1-$P47240" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" $P47240 "$lk_w2-$P47240" &&
    lk_vm_clean "$lk_vc" $P47240 && lk_sshd_t "$lk_vc"; }
lk_log_off() { local l
  l=$(cd "$lk_pc" && "$D" network log -n 100000 2>/dev/null)
  grep -E 'REFUSED|:80 ' <<<"$l" | sort -u -k2 | tail -5
  grep -Eq "  link +db:$P47240  to $lk_v1\$" <<<"$l" && grep -Eq "  link +$lk_n2:80  to $lk_v2\$" <<<"$l" &&
    grep -Eq "  REFUSED  db:$P47243  to $lk_v1: unpublished\$" <<<"$l" && lk_event "$lk_vc" link db $P47240 "\"target\":\"$lk_v1\""; }
# The link session reuses what the target's backend said of its run for a second, so a connection
# within a second of the stop may not see it: lk_stopped connects again until one does.
lk_stopped_seen() { lk_gets "$lk_vc" "$lk_n2" $P47240 "" && lk_event "$lk_vc" link_target_stopped "$lk_n2" $P47240; }
lk_stopped() {
  lk_dv "$lk_p2" stop && lk_until 30 lk_not lk_running "$lk_v2" || return 1
  lk_until 5 lk_stopped_seen || return 1
  lk_up "$lk_pc" && lk_said "devvm: link_target_stopped target=$lk_v2 vm=$lk_vc" || return 1
  sleep 5; lk_shows "$lk_pc" "^$lk_n2 +$P47240 +$lk_v2 +$lk_p2 +Stopped +yes\$" link &&
    lk_shows "$lk_pc" "  REFUSED  $lk_n2:$P47240  to $lk_v2: it isn't running\$" network log -n 100000 &&
    ! lk_running "$lk_v2"; }
lk_restarted() { local k0 k1
  k0=$(grep "^$lk_v2 " "$(vmdir "$lk_v2")/run/known_hosts" | sort)
  if ! { lk_up "$lk_p2" && lk_running "$lk_v2" && lk_serve "$lk_v2" "$lk_w2" $P47240 80; }; then tail -5 "$work/last.log"; return 1; fi
  k1=$(grep "^$lk_v2 " "$(vmdir "$lk_v2")/run/known_hosts" | sort)
  lk_until 20 lk_gets "$lk_vc" "$lk_n2" $P47240 "$lk_w2-$P47240" || return 1
  # Lima keeps a VM's host keys.
  echo "host key kept: $([[ $k0 == "$k1" ]] && echo yes || echo no)"; }
lk_rm_target() { local s
  s=$(lk_session "$lk_vc")
  lk_dv "$lk_p2" rm --force --no-export && lk_said "==> links_target_gone count=1 vm=$lk_v2 vms=$lk_vc" || return 1
  ! vm_state "$(vmdir "$lk_vc")" | grep '^links\.links\.[0-9]*\.target=' | grep -qF "$lk_v2" && lk_until 15 lk_not lk_resolves "$lk_vc" "$lk_n2" &&
    lk_until 15 lk_not test -e "$(lk_sock "$lk_vc" 127.77.0.2 $P47240)" && [[ ! -e $(lk_sock "$lk_vc" 127.77.0.2 80) ]] &&
    [[ -n $s && $(lk_session "$lk_vc") == "$s" ]] && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240"; }
lk_made_again() { local id0
  id0=$(vm_state "$(vmdir "$lk_vc")" | grep '^links\.links\.')
  launch "$lk_p2" exit --no-network-policy --no-git-protect --cpus 2 --memory 2 --ports none
  if ! { [[ $vm == "$lk_v2" ]] && lk_running "$lk_v2"; }; then echo "not made again: ${vm:-none}"; return 1; fi
  ! lk_shows "$lk_pc" "$lk_v2" link && ! lk_resolves "$lk_vc" "$lk_n2" &&
    [[ $(vm_state "$(vmdir "$lk_vc")" | grep '^links\.links\.') == "$id0" ]] && lk_dv "$lk_p2" link publish $P47240 && lk_serve "$lk_v2" "$lk_w2" $P47240; }
# The target gets network policy first, while it runs: until its next start it runs outside its
# proxy, its servers still up, and the client's link to it is refused then, and warned of by
# devvm link add and the client's launch (lk_policy_unenforced).
lk_policy_on() {
  if ! { lk_dv "$lk_p1" network on && vm_state "$(vmdir "$lk_v1")" | grep -qx net.on=true; }; then echo "$lk_v1:"; tail -5 "$work/last.log"; return 1; fi
  if ! { lk_dv "$lk_pc" network on && lk_up "$lk_pc" restart && vm_state "$(vmdir "$lk_vc")" | grep -qx net.on=true && vm_state "$(vmdir "$lk_vc")" | grep -qx net.made.check=ok; }; then
    echo "$lk_vc:"; tail -5 "$work/last.log"; return 1
  fi; }
lk_policy_unenforced() {
  lk_shows "$lk_p1" "^$lk_v1 +Running .* unenforced " ls || { echo "devvm ls doesn't show it unenforced"; return 1; }
  lk_root "$lk_v1" "test -s /var/tmp/e2e-link-127.0.0.1-$P47240.pid" || { echo "the target's server isn't up"; return 1; }
  lk_gets "$lk_vc" db $P47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db $P47240 '"reason":"unenforced"' || return 1
  # devvm link add and the client's launch warn of it, refusing nothing.
  lk_dv "$lk_pc" link add "${lk_x}db" $P47240 && lk_said "==> link_exists name=db port=$P47240 vm=$lk_vc" &&
    lk_said "devvm: link_target_unenforced target=$lk_v1 vm=$lk_vc" || return 1
  lk_up "$lk_pc" && lk_said "devvm: link_target_unenforced target=$lk_v1 vm=$lk_vc"; }
lk_policy_target() {
  if ! { lk_up "$lk_p1" restart && vm_state "$(vmdir "$lk_v1")" | grep -qx net.made.check=ok; }; then echo "$lk_v1:"; tail -5 "$work/last.log"; return 1; fi
  lk_serve "$lk_v1" "$lk_w1" $P47240 80; }
# The client got its link before its restart with network policy; lk_proxy_added adds one while it
# runs.
lk_policy_reach() {
  lk_user "$lk_vc" '[ -n "$HTTP_PROXY" ]' || { echo "no proxy settings"; return 1; }
  lk_until 20 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240" && lk_curls "$lk_vc" http://db.devvm.internal:$P47240/ "$lk_w1-$P47240" &&
    lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && ! lk_user "$lk_vc" "curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1/" >/dev/null 2>&1; }
# lk_via VM URL [-p]: the status curl in VM gets for URL through the VM's own proxy, whatever its
# NO_PROXY says (-p: by CONNECT, its answer's status), and the body's line on stdout's next line.
# lk_boot VM: its boot and its init's start, which a restart of the VM changes on every backend.
lk_via() { local w='%{http_code}'; [[ ${3:-} != -p ]] || w='%{http_connect}'
  lk_user "$1" "curl -s -m 10 --noproxy '' -x \"\$HTTP_PROXY\" ${3:-} -w '$w\\n' $2 2>/dev/null | tail -2 | tac 2>/dev/null || true" 2>/dev/null; }
lk_boot() { lk_root "$1" 'cat /proc/sys/kernel/random/boot_id; cut -d" " -f22 /proc/1/stat'; }
# lk_via_gets VM URL WORD [-p]: through the VM's proxy, URL answers 200 with WORD.
lk_via_gets() { local a; a=$(lk_via "$1" "$2" "${4:-}"); echo "$2${4:+ ($4)}: $(paste -sd ' ' - <<<"$a")"
  [[ $(head -1 <<<"$a") == 200 && $(sed -n 2p <<<"$a") == "$3" ]]; }
lk_proxy_added() { local b0 b1 k0 k1 n
  b0=$(lk_boot "$lk_vc") k0=$(grep "^$lk_vc " "$(vmdir "$lk_vc")/run/known_hosts" | sort)
  n=$(lk_user "$lk_vc" 'printf "%s|%s" "$NO_PROXY" "$no_proxy"'); echo "NO_PROXY|no_proxy: $n"
  [[ $n == "localhost,127.0.0.1,::1|localhost,127.0.0.1,::1" ]] || return 1
  lk_start "$lk_v1" serve 127.0.0.1 $P47242 "$lk_w1-$P47242" &&
    lk_dv "$lk_pc" link add "${lk_x}db" $P47242 --as db && lk_said "==> link_added name=db port=$P47242 target=$lk_v1 vm=$lk_vc" || return 1
  lk_until 15 lk_via_gets "$lk_vc" http://db:$P47242/ "$lk_w1-$P47242" &&
    lk_via_gets "$lk_vc" http://db.devvm.internal:$P47242/ "$lk_w1-$P47242" && lk_via_gets "$lk_vc" http://db:$P47242/ "$lk_w1-$P47242" -p &&
    lk_curls "$lk_vc" http://db:$P47242/ "$lk_w1-$P47242" && lk_gets "$lk_vc" db $P47242 "$lk_w1-$P47242" || return 1
  b1=$(lk_boot "$lk_vc") k1=$(grep "^$lk_vc " "$(vmdir "$lk_vc")/run/known_hosts" | sort)
  echo "boot and init before: $(paste -sd ' ' - <<<"$b0"), after: $(paste -sd ' ' - <<<"$b1")"
  [[ -n $b0 && $b0 == "$b1" && $k0 == "$k1" ]] && [[ $(lk_user "$lk_vc" 'printf "%s" "$NO_PROXY"') == localhost,127.0.0.1,::1 ]] &&
    lk_event "$lk_vc" link db $P47242 "\"target\":\"$lk_v1\""; }
lk_proxy_own() { local u c l0 l1 log; log=$(vmdir "$lk_vc")/logs/network.jsonl
  l0=$(grep -c '"event":"link' "$log" 2>/dev/null)
  for u in "http://${lk_x}db:$P47240/" http://db:$P47244/ http://db:$P47243/ http://db.devvm.internal.devvm.internal:$P47240/ \
    http://devvm.internal:$P47240/ http://127.77.0.1:$P47240/; do
    c=$(lk_via "$lk_vc" "$u" | head -1); echo "$u: $c"; [[ $c == 403 ]] || return 1
    c=$(lk_via "$lk_vc" "$u" -p | head -1); echo "$u (CONNECT): $c"; [[ $c == 403 ]] || return 1
  done
  l1=$(grep -c '"event":"link' "$log" 2>/dev/null); echo "link events: $l0 before, $l1 after"
  # The target has network policy and no links: the client's link names are nothing to its proxy.
  c=$(lk_via "$lk_v1" http://db:$P47240/ | head -1); echo "the target's proxy, http://db:$P47240/: $c"; [[ $c == 403 ]] || return 1
  c=$(lk_via "$lk_v1" http://db:$P47240/ -p | head -1); echo "  (CONNECT): $c"
  [[ $c == 403 && $l0 == "$l1" ]] && lk_via_gets "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240"; }
lk_log_on() { local t0 last
  t0=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && vm_state "$(vmdir "$lk_vc")" | grep -qx net.on=true || return 1
  last=$(cat "$(vmdir "$lk_vc")/logs/network.jsonl.1" "$(vmdir "$lk_vc")/logs/network.jsonl" 2>/dev/null |
    grep -F "\"event\":\"link\",\"host\":\"db\",\"port\":$P47240," | tail -1)
  echo "the last: ${last:-none}, since $t0"
  [[ $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") > "$t0" || $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") == "$t0" ]] &&
    lk_shows "$lk_pc" "  link +db:$P47240  to $lk_v1\$" network log -n 100000; }
lk_mismatch_add() {
  ! lk_dv "$lk_pc" link add "$lk_n2" $P47240 && lk_said "devvm: link_policy_mismatch target=$lk_v2 vm=$lk_vc" && ! lk_resolves "$lk_vc" "$lk_n2"; }
lk_off_refused() {
  ! lk_dv "$lk_p1" network off && lk_said "devvm: network_off_linked vm=$lk_v1 vms=$lk_vc" && vm_state "$(vmdir "$lk_v1")" | grep -qx net.on=true; }
lk_policy_broken() {
  lk_set_net "$lk_v1" off || return 1
  lk_gets "$lk_vc" db $P47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db $P47240 '"reason":"policy"' &&
    lk_up "$lk_pc" && lk_said "devvm: link_policy_mismatch target=$lk_v1 vm=$lk_vc" && lk_running "$lk_vc" &&
    lk_until 10 lk_not lk_resolves "$lk_vc" db; }
lk_policy_back() {
  lk_set_net "$lk_v1" on && lk_up "$lk_pc" && ! grep -q link_policy_mismatch "$work/last.log" &&
    lk_until 10 lk_resolves "$lk_vc" db && lk_until 20 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240"; }
lk_alpine() { local pa va r=0
  pa=$(newproj "${lk_x}alp")
  nd launch "$pa" exit --from "$from_alpine" --no-network-policy --no-git-protect --cpus 2 --memory 2 --ports none; va=$vm
  if ! { [[ -n $va ]] && vm_state "$(vmdir "$va")" | grep -qx tools.made.libc=musl; }; then echo "no Alpine VM"; tail -5 "$work/last.log"; return 1; fi
  lk_dv "$pa" link add "${lk_x}db" 80 && lk_said "==> link_added name=${lk_x}db port=80 target=$lk_v1 vm=$va" || r=1
  (( r == 0 )) && echo "its ip_unprivileged_port_start: $(lk_root "$va" 'cat /proc/sys/net/ipv4/ip_unprivileged_port_start')" &&
    lk_until 20 lk_gets "$va" "${lk_x}db" 80 "$lk_w1-80" || r=1
  (cd "$pa" && quiet "$D" rm --force --no-export)
  return "$r"; }
# Groups, on links: the client publishes a port of its own first, so each of the three has one the
# others reach. The client's own link to the first target, db, names its group links too. A server
# that doesn't start fails this check alone: the three still join, for the checks after.
gp_join() { local na=${lk_x}app r=0
  lk_dv "$lk_pc" link publish $P47246 || { echo "failed: devvm link publish $P47246"; return 1; }
  lk_start "$lk_vc" serve 127.0.0.1 $P47246 "$na-$P47246" || r=1
  gp_joined && (( r == 0 )); }
gp_joined() { local n1=${lk_x}db na=${lk_x}app
  lk_dv "$lk_pc" group join "$lk_g" && lk_said "==> group_made group=$lk_g vm=$lk_vc" && lk_said "group_trust group=$lk_g" || return 1
  lk_dv "$lk_p1" group join "$lk_g" && lk_said "==> group_joined group=$lk_g vm=$lk_v1" && lk_said "group_trust group=$lk_g" &&
    lk_said "==> group_links_added group=$lk_g links=db:80,$P47241-$P47242,$P47244 vm=$lk_vc" &&
    lk_said "==> group_links_added group=$lk_g links=$na:$P47246 vm=$lk_v1" || return 1
  lk_dv "$lk_p2" group join "$lk_g" && lk_said "==> group_joined group=$lk_g vm=$lk_v2" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:$P47240 vm=$lk_v1" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:$P47240 vm=$lk_vc" &&
    lk_said "==> group_links_added group=$lk_g links=$na:$P47246, $n1:80,$P47240-$P47242,$P47244 vm=$lk_v2" || return 1
  lk_until 15 lk_gets "$lk_v2" "$n1" $P47240 "$lk_w1-$P47240" && lk_until 15 lk_gets "$lk_v2" "$na" $P47246 "$na-$P47246" &&
    lk_until 15 lk_gets "$lk_v1" "$lk_n2" $P47240 "$lk_w2-$P47240" && lk_until 15 lk_gets "$lk_v1" "$na" $P47246 "$na-$P47246" &&
    lk_until 15 lk_gets "$lk_vc" "$lk_n2" $P47240 "$lk_w2-$P47240" && lk_until 15 lk_gets "$lk_vc" db $P47244 "$lk_w1-$P47244" &&
    lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && lk_shows "$lk_pc" "^$lk_n2 +$P47240 +$lk_v2 +$lk_p2 +Running +yes\$" link; }
gp_show() { local l
  l=$(cd "$lk_pc" && "$D" group 2>/dev/null); echo "$l"
  grep -Eq "^${lk_x}db +$lk_v1 +$lk_p1 +80,$P47240-$P47242,$P47244\$" <<<"$l" && grep -Eq "^$lk_n2 +$lk_v2 +$lk_p2 +$P47240\$" <<<"$l" &&
    grep -Eq "^${lk_x}app +$lk_vc +$lk_pc +$P47246\$" <<<"$l" && lk_shows "$lk_pc" '^  "version": 1,$' group --json &&
    lk_shows "$lk_p2" "^  \"group\": \"$lk_g\",\$" group --json; }
gp_publish() {
  lk_start "$lk_v2" serve 127.0.0.1 $P47245 "$lk_w2-$P47245" && lk_dv "$lk_p2" link publish $P47245 &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:$P47245 vm=$lk_v1" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:$P47245 vm=$lk_vc" || return 1
  lk_until 15 lk_gets "$lk_v1" "$lk_n2" $P47245 "$lk_w2-$P47245" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" $P47245 "$lk_w2-$P47245"; }
# Only a member is refused: outside the group, network on would turn the second target's policy on,
# failing the checks after this one that need it off.
gp_refused() {
  vm_state "$(vmdir "$lk_v2")" | grep -qx "links.group=$lk_g" || { echo "$lk_v2 isn't in $lk_g: network on not tried"; return 1; }
  ! lk_dv "$lk_p2" network on &&lk_said "devvm: group_policy_mixed group=$lk_g vm=$lk_v2" && vm_state "$(vmdir "$lk_v2")" | grep -qx net.on=false &&
    ! lk_dv "$lk_p2" group join "${lk_x}other" && lk_said "devvm: group_other group=$lk_g vm=$lk_v2" &&
    ! lk_dv "$lk_pc" link rm "$lk_n2" $P47240 && lk_said "devvm: link_from_group group=$lk_g name=$lk_n2 port=$P47240 vm=$lk_vc"; }
# Each leaves, whatever comes of the checks, so the sections after run without a group.
gp_leave() { local r=0
  lk_dv "$lk_p2" group leave && lk_said "==> group_left group=$lk_g vm=$lk_v2" &&
    lk_said "==> group_links_removed group=$lk_g links=$lk_n2:$P47240,$P47245 vm=$lk_v1" &&
    lk_said "==> group_links_removed group=$lk_g links=$lk_n2:$P47240,$P47245 vm=$lk_vc" &&
    lk_said "==> group_links_removed group=$lk_g links=${lk_x}app:$P47246, ${lk_x}db:80,$P47240-$P47242,$P47244 vm=$lk_v2" || r=1
  (( r == 0 )) && lk_until 15 lk_not lk_resolves "$lk_v1" "$lk_n2" && lk_until 15 lk_not lk_resolves "$lk_vc" "$lk_n2" &&
    lk_until 15 lk_not lk_resolves "$lk_v2" "${lk_x}db" || r=1
  lk_dv "$lk_p1" group leave || r=1
  lk_dv "$lk_pc" group leave && lk_said "==> group_gone group=$lk_g" || r=1
  # The client's own link to db stays; the group's to db's other ports go.
  (( r == 0 )) && lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && lk_until 15 lk_not lk_listening "$lk_vc" 127.77.0.1:$P47244 &&
    lk_shows "$lk_pc" "^db +$P47240 +$lk_v1 " link && ! lk_shows "$lk_pc" "^db +$P47244 " link || r=1
  lk_dv "$lk_p2" link unpublish $P47245; lk_end "$lk_v2" 127.0.0.1 $P47245
  lk_dv "$lk_pc" link unpublish $P47246; lk_end "$lk_vc" 127.0.0.1 $P47246
  return "$r"; }
# The client alone in a group as db, the first target's folder part: the target is refused, as
# itself or another name, and stays out. The client leaves whatever comes of the checks.
gp_hostname() { local r=0 g=${lk_x}h n1=${lk_x}db
  lk_dv "$lk_pc" group join "$g" --as "$n1" && lk_said "==> group_made group=$g vm=$lk_vc" || r=1
  (( r == 0 )) && ! lk_dv "$lk_p1" group join "$g" && lk_said "devvm: group_name_hostname group=$g member=$lk_vc name=$n1 vm=$lk_v1" &&
    ! lk_dv "$lk_p1" group join "$g" --as "${n1}2" && lk_said "devvm: group_name_hostname group=$g member=$lk_vc name=$n1 vm=$lk_v1" &&
    ! vm_state "$(vmdir "$lk_v1")" | grep -q "^links\.group=$g" || r=1
  lk_dv "$lk_pc" group leave && lk_said "==> group_gone group=$g" || r=1
  return "$r"; }
# All or none, with the client and the first target with network policy and the second without.
gp_policy() { local r=0 on=${lk_x}on off=${lk_x}off
  lk_dv "$lk_pc" group join "$on" && lk_said "==> group_made group=$on vm=$lk_vc" || r=1
  (( r == 0 )) && ! lk_dv "$lk_p2" group join "$on" && lk_said "devvm: group_policy_on group=$on vm=$lk_v2" || r=1
  (( r == 0 )) && lk_dv "$lk_p2" group join "$off" && ! lk_dv "$lk_p1" group join "$off" &&
    lk_said "devvm: group_policy_off group=$off vm=$lk_v1" && ! vm_state "$(vmdir "$lk_v1")" | grep -q "^links\.group=$off" || r=1
  # The client's own links to db are $P47240 and $P47242 (lk_proxy_added).
  (( r == 0 )) && lk_dv "$lk_p1" group join "$on" && lk_said "==> group_links_added group=$on links=db:80,$P47241,$P47244 vm=$lk_vc" &&
    lk_until 15 lk_gets "$lk_vc" db 80 "$lk_w1-80" && lk_curls "$lk_vc" http://db:80/ "$lk_w1-80" || r=1
  lk_dv "$lk_p1" group leave; lk_dv "$lk_pc" group leave; lk_dv "$lk_p2" group leave
  (( r == 0 )) && ! { vm_state "$(vmdir "$lk_v1")"; vm_state "$(vmdir "$lk_vc")"; vm_state "$(vmdir "$lk_v2")"; } | grep -q '^links\.group=.'; }
lk_rm_client() {
  lk_dv "$lk_pc" rm --force --no-export || { tail -5 "$work/last.log"; return 1; }
  lk_until 10 lk_not lk_session "$lk_vc" || { echo "its session runs"; return 1; }
  [[ -z $(lk_session "$lk_vc") ]] && ! ls "$XDG_DATA_HOME/devvm/run/"link-* >/dev/null 2>&1 &&
    [[ ! -e $(vmdir "$lk_vc") ]] && ! st_all | grep -qx "links\.links\.[0-9]*\.target=$lk_vc"; }

if $clean; then
  section "cleaning cached images"
  for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
  rm -rf "$images" "$share"
  # A newest base's packages come from the mirrors, not from an earlier run's cache.
  ! $newest || rm -rf "$XDG_DATA_HOME/devvm/packages"
fi

if begin images; then
section "images"
check "devvm doctor finds everything it needs on this host" bash -c "'$D' doctor </dev/null >'$work/last.log' 2>&1 &&
  '$D' doctor --json </dev/null | grep -qxF '  \"problems\": [],'"
# base stays cached between runs (--clean deletes it); the fixture layers' images are this run's.
check "build base" quiet env DEVVM_MESSAGES=names "$D" build base
# Its key: the build's, else the newest base from Ubuntu's image (from's bases are cached too).
kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
# A newest base pins the newest release and boots from it: a fallback to the release before
# (origin_no_boot) passes for a user, but is the drift this run looks for.
if $newest; then
  check "base pinned Ubuntu's newest cloud image and booted from it, with no fallback to the release before" bash -c "
    grep -qx '==> origin_pinned origin=$base_release' '$work/last.log' && ! grep -q origin_no_boot '$work/last.log'"
fi
if [[ -n $kb ]] && $features_on; then
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/last.log")
  check "base ran base.sh, then every feature" bash -c "
    for f in base $features; do grep -q \"^# \\[[0-9]*/[0-9]*\\] running_script script=\$f.sh vm=devvm-img-$kb\$\" '$runlog' || exit 1; done"
elif $features_on; then
  skip "base ran base.sh, then every feature" "base is cached; from checks the features"
fi
fixture=$layers/e2efix.sh
token=e2e-fixture-ran-$$-$(date +%s)   # this run's: the run logs keep earlier runs'
rm -f "$fixture" "${fixture%.sh}.policy"
check "layer new writes a starter layer" quiet "$D" layer new e2efix
printf 'echo %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" >> "$fixture"
check "build the fixture layer on base" quiet "$D" build e2efix
owned e2efix
kf=$(keys_of e2efix | head -1)
# A cached base's key: the one the fixture was built on (other selections' bases, with other
# features, are cached too, and may be newer).
[[ -n $kb ]] || kb=$(rec "$kf" chain | sed -n 's/^base=\([0-9a-f]*\) e2efix=.*$/\1/p')
# pinned FIELD: the cloud image pinned in the sandbox, as devvm recorded it.
pinned() { sed -n "s/^  \"$1\": \"\(.*\)\",\{0,1\}\$/\1/p" "$images/ubuntu.json" 2>/dev/null; }
export -f pinned
check "base is a stopped image, recorded with Ubuntu's pinned cloud image and its C library" bash -c "
  [[ -n '$kb' && \$(status_of devvm-img-$kb) == Stopped ]] &&
  [[ \$(rec $kb chain) == 'base=$kb' && \$(rec $kb libc) == gnu && -z \$(rec $kb from) ]] &&
  rec $kb built | grep -qE '^[0-9-]+T[0-9:]+Z$' && [[ \$(pinned release) == $base_release ]] &&
  [[ \$(rec $kb digest) == \"https://cloud-images.ubuntu.com/releases/resolute/$base_release/\$(pinned file) \$(pinned digest)\" ]] &&
  pinned digest | grep -qE '^sha256:[0-9a-f]{64}$'"
check "base mounts nothing from the host" bash -c "[[ -z \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' devvm-img-$kb) ]]"
check "devvm limactl runs Lima on devvm's Lima home, whatever LIMA_HOME says, with its exit code" bash -c "
  [[ -n '$kb' ]] && LIMA_HOME='$work/no-lima' '$D' limactl list -q </dev/null | grep -qx devvm-img-$kb &&
  ! '$D' limactl e2e-no-such-command </dev/null >/dev/null 2>&1"
check "building base again reuses it" bash -c "out=\$('$D' build base </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=base key=$kb)\" <<<\"\$out\""
check "its script ran in its image, cloned from base's" bash -c "
  [[ -n '$kf' && \$(status_of devvm-img-$kf) == Stopped ]] &&
  [[ \$(rec $kf chain) == 'base=$kb e2efix=$kf' ]] &&
  grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
echo "# edited" >> "$fixture"
check "editing it builds a new image; base and the old one stay" bash -c "'$D' build e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 2 ]] && [[ \$(status_of devvm-img-$kf) == Stopped ]] && [[ -f '$images/$kb/record.json' ]]"
owned e2efix
check "rebuild makes a new one and deletes the stale ones, not base" bash -c "'$D' rebuild --force e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 1 ]] && [[ \$(keys_of e2efix) != $kf ]] && [[ -z \$(status_of devvm-img-$kf) ]] &&
  [[ \$(status_of devvm-img-$kb) == Stopped ]] && grep -qF '\"name\": \"e2efix\"' '$images/rebuilds.json'"
owned e2efix
kf=$(keys_of e2efix | head -1)
dg=$(rec "$kb" digest | sed -n 's/^.* sha256:\([0-9a-f]\{12\}\)[0-9a-f]*$/\1/p')
check "devvm images lists base and the fixture, current, with Ubuntu's release, digest and size" bash -c "
  out=\$('$D' images </dev/null 2>/dev/null) &&
  grep -qE '^base +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kb +current +[0-9.]+[KMGT]$' <<<\"\$out\" &&
  grep -qE '^e2efix +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kf +current +[0-9.]+[KMGT]$' <<<\"\$out\" &&
  grep -qE '^NAME +TOOL +VERSION +LIBC +SIZE +VMS +STATE$' <<<\"\$out\""
check "images rm deletes an image by name, with its record, not base" bash -c "
  [[ -n '$kf' ]] && '$D' images rm --force e2efix </dev/null >/dev/null 2>&1 &&
  [[ -z \$(status_of devvm-img-$kf) ]] && [[ ! -e '$images/$kf' ]] && [[ \$(status_of devvm-img-$kb) == Stopped ]]"

# The user's CAs: a build trusts them while its scripts run, with the variables for tools that don't
# read the system's store, and its image keeps none of them, so its key is the same without them. A
# copy of the image, booted without devvm (no setup), shows what the image holds.
e2e_ca
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
catoken=e2e-ca-trusted-$$-$(date +%s)
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt ]]' \
  'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /etc/ssl/devvm.pem' "$catoken" > "$layers/e2eca.sh"
check "a build trusts the CAs in devvm's CA folder while its scripts run" bash -c "'$D' build e2eca </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$catoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2eca
kc=$(keys_of e2eca | head -1)
rm -rf "$cafolder"
check "its key is the same without them: nothing to build" bash -c "[[ -n '$kc' ]] && out=\$('$D' build e2eca </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=e2eca key=$kc)\" <<<\"\$out\" && [[ \$(keys_of e2eca) == '$kc' ]]"
created+=(devvm-e2e-cacheck)
check "its image keeps none of them" bash -c "[[ -n '$kc' ]] &&
  limactl clone --tty=false --mount-none devvm-img-$kc devvm-e2e-cacheck >/dev/null 2>&1 &&
  limactl start --tty=false devvm-e2e-cacheck >/dev/null 2>&1 && ca_gone devvm-e2e-cacheck"
limactl delete -f devvm-e2e-cacheck >/dev/null 2>&1
rm -f "$layers/e2eca.sh"

# Java's cacerts: ca-certificates-java's hook adds the CAs a build trusts (debian:devvm-extra-N.pem)
# to it, and it's told of each one devvm takes out again, so the image's cacerts keeps none. A
# project VM on that image gets the CA in cacerts at its launch, and drops it at the launch after
# it's removed from devvm's CA folder.
e2e_ca
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
jtoken=e2e-java-ca-$$-$(date +%s)
jlist='keytool -list -cacerts -storepass changeit 2>/dev/null | grep -i "^debian:devvm-extra-"'
jnone="keytool -list -cacerts -storepass changeit >/dev/null 2>&1 && ! { $jlist; }"
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends default-jre-headless ca-certificates-java >/dev/null' \
  "$jlist" "$jtoken" > "$layers/e2ejava.sh"
check "a layer that installs Java has the CAs in Java's cacerts while it builds" bash -c "'$D' build e2ejava </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$jtoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2ejava
kj=$(keys_of e2ejava | head -1)
created+=(devvm-e2e-javacheck)
check "its image's cacerts keeps none of them" bash -c "[[ -n '$kj' ]] &&
  limactl clone --tty=false --mount-none devvm-img-$kj devvm-e2e-javacheck >/dev/null 2>&1 &&
  limactl start --tty=false devvm-e2e-javacheck >/dev/null 2>&1 &&
  limactl shell --workdir / devvm-e2e-javacheck bash -c '$jnone' </dev/null"
limactl delete -f devvm-e2e-javacheck >/dev/null 2>&1
if [[ -n $kj ]]; then
  pj=$(newproj e2ejava)
  launch "$pj" 'exit' --no-network-policy --layers e2ejava
  check "a project VM with Java has the CA in cacerts" bash -c "[[ -n '$vm' ]] &&
    limactl shell --workdir / '$vm' bash -c '$jlist' </dev/null"
  rm -rf "$cafolder"
  check "removed from devvm's CA folder, it's gone from cacerts from the VM's next launch" bash -c "[[ -n '$vm' ]] &&
    cd '$pj' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
    limactl shell --workdir / '$vm' bash -c '$jnone' </dev/null"
  [[ -z $vm ]] || (cd "$pj" && "$D" rm --force --no-export </dev/null >/dev/null 2>&1)
else
  skip "a project VM with Java has the CA in cacerts" "the Java layer didn't build"
  skip "removed from devvm's CA folder, it's gone from cacerts from the VM's next launch" "the Java layer didn't build"
fi
rm -rf "$cafolder"
rm -f "$layers/e2ejava.sh"

# The host's proxy: a build goes through a proxy of devvm's for it alone, which alone has the
# host's settings. The build's commands get only that proxy's address with a password of the
# build's, sudo keeping them, and its image keeps none of it (Lima writes the settings it
# propagates into the guest's /etc/environment). The host's proxy is a closed port with a password
# of this run's, and NO_PROXY=* keeps every tool off the proxies, as the layer needs no network.
# Its disk and Lima's files for it are read wherever they have data, free space included, skipping
# only the disk's holes.
pxpass=e2e-proxy-pw-$$-$(date +%s)
pxtoken=e2e-proxy-seen-$$-$(date +%s)
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  '[[ $HTTPS_PROXY == http://devvm:*@192.168.5.2:* && $http_proxy == "$HTTPS_PROXY" && $(sudo printenv HTTPS_PROXY) == "$HTTPS_PROXY" ]]' \
  '[[ $HTTPS_PROXY != *e2e-proxy-pw-* && $NO_PROXY == "*" ]]' \
  "$pxtoken" > "$layers/e2eproxy.sh"
check "a build's commands get a proxy of devvm's for the build, never the host's own settings" bash -c "
  env HTTPS_PROXY='http://corp:$pxpass@127.0.0.1:9' HTTP_PROXY='http://corp:$pxpass@127.0.0.1:9' NO_PROXY='*' \
    '$D' build e2eproxy </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$pxtoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log &&
  ! ps -axww -o command= | grep -q -- '[_]proxy -config $XDG_DATA_HOME/devvm/run/builds/.*/build-proxy/proxy.json' &&
  [[ -z \$(ls -d '$XDG_DATA_HOME'/devvm/run/builds/*/run/build-proxy 2>/dev/null) ]]"
owned e2eproxy
kp=$(keys_of e2eproxy | head -1)
check "its image keeps none of them: not its disk, Lima's files for it or devvm's logs" bash -c "
  [[ -n '$kp' && -d '$LIMA_HOME/devvm-img-$kp' ]] &&
  printf '%s\n' '$pxpass' | no_leak '$LIMA_HOME/devvm-img-$kp' '$XDG_DATA_HOME/devvm/logs' '$(vmdir "devvm-img-$kp")/logs'"
rm -f "$layers/e2eproxy.sh"

# A failed script keeps its image's VM, stopped and without a record, for inspection.
printf '#!/bin/bash\nexit 3\n' > "$layers/e2efail.sh"
DEVVM_MESSAGES=names "$D" build e2efail </dev/null >"$work/fail.log" 2>&1; rc=$?
kx=$(sed -n 's/^==> building_image image=e2efail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
[[ -z $kx ]] || created+=("devvm-img-$kx")
check "a failed script keeps its VM, stopped, and says how to look inside" bash -c "
  [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: script_failed script=e2efail\.sh vm=devvm-img-$kx$' '$work/fail.log' &&
  [[ \$(status_of devvm-img-$kx) == Stopped ]] && [[ ! -e '$images/$kx' ]]"
check "devvm images lists it as unfinished, and images rm deletes it by its key" bash -c "
  [[ -n '$kx' ]] && '$D' images </dev/null 2>/dev/null | grep -qE '^- +- +- +$kx +unfinished +' &&
  '$D' images rm --force '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(status_of devvm-img-$kx) ]]"
rm -f "$layers/e2efail.sh"

# Ctrl+C during a build: the run's names (DEVVM_MESSAGES) say which image it builds, its run log,
# and (in that log) when its script runs.
printf '#!/bin/bash\nsleep 600\n' > "$layers/e2eslow.sh"
DEVVM_MESSAGES=names "$D" build e2eslow </dev/null >"$work/slow.log" 2>&1 &
slow=$!
for _ in $(seq 300); do
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/slow.log")
  [[ -n $runlog ]] && grep -qE '^# \[[0-9]+/[0-9]+\] running_script script=e2eslow\.sh ' "$runlog" 2>/dev/null && break
  sleep 1
done
ks=$(sed -n 's/^==> building_image image=e2eslow key=\([0-9a-f]*\)$/\1/p' "$work/slow.log")
[[ -z $ks ]] || created+=("devvm-img-$ks")
kill -INT "$slow" 2>/dev/null
wait "$slow"; rc=$?
check "Ctrl+C during a build deletes the unfinished image, and exits 130" bash -c "
  [[ -n '$ks' && $rc -eq 130 ]] && grep -qx '==> interrupted_image vm=devvm-img-$ks' '$work/slow.log' &&
  [[ -z \$(status_of devvm-img-$ks) ]] && [[ ! -e '$images/$ks' ]] && [[ \$(status_of devvm-img-$kb) == Stopped ]]"
rm -f "$fixture" "$layers/e2eslow.sh"

# base's packages: its first build recorded each package apt downloaded, kept in devvm's cache under
# its SHA-256. Built again under the same key, with the mirror set to an address nothing answers on,
# it installs them from the cache alone: apt reads only the build's own folder.
set_json=$XDG_DATA_HOME/devvm/packages/sets/$kb.json
check "base recorded its packages, each in the cache under its own SHA-256" bash -c "
  [[ -n '$kb' && -f '$set_json' ]] && sums=\$(sed -n 's/^ *\"sha256\": \"\([0-9a-f]*\)\",\$/\1/p' '$set_json') &&
  [[ -n \$sums ]] && for s in \$sums; do
    [[ \$({ sha256sum 2>/dev/null || shasum -a 256; } < '$XDG_DATA_HOME/devvm/packages/'\$s.deb | cut -c1-64) == \$s ]] || exit 1
  done"
cp "$XDG_CONFIG_HOME/devvm/config.toml" "$work/config.toml.saved"
printf 'ubuntu-mirror = "http://127.0.0.1:9/ubuntu"\n' >> "$XDG_CONFIG_HOME/devvm/config.toml"
[[ -z $kb ]] || { limactl delete -f "devvm-img-$kb" >/dev/null 2>&1; rm -rf "${images:?}/$kb"; }
check "with the mirror unreachable, base builds again under the same key from the cache, no network for its packages" bash -c "
  [[ -n '$kb' ]] && DEVVM_MESSAGES=names '$D' build base </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '==> building_image image=base key=$kb' '$work/last.log' &&
  runlog=\$(sed -n 's/^ *log_details file=//p' '$work/last.log') && [[ -f \$runlog ]] &&
  grep -q ' file:/var/cache/devvm/packages/in ' \"\$runlog\" && ! grep -qE '^(Get|Hit|Err|Ign):[0-9]+ https?://' \"\$runlog\" &&
  [[ \$(status_of devvm-img-$kb) == Stopped && -f '$images/$kb/record.json' ]]"
cp "$work/config.toml.saved" "$XDG_CONFIG_HOME/devvm/config.toml"
# The golden fixtures (--golden): real Lima's dummy instances and its version, base's disk, and
# the Keychain's and Time Machine's answers.
if $golden; then
  if (cd "$root" && STUB_GOLDEN=$golden_dir CGO_ENABLED=0 "${gocmd[@]}" test -count=1 -run '^TestLimactlStubContract$' ./internal/backend/lima) >"$work/golden.log" 2>&1; then
    echo "golden: limactl-version.txt limactl-list-stopped.json"
  else
    echo "golden: capturing real Lima's dummy instances failed"; tail -5 "$work/golden.log"
  fi
  [[ $host_os != Linux || -z $kb ]] || golden qemu-img-info.json qemu-img info -U --output=json "$DEVVM_LIMA_HOME/devvm-img-$kb/disk"
  if [[ $host_os == Darwin ]]; then
    golden security-missing.txt bash -c 'security find-generic-password -s devvm-go-e2e-golden -a none -w; echo "exit $?"'
    golden tmutil-isexcluded.txt tmutil isexcluded "$work"
  fi
fi
fi

if begin project; then
section "a base project"
p=$(newproj web); git -C "$p" init -q
# The shell devvm opens reads its commands from devvm's input: these, then it ends.
launch "$p" 'pwd; echo e2e-shell-ran' --no-network-policy --cpus 2 --memory 3 --disk 120
# The launch's output, for the .git protection checks: the checks before them overwrite last.log.
cp "$work/last.log" "$work/web-launch.log"
# A running VM, as the golden fixtures have it (--golden), and its overlay disk on Linux.
if [[ -n $vm ]]; then
  golden limactl-list-running.json limactl list --json "$vm"
  [[ $host_os != Linux ]] || golden qemu-img-info-overlay.json qemu-img info -U --output=json "$DEVVM_LIMA_HOME/$vm/disk"
fi
check "devvm creates the VM from base, and opens a shell in the folder" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx '$p' '$work/last.log' && grep -qx e2e-shell-ran '$work/last.log' &&
  vm_state '$(vmdir "$vm")' | grep -qx image=base && vm_state '$(vmdir "$vm")' | grep -qx net.on=false &&
  vm_state '$(vmdir "$vm")' | grep -qx 'dir=$p'"
# devvm in a subfolder opens the folder's VM there, says so, and makes none.
mkdir -p "$p/src/lib"
check "devvm in a subfolder opens the folder's VM in the subfolder" bash -c "
  cd '$p/src/lib' && printf 'pwd\n' | '$D' >'$work/last.log' 2>&1 &&
  grep -qx '$p/src/lib' '$work/last.log' && grep -qF \"\$(m using_parent_vm dir='$p' vm='$vm')\" '$work/last.log' &&
  ! for f in '$vms'/*/vm.json; do vm_state \"\$f\"; done | grep -qE '^dir=$p/src(/lib)?\$'"
check "it gets --cpus, --memory and --disk" in_vm "$vm" \
  '[[ $(nproc) == 2 ]] && (( $(awk "/^MemTotal:/ { print int(\$2 / 1048576 + 0.5) }" /proc/meminfo) == 3 )) && (( $(df -BG --output=size / | tail -1 | tr -dc 0-9) >= 115 ))'
check "devvm size shows it" bash -c "cd '$p' && [[ \$('$D' size 2>&1) == 'cpus=2 memory=3GiB disk=120GiB' ]]"
check "ls lists it, and no image" bash -c "'$D' ls >'$work/ls.out' 2>&1 &&
  grep -Eq '^$vm +Running +base +- +(on|off) +off +- +$p\$' '$work/ls.out' && ! grep -q '^devvm-img-' '$work/ls.out'"
check "the folder is mounted read-write" bash -c "limactl shell --workdir / $vm bash -c 'echo from-vm > $p/vm.txt' </dev/null && grep -qx from-vm '$p/vm.txt'"
# On Linux hosts the mount is 9p, whose subfolders' events reach no watcher, even the VM's own.
if [[ "$(uname -s)" == Darwin ]]; then
  check "a watcher in the VM sees files changed, made and deleted in the VM" watch_sees in_vm "$vm" "$p" vm
else
  check "a watcher in the VM sees files changed, made and deleted in the VM, but not in a subfolder (9p)" watch_sees in_vm "$vm" "$p" vm no
fi
check "... but none changed, made or deleted on this machine (Lima's mounts pass on no file events: polling)" watch_misses in_vm "$vm" "$p"
# Only the project folder and the shared tools: the VM's own mounts list holds nothing else (in a
# nested Linux test host, the guest's home has the host's path, so looking for files there proves
# nothing).
check "nothing else from the host is mounted" bash -c "[[ \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$p $share ' ]]"
check "SSH agent forwarding is off" bash -c "! limactl shell --workdir / $vm ssh-add -l </dev/null"
check "without the docker layer, it has no Docker" in_vm "$vm" '! command -v dockerd && ! command -v docker && ! systemctl cat docker.service >/dev/null 2>&1'
check "hostname is the folder" bash -c "[[ \$(limactl shell --workdir / $vm hostname </dev/null) == web ]]"
# Every port (the default), through devvm's forwarder over Lima's ssh, Lima's own forwards off: a
# program on this machine on every address of a port (with SO_REUSEADDR, as most servers do) keeps
# its localhost connections while the VM listens on it too; a VM server on another port reaches this
# machine (the control).
check "Lima forwards nothing itself; devvm's forwarder runs, with the host keys devvm pinned for this start" bash -c "
  [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $vm) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]] &&
  ps -axww -o command= | grep -- '[_]ports -vm $vm ' | grep -qF -- ' -known-hosts $(vmdir "$vm")/run/known_hosts ' &&
  grep -q '^$vm ssh-ed25519 ' '$(vmdir "$vm")/run/known_hosts'"
# Without network policy too: so no other VM's forwarder takes it before Lima binds it.
check "its ssh port is the one its state records, on this machine's loopback alone" bash -c "
  sp=\$(st_get '$(vmdir "$vm")' ports.host.ssh) && [[ -n \$sp ]] && ! vm_state '$(vmdir "$vm")' | grep -qx net.on=true &&
  [[ \$(limactl list --format '{{.SSHLocalPort}}' $vm) == \"\$sp\" ]] && loopback_only \"\$sp\""
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalPort => shift, Listen => 5, ReuseAddr => 1) or die;
  while ($c = $s->accept) { print $c "host-program\n"; close $c }' $P47161 &
hostprog=$!; sleep 0.5
lima_serve "$vm" $P47160 $P47161
in_vm "$vm" "setsid -f python3 -c 'import socket,time; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((\"0.0.0.0\",$P47162)); time.sleep(600)' devvm-e2e-udp </dev/null >/dev/null 2>&1" >/dev/null 2>&1
check "a VM server on a port reaches this machine" bash -c 'for i in $(seq 20); do nc -z 127.0.0.1 $P47160 && exit 0; sleep 1; done; exit 1'
check "through devvm's forwarder alone: Lima's hostagent has none of its ports, TCP or UDP" bash -c "sleep 3; nc -z 127.0.0.1 $P47160 &&
  ! lima_listens tcp $P47160 && ! lima_listens udp $P47162 && pgrep -f -- '[_]ports -vm $vm ' >/dev/null"
check "the forward listens on this machine's loopback alone" loopback_only $P47160
check "a port a program here has on every address keeps its localhost connections while the VM listens on it" bash -c "sleep 3
  out=\$(nc -w 2 127.0.0.1 $P47161 </dev/null); echo \"$P47161 answered by: \${out:-nothing}\"; [[ \$out == host-program ]] &&
  grep -q 'port $P47161: not forwarded' '$(vmdir "$vm")/logs/ports.log'"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
# A server on a link's address (127.77.0.0/16) is never forwarded, while the VM's own on 127.0.0.1
# with the same port is (the control).
vm_say "$vm" 127.77.0.1 $P47163 link
vm_say "$vm" 127.77.0.1 $P47164 link
vm_say "$vm" 127.0.0.1 $P47164 own
check "a server on a link's address isn't forwarded, the VM's own on 127.0.0.1 with its port is" bash -c "
  limactl shell --workdir / $vm ss -Hltn 'sport = :$P47163' </dev/null | grep -q '127\.77\.0\.1:$P47163' &&
  for i in \$(seq 20); do [[ \$(nc -w 2 127.0.0.1 $P47164 </dev/null) == own ]] && break; sleep 1; done &&
  [[ \$(nc -w 2 127.0.0.1 $P47164 </dev/null) == own ]] && sleep 3 && ! nc -z 127.0.0.1 $P47163 && ! nc -z ::1 $P47163 &&
  ! grep -q 'port $P47163' '$(vmdir "$vm")/logs/ports.log'"
in_vm "$vm" "pkill -f '[h]ttp.server -b 0.0.0.0 ${P47160%?}'; pkill -f '[d]evvm-e2e-udp'; pkill -f '[d]evvm-e2e-say'" >/dev/null 2>&1
check "Exec's arguments, folder and input reach the guest as given" go_e2e TestE2EExecQuoting backend/registry DEVVM_E2E_VM="$vm"

section "a company's CA"
# The run's CA, in devvm's CA folder: the running VM trusts it from its next launch, in its system's
# store, so an HTTPS server here it signed is trusted; emptied, the next launch takes it out again.
e2e_ca; ca_serve $P47195
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
check "a CA in devvm's CA folder is trusted in the VM from its next launch" bash -c "cd '$p' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm sh -c 'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /usr/local/share/ca-certificates/devvm-extra-1.crt &&
    cmp -s /etc/ssl/devvm.pem /usr/local/share/ca-certificates/devvm-extra-1.crt && curl -sSf -o /dev/null https://host.lima.internal:$P47195/' </dev/null"
check "the VM's environment points tools that don't read the system's store at it" in_vm "$vm" \
  '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt && $REQUESTS_CA_BUNDLE == $SSL_CERT_FILE && $PIP_CERT == $SSL_CERT_FILE ]]'
rm -rf "$cafolder"
check "emptied, the VM trusts it no more from its next launch" bash -c "cd '$p' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm sh -c '! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 && [ ! -s /etc/ssl/devvm.pem ] &&
    ! curl -sSf -o /dev/null https://host.lima.internal:$P47195/' </dev/null"
ca_stop

section "ssh by name"
# An ~/.ssh/config of its own, set up as the docs say, with a Host * that forwards the agent: yours
# is never read or changed. ssh gets no agent but a throwaway one.
sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc"
printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$XDG_DATA_HOME/devvm/ssh_config" > "$sc/config"
kh=$(vmdir "$vm")/run/known_hosts
agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
check "the ssh config lists it by the folder's name" grep -q "^Host devvm-web $vm\$" "$XDG_DATA_HOME/devvm/ssh_config"
check "its host keys are pinned, as the VM has them" bash -c "
  [[ \$(grep -c '^$vm ' '$kh') -gt 0 && \$(grep -c '^$vm ' '$kh') == \$(limactl shell --workdir / $vm sh -c 'cat /etc/ssh/ssh_host_*_key.pub' </dev/null | wc -l | tr -d ' ') ]]"
check "ssh devvm-web reaches it" bash -c "[[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "agent forwarding stays off, whatever Host * says" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -F '$sc/config' devvm-web 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "a changed host key is refused" bash -c "
  cp '$kh' '$sc/kh' && ssh-keygen -q -t ed25519 -N '' -f '$sc/other' &&
  { grep -v '^$vm ' '$sc/kh' || true; echo \"$vm \$(cut -d' ' -f1,2 '$sc/other.pub')\"; } > '$kh' &&
  { env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web true </dev/null 2>'$sc/err'; r=\$?; cp '$sc/kh' '$kh'; } &&
  (( r == 255 )) && grep -q 'Host key verification failed' '$sc/err'"
# lima_sshd VM HOST: its sshd binds remote forwards only on links' addresses (rfwd_links) and refuses
# X11, as sshd -T shows; a local forward works (the control).
lima_sshd() { local t want
  rfwd_links "$2" env -u SSH_AUTH_SOCK ssh -F "$sc/config" || return 1
  t=$(limactl shell --workdir / "$1" sudo sshd -T </dev/null) || return 1
  for want in 'x11forwarding no' 'allowtcpforwarding yes' 'allowstreamlocalforwarding local' 'gatewayports clientspecified' \
    'permitlisten 127.77.*:*' 'permittunnel no'; do
    grep -qxF "$want" <<<"$t" || { echo "not $want"; return 1; }
  done
  [[ $(env -u SSH_AUTH_SOCK ssh -F "$sc/config" -W 127.0.0.1:22 "$2" </dev/null | head -c 4) == SSH- ]]; }
check "its sshd binds remote forwards only on 127.77.0.0/16, refuses socket forwards and X11; a local forward works (the control)" lima_sshd "$vm" devvm-web
check "the entry turns off X11, tunnels and adding keys to an agent, whatever Host * says" bash -c "
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n  Tunnel yes\n' '$XDG_DATA_HOME/devvm/ssh_config' > '$sc/hostile-config' &&
  g=\$(ssh -G -F '$sc/hostile-config' devvm-web) &&
  grep -qx 'forwardx11 no' <<<\"\$g\" && grep -qx 'forwardx11trusted no' <<<\"\$g\" && grep -qx 'addkeystoagent false' <<<\"\$g\" &&
  grep -qx 'tunnel false' <<<\"\$g\" && grep -qx 'permitremoteopen none' <<<\"\$g\" && grep -qx 'updatehostkeys false' <<<\"\$g\""

section ".git protection"
if [[ "$(uname -s)" != Darwin ]]; then
  check "off on Linux (no chflags), and says so" bash -c "vm_state '$(vmdir "$vm")' | grep -qx git.mode=off &&
    grep -qF \"\$(m git_protect_needs_macos_new)\" '$work/web-launch.log'"
else
  check "hooks and config are locked" bash -c "ls -lOd '$p/.git/config' | grep -q uchg && ls -lOd '$p/.git/hooks' | grep -q uchg &&
    vm_state '$(vmdir "$vm")' | grep -qx git.mode=on"
  check "root in the VM can't add a hook" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x > $p/.git/hooks/pre-commit' </dev/null && [[ ! -e '$p/.git/hooks/pre-commit' ]]"
  check "root in the VM can't change the config" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x >> $p/.git/config' </dev/null"
  check "git in the VM can commit" in_vm "$vm" "cd $p && for i in 1 2 3 4 5 6 7 8 9 10; do git status >/dev/null 2>&1 && break; sleep 1; done &&
    git add -A && git -c user.email=e@e -c user.name=e commit -qm vm"
fi

section "an existing VM"
check "devvm stop stops it and its port forwarder, and drops its ssh config entry" bash -c "cd '$p' && '$D' stop </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(status_of $vm) == Stopped ]] && grep -qF \"\$(m stopped vm=$vm)\" '$work/last.log' && ! pgrep -f -- '[_]ports -vm $vm ' >/dev/null &&
  ! grep -q '^Host devvm-web ' '$XDG_DATA_HOME/devvm/ssh_config'"
# A program here that took its ssh port while it was stopped, on 127.0.0.1 alone and accepting
# nothing (SO_REUSEADDR, as the port's last connections may still be in TIME_WAIT): the VM gets a
# new one, recorded, and the program keeps its own.
old_sp=$(st_get "$(vmdir "$vm")" ports.host.ssh)
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "127.0.0.1", LocalPort => shift, Listen => 1, ReuseAddr => 1) or die "$!\n"; sleep 600' "$old_sp" &
hostprog=$!; sleep 0.5
check "stopped, devvm starts it again, on a new ssh port when a program here has its own" bash -c "[[ -n '$old_sp' && \$(status_of $vm) == Stopped ]] && kill -0 '$hostprog' &&
  cd '$p' && printf 'echo e2e-again\n' | '$D' >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] && grep -qx e2e-again '$work/last.log' &&
  sp=\$(st_get '$(vmdir "$vm")' ports.host.ssh) && [[ -n \$sp && \$sp != '$old_sp' ]] &&
  [[ \$(limactl list --format '{{.SSHLocalPort}}' $vm) == \"\$sp\" ]] && kill -0 '$hostprog'"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
check "running, devvm opens it as it is" bash -c "cd '$p' && printf 'echo e2e-running\n' | '$D' 2>&1 | grep -qx e2e-running"
# devvm host with a home of its own, so the Include line is offered for an empty one (and, without a
# terminal, not added).
mkdir -p "$sc/home"
check "devvm host starts a stopped VM and names it, without a shell; ssh reaches it" bash -c "cd '$p' &&
  HOME='$sc/home' '$D' stop </dev/null >/dev/null 2>&1 && [[ \$(status_of $vm) == Stopped ]] &&
  HOME='$sc/home' '$D' host </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | head -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | tail -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_include_yourself | head -1)\" '$work/last.log' && [[ ! -e '$sc/home/.ssh' ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "VS Code's settings in the VM: no port forwarding, no git sign-in relay" in_vm "$vm" \
  '[[ $(jq -c . ~/.vscode-server/data/Machine/settings.json) == "{\"remote.autoForwardPorts\":false,\"git.terminalAuthentication\":false,\"github.gitAuthentication\":false}" ]]'
in_vm "$vm" 'echo "{\"editor.fontSize\": 14, \"remote.autoForwardPorts\": true}" > ~/.vscode-server/data/Machine/settings.json' >/dev/null 2>&1
(cd "$p" && HOME=$sc/home "$D" host </dev/null >"$work/last.log" 2>&1)
check "changed back in the VM, devvm host turns them off again and keeps the others" in_vm "$vm" \
  'jq -e ".[\"editor.fontSize\"] == 14 and .[\"remote.autoForwardPorts\"] == false and .[\"github.gitAuthentication\"] == false" ~/.vscode-server/data/Machine/settings.json >/dev/null'
check "restart, and its new host keys are pinned" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "a running VM's new ports apply at once" bash -c "cd '$p' &&
  '$D' ports $P47170-$P47173 2>&1 | grep -qF \"\$(m ports_forwarding ports=$P47170-$P47173 vm=$vm)\" && [[ \$('$D' ports 2>/dev/null) == $P47170-$P47173 ]] &&
  ps -axww -o command= | grep -- '[_]ports -vm $vm ' | grep -q -- ' -only $P47170-$P47173\$'"
check "a bad port list is a usage error, exit 2, naming devvm ports --help, and the VM's ports stay" bash -c "cd '$p' &&
  { '$D' ports $P47170-x </dev/null >'$work/last.log' 2>&1; [[ \$? == 2 ]]; } &&
  grep -qxF \"\$(m usage_help 'command=devvm ports')\" '$work/last.log' && [[ \$('$D' ports 2>/dev/null) == $P47170-$P47173 ]]"
check "a word that's neither a command nor an image is a usage error suggesting the command, and the VM runs on" bash -c "cd '$p' &&
  { '$D' stoop </dev/null >'$work/last.log' 2>&1; [[ \$? == 2 ]]; } &&
  grep -qF \"\$(m no_command_like command=stop word=stoop)\" '$work/last.log' && [[ \$(status_of $vm) == Running ]]"
check "a port added to a running VM reaches this machine at once, and once removed doesn't, the VM never restarted" ports_live "$p" "$vm" $P47178 lima_serve lima_boot
in_vm "$vm" "pkill -f '[h]ttp.server -b 0.0.0.0 $P47178'" >/dev/null 2>&1
check "its new size waits for its next start" bash -c "cd '$p' && '$D' size --cpus 3 2>&1 | grep -qF \"\$(m size_next_start vm=$vm)\" &&
  '$D' size 2>&1 >/dev/null | grep -qF \"\$(m size_pending size=cpus=3 vm=$vm)\""
check "and applies on restart, its ports kept" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' ports 2>&1) == $P47170-$P47173 && \$(limactl shell --workdir / $vm nproc </dev/null) == 3 &&
     \$('$D' size 2>&1) == 'cpus=3 memory=3GiB disk=120GiB' ]]"
check "a smaller disk is refused" bash -c "cd '$p' && ! '$D' size --disk 100 </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m disk_shrink have=120 vm=$vm want=100)\" '$work/last.log'"
# Listeners on each kind of address Lima matches differently (loopback, every address; IPv4 and 6), on
# listed ports (47170-47173 in slot 0) and unlisted ones (47174-47177). Their output goes nowhere, or
# the SSH session waits for them.
in_vm "$vm" "p=$P47170;"' for r in 1 2; do for a in 127.0.0.1 ::1 0.0.0.0 ::; do
  setsid -f python3 -m http.server -b $a $p </dev/null >/dev/null 2>&1; p=$((p + 1)); done; done' >/dev/null 2>&1
check "only the listed ports reach this machine, whatever the address" bash -c \
  'for i in $(seq 20); do nc -z 127.0.0.1 $P47173 && break; sleep 1; done; sleep 3
  for p in $P47170 $P47171 $P47172 $P47173; do nc -z 127.0.0.1 $p || exit 1; done
  for p in $P47174 $P47175 $P47176 $P47177; do ! nc -z 127.0.0.1 $p || exit 1; done'
check "a listed port a program on this machine has, on every address, stays its own until it's freed" host_port_kept "$p" "$vm" $P47170 $P47171 lima_serve
if [[ "$(uname -s)" == Darwin ]]; then
  check "a swapped .git is reported" bash -c "limactl shell --workdir / $vm bash -c 'cd $p && mv .git .git-old && git init -q' </dev/null;
    cd '$p' && printf 'exit\n' | '$D' 2>&1 | grep -qF \"\$(m git_replaced git=$p/.git | head -1)\""
  rm -rf "$p/.git"; mv "$p/.git-old" "$p/.git"
  # (virtiofs shows the VM the restored .git after a moment.) Locks are set while it's stopped:
  # devvm stop sets them.
  check "a submodule git folder the VM adds is reported, and locked by devvm stop" bash -c "limactl shell --workdir / $vm bash -c 'cd $p &&
      for i in 1 2 3 4 5 6 7 8 9 10; do git rev-parse >/dev/null 2>&1 && break; sleep 1; done && git init -q vmsub &&
      git -C vmsub -c user.email=e@e -c user.name=e commit -q --allow-empty -m s &&
      git update-index --add --cacheinfo 160000,\$(git -C vmsub rev-parse HEAD),vmsub' </dev/null &&
    cd '$p' && '$D' stop </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m git_embedded_new git=$p/vmsub/.git | head -1)\" '$work/last.log' &&
    [[ \$(status_of $vm) == Stopped ]] && ls -lOd '$p/vmsub/.git/config' | grep -q uchg"
  check "git-protect on a running VM waits for it to stop, and devvm stop applies it" bash -c "cd '$p' &&
    printf 'exit\n' | '$D' >/dev/null 2>&1 && [[ \$(status_of $vm) == Running ]] && '$D' git-protect off </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_next_stop mode=off vm=$vm | head -1)\" && ls -lOd '$p/.git/config' | grep -q uchg &&
    '$D' stop </dev/null >/dev/null 2>&1 && ! ls -lOd '$p/.git/config' | grep -q uchg &&
    [[ \$('$D' git-protect 2>/dev/null) == off ]] && printf 'exit\n' | '$D' >/dev/null 2>&1 && [[ \$(status_of $vm) == Running ]]"
  check "stopped outside devvm, its port forwarder ends with it (its lifeline)" bash -c "limactl stop $vm >/dev/null 2>&1 &&
    for i in \$(seq 10); do pgrep -f -- '[_]ports -vm $vm ' >/dev/null || exit 0; sleep 1; done; exit 1"
  check "git-protect strict keeps host git status out of submodules" bash -c "limactl stop $vm >/dev/null 2>&1; cd '$p' &&
    '$D' git-protect strict </dev/null >/dev/null 2>&1 && [[ \$(git config --file .git/config diff.ignoreSubmodules) == all &&
    \$('$D' git-protect 2>/dev/null) == strict ]] && ls -lOd '$p/.git/config' | grep -q uchg"
  check "git-protect off unlocks, and on locks again" bash -c "cd '$p' && '$D' git-protect off </dev/null >/dev/null 2>&1 &&
    ! ls -lOd '$p/.git/config' | grep -q uchg && ! git config --file .git/config diff.ignoreSubmodules &&
    '$D' git-protect on </dev/null >/dev/null 2>&1 && ls -lOd '$p/.git/config' | grep -q uchg && [[ \$('$D' git-protect 2>/dev/null) == on ]]"
else
  check "git-protect on is refused off macOS" bash -c "cd '$p' && '$D' git-protect on </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_on_needs_macos | head -1)\""
fi
check "rm deletes only this VM, with its state and ssh config entry, and unlocks" bash -c "cd '$p' && '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$(vmdir "$vm")' ]] && ! grep -q '^Host devvm-web ' '$XDG_DATA_HOME/devvm/ssh_config' &&
  ! grep -q '^$vm ' '$kh' && { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$p/.git/config' | grep -q uchg; } &&
  ! pgrep -f -- '[_]ports -vm $vm ' >/dev/null &&
  [[ -z \$(ls '$XDG_DATA_HOME/devvm/exports' 2>/dev/null) ]]"

section "a devcontainer.json"
# Its forwardPorts beyond the user's ports said, not forwarded (the default is none for this VM), its
# variables in the shell and its commands, its postCreateCommand once, its postStartCommand at each
# start; nothing written to the folder, and a changed file only warned about.
dc=$(newproj dc); mkdir -p "$dc/.devcontainer"
printf '%s\n' '{' '  // e2e' '  "name": "dc",' "  \"forwardPorts\": [$P47180]," \
  '  "containerEnv": {"E2E_DC": "dc-${containerWorkspaceFolderBasename}"},' \
  '  "postCreateCommand": "echo \"$E2E_DC $(pwd)\" >> ~/dc-created",' \
  '  "postStartCommand": ["sh", "-c", "echo started >> ~/dc-started"],' '}' > "$dc/.devcontainer/devcontainer.json"
cp "$dc/.devcontainer/devcontainer.json" "$work/dc.json"
quiet "$D" ports -g none
launch "$dc" 'echo "dc-env=$E2E_DC"' --no-network-policy
"$D" ports -g all </dev/null >/dev/null 2>&1
check "devvm makes the VM as its devcontainer.json says, and says so" bash -c "[[ -n '$vm' && \$(status_of '$vm') == Running ]] &&
  grep -qF \"\$(m devcontainer_using file=.devcontainer/devcontainer.json vm=$vm)\" '$work/last.log' &&
  vm_state '$(vmdir "$vm")' | grep -Eqx 'devcontainer\.sum=[0-9a-f]{64}' &&
  vm_state '$(vmdir "$vm")' | grep -qx 'devcontainer.file=.devcontainer/devcontainer.json' && vm_state '$(vmdir "$vm")' | grep -qx ports.forward=none"
check "its forwardPorts don't widen your ports: said, not added" \
  grep -qF "$(m devcontainer_ports_not_forwarded file=.devcontainer/devcontainer.json ports=$P47180)" "$work/last.log"
check "its shell has the file's variables" grep -qx dc-env=dc-dc "$work/last.log"
check "postCreateCommand ran once, in the VM, in the folder, with them" in_vm "$vm" "[[ \$(cat ~/dc-created) == 'dc-dc $dc' ]]"
check "postStartCommand ran at its start" in_vm "$vm" '[[ $(cat ~/dc-started) == started ]]'
check "nothing was written to the project folder" bash -c "[[ \$(ls -A '$dc') == .devcontainer && \$(ls -A '$dc/.devcontainer') == devcontainer.json ]] &&
  cmp -s '$dc/.devcontainer/devcontainer.json' '$work/dc.json'"
in_vm "$vm" "setsid -f python3 -m http.server -b 0.0.0.0 $P47180 </dev/null >/dev/null 2>&1; setsid -f python3 -m http.server -b 0.0.0.0 $P47181 </dev/null >/dev/null 2>&1" >/dev/null 2>&1
check "... and don't reach this machine" bash -c \
  'sleep 5; ! nc -z 127.0.0.1 $P47180 && ! nc -z 127.0.0.1 $P47181'
check "launched again, running: nothing runs again" bash -c "cd '$dc' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm bash -c '[[ \$(wc -l < ~/dc-created) == 1 && \$(wc -l < ~/dc-started) == 1 ]]' </dev/null"
check "restarted: postStartCommand runs again, postCreateCommand doesn't" bash -c "cd '$dc' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm bash -c '[[ \$(wc -l < ~/dc-created) == 1 && \$(wc -l < ~/dc-started) == 2 ]]' </dev/null"
printf '%s\n' "{\"forwardPorts\": [$P47182], \"postCreateCommand\": \"touch ~/dc-changed\"}" >"$dc/.devcontainer/devcontainer.json"
check "a changed file is warned about, and changes nothing" bash -c "cd '$dc' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m devcontainer_changed file=.devcontainer/devcontainer.json vm=$vm | head -1)\" '$work/last.log' &&
  vm_state '$(vmdir "$vm")' | grep -qx ports.forward=none && limactl shell --workdir / $vm bash -c '[[ ! -e ~/dc-changed ]]' </dev/null"
check "rm deletes it" bash -c "cd '$dc' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 && ! limactl list -q | grep -qx $vm"
printf '%s\n' '{"image": "localhost:5000/app:1"}' > "$dc/.devcontainer/devcontainer.json"
check "an image on this machine's registry is refused, nothing made" bash -c "cd '$dc' && ! '$D' --no-network-policy </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m devcontainer_image_not_public file=.devcontainer/devcontainer.json from=localhost:5000/app:1)\" '$work/last.log' &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$(vmdir "$vm")' ]]"

section "agent folders and the VM's setup"
# A home of its own: a fake agent config, history and git identity, and a fake token; nothing of
# yours is read.
ah=$work/agent-home; rm -rf "$ah"; mkdir -p "$ah/.claude/skills/e2e" "$ah/.claude/projects/p"; ah=$(cd "$ah" && pwd -P)
echo '# e2e skill' > "$ah/.claude/skills/e2e/SKILL.md"
echo '{"theme":"e2e-settings"}' > "$ah/.claude/settings.json"
echo '{"prompt":"e2e-history"}' > "$ah/.claude/projects/p/h.jsonl"
printf '[user]\n\tname = E2E User\n\temail = e2e@example.com\n' > "$ah/.gitconfig"
# A fake file outside the agent folders, and links to it from inside one: relative and absolute.
mkdir -p "$ah/outside"; echo e2e-outside-marker > "$ah/outside/e2e-not-a-secret.txt"
ln -s ../../outside/e2e-not-a-secret.txt "$ah/.claude/skills/escape"
ln -s "$ah/outside/e2e-not-a-secret.txt" "$ah/.claude/skills/escape-abs"
a=$(newproj agents)
(cd "$a" && HOME=$ah TZ=Europe/Paris CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake-plain quiet "$D" --no-network-policy --no-git-protect --agents claude)
vm=$(vm_of "$a"); [[ -z $vm ]] || created+=("$vm")
check "an agent folder is mounted read-only at its host path, and linked into the VM's home" bash -c "
  limactl list --format '{{range .Config.Mounts}}{{.Location}}|{{.Writable}} {{end}}' $vm | grep -qF '$ah/.claude/skills|false ' &&
  ! limactl shell --workdir / $vm sudo touch '$ah/.claude/skills/x' </dev/null &&
  limactl shell --workdir / $vm bash -c 'grep -q e2e ~/.claude/skills/e2e/SKILL.md' </dev/null"
check "a symlink in an agent folder, relative or absolute, can't reach the host's files outside it" bash -c "
  s='$ah/.claude/skills' && l=\$(limactl shell --workdir / $vm readlink \"\$s/escape\" \"\$s/escape-abs\" </dev/null) && echo \"links: \$l\" &&
  o=\$(limactl shell --workdir / $vm sudo cat \"\$s/escape\" \"\$s/escape-abs\" </dev/null 2>&1); echo \"read: \$o\" &&
  [[ \$l == *outside/e2e-not-a-secret.txt*outside/e2e-not-a-secret.txt && \$o != *e2e-outside-marker* ]]"
check "its settings are copied in, its history never" bash -c "
  limactl shell --workdir / $vm bash -c 'grep -q e2e-settings ~/.claude/settings.json && [[ ! -e ~/.claude/projects/p ]]' </dev/null &&
  ! limactl shell --workdir / $vm sudo grep -rqs e2e-history /home /root /etc </dev/null"
check "the VM gets the host's git identity, timezone and its hostname" in_vm "$vm" \
  "[[ \$(git config --global user.name) == 'E2E User' && \$(git config --global user.email) == e2e@example.com &&
     \$(timedatectl show -p Timezone --value) == Europe/Paris && \$(hostname) == agents ]]"
check "without network policy, the Claude token itself" in_vm "$vm" '[[ $CLAUDE_CODE_OAUTH_TOKEN == sk-ant-oat01-e2efake-plain ]]'
mkdir -p "$ah/.claude/agents"
check "a new agent folder is noticed on launch, and waits for a restart" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' 2>&1 |
  grep -qF \"\$(m mounts_next_start vm=$vm | head -1)\""
check "restart mounts it" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 &&
  limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm | grep -qF '$ah/.claude/agents ' &&
  limactl shell --workdir / $vm ls '$ah/.claude/agents' </dev/null"
rm -rf "$ah/.claude/skills"
check "a removed one is unmounted, and the VM starts" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 &&
  ! limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm | grep -q skills && [[ \$(status_of $vm) == Running ]]"
in_vm "$vm" 'mkdir -p ~/.claude/projects/e2e && echo e2e-transcript > ~/.claude/projects/e2e/t.jsonl && echo secret > ~/.claude/.credentials.json'
limactl stop "$vm" >/dev/null 2>&1
check "export starts the stopped VM and saves its agents' history to a file, without their logins" bash -c "cd '$a' &&
  HOME='$ah' '$D' history export '$work/export/h.tar.gz' </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  tar -tzf '$work/export/h.tar.gz' > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list'"
check "rm exports its agents' history, without their logins, only yours to read, then deletes it" bash -c "ex='$XDG_DATA_HOME/devvm/exports' &&
  mkdir -p \"\$ex\" && chmod 755 \"\$ex\" && cd '$a' && HOME='$ah' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && f=\$(ls \"\$ex\"/$vm-*.tar.gz) && tar -tzf \"\$f\" > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list' &&
  [[ \$(stat -c %a \"\$ex\" 2>/dev/null || stat -f %Lp \"\$ex\") == 700 ]] && [[ \$(stat -c %a \"\$f\" 2>/dev/null || stat -f %Lp \"\$f\") == 600 ]]"

section "SSH agents"
# A home of its own, without keys. A throwaway agent with a throwaway key stands for yours; devvm's
# own agent (its socket in the sandbox's data folder) gets another.
sa=$work/agent-ssh; rm -rf "$sa"; mkdir -p "$sa/home"; sa=$(cd "$sa" && pwd -P)
ssh-keygen -q -t ed25519 -N '' -C devvm-e2e-user-key -f "$sa/user-key"
ssh-keygen -q -t ed25519 -N '' -C devvm-e2e-key -f "$sa/devvm-key"
user_agent_pid=$(ssh-agent -a "$sa/user.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
SSH_AUTH_SOCK=$sa/user.sock ssh-add -q "$sa/user-key" 2>/dev/null
# as_user CMD...: CMD with that home and agent.
as_user() { HOME=$sa/home SSH_AUTH_SOCK=$sa/user.sock "$@"; }
export -f as_user; export sa
g=$(newproj agentvm)
(cd "$g" && as_user quiet "$D" --no-network-policy --no-git-protect --ssh-agent)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "--ssh-agent: the VM uses your agent's keys" bash -c "
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-user-key ]]"
check "ssh-agent add loads a key into devvm's own agent" bash -c "
  as_user '$D' ssh-agent add '$sa/devvm-key' </dev/null 2>&1 | grep -qF \"\$(m agent_holds keys=devvm-e2e-key)\""
check "ssh-agent devvm while it runs: after a launch, devvm's shell and ssh by name still use yours, until a restart" bash -c "cd '$g' &&
  as_user '$D' ssh-agent devvm </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' >>'$work/last.log' 2>&1 &&
  [[ \$(as_user ssh -F '$XDG_DATA_HOME/devvm/ssh_config' $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-user-key ]] &&
  [[ \$(printf 'ssh-add -l\nexit\n' | as_user '$D' 2>/dev/null | awk '/SHA256:/ { print \$3 }') == devvm-e2e-user-key ]]"
check "ssh-agent devvm, then restart: the VM uses only that key, not yours" bash -c "cd '$g' &&
  as_user '$D' ssh-agent devvm </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "so does an editor connecting by name, whatever agent it has" bash -c "
  [[ \$(as_user ssh -F '$XDG_DATA_HOME/devvm/ssh_config' $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "ssh-agent off, then restart: no agent in the VM" bash -c "cd '$g' &&
  as_user '$D' ssh-agent off </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  ! as_user limactl shell --workdir / $vm ssh-add -l </dev/null &&
  grep -A8 '^Host .* $vm\$' '$XDG_DATA_HOME/devvm/ssh_config' | grep -q '^  ForwardAgent no\$'"
check "ssh-agent rm takes the key out of devvm's own agent and its record, and leaves the key file" bash -c "cd '$g' &&
  as_user '$D' ssh-agent </dev/null 2>/dev/null | grep -q devvm-e2e-key &&
  as_user '$D' ssh-agent rm '$sa/devvm-key' </dev/null >'$work/last.log' 2>&1 &&
  ! SSH_AUTH_SOCK='$XDG_DATA_HOME/devvm/agent.sock' ssh-add -l </dev/null | grep -q devvm-e2e-key &&
  ! grep -q devvm-key '$XDG_DATA_HOME/devvm/agent-keys.json' && [[ -f '$sa/devvm-key' ]] &&
  [[ \$(as_user '$D' ssh-agent </dev/null 2>/dev/null) == off ]]"
check "rm deletes the SSH agent VM" bash -c "cd '$g' && as_user '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 && ! limactl list -q | grep -qx $vm"
kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock"

section "devvm layer add"
export -f in_vm
# Two small layers of the sandbox's own, each leaving a file in the system.
for l in e2eadd e2eadd2; do
  printf '#!/bin/bash\nset -euo pipefail\necho %s | sudo tee /etc/devvm-%s >/dev/null\n' "$l" "$l" > "$layers/$l.sh"
  : > "$layers/$l.policy"
done
r=$(newproj remade)
launch "$r" 'echo in-home > ~/e2e-home.txt; echo in-project > e2e-project.txt' --no-network-policy --no-git-protect --ports 3000
remade=$vm
aside="devvm-${remade: -14}-old"; created+=("$aside")
rd=$(vmdir "$remade")
id=$(st_get "$rd" id)
check "a VM with ports, a rule and a secret binding" bash -c "cd '$r' &&
  '$D' ports 3000,5173 </dev/null >/dev/null 2>&1 && '$D' network allow example.com </dev/null >/dev/null 2>&1 &&
  echo ghp_E2EFAKE | '$D' secret set github --stdin >/dev/null 2>&1 && '$D' secret use github </dev/null >/dev/null 2>&1 &&
  in_vm '$remade' 'grep -qx in-home ~/e2e-home.txt'"
check "layer add without a terminal or --force refuses, and changes nothing" bash -c "cd '$r' &&
  ! '$D' layer add e2eadd </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m force_needed command='devvm layer add e2eadd')\" '$work/last.log' && [[ \$(st_get '$rd' image) == base ]]"
check "layer add --force makes it again with the layer, in a shell" bash -c "cd '$r' &&
  printf 'echo e2e-remade-shell\n' | '$D' layer add e2eadd --force >'$work/last.log' 2>&1 &&
  grep -qx e2e-remade-shell '$work/last.log' && [[ \$(st_get '$rd' image) == e2eadd ]] &&
  in_vm '$remade' 'grep -qx e2eadd /etc/devvm-e2eadd' && ! limactl list -q | grep -qx '$aside'"
owned e2eadd
check "it keeps its ID, ports, rule, secret binding and network choice" bash -c "
  [[ \$(st_get '$rd' id) == '$id' && \$(st_get '$rd' ports.forward) == 3000,5173 && \$(st_get '$rd' secrets.names) == github ]] &&
  [[ \$(st_get '$rd' net.on) == false ]] && grep -qx example.com '$rd/policy' && [[ ! -e '$rd/home.tar.gz' ]]"
check "it keeps the project folder and its home folder" bash -c "
  grep -qx in-project '$r/e2e-project.txt' && in_vm '$remade' 'grep -qx in-home ~/e2e-home.txt'"
check "layer add --clean-home adds the next layer on top, with the image's home folder" bash -c "cd '$r' &&
  printf 'exit\n' | '$D' layer add e2eadd2 --force --clean-home >'$work/last.log' 2>&1 &&
  [[ \$(st_get '$rd' image) == e2eadd+e2eadd2 ]] && in_vm '$remade' 'grep -qx e2eadd /etc/devvm-e2eadd && grep -qx e2eadd2 /etc/devvm-e2eadd2' &&
  in_vm '$remade' '[[ ! -e ~/e2e-home.txt ]]' && [[ \$(st_get '$rd' id) == '$id' ]] && grep -qx in-project '$r/e2e-project.txt'"
owned e2eadd+e2eadd2
check "layer add of a layer it has changes nothing" bash -c "cd '$r' &&
  '$D' layer add e2eadd --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m has_layers layers=e2eadd vm='$remade')\" '$work/last.log' && [[ \$(st_get '$rd' image) == e2eadd+e2eadd2 ]]"
check "update on a current VM says so and changes nothing" bash -c "cd '$r' &&
  '$D' update --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m vm_current image=e2eadd+e2eadd2 vm='$remade')\" '$work/last.log' && ! limactl list -q | grep -qx '$aside'"
printf 'echo e2eupd | sudo tee /etc/devvm-e2eupd >/dev/null\n' >> "$layers/e2eadd2.sh"
chain=$(st_get "$rd" chain)
check "an edited layer's script makes it outdated, and the warning names devvm update" bash -c "cd '$r' &&
  in_vm '$remade' 'echo in-home > ~/e2e-home.txt' &&
  printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m vm_outdated changed=e2eadd+e2eadd2)\" '$work/last.log'"
check "update --force makes it again from the image's new build, keeping home and settings" bash -c "cd '$r' &&
  printf 'exit\n' | '$D' update --force >'$work/last.log' 2>&1 &&
  [[ \$(st_get '$rd' image) == e2eadd+e2eadd2 && \$(st_get '$rd' chain) != '$chain' ]] &&
  in_vm '$remade' 'grep -qx e2eupd /etc/devvm-e2eupd && grep -qx in-home ~/e2e-home.txt' &&
  [[ \$(st_get '$rd' id) == '$id' && \$(st_get '$rd' ports.forward) == 3000,5173 && \$(st_get '$rd' secrets.names) == github ]] &&
  grep -qx example.com '$rd/policy' && grep -qx in-project '$r/e2e-project.txt' && ! limactl list -q | grep -qx '$aside'"
owned e2eadd+e2eadd2
check "once updated, a launch doesn't warn" bash -c "cd '$r' &&
  printf 'exit\n' | '$D' >'$work/last.log' 2>&1 && ! grep -qF \"\$(m vm_outdated changed=e2eadd+e2eadd2)\" '$work/last.log'"
check "layer rm --force makes it again without the layer, keeping home and settings" bash -c "cd '$r' &&
  printf 'exit\n' | '$D' layer rm e2eadd --force >'$work/last.log' 2>&1 &&
  [[ \$(st_get '$rd' image) == e2eadd2 ]] &&
  in_vm '$remade' '[[ ! -e /etc/devvm-e2eadd ]] && grep -qx e2eadd2 /etc/devvm-e2eadd2 && grep -qx in-home ~/e2e-home.txt' &&
  [[ \$(st_get '$rd' id) == '$id' && \$(st_get '$rd' ports.forward) == 3000,5173 && \$(st_get '$rd' secrets.names) == github ]] &&
  grep -qx example.com '$rd/policy' && grep -qx in-project '$r/e2e-project.txt' && ! limactl list -q | grep -qx '$aside'"
owned e2eadd2
check "rm deletes the VM made again" bash -c "cd '$r' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qxE '$remade|$aside'"
"$D" secret rm github --force </dev/null >/dev/null 2>&1
rm -f "$layers"/e2eadd*.sh "$layers"/e2eadd*.policy
fi

if begin clone; then
section "--clone"
# A home of its own: no ~/.ssh/config, so nothing of yours is read or offered the Include line. A
# throwaway repo, with a branch besides main.
ch=$work/clone-home; rm -rf "$ch"; mkdir -p "$ch"
export GIT_CONFIG_GLOBAL=$ch/.gitconfig GIT_CONFIG_NOSYSTEM=1   # host git: none of your settings
c=$(newproj cloned)
git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$c" branch other
# The repo's url.*.insteadOf rewrites the VM's URL to another host, which devvm's push ignores.
git -C "$c" config url.elsewhere.invalid:/x.insteadOf devvm-
(cd "$c" && printf 'pwd; git branch --show-current\n' | HOME=$ch "$D" --clone --no-network-policy >"$work/last.log" 2>&1)
git -C "$c" config --unset url.elsewhere.invalid:/x.insteadOf
vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
cgit="GIT_SSH_COMMAND='ssh -F $XDG_DATA_HOME/devvm/ssh_config' git -C '$c'"
check "the VM has its own clone, with every branch, though insteadOf sent its URL elsewhere, only the shared tools mounted, and the shell opens in it" bash -c "
  [[ -n '$vm' && \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$share ' ]] &&
  [[ \$(limactl shell --workdir / $vm git -C '$c' branch --format='%(refname:short)' </dev/null | tr '\n' ' ') == 'main other ' ]] &&
  limactl shell --workdir / $vm git -C '$c' log --oneline </dev/null | grep -q one &&
  grep -qx '$c' '$work/last.log' && grep -qx main '$work/last.log' && vm_state '$(vmdir "$vm")' | grep -qx clone.on=true"
check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
check "nothing of .git is locked or watched" bash -c "vm_state '$(vmdir "$vm")' | grep -qx git.mode=clone &&
  { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$c/.git/config' | grep -q uchg; }"
in_vm "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "rm without a terminal refuses with unfetched commits, even with --force, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_fetch 'command=devvm rm' remote=devvm | head -1)\" '$work/last.log' && limactl list -q | grep -qx $vm"
check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
in_vm "$vm" "cd '$c' && echo c > c" >/dev/null 2>&1
check "then rm refuses with uncommitted changes in the clone, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_delete 'command=devvm rm' | head -1)\" '$work/last.log' && grep -qx '?? c' '$work/last.log' && limactl list -q | grep -qx $vm"
in_vm "$vm" "cd '$c' && rm c && git worktree add -q .worktrees/a && mkdir -p .git/info && echo .worktrees/ >> .git/info/exclude && echo w > .worktrees/a/w" >/dev/null 2>&1
check "then rm refuses with uncommitted changes in a worktree in the clone, shown under its path, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m clone_worktree_dirty path='$c/.worktrees/a' | head -1)\" '$work/last.log' && grep -qx '?? w' '$work/last.log' &&
  ! grep -qF \"\$(m clone_dirty | head -1)\" '$work/last.log' &&
  grep -qF \"\$(m no_terminal_delete 'command=devvm rm' | head -1)\" '$work/last.log' && limactl list -q | grep -qx $vm"
check "rm --no-clone-check deletes it unchecked after a warning, the worktree's change with it, and the remote fully; the fetched work stays as a local branch" bash -c "cd '$c' && HOME='$ch' '$D' rm --force --no-export --no-clone-check </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m clone_check_skipped 'command=devvm rm' vm=$vm | head -1)\" '$work/last.log' && ! grep -qF \"\$(m clone_worktree_dirty path='$c/.worktrees/a' | head -1)\" '$work/last.log' &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$(vmdir "$vm")' ]] && ! git -C '$c' remote | grep -q devvm &&
  [[ -z \$(git -C '$c' for-each-ref refs/remotes) ]] && git -C '$c' log --oneline saved/devvm/main | grep -q from-vm &&
  grep -qF \"\$(m saved_branches 'branches=\"saved/devvm/main\"' | head -1)\" '$work/last.log' && [[ ! -e '$ch/.ssh/config' ]]"
# A git worktree of the repo, on a branch of its own whose upstream is main: its VM's clone has
# only those two, not the repo's other branches.
w=$(newproj cloned-wt)
git -C "$c" worktree add -q -b wtb "$w" main && git -C "$w" branch -q --set-upstream-to=main
(cd "$w" && printf 'git branch --show-current\n' | HOME=$ch "$D" --clone --no-network-policy >"$work/last.log" 2>&1)
wvm=$(vm_of "$w"); [[ -z $wvm ]] || created+=("$wvm")
check "a git worktree's VM has a clone of only its branch and that branch's upstream" bash -c "
  [[ -n '$wvm' && \$(limactl shell --workdir / $wvm git -C '$w' branch --format='%(refname:short)' </dev/null | tr '\n' ' ') == 'main wtb ' ]] &&
  grep -qx wtb '$work/last.log'"
(cd "$w" && HOME=$ch "$D" rm --force --no-export </dev/null >/dev/null 2>&1)
git -C "$c" worktree remove --force "$w"
# devvm clone on and off: a VM that mounts its folder, made again with its own clone of the repo,
# then made again mounting the folder, its clone's work fetched first; its home and ID kept.
s=$(newproj cloned-switch)
git -C "$s" init -q -b main && echo a > "$s/a" && git -C "$s" add a && git -C "$s" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$s" branch other
(cd "$s" && printf 'echo in-home > ~/e2e-home.txt\n' | HOME=$ch "$D" --no-network-policy >"$work/last.log" 2>&1)
svm=$(vm_of "$s"); [[ -z $svm ]] || created+=("$svm")
saside="devvm-${svm: -14}-old"; created+=("$saside")
sd=$(vmdir "$svm")
sid=$(st_get "$sd" id)
sgit="GIT_SSH_COMMAND='ssh -F $XDG_DATA_HOME/devvm/ssh_config' git -C '$s'"
check "clone without a terminal refuses on, and shows off" bash -c "cd '$s' &&
  ! HOME='$ch' '$D' clone on </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m force_needed 'command=devvm clone on')\" '$work/last.log' &&
  [[ \$(HOME='$ch' '$D' clone </dev/null 2>/dev/null) == off ]]"
check "clone on --force makes it again with its own clone of every branch, only the shared tools mounted, .git unlocked, its home and ID kept" bash -c "cd '$s' &&
  printf 'pwd\n' | HOME='$ch' '$D' clone on --force >'$work/last.log' 2>&1 && grep -qx '$s' '$work/last.log' &&
  [[ \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $svm) == '$share ' ]] &&
  [[ \$(limactl shell --workdir / $svm git -C '$s' branch --format='%(refname:short)' </dev/null | tr '\n' ' ') == 'main other ' ]] &&
  [[ \$(st_get '$sd' id) == '$sid' && \$(st_get '$sd' clone.on) == true && \$(st_get '$sd' git.mode) == clone ]] &&
  [[ \$(git -C '$s' remote get-url devvm) == '$svm:$s' ]] && in_vm '$svm' 'grep -qx in-home ~/e2e-home.txt' &&
  ! limactl list -q | grep -qx '$saside' && { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$s/.git/config' | grep -q uchg; }"
in_vm "$svm" "cd '$s' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "clone off without a terminal refuses with unfetched commits, even with --force, and keeps the clone" bash -c "cd '$s' &&
  ! HOME='$ch' '$D' clone off --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_fetch 'command=devvm clone off' remote=devvm | head -1)\" '$work/last.log' &&
  [[ \$(st_get '$sd' clone.on) == true ]] && limactl list -q | grep -qx $svm"
check "once fetched, clone off --force mounts the folder again, .git protected, its home and ID kept, the remote gone and its work a local branch" bash -c "cd '$s' &&
  $sgit fetch -q devvm && printf 'pwd\n' | HOME='$ch' '$D' clone off --force >'$work/last.log' 2>&1 && grep -qx '$s' '$work/last.log' &&
  limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $svm | grep -qF '$s ' &&
  [[ \$(st_get '$sd' id) == '$sid' && \$(st_get '$sd' clone.on) != true ]] && in_vm '$svm' 'grep -qx in-home ~/e2e-home.txt && test -f $s/a' &&
  ! git -C '$s' remote | grep -q devvm && [[ -z \$(git -C '$s' for-each-ref refs/remotes) ]] &&
  git -C '$s' log --oneline saved/devvm/main | grep -q from-vm && ! limactl list -q | grep -qx '$saside' &&
  { [[ \$(uname -s) != Darwin ]] || { [[ \$(st_get '$sd' git.mode) == on ]] && ls -lOd '$s/.git/config' | grep -q uchg; }; }"
(cd "$s" && HOME=$ch "$D" rm --force --no-export </dev/null >/dev/null 2>&1)
unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
fi

if begin gc; then
section "gc"
# A folder named img-*: its VM is named like an image (devvm-img-keep-<hash>), and isn't one.
g=$(newproj img-keep)
launch "$g" exit --no-network-policy
kg=$(st_get "$(vmdir "$vm")" chain | sed -n 's/^base=\([0-9a-f]*\)$/\1/p')
gfix=$layers/e2egc.sh
printf '#!/bin/bash\necho gc\n' > "$gfix"
check "a fixture layer, edited after its build, is stale" bash -c "'$D' build e2egc </dev/null >'$work/last.log' 2>&1 &&
  echo '# edited' >> '$gfix' && '$D' images </dev/null 2>/dev/null | grep -qE '^e2egc .* stale '"
owned e2egc
ko=$(keys_of e2egc | head -1)
# An old tool image no VM lists: unused only once there's a tools lock.
old=node-0.0.1-000000000000.erofs
[[ ! -f $lock ]] || { : > "$share/$old"; printf '{"sha256": "x", "libc": "gnu", "backend": "lima"}\n' > "$share/$old.json"; }
check "gc deletes the stale image and its record; base and the img-* project VM stay" bash -c "
  [[ -n '$vm' && -n '$kg' && -n '$ko' ]] && '$D' gc --force </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(status_of devvm-img-$ko) && ! -e '$images/$ko' ]] &&
  [[ \$(status_of devvm-img-$kg) == Stopped && -f '$images/$kg/record.json' ]] && [[ \$(status_of $vm) == Running ]]"
if [[ -f $lock ]]; then
  check "gc deletes a tool image no VM uses, with its record" bash -c "[[ ! -e '$share/$old' && ! -e '$share/$old.json' ]]"
else
  skip "gc deletes a tool image no VM uses, with its record" "no tools lock yet"
fi
check "space lists the Lima VM and images, and changes nothing" bash -c "'$D' space </dev/null 2>/dev/null >'$work/last.log' &&
  grep -qE '^VMs +[0-9]+ ' '$work/last.log' && grep -qE '^images +[0-9]+ ' '$work/last.log' &&
  [[ \$(status_of $vm) == Running && -f '$(vmdir "$vm")/vm.json' ]]"
# The img-* VM deleted outside devvm, its state an hour old: gc --orphans forgets what devvm kept.
quiet limactl delete -f "$vm"
touch -t 202001010000 "$(vmdir "$vm")/vm.json"
check "gc --orphans forgets a VM limactl deleted, and nothing of base's" bash -c "'$D' gc --orphans --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m forgot_orphan vm=$vm)\" '$work/last.log' && [[ ! -e '$(vmdir "$vm")' ]] &&
  [[ \$(status_of devvm-img-$kg) == Stopped && -f '$images/$kg/record.json' ]]"
rm -f "$gfix" "$share/$old" "$share/$old.json"
(cd "$g" && quiet "$D" rm --force --no-export)
fi

if begin tools; then
# What a user gets: upstream's newest versions, each download with its checksum (rust: rustup checks),
# resolved by devvm's pinned mise, downloaded and checked on the first run. This section alone uses
# them: pin_lock puts the pins back at its end.
section "shared tool versions"
march=arm64; [[ "$(uname -m)" != x86_64 ]] || march=x64   # the lock's platform: mise's name for the arch
ntools=$(awk -F' *= *' '/^\[tools\]/ { s = 1; next } /^\[/ { s = 0 } s && NF == 2' "$root/assets/shared-tools/mise.toml" | wc -l)
check "the shared tools resolve to upstream's newest, with checksums" bash -c "'$D' _refresh-tools </dev/null >'$work/last.log' 2>&1 &&
  (( \$(grep -c '^\[\[tools\.' '$lock') == $ntools )) &&
  (( \$(grep -A1 '\"platforms.linux-$march\"\]' '$lock' | grep -c '^checksum = \"sha256:') == $ntools - 1 ))"
section "shared tools"
t=$(newproj tools); echo '{}' > "$t/package.json"
# A package.json: node, detected. Its tool images, of the newest versions, stay cached in the share
# until upstream moves or a later run's gc finds them unused.
launch "$t" exit --no-network-policy --no-git-protect
check "a node VM, with node's shared tools in its state and the share" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && vm_state '$(vmdir "$vm")' | grep -qx image=base &&
  vm_state '$(vmdir "$vm")' | grep -qx tools.names=node && vm_state '$(vmdir "$vm")' | grep -qx tools.made.libc=gnu &&
  for f in \$(st_get '$(vmdir "$vm")' tools.made.files); do [[ -f '$share'/\$f ]] && grep -qxF '  \"backend\": \"lima\",' '$share'/\$f.json || exit 1; done &&
  vm_state '$(vmdir "$vm")' | grep -q '^tools\.made\.files=node-$(lockv node)-.* pnpm-$(lockv pnpm)-.* bun-$(lockv bun)-'"
check "Ubuntu's base has no glibc version to check, and each tool image records the newest glibc it needs" bash -c "
  ! vm_state '$(vmdir "$vm")' | grep -q '^tools\.made\.glibc=' &&
  for f in \$(st_get '$(vmdir "$vm")' tools.made.files); do grep -qE '^  \"glibc\": \"[0-9]+\.[0-9]+\",?\$' '$share'/\$f.json || exit 1; done"
for f in $(st_get "$(vmdir "$vm")" tools.made.files); do
  echo "        glibc: $f needs $(sed -n 's/^  "glibc": "\(.*\)",\{0,1\}$/\1/p' "$share/$f.json" | grep . || echo none)"
done
check "node, npm, pnpm, bun and yarn work, node at the locked version" in_vm "$vm" \
  "[[ \$(node -v) == v$(lockv node) ]] && npm -v && pnpm -v && bun -v && yarn -v"
check "each tool is a read-only image from the share" in_vm "$vm" \
  "[[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]] && ! touch /opt/devvm/tools/node/$(lockv node)/x 2>/dev/null"
check "the share is read-only even for root" bash -c "limactl shell --workdir / $vm sudo touch /mnt/devvm-tools/x </dev/null 2>&1 | grep -q 'Read-only file system'"
check "npm i -g installs into the VM's home" in_vm "$vm" "npm i -g cowsay@1.6.0 >/dev/null 2>&1 && [[ \$(command -v cowsay) == \$HOME/.npm-global/bin/cowsay ]]"
# Node reads its own CA list, and NODE_EXTRA_CA_CERTS: a CA in devvm's CA folder, from the VM's
# next launch, makes an HTTPS server here it signed trusted.
e2e_ca; ca_serve $P47195
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
(cd "$t" && printf 'exit\n' | "$D" >"$work/last.log" 2>&1)
check "node trusts a CA in devvm's CA folder" in_vm "$vm" \
  'node -e "fetch(\"https://host.lima.internal:'"$P47195"'/\").then(r => process.exit(r.ok ? 0 : 1), e => { console.error(e); process.exit(1) })"'
rm -rf "$cafolder"; ca_stop
# ssh with a command runs a non-login, non-interactive shell, as editors and agents' tools do.
check "a non-login shell finds the tools too" bash -c "ssh -F '$LIMA_HOME/$vm/ssh.config' lima-$vm 'node -v' </dev/null"
# The checks above pass with a download of the locked version too: these tell the share's from one.
# Before the .nvmrc check, which installs into ~/.local/share/mise/installs on purpose.
for tool in node pnpm bun; do
  check "$tool runs from the share, not a download" in_vm "$vm" \
    "p=\$(readlink -f \"\$(mise which $tool)\") && [[ \$p == /opt/devvm/tools/$tool/$(lockv "$tool")/* ]] || { echo \"\$p\"; false; }"
done
check "mise installed none of node, pnpm and bun into the VM" in_vm "$vm" \
  "[[ ! -e ~/.local/share/mise/installs/node && ! -e ~/.local/share/mise/installs/pnpm && ! -e ~/.local/share/mise/installs/bun ]]"
check "an .nvmrc picks its own version, installed into the VM" in_vm "$vm" "cd $t && echo 22.20.0 > .nvmrc && [[ \$(node -v) == v22.20.0 ]]; r=\$?; rm .nvmrc; exit \$r"
check "with the .nvmrc gone, node runs from the share again" in_vm "$vm" \
  "for d in $t ~; do p=\$(cd \"\$d\" && readlink -f \"\$(mise which node)\") && [[ \$p == /opt/devvm/tools/node/$(lockv node)/* ]] || { echo \"\$d: \$p\"; exit 1; }; done"
# Started without devvm (so no setup): the boot itself mounts them.
check "the tools are mounted at boot" bash -c "limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  limactl shell --workdir / $vm bash -lc 'node -v' </dev/null | grep -qx v$(lockv node)"
limactl stop "$vm" >/dev/null 2>&1   # two VMs at once starve a nested Linux host
# A tool image gone from the share stops the launch, named, and mise downloads nothing in its place;
# back in the share, the next launch mounts it.
nodef=$(st_get "$(vmdir "$vm")" tools.made.files | tr ' ' '\n' | grep '^node-')
mv "$share/$nodef" "$work/$nodef"
check "a tool missing from the share stops the launch, named, and isn't downloaded" bash -c "cd '$t' &&
  ! printf 'node -v\n' | '$D' --no-network-policy --no-git-protect >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m tools_unmounted 'tools=node $(lockv node)' vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -qx v$(lockv node) '$work/last.log' &&
  limactl shell --workdir / $vm bash -lc '! mountpoint -q /opt/devvm/tools/node/$(lockv node) && [[ ! -e ~/.local/share/mise/installs/node/$(lockv node) ]]' </dev/null"
mv "$work/$nodef" "$share/$nodef"
check "back in the share, the next launch mounts it" bash -c "cd '$t' &&
  printf 'node -v\n' | '$D' --no-network-policy --no-git-protect >'$work/last.log' 2>&1 && grep -qx v$(lockv node) '$work/last.log' &&
  limactl shell --workdir / $vm bash -lc '[[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]]' </dev/null"
limactl stop "$vm" >/dev/null 2>&1
files_before=$(find "$share" -name '*.erofs' | wc -l)
t2=$(newproj tools2); echo '{}' > "$t2/package.json"
launch "$t2" 'node -v' --no-network-policy --no-git-protect
check "a second node VM uses the same tool images: none built" bash -c "
  [[ -n '$vm' ]] && (( \$(find '$share' -name '*.erofs' | wc -l) == $files_before )) && grep -qx v$(lockv node) '$work/last.log'"
check "the tools build left no VM behind" bash -c "[[ -z \$(status_of devvm-tools-build) ]]"
# Upstream can publish mid-run (node's musl builds come days after a release): with the lock
# unchanged, the tools are the newest; changed, the VM may move (if one of its tools did).
check "devvm tools lists node, pnpm and bun, each at the lock's version" bash -c "cd '$t2' && '$D' tools </dev/null >'$work/last.log' 2>&1 &&
  grep -qE '^node +$(lockv node) +- +-$' '$work/last.log' && grep -qE '^pnpm +$(lockv pnpm) +- +-$' '$work/last.log' &&
  grep -qE '^bun +$(lockv bun) +- +-$' '$work/last.log' || { cat '$work/last.log'; false; }"
check "devvm tools update: checks upstream; the newest unless the lock changed" bash -c "cd '$t2' &&
  before=\$(cksum < '$lock'); out=\$(printf 'exit\n' | '$D' tools update 2>&1)
  grep -qF \"\$(m tools_newest vm=$vm | head -1)\" <<<\"\$out\" ||
    { [[ \$(cksum < '$lock') != \"\$before\" ]] && grep -qF \"\$(m tools_switch vm=$vm | head -1)\" <<<\"\$out\"; }"
check "devvm update on a current image never makes the VM again: current, or moved to newer tools" bash -c "cd '$t2' &&
  before=\$(cksum < '$lock'); out=\$(printf 'exit\n' | '$D' update 2>&1)
  ! grep -qF \"\$(m vm_changed changed=base vm=$vm)\" <<<\"\$out\" && ! limactl list -q | grep -qx 'devvm-${vm: -14}-old' &&
  { grep -qF \"\$(m vm_current image=base vm=$vm)\" <<<\"\$out\" ||
    { [[ \$(cksum < '$lock') != \"\$before\" ]] && grep -qF \"\$(m tools_switch vm=$vm | head -1)\" <<<\"\$out\"; }; }"
check "devvm tools rm takes node out of the VM" bash -c "cd '$t2' && printf 'exit\n' | '$D' tools rm node >'$work/last.log' 2>&1 &&
  vm_state '$(vmdir "$vm")' | grep -qx image=base && ! vm_state '$(vmdir "$vm")' | grep -q '^tools\.made\.files=.*node' &&
  ! vm_state '$(vmdir "$vm")' | grep -q '^tools\.names=.*node' &&
  limactl shell --workdir / $vm bash -lc '! command -v node' </dev/null"
check "devvm node is an unknown command: a usage error in one line, nothing changed" bash -c "cd '$t2' &&
  ! DEVVM_MESSAGES=names '$D' node </dev/null >'$work/last.log' 2>&1; cat '$work/last.log'
  [[ \$(grep -c '^devvm: ' '$work/last.log') -eq 1 ]] && grep -q '^devvm: no_command' '$work/last.log' &&
  grep -qx 'usage_help command=devvm' '$work/last.log' && ! vm_state '$(vmdir "$vm")' | grep -q '^tools\.names=.*node'"
check "--tools on it adds nothing: warned, naming devvm tools, and opened as it is" bash -c "cd '$t2' &&
  printf 'exit\n' | DEVVM_MESSAGES=names '$D' --tools node >'$work/last.log' 2>&1 &&
  grep -qx 'devvm: parts_flags_ignored vm=$vm' '$work/last.log' && ! vm_state '$(vmdir "$vm")' | grep -q '^tools\.names=.*node'"
check "devvm tools add node mounts it again, nothing built" bash -c "cd '$t2' && n=\$(find '$share' -name '*.erofs' | wc -l) &&
  printf 'node -v\nexit\n' | '$D' tools add node >'$work/last.log' 2>&1 && grep -qx v$(lockv node) '$work/last.log' &&
  vm_state '$(vmdir "$vm")' | grep -qx image=base && vm_state '$(vmdir "$vm")' | grep -qx tools.names=node && (( \$(find '$share' -name '*.erofs' | wc -l) == n ))"
pin_lock || echo "  e2e: couldn't put the pinned tool lock back"
fi

if begin network; then
section "network policy"
# A VM with Claude Code and Codex, so the claude token is bound to it: a fake one in devvm's environment (its
# proxy inherits it), a fake ~/.claude.json with an MCP server's header and a fake Codex login, in a
# home of its own: no real token or login is read.
nh=$work/net-home; rm -rf "$nh"; mkdir -p "$nh/.codex"
printf '%s\n' '{"mcpServers":{"e2e":{"type":"http","url":"https://api.github.com/devvm-e2e-mcp","headers":{"X-Devvm-E2e":"e2efake-mcp"}}}}' > "$nh/.claude.json"
echo '{"OPENAI_API_KEY":"sk-e2efake-codex","last_refresh":null}' > "$nh/.codex/auth.json"
cp "$nh/.claude.json" "$work/claude-before.json"; cp "$nh/.codex/auth.json" "$work/codex-before.json"
n=$(newproj net)
ps_watch
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" --network-policy --no-git-protect --agents claude,codex >"$work/last.log" 2>&1)
ps_unwatch
vm=$(vm_of "$n"); [[ -z $vm ]] || created+=("$vm")
pp=$(st_get "$(vmdir "$vm")" ports.host.proxy)
pxc=$(vmdir "$vm")/run/proxy.json
salt=$(st_get "$(vmdir "$vm")" net.made.proxy_salt)
events=$(vmdir "$vm")/logs/network.jsonl
# pgrep -f looks for [_]proxy: Linux's pgrep also matches the check's own shell, whose command line
# holds the pattern.
check "--network-policy: the VM runs behind its proxy, and the self-check passed" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && vm_state '$(vmdir "$vm")' | grep -qx net.on=true && vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok &&
  pgrep -f -- '[_]proxy -config $pxc' >/dev/null && grep -q '\"listen\": \"127.0.0.1:$pp\"' '$pxc'"
check "the proxy's config is yours alone, holds its password, and is all its arguments name" bash -c "
  [[ \$(stat -c %a '$pxc' 2>/dev/null || stat -f %Lp '$pxc') == 600 ]] && grep -q '^  \"version\": 1,\$' '$pxc' &&
  key=\$(st_get '$(vmdir "$vm")' net.made.proxy_key) && grep -qF \"\\\"password\\\": \\\"\$key\\\"\" '$pxc' &&
  [[ \$(ps -ww -o command= -p \"\$(pgrep -f -- '[_]proxy -config $pxc' | head -1)\" | awk '{ \$1 = \"\"; print }') == ' _proxy -config $pxc' ]]"
# The control: the launch set the VM's environment (limactl edit, its password's stand-in written
# as <password> in the run log), and the guest has the password.
check "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it" bash -c "
  key=\$(st_get '$(vmdir "$vm")' net.made.proxy_key) && [[ \${#key} -eq 64 ]] && [[ -s '$work/ps.log' ]] &&
  ! grep -qFf <(echo \"\$key\") '$work/ps.log' && ! ps -axww -o command= | grep -qFf <(echo \"\$key\") &&
  grep -l 'limactl edit .*devvm:<password>@' '$XDG_DATA_HOME'/devvm/logs/runs/*.log >/dev/null &&
  limactl shell --workdir / $vm bash -lc 'printenv HTTPS_PROXY' </dev/null | grep -qF \":\$key@\""
check "ssh devvm-net reaches it, confined, its host key checked" bash -c "
  grep -q '^Host devvm-net $vm\$' '$XDG_DATA_HOME/devvm/ssh_config' &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_DATA_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]]"
check "devvm host runs the self-check, which passes, and names it" bash -c "cd '$n' &&
  HOME='$nh' CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake '$D' host </dev/null >'$work/last.log' 2>&1 && vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok &&
  grep -qF \"\$(m ssh_host_info dir=$n host=devvm-net | head -1)\" '$work/last.log'"
check "direct connections fail, even as root: an address, other ports" bash -c "
  ! limactl shell --workdir / $vm sudo curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm sudo curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm sudo timeout 5 bash -c 'exec 3<>/dev/tcp/1.1.1.1/53' </dev/null &&
  ! limactl shell --workdir / $vm sudo timeout 5 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22' </dev/null"
check "an allowed host works, through the proxy" in_vm "$vm" "curl -fsS -m 20 -o /dev/null https://github.com"
check "another host is blocked by the proxy, and so is another port of an allowed one" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://example.com' </dev/null 2>&1 | grep -q 403 &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/' </dev/null) == 403 ]] &&
  ! limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://github.com:8443' </dev/null"
check "devvm network log shows the block and the allowed connection" bash -c "cd '$n' &&
  '$D' network log | grep -q '  BLOCKED  example.com:443  ' && '$D' network log | grep -q '  allowed  github.com:443  rule '"
check "devvm network log --blocked groups it, with the command that allows it" bash -c "cd '$n' &&
  '$D' network log --blocked | grep -Eq '^[0-9TZ:-]+ +[0-9]+ +example\.com:443 +devvm network allow example\.com\$'"
# The command devvm network log suggests, run as it is: the same proxy allows the host at once, and after
# devvm network rm blocks it again.
check "devvm network allow applies at once to the running VM's proxy, devvm network shows it after on, and rm takes it back" bash -c "cd '$n' &&
  pid=\$(pgrep -f -- '[_]proxy -config $pxc') && [[ -n \$pid ]] &&
  cmd=\$('$D' network log --blocked | awk '\$3 == \"example.com:443\" { sub(/^.* devvm /, \"\"); print }') &&
  [[ \$cmd == 'network allow example.com' ]] && '$D' \$cmd </dev/null >/dev/null &&
  '$D' network >'$work/e' && [[ \$(head -1 '$work/e') == on ]] && grep -qx '  example.com' '$work/e' &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://example.com' </dev/null &&
  '$D' network rm example.com >/dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/' </dev/null) == 403 ]] &&
  [[ \$(pgrep -f -- '[_]proxy -config $pxc') == \$pid ]]"
# A line that reads as a deny but isn't a rule: the running proxy denies everything, refused with no
# allow command, devvm network rm warns of it, and once it's gone the allowed host works again.
# The VM's own file is put back whatever happens, so the checks after this one see its rules.
check "a deny that isn't a rule makes the running proxy refuse everything until it's gone" bash -c "cd '$n' &&
  cp '$(vmdir "$vm")/policy' '$work/vm.policy' && echo '! example.org' >> '$(vmdir "$vm")/policy' &&
  { [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://github.com/' </dev/null) == 403 ]] &&
    '$D' network log | grep -q '  BLOCKED  github.com:80  .*reads as a deny' &&
    '$D' network log --blocked | grep -Eq ' github\.com:80 +-\$' &&
    DEVVM_MESSAGES=names '$D' network rm nothere.test </dev/null 2>&1 | grep -q '^devvm: policy_broken_deny '; ok=\$?; } ;
  cp '$work/vm.policy' '$(vmdir "$vm")/policy' && [[ \${ok:-1} == 0 ]] &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://github.com' </dev/null"
check "DNS fails at once, not after a timeout" in_vm "$vm" 'timeout 3 getent hosts example.com; [ $? -eq 2 ]'
# Lima's network's DNS (192.168.5.3) and the host's address on it, asked directly, as root could:
# a refusal or no answer passes, an address for example.com fails.
lima_dns_asked=$(cat <<'EOF'
python3 - <<'PY'
import socket, sys
q = bytes.fromhex("123401000001000000000000076578616d706c6503636f6d0000010001")
for h in ("192.168.5.3", "192.168.5.2"):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(3)
    try:
        s.sendto(q, (h, 53))
        d = s.recv(512)
    except OSError as e:
        print(h, e)
        continue
    print(h, "rcode", d[3] & 15, "answers", int.from_bytes(d[6:8], "big"))
    if d[3] & 15 == 0 and int.from_bytes(d[6:8], "big") > 0:
        sys.exit("example.com answered by " + h)
PY
EOF
)
check "... and Lima's network's DNS addresses, asked directly, answer nothing" in_vm "$vm" "$lima_dns_asked"
# A service on the host's own loopback (a devvm _proxy, which answers anything with 407): unreachable
# directly, and refused by the VM's proxy (internal addresses need a rule naming them).
proxy_config "$work/listener.json" e2e-listener 127.0.0.1:$P18765 listener-key
"$D" _proxy -config "$work/listener.json" >/dev/null 2>&1 &
listener=$!
sleep 1
check "the host's loopback services are unreachable, directly or through the proxy" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:$P18765/) == 407 ]] &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null http://192.168.5.2:$P18765/ </dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} http://192.168.5.2:$P18765/' </dev/null) == 403 ]] &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} --noproxy \"\" -x \"\$HTTP_PROXY\" http://127.0.0.1:$P18765/' </dev/null) == 403 ]]"
kill "$listener" 2>/dev/null; listener=""
# A company proxy: a devvm _proxy of this run's, with a password of its own, allowing any host. The
# VM's next launch with HTTPS_PROXY and HTTP_PROXY in devvm's environment starts its proxy again
# through it, which sends an allowed host there by name, and refuses one the VM's policy doesn't
# allow before it gets there. The launch after, without them, starts it again going directly.
# Each step says what failed ("failed: ..."), then corp_why shows the VM's status, the launch's
# output, the end of the VM's proxy's log and events, and the company proxy's own log and events.
# step WHAT CMD...: CMD, or "failed: WHAT" and false.
step() { local what=$1; shift; "$@" || { echo "failed: $what"; return 1; }; }
corp_why() {
  echo "the VM: $(status_of "$vm"); its proxy: $(corp_proxy_args | cut -c1-60 | paste -sd ' ' -)"
  echo "the launch:"; tail -n 20 "$work/last.log"
  echo "the proxy log:"; tail -n 10 "$(vmdir "$vm")/logs/proxy.log"
  echo "its events:"; tail -n 5 "$events"
  echo "the company proxy's log (it runs: $(kill -0 "${corp:-0}" 2>/dev/null && echo yes || echo no)):"; tail -n 5 "$work/corp.log"
  echo "its events:"; tail -n 5 "$work/corp.jsonl"; }
# corp_proxy_args: the command line of each process of the VM's proxy.
corp_proxy_args() { local p
  p=$(pgrep -f -- "[_]proxy -config $pxc" | paste -sd , -) && ps -ww -o command= -p "$p"; }
corp_launch() { (cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$@" "$D" >"$work/last.log" 2>&1); }
# corp_upstream: the VM's proxy runs, its config naming a company proxy.
corp_upstream() { [[ -n $(corp_proxy_args) ]] && grep -q '^  "upstream": {' "$pxc"; }
corp_saw() { grep -q "$1" "$work/corp.jsonl"; }
corp_through() {
  if ! { step "the launch" corp_launch env HTTPS_PROXY="$corpurl" HTTP_PROXY="$corpurl" &&
    step "the VM's proxy doesn't go upstream" corp_upstream &&
    step "github.com in the VM" in_vm "$vm" 'curl -fsS -m 30 -o /dev/null https://github.com' &&
    step "the company proxy didn't carry github.com" corp_saw '"event":"tunnel","host":"github.com","port":443' &&
    step "the VM's events show no upstream for github.com" grep -q "\"host\":\"github.com\".*\"upstream\":\"127.0.0.1:$P18766\"" "$events" &&
    step "example.com wasn't refused" grep -q 403 <<<"$(in_vm "$vm" 'curl -sS -m 20 -o /dev/null https://example.com' 2>&1)" &&
    step "example.com reached the company proxy" lk_not corp_saw '"host":"example.com"'; }; then
    corp_why; return 1
  fi; }
corp_direct() { local n0
  if ! { step "the launch" corp_launch env -u HTTPS_PROXY -u HTTP_PROXY -u https_proxy -u http_proxy &&
    step "the VM's proxy still goes upstream" lk_not corp_upstream &&
    n0=$(grep -c . "$work/corp.jsonl") &&
    step "github.com in the VM" in_vm "$vm" 'curl -fsS -m 30 -o /dev/null https://github.com' &&
    step "the company proxy saw it" test "$(grep -c . "$work/corp.jsonl")" = "$n0"; }; then
    corp_why; return 1
  fi; }
corp_start
check_lines=60 check "behind a company proxy, an allowed host goes through it by name, and a blocked one never reaches it" corp_through
# The password is read from its file, never written into the check: its own arguments would hold it.
check "the company proxy's password is in no process's arguments, log or VM" bash -c "
  ! ps -axww -o command= | grep -qFf '$work/corp.auth' &&
  ! grep -rqsFf '$work/corp.auth' '$XDG_DATA_HOME/devvm/logs' '$(vmdir "$vm")/logs' &&
  ! limactl shell --workdir / $vm sudo grep -rqsFf /dev/stdin /etc /home /root <'$work/corp.auth' &&
  ! limactl shell --workdir / $vm bash -lc env </dev/null | grep -qFf '$work/corp.auth'"
check_lines=60 check "without them, the next launch starts the VM's proxy again, going directly" corp_direct
corp_stop
check "the proxy refuses anything on the host without the VM's password" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:$pp http://devvm.proxy/blocked) == 407 ]]"
check "the token never reaches the VM" bash -c "
  ! limactl shell --workdir / $vm sudo grep -rqs -e e2efake /etc /home /root </dev/null &&
  ! limactl shell --workdir / $vm bash -lc 'env' </dev/null | grep -q e2efake"
# A second VM with network policy, small and with no agents, in the same home: each VM has CAs of
# its own, the current one and the next, which its launch installs; its proxy presents only its
# own, and it trusts no other VM's. The proxies' passwords go into curl's config files, the run's
# alone, written by the shell's own printf: in no process's arguments.
n2=$(newproj net2)
vm1=$vm
(cd "$n2" && printf 'exit\n' | HOME=$nh "$D" --network-policy --no-git-protect --agents none --cpus 2 --memory 2 --ports none >"$work/last.log" 2>&1)
vm2=$(vm_of "$n2"); [[ -z $vm2 ]] || created+=("$vm2")
vm=$vm1
ca1=$(vmdir "$vm")/run/ca ca2=$(vmdir "$vm2")/run/ca
pp2=$(st_get "$(vmdir "$vm2")" ports.host.proxy)
i=0
for d in "$(vmdir "$vm")" "$(vmdir "$vm2")"; do
  i=$((i + 1))
  (umask 077 && key=$(st_get "$d" net.made.proxy_key) && printf 'proxy-user = "devvm:%s"\n' "$key" >"$work/px$i.cfg")
done
# A certificate's second PEM line holds its serial, and a key's its secret, so grep finds that
# certificate or key in any file that holds it.
check "each VM's launch installs its own CA and the next one, which only the host keeps the keys of" bash -c "
  [[ -n '$vm2' ]] && ! cmp -s '$ca1/ca.pem' '$ca2/ca.pem' && ! cmp -s '$ca1/next.pem' '$ca2/next.pem' &&
  limactl shell --workdir / $vm cat /usr/local/share/ca-certificates/devvm-proxy.crt </dev/null | cmp -s - '$ca1/ca.pem' &&
  limactl shell --workdir / $vm cat /usr/local/share/ca-certificates/devvm-proxy-next.crt </dev/null | cmp -s - '$ca1/next.pem' &&
  limactl shell --workdir / $vm2 cat /usr/local/share/ca-certificates/devvm-proxy.crt </dev/null | cmp -s - '$ca2/ca.pem' &&
  limactl shell --workdir / $vm2 cat /usr/local/share/ca-certificates/devvm-proxy-next.crt </dev/null | cmp -s - '$ca2/next.pem' &&
  ! limactl shell --workdir / $vm sudo grep -rqsF -f /dev/stdin /etc /home /root /usr/local < <(sed -n 2p '$ca1/ca.key' '$ca1/next.key') &&
  [[ \$(stat -c %a '$ca1/ca.key' 2>/dev/null || stat -f %Lp '$ca1/ca.key') == 600 ]] &&
  [[ \$(stat -c %a '$ca1/next.key' 2>/dev/null || stat -f %Lp '$ca1/next.key') == 600 ]]"
check "a VM trusts no other VM's CA" bash -c "
  ! limactl shell --workdir / $vm2 sudo grep -rqsF -f /dev/stdin /etc/ssl /usr/local/share/ca-certificates < <(sed -n 2p '$ca1/ca.pem' '$ca1/next.pem') &&
  ! limactl shell --workdir / $vm sudo grep -rqsF -f /dev/stdin /etc/ssl /usr/local/share/ca-certificates < <(sed -n 2p '$ca2/ca.pem' '$ca2/next.pem') &&
  limactl shell --workdir / $vm sudo grep -qsF -f /dev/stdin /etc/ssl/devvm.pem < <(sed -n 2p '$ca1/next.pem')"
check "each VM's proxy serves its own CAs, and presents its own CA's certificates, which the other VM's CAs don't verify" bash -c "
  curl -fsS -m 20 -K '$work/px1.cfg' -x http://127.0.0.1:$pp http://devvm.proxy/ca.pem | cmp -s - <(cat '$ca1/ca.pem' '$ca1/next.pem') &&
  curl -fsS -m 20 -K '$work/px2.cfg' -x http://127.0.0.1:$pp2 http://devvm.proxy/ca.pem | cmp -s - <(cat '$ca2/ca.pem' '$ca2/next.pem') &&
  curl -sS -m 30 -o /dev/null -K '$work/px1.cfg' -x http://127.0.0.1:$pp --cacert <(cat '$ca1/ca.pem' '$ca1/next.pem') https://api.anthropic.com/devvm-e2e-ca &&
  { curl -sS -m 30 -o /dev/null -K '$work/px1.cfg' -x http://127.0.0.1:$pp --cacert <(cat '$ca2/ca.pem' '$ca2/next.pem') https://api.anthropic.com/devvm-e2e-ca; [[ \$? == 60 ]]; }"
check "devvm rm deletes a VM's CA with its folder, and leaves the other VM's" bash -c "cd '$n2' &&
  HOME='$nh' '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 && [[ ! -e '$ca2' ]] && [[ -s '$ca1/ca.key' ]]"
rm -f "$work/px1.cfg" "$work/px2.cfg"
check "the VM gets its own placeholder, not the token" in_vm "$vm" "[[ \$CLAUDE_CODE_OAUTH_TOKEN == sk-ant-oat01-devvm-proxy-managed-$salt ]]"
check "the VM's Codex and Claude Code (MCP) logins hold only placeholders" bash -c "
  ! limactl shell --workdir / $vm bash -c 'cat ~/.codex/auth.json ~/.claude.json' </dev/null | grep -e e2efake &&
  limactl shell --workdir / $vm bash -c 'cat ~/.codex/auth.json' </dev/null | grep -q devvm-proxy-managed"
# Requests verify the proxy's certificates: no -k.
check "the proxy puts the token in for its host" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.anthropic.com/v1/models -H \"Authorization: Bearer \$CLAUDE_CODE_OAUTH_TOKEN\"' </dev/null &&
  grep '\"path\":\"/v1/models\"' '$events' | grep -q '\"secrets\":\[\"claude\"\]'"
check "but not for another VM's placeholder" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.anthropic.com/devvm-e2e-other -H \"Authorization: Bearer sk-ant-oat01-devvm-proxy-managed-$(printf '0%.0s' {1..32})\"' </dev/null &&
  grep -q '/devvm-e2e-other' '$events' && ! grep '/devvm-e2e-other' '$events' | grep -q secrets"
check "and an MCP server's header for its host, from the VM's copy of the login" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.github.com/devvm-e2e-mcp -H \"X-Devvm-E2e: \$(jq -r .mcpServers.e2e.headers[\\\"X-Devvm-E2e\\\"] ~/.claude.json)\"' </dev/null &&
  grep '/devvm-e2e-mcp' '$events' | grep -q '\"secrets\":\[\"claude-mcp-'"
check "and Codex's API key, from the VM's copy of its login" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.openai.com/v1/models -H \"Authorization: Bearer \$(jq -r .OPENAI_API_KEY ~/.codex/auth.json)\"' </dev/null &&
  grep '\"path\":\"/v1/models\"' '$events' | grep -q '\"secrets\":\[\"codex-key\"\]'"
check "the host's login files are unchanged" bash -c "cmp -s '$nh/.codex/auth.json' '$work/codex-before.json' && cmp -s '$nh/.claude.json' '$work/claude-before.json'"
# devvm secret with a fake GitHub token: in the Keychain under this run's service on macOS, else in
# the sandbox's config folder; deleted at the end. macOS finds the Keychain through HOME, so the
# VM's home links the Keychains folder (the link alone goes at the end).
if [[ "$(uname -s)" == Darwin ]]; then mkdir -p "$nh/Library" && ln -s "$HOME/Library/Keychains" "$nh/Library/Keychains"; fi
# The fake token goes in a file, written by the shell's own printf, so it's in no process's arguments.
printf 'ghp_E2EFAKE\n' >"$work/github.token"
check "devvm secret set saves a token, and devvm secret lists it saved" bash -c "
  HOME='$nh' '$D' secret set github --stdin <'$work/github.token' >/dev/null && HOME='$nh' '$D' secret | grep -Eq '^github +yes ' &&
  { [[ \$(uname -s) != Darwin ]] || security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github -w | cmp -s - '$work/github.token'; }"
check "no devvm command prints a secret" bash -c "! HOME='$nh' '$D' secret get github >'$work/get.out' 2>&1 && ! grep -qF -f '$work/github.token' '$work/get.out'"
check "secret use github: its proxy gets the binding at once, the VM its own placeholder on its next launch" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret use github >/dev/null && grep -q '\"name\":\"github\"' '$(vmdir "$vm")/run/secrets.json' &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'echo \$GH_TOKEN' </dev/null) == devvm-proxy-managed-github-$salt ]] &&
  ! limactl shell --workdir / $vm bash -lc 'env; cat ~/.config/devvm/env' </dev/null | grep -qF -f '$work/github.token' &&
  HOME='$nh' '$D' secret | grep -Eq '^github +yes +$vm\$'"
check "the proxy puts the GitHub token in for its host" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.github.com/user -H \"Authorization: token \$GH_TOKEN\"' </dev/null &&
  grep '\"path\":\"/user\"' '$events' | grep -q '\"secrets\":\[\"github\"\]'"
check "secret unuse and rm take the binding and the token away at once" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret unuse github >/dev/null && ! grep -q '\"name\":\"github\"' '$(vmdir "$vm")/run/secrets.json' &&
  HOME='$nh' '$D' secret rm --force github >/dev/null && HOME='$nh' '$D' secret | grep -Eq '^github +no ' &&
  { [[ \$(uname -s) != Darwin ]] || ! security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github >/dev/null 2>&1; }"
check "the self-check passes on the VM devvm started confined" go_e2e TestE2ESelfCheck netpolicy DEVVM_E2E_VM="$vm" DEVVM_E2E_WANT=
# unenforced_stop VM: VM, started outside devvm, fails the self-check, which stops it.
unenforced_stop() {
  limactl stop "$1" >/dev/null 2>&1 && limactl start --tty=false "$1" >/dev/null 2>&1 &&
    go_e2e TestE2ESelfCheck netpolicy DEVVM_E2E_VM="$1" DEVVM_E2E_WANT="net_unenforced_stop vm=$1" &&
    [[ $(status_of "$1") == Stopped ]] && vm_state "$(vmdir "$1")" | grep -qx net.made.check=unenforced
}
check "a VM started outside devvm is unenforced, and the self-check stops it" unenforced_stop "$vm"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$n' && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(status_of '$vm') == Running ]] && vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok"
check "devvm stop stops it and its proxy at once, and devvm starts both again" bash -c "cd '$n' &&
  pgrep -f -- '[_]proxy -config $pxc' >/dev/null && HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ \$(status_of '$vm') == Stopped ]] && ! pgrep -f -- '[_]proxy -config $pxc' >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$(status_of '$vm') == Running ]] &&
  pgrep -f -- '[_]proxy -config $pxc' >/dev/null && vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok"
# macOS: the hostagent's Seatbelt profile denies it every bind but its ssh port's, so Lima forwards
# nothing even without its ignore rules. A limactl first on PATH puts Lima's own rule (forward
# everything) back for one start, inside the sandbox devvm starts limactl in: Lima tries the guest's
# ports (its log says so), the sandbox refuses them, and devvm's forwarder has them.
if [[ "$(uname -s)" == Darwin ]]; then
  sp=$(st_get "$(vmdir "$vm")" ports.host.ssh); ha=$DEVVM_LIMA_HOME/$vm/ha.stderr.log
  # ha_listens: the hostagent's TCP listeners and UDP sockets, one per line.
  ha_listens() { lsof -nP -a -p "$(cat "$DEVVM_LIMA_HOME/$vm/ha.pid")" -iTCP -sTCP:LISTEN -iUDP 2>/dev/null | awk 'NR > 1 { print $8, $9 }'; }
  export -f ha_listens in_vm; export vm sp ha
  check "the hostagent's profile denies every bind but its ssh port's (TCP) and its own unix sockets'; it has no other" bash -c "
    sb='$(vmdir "$vm")/run/sandbox.sb' && [[ -n '$sp' && \$(grep -c network-bind \"\$sb\") == 3 ]] &&
    grep -qxF '(deny network-bind)' \"\$sb\" && grep -qxF '(allow network-bind (local tcp \"localhost:$sp\"))' \"\$sb\" &&
    grep -qxF \"(allow network-bind (local unix-socket (subpath \\\"\$(cd '$DEVVM_LIMA_HOME/$vm' && pwd -P)\\\")))\" \"\$sb\" &&
    bind='import socket; socket.socket().bind((\"127.0.0.1\", $P47197))' && python3 -c \"\$bind\" &&
    ! /usr/bin/sandbox-exec -f \"\$sb\" python3 -c \"\$bind\" 2>/dev/null &&
    udp='import socket; socket.socket(socket.AF_INET, socket.SOCK_DGRAM).bind((\"127.0.0.1\", $sp))' && python3 -c \"\$udp\" &&
    ! /usr/bin/sandbox-exec -f \"\$sb\" python3 -c \"\$udp\" 2>/dev/null &&
    l=\$(ha_listens) && echo \"hostagent's: \$l\" && [[ \$l == 'TCP 127.0.0.1:$sp' ]] &&
    grep -A1 '^hostResolver:' '$DEVVM_LIMA_HOME/$vm/lima.yaml' | grep -q 'enabled: false'"
  lima_real=$(command -v limactl); mkdir -p "$work/lima-bypass"
  cat > "$work/lima-bypass/limactl" <<EOF
#!/bin/bash
# e2e: Lima's own rule for this start, set where limactl start runs: inside devvm's sandbox.
if [[ \$1 == start ]]; then "$lima_real" edit --tty=false --set '.portForwards = []' "\${@: -1}" >/dev/null 2>&1 || exit 1; fi
exec "$lima_real" "\$@"
EOF
  chmod 755 "$work/lima-bypass/limactl"
  check "with Lima's own forwards on, the sandbox refuses the hostagent a VM server's port, TCP or UDP" bash -c "cd '$n' &&
    HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 &&
    printf 'exit\n' | PATH='$work/lima-bypass':\$PATH HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$(status_of '$vm') == Running ]] &&
    grep -qx 'portForwards: \[\]' '$DEVVM_LIMA_HOME/$vm/lima.yaml' && [[ \$(st_get '$(vmdir "$vm")' ports.host.ssh) == '$sp' ]] &&
    in_vm '$vm' \"setsid -f python3 -m http.server -b 0.0.0.0 $P47195 </dev/null >/dev/null 2>&1; setsid -f python3 -c 'import socket,time; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((\\\"0.0.0.0\\\",$P47196)); time.sleep(600)' devvm-e2e-udp </dev/null >/dev/null 2>&1\" &&
    refused() { grep -q 'listen tcp 127.0.0.1:$P47195: bind: operation not permitted' \"\$ha\" &&
      grep -Eq 'listen udp [0-9.]+:$P47196: bind: operation not permitted' \"\$ha\"; } &&
    { for i in \$(seq 20); do refused && break; sleep 1; done; refused; } && grep -q 'Forwarding TCP from 0.0.0.0:$P47195' \"\$ha\" &&
    ! lima_listens tcp $P47195 && ! lima_listens udp $P47196 && l=\$(ha_listens) && echo \"hostagent's: \$l\" && [[ \$l == 'TCP 127.0.0.1:$sp' ]]"
  check "... and the VM works: ssh, devvm's forwarder (on loopback alone), the proxy, the self-check" bash -c "
    for i in \$(seq 20); do nc -z 127.0.0.1 $P47195 && break; sleep 1; done; nc -z 127.0.0.1 $P47195 &&
    { loopback_only $P47195 || { echo 'not on loopback alone:'; lsof -nP -iTCP:$P47195 -sTCP:LISTEN; false; }; } &&
    { pgrep -f -- '[_]ports -vm $vm ' >/dev/null || { echo 'no forwarder'; false; }; } &&
    { ! lima_listens tcp $P47195 || { echo 'limactl listens'; false; }; } &&
    { [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_DATA_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]] || { echo 'ssh failed'; false; }; } &&
    { in_vm '$vm' 'curl -fsS -m 20 -o /dev/null https://github.com' || { echo 'the proxy failed'; false; }; } &&
    { vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok || { echo 'no self-check:'; vm_state '$(vmdir "$vm")' | grep '^net\.'; false; }; }"
  in_vm "$vm" "pkill -f '[h]ttp.server -b 0.0.0.0 ${P47195%?}'; pkill -f '[d]evvm-e2e-udp'" >/dev/null 2>&1
  check "the next start puts Lima's ignore rules back" bash -c "cd '$n' &&
    HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
    [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $vm) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]] &&
    vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok"
else
  skip "the hostagent's profile denies every bind but its ssh port's (TCP) and its own unix sockets'; it has no other" "Seatbelt is macOS's"
  skip "with Lima's own forwards on, the sandbox refuses the hostagent a VM server's port, TCP or UDP" "Seatbelt is macOS's"
  skip "... and the VM works: ssh, devvm's forwarder (on loopback alone), the proxy, the self-check" "Seatbelt is macOS's"
  skip "the next start puts Lima's ignore rules back" "Seatbelt is macOS's"
fi
# Ctrl+C while Lima's start waits for the VM, its hostagent running it: devvm stops the VM and its
# proxy, so it never runs without the record of its confined start, and ls shows its policy on.
(cd "$n" && HOME=$nh "$D" stop </dev/null >/dev/null 2>&1)
(cd "$n" && exec env HOME="$nh" CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake DEVVM_MESSAGES=names "$D" </dev/null >"$work/int.log" 2>&1) &
ip=$!
for _ in $(seq 300); do [[ -S $DEVVM_LIMA_HOME/$vm/ha.sock ]] && break; sleep 0.2; done
kill -INT "$ip" 2>/dev/null
wait "$ip"; rc=$?
check "Ctrl+C during its start stops it and its proxy, and ls shows its policy on" bash -c "
  [[ $rc -eq 130 ]] && grep -qx '==> interrupted_start vm=$vm' '$work/int.log' && [[ \$(status_of '$vm') == Stopped ]] &&
  ! pgrep -f -- '[_]proxy -config $pxc' >/dev/null &&
  '$D' ls | grep -Eq '^$vm +Stopped +base +[a-z,]*claude[a-z,]* +off +on +- +/'"
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" >"$work/last.log" 2>&1)
check "ls shows its policy on, and unenforced once started outside devvm" bash -c "
  '$D' ls | grep -Eq '^$vm +Running +base +[a-z,]*claude[a-z,]* +off +on +- +/' &&
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  '$D' ls | grep -Eq '^$vm +Running +base +[a-z,]*claude[a-z,]* +off +unenforced +- +/'"
check "started outside devvm again: without a terminal, devvm stops it and opens nothing" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! printf 'echo e2e-opened\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -q e2e-opened '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
check "so does devvm host, which names nothing" bash -c "
  limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! HOME='$nh' '$D' host </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -qF \"\$(m ssh_host_info dir=$n host=devvm-net | head -1)\" '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
# Stopped by the self-check, it's still unenforced: its last self-check failed.
check "devvm network off: its next start is direct, without a proxy" bash -c "cd '$n' &&
  [[ \$('$D' network | head -1) == unenforced ]] && '$D' network off </dev/null >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$('$D' network | head -1) == off ]] &&
  [[ \$(status_of '$vm') == Running ]] && ! pgrep -f -- '[_]proxy -config $pxc' >/dev/null &&
  limactl shell --workdir / $vm curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1 </dev/null"
# Without network policy, behind the run's company proxy on this machine's loopback, its password
# fake: the shell's HTTPS_PROXY reaches the guest, which uses it, and Lima, which points it at the
# VM's gateway, logs it as it starts the VM. Neither devvm's run logs nor Lima's hostagent log keep
# the password: the run log has the line with <password>, the hostagent log with as many stars. The
# check reads the password from its file, so it's in no command's arguments.
corp_start
check_lines=60 check "without network policy, the shell's proxy password reaches the guest, and no run log or Lima log keeps it" bash -c "cd '$n' &&
  pw=\$(cat '$work/corp.auth') && url=\"http://devvm:\$pw@127.0.0.1:$P18766\" && stars=\$(printf '%*s' \${#pw} '' | tr ' ' '*') &&
  printf 'exit\n' | HOME='$nh' HTTPS_PROXY=\"\$url\" HTTP_PROXY=\"\$url\" '$D' restart >'$work/last.log' 2>&1 &&
  grep -qF 'devvm:<password>@127.0.0.1:$P18766' \"\$(ls -t '$XDG_DATA_HOME'/devvm/logs/runs/*.log | head -1)\" &&
  [[ \$('$D' network | head -1) == off ]] &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'printenv HTTPS_PROXY' </dev/null) == \"http://devvm:\$pw@192.168.5.2:$P18766\" ]] &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 30 -o /dev/null https://github.com' </dev/null &&
  grep -q '\"event\":\"tunnel\",\"host\":\"github.com\",\"port\":443' '$work/corp.jsonl' &&
  ! grep -rqsFf '$work/corp.auth' '$XDG_DATA_HOME/devvm/logs' '$(vmdir "$vm")/logs' '$DEVVM_LIMA_HOME/$vm/ha.stderr.log' '$work/last.log' &&
  grep -qF \"devvm:\$stars@127.0.0.1:$P18766\" '$DEVVM_LIMA_HOME/$vm/ha.stderr.log'"
corp_stop
check "devvm network on while it runs, without a terminal: unenforced until devvm restart starts it confined" bash -c "cd '$n' &&
  '$D' network on </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m net_set_running state=on vm=$vm | head -1)\" '$work/last.log' &&
  [[ \$('$D' network | head -1) == unenforced ]] &&
  printf 'exit\n' | HOME='$nh' '$D' restart >'$work/last.log' 2>&1 && [[ \$('$D' network | head -1) == on ]] &&
  vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok && pgrep -f -- '[_]proxy -config $pxc' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "devvm network off while it runs confined, without a terminal: launched, it keeps its proxy until it next starts" bash -c "cd '$n' &&
  '$D' network off </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m net_set_running state=off vm=$vm | head -1)\" '$work/last.log' && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m net_off_next_start vm=$vm | head -1)\" '$work/last.log' &&
  pgrep -f -- '[_]proxy -config $pxc' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "rm --force deletes it, its proxy and its state" bash -c "cd '$n' && HOME='$nh' '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$(vmdir "$vm")' ]] && ! pgrep -f -- '[_]proxy -config $pxc' >/dev/null"
fi

if begin desktop; then
section "desktop"
# base has no desktop: a VM of base has none of its programs, and devvm desktop refuses it, naming the
# layer.
b=$(newproj nodesk)
launch "$b" exit --no-network-policy --no-git-protect --layers none
check "a VM of base has none of the desktop's programs" bash -c "[[ -n '$vm' ]] && vm_state '$(vmdir "$vm")' | grep -qx image=base &&
  limactl shell --workdir / '$vm' bash -c '! command -v Xvnc && ! command -v startxfce4 && [[ ! -e /usr/local/bin/devvm-desktop ]]' </dev/null"
check "... and devvm desktop refuses it, naming the layer" bash -c "
  ! (cd '$b' && DEVVM_MESSAGES=names '$D' desktop --no-open </dev/null >'$work/last.log' 2>&1) &&
  grep -qx 'devvm: desktop_no_layer vm=$vm' '$work/last.log'"
(cd "$b" && quiet "$D" rm --force --no-export)
# A VM with the desktop layer and network policy: devvm desktop serves its desktop on this machine's
# loopback alone, a viewer checked against its password here; the VM's server listens on a socket of
# its user's alone, never a port, and gets no way out. Then root in the VM replaces the server with
# one that asks for no password and records what reaches it: a viewer still needs the password, and
# its clipboard still never reaches the VM.
p=$(newproj desk)
launch "$p" exit --network-policy --no-git-protect --layers desktop
check "--layers desktop makes a VM of the desktop image" bash -c "[[ -n '$vm' ]] && vm_state '$(vmdir "$vm")' | grep -qx image=desktop"
ps_watch
desktop_serve "$p"
ps_unwatch
check "devvm desktop serves its desktop on this machine's 127.0.0.1 alone, its port and password kept" bash -c "
  [[ -n '$vm' && -n '$desk_port' ]] && loopback_only '$desk_port' &&
  vm_state '$(vmdir "$vm")' | grep -qx 'ports.host.desktop=$desk_port' && vm_state '$(vmdir "$vm")' | grep -qx 'desktop.made.password=$desk_pw'"
check "a viewer with a wrong password is refused, one with the password gets the VM's desktop" desktop_view
check "the VM's server listens on its user's own socket, on no TCP port, the clipboard off" in_vm "$vm" "$desktop_in_vm"
check "the password is in no file of the VM's" bash -c 'printf "%s\n" "$1" | limactl shell --workdir / "$2" sudo sh -c "$3"' \
  _ "$desk_pw" "$vm" "$desktop_secret"
check "... nor any run log, nor any host process's arguments while devvm desktop started" bash -c "
  [[ -s '$work/ps.log' ]] && ! grep -rqsF '$desk_pw' '$XDG_DATA_HOME/devvm/logs' '$(vmdir "$vm")/logs' && ! grep -F '$desk_pw' '$work/ps.log'"
check "with network policy, the VM still reaches only its proxy, and not the desktop's port here" bash -c "
  vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm timeout 5 bash -c '</dev/tcp/host.lima.internal/$desk_port' </dev/null"
# Root's own server in its place: no password, its socket anyone's, everything that reaches it
# recorded in /tmp/devvm-e2e-rfb.bin, each connection counted.
cat > "$p/fake-rfb.py" <<'P'
import os, socket
p = "/run/devvm-desktop/vnc.sock"
if os.path.exists(p):
    os.unlink(p)
s = socket.socket(socket.AF_UNIX)
s.bind(p)
os.chmod(p, 0o666)
s.listen(4)
n = 0
while True:
    c, _ = s.accept()
    n += 1
    open("/tmp/devvm-e2e-rfb.count", "w").write(str(n))
    c.sendall(b"RFB 003.008\n"); c.recv(12); c.sendall(b"\x01\x01"); c.recv(1); c.sendall(b"\0\0\0\0")
    c.recv(1)
    c.sendall(b"\0\x01\0\x01" + b"\0" * 16 + b"\0\0\0\0")
    data = b""
    while True:
        b = c.recv(4096)
        if not b:
            break
        data += b
    open("/tmp/devvm-e2e-rfb.bin", "wb").write(data)
    c.close()
P
in_vm "$vm" "sudo pkill -x Xvnc; sleep 1; sudo rm -f /tmp/devvm-e2e-rfb.*; sudo setsid -f python3 '$p/fake-rfb.py' </dev/null >/dev/null 2>&1; sleep 1" >/dev/null 2>&1
check "root's own server, without a password: a viewer with a wrong one is still refused, the password let in" \
  desktop_view DEVVM_E2E_CLIPBOARD=devvm-e2e-clip
check "... the refused viewer never reached the VM, nor the viewer's clipboard: only its key event did" bash -c "sleep 1
  limactl shell --workdir / $vm sh -c '[ \"\$(cat /tmp/devvm-e2e-rfb.count)\" = 1 ] &&
    od -An -tx1 /tmp/devvm-e2e-rfb.bin | tr -d \" \\n\" | grep -q 040100000000006b &&
    ! grep -q devvm-e2e-clip /tmp/devvm-e2e-rfb.bin' </dev/null"
desktop_end
check "Ctrl+C ends devvm desktop, its port closed" bash -c "[[ -n '$desk_port' ]] && ! nc -z 127.0.0.1 '$desk_port'"
in_vm "$vm" "sudo pkill -f '[f]ake-rfb.py'; sudo rm -f /run/devvm-desktop/vnc.sock /tmp/devvm-e2e-rfb.*" >/dev/null 2>&1
desktop_serve "$p" --clipboard
check "devvm desktop --clipboard starts its server again, on the same port, the clipboard on" bash -c "
  [[ -n '$desk_port' && '$desk_port' == \$(st_get '$(vmdir "$vm")' ports.host.desktop) ]] &&
  limactl shell --workdir / $vm bash -lc 'DISPLAY=:1 XAUTHORITY=\$HOME/.Xauthority vncconfig -get AcceptCutText | grep -qx on' </dev/null"
check "... and a viewer with the password gets it" desktop_view
desktop_end
fi

if begin nix; then
section "the nix layer"
# Behind a company proxy, the layer's script (as a layer of the run's own, so it builds each run) gets
# its build proxy's settings, which Nix's installer would write into nix-daemon's: none is anywhere
# on its disk. A layer of yours gets no prelude, so its copy starts with devvm's.
corp_start
cat "$root/assets/prelude.sh" "$root/assets/layers/nix.sh" > "$layers/e2enix.sh"
check "built behind a company proxy, the layer leaves no proxy settings for nix-daemon in its image" bash -c "
  env HTTPS_PROXY='$corpurl' HTTP_PROXY='$corpurl' '$D' build e2enix </dev/null >'$work/last.log' 2>&1 &&
  k=\$(keys_of e2enix | head -1) && [[ -n \$k && -d '$LIMA_HOME/devvm-img-'\$k ]] &&
  grep -q 'releases\.nixos\.org' '$work/corp.jsonl' &&
  printf '%s\n' 'Environment=http_proxy=' 'Environment=https_proxy=' '$corppw' | no_leak '$LIMA_HOME/devvm-img-'\$k"
owned e2enix
corp_stop
rm -f "$layers/e2enix.sh"
# A node project with a flake: node, detected, and --layers nix, with network policy. The flake's dev shell has a node of its
# own, from nixpkgs at a pinned revision (nixos-26.05's), cached by Nix's servers.
nixpkgs_rev=6f06d204d8f2d8761d9da78778745974a3216381
x=$(newproj nixflake); echo '{}' > "$x/package.json"; echo 'use flake' > "$x/.envrc"
cat > "$x/flake.nix" <<EOF
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/$nixpkgs_rev";
  outputs = { self, nixpkgs }: {
    devShells = nixpkgs.lib.genAttrs [ "aarch64-linux" "x86_64-linux" ] (s: let pkgs = nixpkgs.legacyPackages.\${s}; in {
      default = pkgs.mkShellNoCC { packages = [ (pkgs.writeShellScriptBin "node" "echo e2e-flake-node") ]; };
    });
  };
}
EOF
# The shared Nix cache starts empty, so the first VM fetches into it and the second gets hits.
nixcache=$XDG_DATA_HOME/devvm/cache/nix-cache
rm -rf "$nixcache"
launch "$x" exit --network-policy --no-git-protect --layers nix
export -f in_vm
nv=$(sed -n 's/^nix_version=//p' "$root/assets/layers/nix.sh")
dv=$(sed -n 's/^direnv_version=//p' "$root/assets/layers/nix.sh")
check "a flake.nix and a package.json: a node VM with the nix layer and network policy, Nix and direnv at their pins" bash -c "
  [[ -n '$vm' ]] && grep -qF \"\$(m new_vm_gets layers=nix tools=node vm=$vm)\" '$work/last.log' &&
  vm_state '$(vmdir "$vm")' | grep -qx image=nix && vm_state '$(vmdir "$vm")' | grep -qx tools.names=node &&
  vm_state '$(vmdir "$vm")' | grep -qx net.on=true && vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok &&
  in_vm '$vm' '[[ \$(nix --version) == \"nix (Nix) $nv\" && \$(direnv version) == $dv ]]'"
check "Nix's store is root's, builds are sandboxed, only root is trusted, and the user's own caches are ignored" in_vm "$vm" '
  [[ -z $(sudo find /nix/store -mindepth 1 -maxdepth 1 ! -user root -print -quit) ]] &&
  [[ $(nix config show sandbox) == true && $(nix config show sandbox-fallback) == false ]] &&
  [[ $(nix config show trusted-users) == root ]] &&
  nix store info --option substituters https://example.invalid 2>&1 | grep -q "ignoring untrusted substituter"'
check "nix-daemon gets the VM's proxy alone, and its NO_PROXY, in devvm's drop-in" in_vm "$vm" '
  [[ $(ls /etc/systemd/system/nix-daemon.service.d) == devvm-proxy.conf && $NO_PROXY == localhost,127.0.0.1,::1 ]] &&
  e=$(systemctl show nix-daemon.service -p Environment --value) &&
  [[ $e == "http_proxy=$HTTP_PROXY https_proxy=$HTTP_PROXY no_proxy=$NO_PROXY" ]]'
check "the VM's Nix tries the shared cache first, then cache.nixos.org, trusting cache.nixos.org's key alone" in_vm "$vm" '
  [[ -f /etc/nix/devvm-cache.conf && $(nix config show substituters) == "http://devvm.proxy/nix https://cache.nixos.org"* ]] &&
  [[ $(nix config show trusted-public-keys) == "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=" ]]'
check "nix run nixpkgs#hello works with network policy, through the shared cache" in_vm "$vm" \
  "nix run nixpkgs#hello 2>/tmp/hello.err | grep -qx 'Hello, world!' && grep -q 'from .http://devvm.proxy/nix' /tmp/hello.err &&
   nix build --no-link github:NixOS/nixpkgs/$nixpkgs_rev#hello"
nixev=$(vmdir "$vm")/logs/network.jsonl
check "... which the host fetched into the shared cache, each file the user's alone" bash -c "
  grep -q '\"event\":\"nix-cache\".*\"reason\":\"fetched\"' '$nixev' &&
  ls '$nixcache'/*.narinfo >/dev/null && ls '$nixcache'/nar/*.nar* >/dev/null &&
  [[ -z \$(find '$nixcache' -type f ! -perm 600 -print -quit) && -z \$(find '$nixcache' -type d ! -perm 700 -print -quit) ]]"
check "a VM's writes to the shared cache, and its requests for anything else through it, are refused" in_vm "$vm" '
  c() { curl -s -o /dev/null -w "%{http_code}" --proxy "$HTTP_PROXY" "$@"; }
  z=00000000000000000000000000000000
  [[ $(c -X PUT --data-binary x http://devvm.proxy/nix/$z.narinfo) == 400 ]] &&
  [[ $(c -X POST --data-binary x http://devvm.proxy/nix/nar/${z}00000000000000000000.nar.xz) == 400 ]] &&
  [[ $(c http://devvm.proxy/nix/https://example.com/x) == 400 ]] &&
  [[ $(c "http://devvm.proxy/nix/$z.narinfo?u=https://example.com") == 400 ]] &&
  [[ $(c --path-as-is http://devvm.proxy/nix/nar/../../etc/passwd) == 400 ]] &&
  [[ $(c http://devvm.proxy/nix/nar/${z}00000000000000000000.nar.xz) == 404 ]] &&
  [[ $(c http://devvm.proxy/nix/$z.narinfo) == 404 ]]'
check "... and the host kept none of them, asking cache.nixos.org alone" bash -c "
  [[ ! -e '$nixcache/00000000000000000000000000000000.narinfo' ]] &&
  grep -q '\"path\":\"/nix/00000000000000000000000000000000.narinfo\".*\"reason\":\"not found: cache.nixos.org answered 404\"' '$nixev' &&
  grep -q '\"path\":\"/nix/nar/0*.nar.xz\".*\"reason\":\"not found: no checked narinfo names it\"' '$nixev' &&
  [[ -z \$(find '$nixcache' -type f ! -name '.*' ! -name '*.narinfo' ! -path '*/nar/*' -print -quit) ]]"
# A second VM of a Nix project gets what the first fetched from the shared cache, as hits.
nixvm1=$vm
y=$(newproj nixflake2); cp "$x/flake.nix" "$x/flake.lock" "$x/package.json" "$y/"
launch "$y" exit --network-policy --no-git-protect --layers nix
nixvm2=$vm
check "a second VM gets the paths the first fetched as hits from the shared cache, fetching none again" bash -c "
  [[ -n '$nixvm2' && '$nixvm2' != '$nixvm1' ]] &&
  in_vm '$nixvm2' \"nix build --no-link github:NixOS/nixpkgs/$nixpkgs_rev#hello 2>/tmp/hello.err && grep -q 'from .http://devvm.proxy/nix' /tmp/hello.err\" &&
  ev='$(vmdir "$nixvm2")/logs/network.jsonl' &&
  grep -q '\"event\":\"nix-cache\",.*\"path\":\"/nix/nar/[^\"]*\".*\"reason\":\"hit\"' \"\$ev\" &&
  ! grep -q '\"event\":\"nix-cache\".*\"reason\":\"fetched\"' \"\$ev\""
limactl stop "$nixvm2" >/dev/null 2>&1
vm=$nixvm1
nix_built='derivation { name = "e2e-sandboxed"; system = builtins.currentSystem; builder = "/bin/sh"; args = [ "-c" "echo ok > $out" ]; }'
check "a build runs in Nix's sandbox, which it would fail without" in_vm "$vm" "nix build --no-link --impure --expr '$nix_built'"
check "sudo nix finds Nix, and so does a command over ssh" bash -c "
  in_vm '$vm' 'sudo nix --version' | grep -qx 'nix (Nix) $nv' &&
  ssh -F '$LIMA_HOME/$vm/ssh.config' lima-$vm 'nix --version' </dev/null | grep -qx 'nix (Nix) $nv'"
# An interactive shell, its commands on its input, as a terminal's: direnv's and mise's hooks run
# before each prompt.
printf '%s\n' "cd $x" 'direnv allow' 'cd /' "cd $x" 'echo "e2e-a=$(node -v)"' 'true' 'echo "e2e-b=$(node -v)"' \
  'cd /' 'echo "e2e-c=$(node -v)"' | limactl shell --workdir / "$vm" bash -i >"$work/nix-shell.log" 2>&1
check "the flake's dev shell, through direnv, comes before the shared node, prompt after prompt, and goes" bash -c "
  grep -qx 'e2e-a=e2e-flake-node' '$work/nix-shell.log' && grep -qx 'e2e-b=e2e-flake-node' '$work/nix-shell.log' &&
  grep -qx 'e2e-c=v$(lockv node)' '$work/nix-shell.log'"
check "nix-direnv's cache is in the VM's home, not the project folder" bash -c "
  [[ ! -e '$x/.direnv' && -f '$x/flake.lock' ]] && in_vm '$vm' 'ls -d ~/.cache/direnv/layouts/*/flake-profile-*' >/dev/null"
# A download only the daemon makes (a fixed-output fetch) from a host the policy doesn't allow:
# refused by the proxy, and named by devvm-why.
nix_fetch='derivation { name = "e2e"; system = builtins.currentSystem; builder = "builtin:fetchurl"; url = "https://example.com/devvm-e2e"; outputHashMode = "flat"; outputHashAlgo = "sha256"; outputHash = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="; }'
check "a download from a host that isn't allowed fails, and devvm-why names it" in_vm "$vm" "
  ! nix build --no-link --impure --expr '$nix_fetch' 2>/tmp/fetch.err && grep -q 'response 403' /tmp/fetch.err &&
  devvm-why | grep -q 'devvm network allow example.com'"
check "devvm network off: from the next start, nix-daemon gets no proxy" bash -c "cd '$x' &&
  '$D' network off </dev/null >/dev/null && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  in_vm '$vm' '[[ -z \$(ls -A /etc/systemd/system/nix-daemon.service.d 2>/dev/null) && -z \$(systemctl show nix-daemon.service -p Environment --value) ]]'"
check "... and no shared cache: Nix goes to cache.nixos.org alone" in_vm "$vm" \
  '[[ ! -e /etc/nix/devvm-cache.conf && $(nix config show substituters) != *devvm.proxy* ]]'
limactl stop "$vm" >/dev/null 2>&1
fi

if begin from; then
section "--from"
# Registry images need no Docker: these run with a docker that's installed but not running (it
# logs what devvm asked it, which may only be whether it has the image: launches ask only the
# backends a VM's state names).
mkdir -p "$work/nodocker"; : > "$work/docker-calls"
printf '#!/bin/sh\necho "docker $*" >> "%s"\necho "Cannot connect to the Docker daemon" >&2; exit 1\n' \
  "$work/docker-calls" > "$work/nodocker/docker"
chmod 755 "$work/nodocker/docker"
nd() { PATH="$work/nodocker:$PATH" "$@"; }
export -f nd in_vm; export work
# Each image by its pinned digest (from_node and the others, at the top), which devvm looks up
# nowhere: the bases stay cached until the pins move. The company proxy's check below keeps a tag.
f=$(newproj fromdocker); echo '{}' > "$f/package.json"
fh=$work/from-home; mkdir -p "$fh"   # the Claude Code VM's: no agent config of yours
# tool_base FILE: the base digest in a tool image's record (musl's), else nothing.
tool_base() { sed -n 's/^  "base": "\(.*\)",\{0,1\}$/\1/p' "$share/$1.json"; }
# tools_built_in LOG: the keys of the images that LOG's tool builds cloned, one per line.
tools_built_in() { sed -n 's/^+ limactl clone .* devvm-img-\([0-9a-f]\{12\}\) devvm-tools-build$/\1/p' "$1"; }
export -f tool_base tools_built_in
# newest_log: the newest run log, the last devvm's.
# shellcheck disable=SC2012  # devvm's own names, without spaces
newest_log() { ls -1t "$XDG_DATA_HOME"/devvm/logs/runs/*.log 2>/dev/null | head -1; }
HOME=$fh nd launch "$f" exit --from "$from_node" --no-network-policy --tools node,python --agents claude --layers docker,nix   # node:24 is Debian 12
fromlog=$(newest_log)
# Any VM may mount gnu tool images, so a --from image's code never runs in their builds. They may be
# cached from an earlier run: then the log has no build.
check "its tool images built in a clone of devvm's own Ubuntu base, never the Debian image's, and record no base" bash -c "
  for k in \$(tools_built_in '$fromlog'); do [[ \$(rec \$k name) == base && -z \$(rec \$k from) ]] || { echo \"built in \$k\"; exit 1; }; done &&
  for f in \$(st_get '$(vmdir "$vm")' tools.made.files); do [[ -z \$(tool_base \$f) ]] || exit 1; done"
# A build in Nix's sandbox, which fails without it (the nix layer's sandbox-fallback = false).
nix_built='derivation { name = "e2e-sandboxed"; system = builtins.currentSystem; builder = "/bin/sh"; args = [ "-c" "echo ok > $out" ]; }'
check "a VM with node, python, Claude Code and the docker and nix layers from node:24 (Debian 12) boots, its base keyed by its pinned digest" bash -c "
  [[ -n '$vm' ]] && limactl shell --workdir / '$vm' cat /etc/os-release </dev/null | grep -qx VERSION_CODENAME=bookworm &&
  vm_state '$(vmdir "$vm")' | grep -qx 'from=$from_node' && vm_state '$(vmdir "$vm")' | grep -qx tools.made.libc=gnu &&
  [[ \$(rec \"\$(from_record '$from_node')\" digest) == '$from_node' ]]"
# The newest glibc each tool image needs, as its build measured it, in its record.
glibc_need() { sed -n 's/^  "glibc": "\(.*\)",\{0,1\}$/\1/p' "$share/$1.json"; }
export -f glibc_need; export share
check "its base records the image's glibc, as its VM does, and no tool image of the VM needs a newer one" bash -c "
  g=\$(limactl shell --workdir / '$vm' getconf GNU_LIBC_VERSION </dev/null) && g=\${g#glibc } && [[ -n \$g ]] &&
  [[ \$(rec \"\$(from_record '$from_node')\" glibc) == \$g ]] && vm_state '$(vmdir "$vm")' | grep -qx \"tools.made.glibc=\$g\" &&
  for f in \$(st_get '$(vmdir "$vm")' tools.made.files); do n=\$(glibc_need \$f)
    [[ -z \$n ]] || printf '%s\n%s\n' \"\$n\" \"\$g\" | sort -C -t. -k1,1n -k2,2n || exit 1; done"
for f in $(st_get "$(vmdir "$vm")" tools.made.files); do echo "        glibc: $f needs $(glibc_need "$f" | grep . || echo none)"; done
check "its node, claude and every feature work" in_vm "$vm" \
  'node --version && claude --version && eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
check "python, python3 and claude are the locked versions" in_vm "$vm" \
  "[[ \$(python --version) == 'Python $(lockv python)' && \$(python3 --version) == 'Python $(lockv python)' && \$(claude --version) == '$(lockv claude) '* ]]"
check "an interactive shell starts without errors" bash -c "out=\$(limactl shell --workdir / $vm bash -ic true </dev/null 2>&1) && ! grep -iE 'error|unknown option|not found' <<<\"\$out\""
check "the docker layer's Docker runs in it (Docker's Debian repository), for the VM's user too" in_vm "$vm" \
  'sudo docker run --rm hello-world | grep -q Hello && id -nG | grep -qw docker'
check "the nix layer's Nix runs in it (systemd): nix run nixpkgs#hello, and a build in Nix's sandbox" in_vm "$vm" "
  systemctl is-active --quiet nix-daemon.socket && nix run nixpkgs#hello | grep -qx 'Hello, world!' &&
  nix build --no-link --impure --expr '$nix_built'"
(cd "$f" && HOME=$fh quiet "$D" rm --force --no-export)

# Alpine: apk, OpenRC and musl, so its own (musl) tool images; the docker layer from Alpine's packages;
# the nix layer's daemon as an OpenRC service of devvm's; the desktop layer from Alpine's packages.
al=$(newproj fromalpine); echo '{}' > "$al/package.json"
nd launch "$al" exit --from "$from_alpine" --no-network-policy --layers docker,nix,desktop
check "an Alpine VM runs node from musl tool images" bash -c "[[ -n '$vm' ]] && vm_state '$(vmdir "$vm")' | grep -qx tools.made.libc=musl && [[ \$(rec \"\$(from_record '$from_alpine')\" libc) == musl ]]"
alog=$(newest_log)
check "its musl tool images built in its own base, and are keyed and recorded by that base's digest" bash -c "
  b=\$(from_record '$from_alpine') && d=\$(rec \$b digest) && [[ -n \$d ]] &&
  for k in \$(tools_built_in '$alog'); do [[ \$k == \$b ]] || { echo \"built in \$k\"; exit 1; }; done &&
  for f in \$(st_get '$(vmdir "$vm")' tools.made.files); do [[ \$(tool_base \$f) == \"\$d\" ]] || { echo \"\$f: \$(tool_base \$f)\"; exit 1; }; done"
check "and node, pnpm work there" in_vm "$vm" \
  "grep -q '^ID=alpine' /etc/os-release && [[ \$(node -v) == v$(lockv node) ]] && node -e 'require(\"os\")' && pnpm -v"
check "the docker layer's Docker runs in it (OpenRC)" in_vm "$vm" 'sudo docker run --rm hello-world | grep -q Hello'
check "the desktop layer's programs are in it (Alpine's packages)" in_vm "$vm" \
  'command -v Xvnc && command -v startxfce4 && command -v vncconfig && [[ -x /usr/local/bin/devvm-desktop ]]'
check "ports below 1024 are open to the VM's user after the setup (Alpine's kernel starts at 1024)" in_vm "$vm" \
  '[[ $(cat /proc/sys/net/ipv4/ip_unprivileged_port_start) == 0 ]]'
check "and every feature" in_vm "$vm" 'eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
# As root: syslog's and acpid's pid files are root-only, so a user's rc-status calls them crashed.
check "no crashed services" in_vm "$vm" '[[ -z "$(sudo rc-status --crashed 2>/dev/null)" ]]'
check "the nix layer's Nix runs in it (OpenRC, musl): nix run nixpkgs#hello, and a build in Nix's sandbox" in_vm "$vm" "
  sudo rc-service nix-daemon status >/dev/null && nix run nixpkgs#hello | grep -qx 'Hello, world!' &&
  nix build --no-link --impure --expr '$nix_built'"
# With network policy, OpenRC's service reads the proxy from its settings: a download only the daemon
# makes, from a host that isn't allowed, gets the proxy's 403.
nix_fetch='derivation { name = "e2e"; system = builtins.currentSystem; builder = "builtin:fetchurl"; url = "https://example.com/devvm-e2e"; outputHashMode = "flat"; outputHashAlgo = "sha256"; outputHash = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="; }'
check "devvm network on: after devvm restart, its nix-daemon goes through the proxy" bash -c "cd '$al' &&
  nd '$D' network on </dev/null >/dev/null 2>&1 && printf 'exit\n' | nd '$D' restart >'$work/last.log' 2>&1 &&
  vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok && in_vm '$vm' 'sudo grep -q \"^export http_proxy=http://devvm:\" /etc/conf.d/nix-daemon &&
    ! nix build --no-link --impure --expr '\''$nix_fetch'\'' 2>/tmp/fetch.err && grep -q \"response 403\" /tmp/fetch.err'"
(cd "$al" && quiet "$D" rm --force --no-export)

# Ubuntu's container image has empty package lists, which apt.sh fills from its own sources.
check "base builds from ubuntu:26.04" nd quiet "$D" build --from "$from_ubuntu" base
check "none of them needed Docker" bash -c "! grep -v '^docker image inspect ' '$work/docker-calls'"
check "and the build VM and its temporary folders are gone" bash -c "
  ! limactl list -q 2>/dev/null | grep -qx devvm-oci-build && ! ls -d \"\${TMPDIR:-/tmp}\"/devvm-oci.* 2>/dev/null"
# An image only Docker has (built, never pushed) isn't checked: making one would change Docker's images.

# Behind a company proxy with a password (corp_start), a --from conversion (an Alpine release no
# other check uses, so it's converted each run) and the builds on it go through a proxy of devvm's
# for each build, which alone goes through the company proxy: the registry, base's and its
# features' downloads and a layer's own all reach it, while nothing a build runs sees its password,
# and no converted disk, image or log keeps it. The layer prints what its commands, and sudo's,
# have of the proxy settings; the password is read from its file, never written into a check.
corp_start
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\n%s\n' \
  'echo "e2e-env: $(env | grep -i "_proxy=" | sort | tr "\n" " ")"' \
  'echo "e2e-sudo: $(sudo env | grep -i "_proxy=" | sort | tr "\n" " ")"' \
  'curl -fsS -m 30 -o /dev/null https://github.com' > "$layers/e2ecorp.sh"
check "behind a company proxy, --from's conversion and the builds on it reach it through devvm's own proxies" bash -c "
  HTTPS_PROXY='$corpurl' HTTP_PROXY='$corpurl' PATH='$work/nodocker':\"\$PATH\" '$D' build --from alpine:3.21 e2ecorp \
    </dev/null >'$work/last.log' 2>&1 &&
  grep -q '\"event\":\"tunnel\",\"host\":\"[a-z0-9.-]*docker\.io\"' '$work/corp.jsonl' &&
  grep -q '\"host\":\"dl-cdn\.alpinelinux\.org\"' '$work/corp.jsonl' &&
  grep -q '\"event\":\"tunnel\",\"host\":\"github\.com\"' '$work/corp.jsonl'"
owned e2ecorp
kac=$(for k in $(keys_of e2ecorp); do rec "$k" chain; done | sed -n 's/^base=\([0-9a-f]*\) e2ecorp=.*$/\1/p' | head -1)
[[ -z $kac ]] || created+=("devvm-img-$kac")
kcl=$(keys_of e2ecorp | head -1)
# The run log writes a proxy key (32 to 64 hex digits) as <key>: its stand-in there says the build's
# commands had one, and no key of devvm's is left in the log.
check "what the builds' commands and sudo have is devvm's proxy for the build, with a password of its own" bash -c "
  log=\$(grep -l '^e2e-env: ' '$XDG_DATA_HOME'/devvm/logs/runs/*.log | tail -1) && [[ -n \$log ]] &&
  grep -qE '^e2e-env: .*HTTPS_PROXY=http://devvm:<key>@192\.168\.5\.2:[0-9]+ ' \"\$log\" &&
  grep -qE '^e2e-sudo: .*HTTPS_PROXY=http://devvm:<key>@192\.168\.5\.2:[0-9]+ ' \"\$log\" &&
  ! grep -qE 'devvm:[0-9a-f]{32,64}@' \"\$log\" &&
  ! grep -qF ':$P18766' \"\$log\""
check "the company proxy's password is in no converted disk, image, log, build file or process's arguments" bash -c "
  [[ -n '$kac' && -n '$kcl' && -d '$LIMA_HOME/devvm-img-$kac' && -d '$LIMA_HOME/devvm-img-$kcl' ]] &&
  no_leak '$LIMA_HOME/devvm-img-$kac' '$LIMA_HOME/devvm-img-$kcl' \
    '$XDG_DATA_HOME/devvm/logs' '$XDG_DATA_HOME/devvm/run' '$vms' <'$work/corp.auth' &&
  ! ps -axww -o command= | grep -qFf '$work/corp.auth' &&
  [[ -z \$(ls -d '$XDG_DATA_HOME'/devvm/run/builds/*/run/build-proxy 2>/dev/null) ]] &&
  ! ls -d \"\${TMPDIR:-/tmp}\"/devvm-oci.* 2>/dev/null"
corp_stop
rm -f "$layers/e2ecorp.sh"
fi

# After from, whose Alpine image a link's client starts from in the --from tier.
if begin link; then
section "links between VMs"
link_section lk
fi

# A script error can end a section early without failing a check: each section is compared with its
# last complete run of this same e2e.sh (section_end). Fewer checks than it fails the run, and that
# section's times aren't kept.
section_end
# The results, for a --slots run to add up: "KEY<TAB>VALUE" lines.
{ printf 'pass\t%s\nfail\t%s\nskipped\t%s\nelapsed\t%s\n' "$pass" "$fail" "$skipped" $((SECONDS - started))
  for l in ${failed[@]+"${failed[@]}"}; do printf 'failed\t%s\n' "$l"; done
  for l in ${retried[@]+"${retried[@]}"}; do printf 'retried\t%s\n' "$l"; done
  for l in ${short[@]+"${short[@]}"}; do printf 'short\t%s\n' "$l"; done
} > "$work/summary.new" && mv "$work/summary.new" "$work/summary"
echo
echo "e2e: $pass passed, $fail failed$( (( skipped == 0 )) || echo ", $skipped skipped")$( (( ${#retried[@]} == 0 )) || echo ", ${#retried[@]} retried") in $(mins $((SECONDS - started)))${failed[*]+: ${failed[*]}}"
for l in ${retried[@]+"${retried[@]}"}; do echo "e2e: retried once: $l"; done
for l in ${short[@]+"${short[@]}"}; do echo "e2e: FAILED: $l"; done
# --newest-base: what devvm pinned, and a failure named as upstream drift with its release.
if $newest; then
  got=$(sed -n 's/^  "release": "\(.*\)",\{0,1\}$/\1/p' "$images/ubuntu.json" 2>/dev/null)
  echo "e2e: newest base: Ubuntu listed $base_release as its newest; devvm pinned ${got:-nothing}"
  if (( fail > 0 || ${#short[@]} > 0 )); then
    echo "e2e: UPSTREAM DRIFT: base from Ubuntu's newest cloud image, ${got:-$base_release}, and the mirrors' newest packages, failed the checks above, which e2e's other runs miss as they pin $e2e_ubuntu_release; a user's first build of base gets this now (contributing/maintaining.md says what to do)"
  fi
fi
(( fail == 0 && ${#short[@]} == 0 ))
