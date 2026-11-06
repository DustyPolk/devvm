#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]] [--every-feature] [--slots N | --slot N | --newest-base] [--golden]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images, and
#                builds base again from its recorded packages with the mirror unreachable;
#                project (Go's, tests/e2e/project_test.go, as every section is) makes a VM from base in a project folder, opens it from a subfolder in that
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
go_sections="images project clone gc tools network desktop nix from link"
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
# perl: every section reads VMs' vm.json with JSON::PP (vm_state); no_leak reads disks with
# File::Find (images).
need perl perl perl perl
if command -v perl >/dev/null 2>&1; then need_pm JSON::PP perl perl perl-JSON-PP; fi
if want images; then
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
  [[ -z ${ca_server:-} ]] || kill "$ca_server" 2>/dev/null
  [[ -z ${cad:-} ]] || rm -rf "$cad" "$cafolder"   # the run's own CA, and devvm's CA folder in the sandbox
  ps_unwatch
  pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock" 2>/dev/null   # devvm's own, in the sandbox
  pkill -f "[_]ports -vm .* -lifeline $DEVVM_LIMA_HOME/" 2>/dev/null  # VMs' forwarders, the sandbox's
  rm -rf "$src" "$work/net-home" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/listener.json" "$work/ps.log" "$work/lima-bypass" "$work/go-e2e.log"
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
# A check here that fails twice fails; every other check fails on its first failure. None of this
# script's checks is on it now: the Go sections mark theirs Flaky.
retry=()
# retries LABEL: the check is on the retry list.
retries() { local l; for l in ${retry[@]+"${retry[@]}"}; do [[ $l == "$1" ]] && return 0; done; return 1; }
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
section() { echo; echo "== $*"; }
quiet() { "$@" </dev/null >"$work/last.log" 2>&1; }
# rec KEY FIELD: a string field of the record of the image with that key, images/KEY/record.json,
# which devvm writes one field per line; nothing without one.
rec() { sed -n "s/^  \"$2\": \"\(.*\)\",\{0,1\}\$/\1/p" "$images/$1/record.json" 2>/dev/null; }
# keys_of IMAGE: the keys of the images recorded under that name, one per line.
keys_of() { local d; for d in "$images"/*/; do d=${d%/}; [[ $(rec "${d##*/}" name) != "$1" ]] || echo "${d##*/}"; done; }
# owned IMAGE: the images by that name are this run's, deleted at the end.
owned() { local k; for k in $(keys_of "$1"); do created+=("devvm-img-$k"); done; }
status_of() { limactl list --format '{{.Status}}' "$1" 2>/dev/null; }
export -f rec keys_of status_of; export images
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

if $clean; then
  section "cleaning cached images"
  for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
  rm -rf "$images" "$share"
  # A newest base's packages come from the mirrors, not from an earlier run's cache.
  ! $newest || rm -rf "$XDG_DATA_HOME/devvm/packages"
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
