#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]] [--every-feature] [--slots N | --slot N | --newest-base] [--golden]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images, and
#                builds base again from its recorded packages with the mirror unreachable;
#                project (Go's, tests/e2e/project_test.go, as every section but from and link is) makes a VM from base in a project folder, opens it from a subfolder in that
#                subfolder, and checks its mounts, a file watcher in it (the VM's edits seen, none
#                of this machine's: Lima's mounts pass on no file events), no Docker, every
#                port through devvm's forwarder (never a port a program here has, nor a server on a
#                link's address), ssh by
#                name with pinned host keys, .git protection and shell, then devvm stop, opening it
#                again, restart, devvm size, devvm ports (a running VM's at once, never a listed port a
#                program here has, a bad list a usage error), a mistyped command refused with a suggestion,
#                devvm git-protect and rm, a VM from a devcontainer.json (its ports, variables, commands
#                once and at each start, a changed file, devvm update reading it again, an image on this machine's registry refused), an agent VM's folders and history (devvm history export, rm's export, a new VM's offer, devvm history import and its refusal), and SSH agent forwarding
#                (a throwaway agent's, and devvm's own with a throwaway key); clone makes a --clone
#                VM of a throwaway repo and checks its clone (pushed past an insteadOf), the remote, fetching, and rm's checks
#                for unfetched commits and uncommitted changes and the work it keeps, then a git worktree's clone; gc (Go's,
#                tests/e2e/gc_test.go, which this script hands over to after its own sections) deletes a stale
#                image and an unused tool image, keeping base and a VM named like an image; tools resolves
#                the shared tools' newest versions (every other section has tests/e2e-mise.lock's), makes a node
#                VM and checks its shared tools, devvm tools, tools update, rm and add, devvm node an
#                unknown command and --tools ignored on it; network makes a VM with Claude Code, Codex and opencode, with
#                network policy and checks it reaches only its allowlist, devvm network, its log and rules, its tokens
#                stay on the host (devvm secret too), open too, and the self-check, on relaunch too; devvm stop
#                stops its proxy; nix builds the nix layer behind a company proxy (no proxy left in its
#                image), makes a node VM with the nix layer of a flake project with network policy and checks Nix's
#                pins, store and trust, nix-daemon's proxy, nix run nixpkgs#hello through the shared Nix
#                cache (fetched by the host, a VM's writes and other requests refused, a second VM's
#                hits), sudo nix, the flake's dev shell through direnv before the shared node, a blocked
#                download named, and devvm network open (the proxy and the shared cache stay); link makes two small targets and
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
#                (Go's, tests/e2e/desktop_test.go) a base VM without a desktop (devvm desktop refused), then a VM with the desktop
#                layer and network policy whose desktop devvm
#                desktop serves on this machine's loopback alone: a wrong password refused, the
#                password let in, the VM's server on its user's own socket and no TCP port, the
#                clipboard off, the password in no file, log or process's arguments, no way out of the
#                VM; the local proxy the session uses (no password asked, a blocked host's 403, never
#                forwarded); root's own server in its place (no password) still behind devvm's, the viewer's
#                clipboard never reaching it; --clipboard
#     --from     every section, with from, and every base image feature: the longest. from makes VMs
#                from node:24 (Debian 12, with node, python, the docker and nix layers and claude) and
#                alpine:3 (musl tools, the docker layer on OpenRC, the nix layer's daemon an OpenRC service,
#                through the proxy once network policy is on, the desktop layer's programs, devvm history
#                import and its refusal), each with GNU tar and every
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
# The sections ported to Go, in tests/e2e (its gc_test.go and the rest): this script runs the
# others, then hands these to the Go runner (go test -tags e2e), in its sandbox and under its lock.
go_sections="images project clone gc tools network desktop nix"
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
# go_sel: the chosen sections that are Go's, comma-separated, for its -e2e.only.
go_sel=""
for s in $go_sections; do ! want "$s" || go_sel="$go_sel${go_sel:+,}$s"; done
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
if want link; then need nc netcat netcat-openbsd nmap-ncat; fi
if want link; then
  if [[ $host_os == Darwin ]]; then need lsof lsof - -; else need ss - iproute2 iproute; fi
fi
if want link; then need curl curl curl curl; fi
if want link; then need ssh openssh openssh-client openssh-clients; fi
if want link && ! command -v shasum >/dev/null 2>&1; then need sha256sum coreutils coreutils coreutils; fi
# perl: every section reads VMs' vm.json with JSON::PP (vm_state); no_leak reads disks with
# File::Find (images, from).
need perl perl perl perl
if command -v perl >/dev/null 2>&1; then need_pm JSON::PP perl perl perl-JSON-PP; fi
if want images || want from; then
  if command -v perl >/dev/null 2>&1; then need_pm File::Find perl perl perl-File-Find; fi
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
P47195=$((47195 + po)) P47196=$((47196 + po)) P47197=$((47197 + po)) P47198=$((47198 + po))
P47240=$((47240 + po)) P47241=$((47241 + po)) P47242=$((47242 + po)) P47243=$((47243 + po)) P47244=$((47244 + po))
P47245=$((47245 + po)) P47246=$((47246 + po)) P47248=$((47248 + po)) P47249=$((47249 + po))
P47199=$((47199 + po)) P47613=$((47613 + po)) P18765=$((18765 + po)) P18766=$((18766 + po))
export P47160 P47161 P47162 P47163 P47164 P47170 P47171 P47172 P47173 P47174 P47175 P47176 P47177 P47178 \
  P47180 P47181 P47182 P47195 P47196 P47197 P47198 P47199 P47240 P47241 P47242 P47243 P47244 P47245 P47246 P47248 P47249 \
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
# CLI tests' vm_state prints it: a section's fields dotted (net.tier=allowlist), a list of strings
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
# vm_set FILE KEY JSON: sets KEY (dotted: net.tier) of a VM's vm.json to the JSON value, keeping the
# rest, and writes the file back whole, the user's alone.
vm_set() {
  perl -MJSON::PP -e '
    my ($f, $k, $j) = @ARGV; local $/; open my $fh, "<", $f or die "$f: $!\n"; my $js = JSON::PP->new->allow_nonref;
    my $v = $js->decode(<$fh>); close $fh; my @p = split /\./, $k; my $last = pop @p; my $o = $v;
    $o = ($o->{$_} //= {}) for @p; $o->{$last} = $js->decode($j);
    open $fh, ">", "$f.tmp" or die "$f: $!\n"; print $fh JSON::PP->new->pretty->canonical->encode($v); close $fh;
    chmod 0600, "$f.tmp"; rename "$f.tmp", $f or die "$f: $!\n";' "$1" "$2" "$3"
}
export -f vm_set
projects=$work/p                    # the project folders, this run's
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
  [[ "$(uname -s)" != Darwin ]] || security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a claude >/dev/null 2>&1
  rm -f "$XDG_DATA_HOME/devvm/secrets/github" "$XDG_DATA_HOME/devvm/secrets/claude" "$work/net-home/Library/Keychains"   # the link, never the folder
  [[ -z ${listener:-} ]] || kill "$listener" 2>/dev/null
  [[ -z ${corp:-} ]] || kill "$corp" 2>/dev/null   # the company proxy's stand-in
  [[ -z ${ca_server:-} ]] || kill "$ca_server" 2>/dev/null
  [[ -z ${cad:-} ]] || rm -rf "$cad" "$cafolder"   # the run's own CA, and devvm's CA folder in the sandbox
  ps_unwatch
  pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock" 2>/dev/null   # devvm's own, in the sandbox
  pkill -f "[_]ports -vm .* -lifeline $DEVVM_LIMA_HOME/" 2>/dev/null  # VMs' forwarders, the sandbox's
  rm -rf "$src" "$work/net-home" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/nodocker" "$work/docker-calls" "$work/from-home" \
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
# times go to $work/timings.new/SECTION, and $later is what the chosen sections after it took, Go's
# among them, as they run last.
begin() { local s after=false t
  want "$1" || return 1
  section_end
  cur=$1 later=0; : > "$work/timings.new/$cur"
  for s in $sections_all; do
    if want "$s" && { $after || [[ " $go_sections " == *" $s "* ]]; }; then t=$(awk -F'\t' '{ s += $2 } END { print s + 0 }' "$tdir/$s" 2>/dev/null); later=$((later + ${t:-0})); fi
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
started=$SECONDS ncheck=0 started_at=$(date +%s)
echo "$started_at" > "$work/mark"
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
# lk_prep VM: the helper written to the VM, and the link checks' ports ($P47240-$P47249) reserved in
# it, at once and at every boot. Linux hands out ports from 32768 to 60999 to a connection's own end,
# these among them, and a connection the VM's side closed keeps its port for 60 s (TIME_WAIT), which
# no server can bind on 127.0.0.1 or every address then, SO_REUSEADDR or not: lk_cap's 257
# connections from the client would leave a later server or ssh -R there without its port.
lk_prep() {
  printf '%s\n' "$lk_pl" | lk_put "$1" /var/tmp/e2e-link.pl &&
    lk_root "$1" "echo 'net.ipv4.ip_local_reserved_ports = $P47240-$P47249' > /etc/sysctl.d/90-e2e-link.conf &&
      sysctl -q -p /etc/sysctl.d/90-e2e-link.conf && [ \"\$(cat /proc/sys/net/ipv4/ip_local_reserved_ports)\" = $P47240-$P47249 ]"; }
# lk_start VM serve|hold ADDR PORT [WORD]: the helper's server in the VM, as root, once it listens;
# lk_end VM ADDR PORT ends it. lk_serve VM WORD PORTS...: lk_prep, and a server on the VM's
# 127.0.0.1 on each port, answering WORD-PORT.
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
  lk_prep "$vm" || return 1
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
# lk_set_net VM on|off|TIER: the VM's tier, as its state has it (on: allowlist, off: open).
lk_set_net() { local t=$2; [[ $t != on ]] || t=allowlist; [[ $t != off ]] || t=open; vm_set "$(vmdir "$1")/vm.json" net.tier "\"$t\""; }
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
  local small=(--network open --no-git-protect --cpus 2 --memory 2)
  launch "$lk_p1" exit "${small[@]}" --ports none; lk_v1=$vm
  launch "$lk_p2" exit "${small[@]}" --ports none; lk_v2=$vm
  launch "$lk_pc" exit "${small[@]}" --ports all; lk_vc=$vm
  lk_w1=${lk_x}db lk_w2=${lk_x}api lk_n2=${lk_x}api   # the targets' servers' words; the second's link name
  check "two targets that forward no port and a client that forwards every port, without network policy" lk_made
  lk_serve "$lk_v1" "$lk_w1" $P47240 $P47242 $P47243 $P47244 $P47249 80 >/dev/null 2>&1
  lk_serve "$lk_v2" "$lk_w2" $P47240 80 >/dev/null 2>&1
  lk_prep "$lk_vc" >/dev/null 2>&1
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
  check "devvm network allowlist is refused for a client linked to a target without network policy" lk_on_refused
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
  check "in a group: devvm network allowlist is refused, so is joining another group, and devvm link rm of the group's link" gp_refused
  check "devvm group leave removes only the group's links, in it and the others, naming them; the user's link stays; the last one's leaving removes the group" gp_leave
  check "a join is refused when a member's name is the joiner's own hostname, whatever name it joins as" gp_hostname
  check "devvm network allowlist for a target while it runs, then for the client, restarted behind its proxy" lk_policy_on
  check "the target's network policy on but not yet enforced (devvm ls: unenforced): the link's next connection is refused and logged, and devvm link add and the client's launch warn" lk_policy_unenforced
  check "the target restarted behind its proxy" lk_policy_target
  check "with network policy, the client reaches NAME:PORT and NAME.devvm.internal:PORT by curl through its proxy settings and a raw TCP client; a direct connection fails (the control)" lk_policy_reach
  check "a link added while the client with network policy runs: curl through its proxy reaches it at once, by GET and CONNECT, no link name is in NO_PROXY, and nothing restarts" lk_proxy_added
  check "the client's proxy reaches only the links on its own list: not a name or port it hasn't linked, a name doubled or an address; the target's proxy doesn't reach the client's links" lk_proxy_own
  check "devvm network log shows the links' connections with network policy" lk_log_on
  check "a client with network policy is refused by devvm link add for a target without it" lk_mismatch_add
  check "devvm network open is refused for a target that a client with network policy links to" lk_off_refused
  check "the tier rule: an allowlist client's link to a target on none goes on, to one on internet is refused and warned of, and goes on once the target is back on allowlist" lk_tiers
  check "the rule broken under devvm: the link's next connection is refused, and the client's launch warns and runs on without it" lk_policy_broken
  check "the rule restored, the client's next launch has the link again" lk_policy_back
  if want from; then
    check "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" lk_alpine
  else
    skip "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" "Alpine's image comes from the from section"
  fi
  check "a group is on one tier: an open VM is refused by an allowlist group, and an allowlist one by an open group; two on allowlist reach each other through their group's links" gp_policy
  check "devvm rm of the client leaves no link session, socket, pid file, log or link state" lk_rm_client
  (cd "$lk_p1" && quiet "$D" rm --force --no-export)
  (cd "$lk_p2" && quiet "$D" rm --force --no-export)
  ulimit -S -n "$nofile"
}
lk_made() { local v
  for v in "$lk_v1" "$lk_v2" "$lk_vc"; do
    if ! { [[ -n $v ]] && lk_running "$v" && vm_state "$(vmdir "$v")" | grep -qx net.tier=open; }; then echo "${v:-a VM} wasn't made"; return 1; fi; done
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
  lk_dv "$lk_pc" network allowlist && r=1
  cat "$work/last.log"
  (( r == 0 )) && grep -Eq "^devvm: tier_linked_targets targets=.*$lk_v1.* tier=allowlist vm=$lk_vc\$" "$work/last.log" &&
    grep -Eq "^devvm: tier_linked_targets targets=.*$lk_v2.* tier=allowlist vm=$lk_vc\$" "$work/last.log" && vm_state "$(vmdir "$lk_vc")" | grep -qx net.tier=open; }
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
  # one, before the 256 connections are opened. Their ends in the client never take a link
  # check's port (lk_prep).
  lk_root "$lk_vc" "grep -qx $P47240-$P47249 /proc/sys/net/ipv4/ip_local_reserved_ports" ||
    { echo "failed: the client's ports $P47240-$P47249 aren't reserved"; return 1; }
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
  launch "$lk_p2" exit --network open --no-git-protect --cpus 2 --memory 2 --ports none
  if ! { [[ $vm == "$lk_v2" ]] && lk_running "$lk_v2"; }; then echo "not made again: ${vm:-none}"; return 1; fi
  ! lk_shows "$lk_pc" "$lk_v2" link && ! lk_resolves "$lk_vc" "$lk_n2" &&
    [[ $(vm_state "$(vmdir "$lk_vc")" | grep '^links\.links\.') == "$id0" ]] && lk_dv "$lk_p2" link publish $P47240 && lk_serve "$lk_v2" "$lk_w2" $P47240; }
# The target gets network policy first, while it runs: until its next start it runs outside its
# proxy, its servers still up, and the client's link to it is refused then, and warned of by
# devvm link add and the client's launch (lk_policy_unenforced).
lk_policy_on() {
  if ! { lk_dv "$lk_p1" network allowlist && vm_state "$(vmdir "$lk_v1")" | grep -qx net.tier=allowlist; }; then echo "$lk_v1:"; tail -5 "$work/last.log"; return 1; fi
  if ! { lk_dv "$lk_pc" network allowlist && lk_up "$lk_pc" restart && vm_state "$(vmdir "$lk_vc")" | grep -qx net.tier=allowlist && vm_state "$(vmdir "$lk_vc")" | grep -qx net.made.check=ok; }; then
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
  lk_gets "$lk_vc" db $P47240 "$lk_w1-$P47240" && vm_state "$(vmdir "$lk_vc")" | grep -qx net.tier=allowlist || return 1
  last=$(cat "$(vmdir "$lk_vc")/logs/network.jsonl.1" "$(vmdir "$lk_vc")/logs/network.jsonl" 2>/dev/null |
    grep -F "\"event\":\"link\",\"host\":\"db\",\"port\":$P47240," | tail -1)
  echo "the last: ${last:-none}, since $t0"
  [[ $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") > "$t0" || $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") == "$t0" ]] &&
    lk_shows "$lk_pc" "  link +db:$P47240  to $lk_v1\$" network log -n 100000; }
lk_mismatch_add() {
  ! lk_dv "$lk_pc" link add "$lk_n2" $P47240 && lk_said "devvm: link_tier_mismatch target=$lk_v2 target_tier=open tier=allowlist vm=$lk_vc" && ! lk_resolves "$lk_vc" "$lk_n2"; }
lk_off_refused() {
  ! lk_dv "$lk_p1" network open && lk_said "devvm: tier_linked_clients tier=open vm=$lk_v1 vms=$lk_vc" && vm_state "$(vmdir "$lk_v1")" | grep -qx net.tier=allowlist; }
# The tier rule both ways: the client on allowlist links to a stricter target, none, whose
# connections go on, and not to a lower one, internet, whose next connection is refused; the
# target back on allowlist, they go on again.
lk_tiers() {
  if ! { lk_set_net "$lk_v1" none && lk_up "$lk_p1" && lk_until 20 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240"; }; then
    echo "none:"; tail -5 "$work/last.log"; return 1
  fi
  if ! { lk_set_net "$lk_v1" internet && lk_up "$lk_p1"; }; then echo "internet:"; tail -5 "$work/last.log"; return 1; fi
  lk_gets "$lk_vc" db $P47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db $P47240 '"reason":"policy"' &&
    lk_up "$lk_pc" && lk_said "devvm: link_tier_mismatch target=$lk_v1 target_tier=internet tier=allowlist vm=$lk_vc" || return 1
  lk_set_net "$lk_v1" allowlist && lk_up "$lk_p1" && lk_up "$lk_pc" && lk_until 20 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240"; }
lk_policy_broken() {
  lk_set_net "$lk_v1" off || return 1
  lk_gets "$lk_vc" db $P47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db $P47240 '"reason":"policy"' &&
    lk_up "$lk_pc" && lk_said "devvm: link_tier_mismatch target=$lk_v1 target_tier=open tier=allowlist vm=$lk_vc" && lk_running "$lk_vc" &&
    lk_until 10 lk_not lk_resolves "$lk_vc" db; }
lk_policy_back() {
  lk_set_net "$lk_v1" on && lk_up "$lk_pc" && ! grep -q link_tier_mismatch "$work/last.log" &&
    lk_until 10 lk_resolves "$lk_vc" db && lk_until 20 lk_curls "$lk_vc" http://db:$P47240/ "$lk_w1-$P47240"; }
lk_alpine() { local pa va r=0
  pa=$(newproj "${lk_x}alp")
  nd launch "$pa" exit --from "$from_alpine" --network open --no-git-protect --cpus 2 --memory 2 --ports none; va=$vm
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
# Only a member is refused: outside the group, network allowlist would turn the second target's policy on,
# failing the checks after this one that need it off.
gp_refused() {
  vm_state "$(vmdir "$lk_v2")" | grep -qx "links.group=$lk_g" || { echo "$lk_v2 isn't in $lk_g: network allowlist not tried"; return 1; }
  ! lk_dv "$lk_p2" network allowlist &&lk_said "devvm: group_tier_fixed group=$lk_g vm=$lk_v2" && vm_state "$(vmdir "$lk_v2")" | grep -qx net.tier=open &&
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
  (( r == 0 )) && ! lk_dv "$lk_p2" group join "$on" && lk_said "devvm: group_tier_mismatch group=$on tier=allowlist vm=$lk_v2 vm_tier=open" || r=1
  (( r == 0 )) && lk_dv "$lk_p2" group join "$off" && ! lk_dv "$lk_p1" group join "$off" &&
    lk_said "devvm: group_tier_mismatch group=$off tier=open vm=$lk_v1 vm_tier=allowlist" && ! vm_state "$(vmdir "$lk_v1")" | grep -q "^links\.group=$off" || r=1
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
HOME=$fh nd launch "$f" exit --from "$from_node" --network open --tools node,python --agents claude --layers docker,nix   # node:24 is Debian 12
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
check "its commands run GNU tar, which devvm history import needs" bash -c "limactl shell --workdir / $vm bash -c 'tar --version | head -1 | grep -q \"GNU tar\"' </dev/null"
check "the docker layer's Docker runs in it (Docker's Debian repository), for the VM's user too" in_vm "$vm" \
  'sudo docker run --rm hello-world | grep -q Hello && id -nG | grep -qw docker'
check "the nix layer's Nix runs in it (systemd): nix run nixpkgs#hello, and a build in Nix's sandbox" in_vm "$vm" "
  systemctl is-active --quiet nix-daemon.socket && nix run nixpkgs#hello | grep -qx 'Hello, world!' &&
  nix build --no-link --impure --expr '$nix_built'"
(cd "$f" && HOME=$fh quiet "$D" rm --force --no-export)

# Alpine: apk, OpenRC and musl, so its own (musl) tool images; the docker layer from Alpine's packages;
# the nix layer's daemon as an OpenRC service of devvm's; the desktop layer from Alpine's packages.
al=$(newproj fromalpine); echo '{}' > "$al/package.json"
nd launch "$al" exit --from "$from_alpine" --network open --layers docker,nix,desktop
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
check "devvm network allowlist: after devvm restart, its nix-daemon goes through the proxy" bash -c "cd '$al' &&
  nd '$D' network allowlist </dev/null >/dev/null 2>&1 && printf 'exit\n' | nd '$D' restart >'$work/last.log' 2>&1 &&
  vm_state '$(vmdir "$vm")' | grep -qx net.made.check=ok && in_vm '$vm' 'sudo grep -q \"^export http_proxy=http://devvm:\" /etc/conf.d/nix-daemon &&
    ! nix build --no-link --impure --expr '\''$nix_fetch'\'' 2>/tmp/fetch.err && grep -q \"response 403\" /tmp/fetch.err'"
# devvm history import works on Alpine as on Ubuntu: base's Alpine branch installs GNU tar, which
# the import's listing and unpacking need (busybox's refuses their options), and the rest of its
# script runs on busybox's grep and find. busybox's grep -r over the home and /tmp runs past the
# check's limit there, so the refusal's check looks for the file by name, on the root's disk alone.
check "an Alpine VM's commands run GNU tar" bash -c "limactl shell --workdir / $vm bash -c 'tar --version | head -1 | grep -q \"GNU tar\"' </dev/null"
check "history import puts an export into an Alpine VM" bash -c "x='$work/export/alpine' && rm -rf \"\$x\" && mkdir -p \"\$x/.claude/projects/e2e\" &&
  echo e2e-alpine > \"\$x/.claude/projects/e2e/t.jsonl\" && (cd \"\$x\" && tar -czf ../alpine.tar.gz .claude) &&
  cd '$al' && '$D' history import '$work/export/alpine.tar.gz' </dev/null >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm bash -c '[[ \$(cat ~/.claude/projects/e2e/t.jsonl) == e2e-alpine ]]' </dev/null"
check "and an export with a path outside the agents' history imports nothing there" bash -c "ev='$work/export/evil' && rm -rf \"\$ev\" &&
  mkdir -p \"\$ev/d\" && echo e2e-escape > \"\$ev/e2e-escape\" && (cd \"\$ev/d\" && tar -czPf ../evil.tar.gz ../e2e-escape) &&
  cd '$al' && ! '$D' history import \"\$ev/evil.tar.gz\" </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m history_import_unsafe file=\$ev/evil.tar.gz)\" '$work/last.log' &&
  o=\$(limactl shell --workdir / $vm sudo find /home /root /tmp /etc -xdev -name e2e-escape </dev/null) && [[ -z \$o ]]"
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
# The chosen sections that are Go's, last, in this run's sandbox and under its lock: the Go runner
# gets this run's pid (the lock's), how many checks it ran and expects in all and when it started,
# so its results count on from these, and writes them to summary.go, which this run adds to its own.
# Its own heartbeat and time limits take over from this run's.
if [[ -n $go_sel ]]; then
  kill "$hb" 2>/dev/null; wait "$hb" 2>/dev/null; rm -f "$work/deadline"
  goargs=(-e2e.only="$go_sel" -e2e.slot="$slot" -e2e.lock-holder=$$ -e2e.count="$ncheck" -e2e.total="$total"
    -e2e.started="$started_at" -e2e.summary="$work/summary.go")
  ! $features_on || goargs+=(-e2e.every-feature)
  ! $newest || goargs+=(-e2e.newest-base)
  ! $golden || goargs+=(-e2e.golden)
  rm -f "$work/summary.go"
  (cd "$root/tests/e2e" && CGO_ENABLED=0 "${gocmd[@]}" test -tags e2e -count=1 -timeout 0 -run '^TestE2E$' -args "${goargs[@]}")
  if [[ -f $work/summary.go ]]; then
    while IFS=$'\t' read -r key val; do
      case $key in
        pass) pass=$((pass + val)) ;; fail) fail=$((fail + val)) ;; skipped) skipped=$((skipped + val)) ;;
        failed) failed+=("$val") ;; retried) retried+=("$val") ;; short) short+=("$val") ;;
      esac
    done < "$work/summary.go"
    rm -f "$work/summary.go"
  else
    short+=("the Go runner ($go_sel) ended without its results: look for its errors above")
  fi
fi
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
