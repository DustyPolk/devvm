#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images;
#                project makes a VM from base in a project folder and checks its mounts, ssh by
#                name with pinned host keys, .git protection and shell, then devvm stop, opening it
#                again, restart, devvm size, devvm ports, devvm git-protect and rm, an agent VM's folders and history (devvm export, rm's export), and SSH agent forwarding
#                (a throwaway agent's, and devvm's own with a throwaway key); clone makes a --clone
#                VM of a throwaway repo and checks its clone, the remote, fetching, and rm's checks
#                for unfetched commits and uncommitted changes and the work it keeps; gc deletes a stale
#                image and an unused tool image, keeping base and a VM named like an image; tools makes a node
#                VM and checks its shared tools, update and remove; network makes a claude VM with
#                network policy and checks it reaches only its allowlist, devvm log and policy, its tokens
#                stay on the host (devvm secret too), and the self-check, on relaunch too; devvm stop
#                stops its proxy
#     --from     every section, with from, and every base image feature: the longest. from makes VMs
#                from node:24 (Debian 12, with node, python and claude) and alpine:3 (musl tools,
#                Docker on OpenRC), each with every feature, and builds base from ubuntu:26.04, with a
#                docker that isn't running
#     --only     just these sections; SECTIONS below
#     --clean    delete the sandbox's cached images and tool images first (default: reuse them
#                between runs)
#     --backend smolvm
#                the smolvm backend's own section instead (experimental): base and a layer built as
#                packs, a project VM's mounts, size, ports, Docker and agent forwarding, a node VM's
#                shared tools, what it refuses (agent toggling), ssh for editors' emulation (pinned
#                host keys of each VM's own, its own key alone, no forwarding out even past a hostile
#                sshd, devvm ssh and code, a --clone VM's push and fetch), network policy's emulation
#                (nothing but the proxy, DNS kept on this machine, failing closed, the policy, ssh
#                opening no way out), every port's (servers on the VM's loopback or all its addresses
#                reach this machine's loopback alone, never a port of this machine's or devvm's, no
#                way out with network policy),
#                and that no machine without the sandbox's label is touched. Its machines share your
#                smolvm (macOS): it deletes only those it made, with the sandbox's label.
# The sandbox: Lima home ~/.lt-e2e-go (or $DEVVM_E2E_LIMA_HOME), state under /tmp/devvm-go-e2e. It refuses
# to run against ~/.lima or devvm's own Lima home, deletes only the VMs it created, and uses no real
# tokens. devvm is built from this checkout into the sandbox, with its files next to it.
set -uo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sections_all="images project clone gc tools network from"
usage() { echo "usage: tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...] | --backend smolvm]  (sections: $sections_all)" >&2; exit 2; }
clean=false sel="images project clone gc tools network" backend=lima
while (( $# )); do
  case $1 in
    --clean) clean=true ;;
    --from)  sel=$sections_all ;;
    --only)  [[ -n "${2:-}" ]] || usage; sel=$(tr ',' ' ' <<<"$2"); shift
             for s in $sel; do [[ " $sections_all " == *" $s "* ]] || usage; done ;;
    --backend) [[ ${2:-} == smolvm ]] || usage; backend=smolvm sel=smolvm; shift ;;
    *) usage ;;
  esac
  shift
done
want() { [[ " $sel " == *" $1 "* ]]; }

# Its own Lima home (other sandboxes' images aren't its own), for devvm and for its own limactl calls.
export DEVVM_LIMA_HOME=${DEVVM_E2E_LIMA_HOME:-$HOME/.lt-e2e-go}
export LIMA_HOME=$DEVVM_LIMA_HOME
work=/tmp/devvm-go-e2e
export XDG_DATA_HOME=$work/data XDG_CONFIG_HOME=$work/config
export DEVVM_SECRET_SERVICE=devvm-go-e2e   # its Keychain items: fake tokens only, deleted at the end
unset DEVVM_MESSAGES DEVVM_VERBOSE DEVVM_UBUNTU_URL DEVVM_HOST_MISE CLAUDE_CODE_OAUTH_TOKEN DEVVM_FROM
unset SSH_AUTH_SOCK   # your SSH agent is never reached: checks that need one start their own
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
# A test build: DEVVM_UNREVIEWED counts in it (never in a release).
(cd "$root" && CGO_ENABLED=0 "${gocmd[@]}" build -ldflags "-X github.com/w3cj/devvm/internal/backend/registry.unreviewed=allowed" \
  -o "$src/devvm" ./cmd/devvm) || { echo "e2e: couldn't build devvm" >&2; exit 1; }
# The features are in base's key, so only a run with from (which checks them) builds them; every
# other selection shares one smaller base.
if want from; then for f in "$root"/assets/features/*.sh; do basename "$f" .sh; done > "$XDG_CONFIG_HOME/devvm/base-features"
else : > "$XDG_CONFIG_HOME/devvm/base-features"; fi
D=$src/devvm
images=$XDG_DATA_HOME/devvm/images   # the image records, one per key
layers=$XDG_CONFIG_HOME/devvm/layers
vms=$XDG_DATA_HOME/devvm/vms         # the VMs' state, one per VM
projects=$work/p                     # the project folders, this run's
share=$XDG_DATA_HOME/devvm/tools     # the shared tool images
lock=$XDG_DATA_HOME/devvm/mise.lock  # the shared tools' versions, resolved at the start of each run
export DEVVM_NO_TOOL_CHECK=1         # and not again mid-run
# lockv TOOL: its version in the lock.
lockv() { awk -v t="[[tools.$1]]" '$0 == t { f = 1; next } f && /^version = / { gsub(/"/, "", $3); print $3; exit }' "$lock"; }
# m NAME [name=value...]: a message's text, from devvm, so checks never repeat its wording. Exported
# for the checks' own shells.
m() { "$D" _msg "$@"; }
export -f m; export D

# smol_ls: the sandbox's smolvm machines, "NAME STATE" lines: those with its label. smol_others: every
# other machine smolvm has, "NAME LABEL" (yours, on macOS). smolvm's JSON has each machine's keys
# sorted, labels before name and state.
smol_json() { SMOLVM_USAGE=0 smolvm machine ls --json 2>/dev/null | awk '
  /^  \{/ { lab = ""; n = ""; s = "" }
  /^      "devvm\.data": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); lab = v }
  /^    "name": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); n = v }
  /^    "state": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); s = v }
  /^  \}/ { print n "\t" s "\t" lab }'; }
smol_ls() { smol_json | awk -F'\t' -v want="$XDG_DATA_HOME/devvm" '$3 == want { print $1, $2 }'; }
smol_others() { smol_json | awk -F'\t' -v want="$XDG_DATA_HOME/devvm" '$3 != want { print $1, $3 }'; }
smol_status() { smol_ls | awk -v n="$1" '$1 == n { print $2 }'; }
export -f smol_json smol_ls smol_status
created=()   # VMs this run made; the only ones it deletes
# ps_watch: every host process's command line, sampled every 0.1 s into $work/ps.log until
# ps_unwatch, for checking a secret was in none while devvm ran.
watcher=""
ps_watch() {
  : > "$work/ps.log"
  (while :; do ps -axww -o command= >> "$work/ps.log" 2>/dev/null; sleep 0.1; done) &
  watcher=$!
}
ps_unwatch() { [[ -z $watcher ]] || { kill "$watcher" 2>/dev/null; wait "$watcher" 2>/dev/null; watcher=""; }; }
cleanup() {
  local n
  for n in ${created[@]+"${created[@]}"}; do
    if [[ $backend == smolvm ]]; then
      # Only a machine with the sandbox's label, and the image's pack.
      [[ -z $(smol_status "$n") ]] || SMOLVM_USAGE=0 smolvm machine delete --name "$n" -f >/dev/null 2>&1
      rm -f "$XDG_DATA_HOME/devvm/smolvm/vms/$n.json" "$XDG_DATA_HOME/devvm/smolvm/vms/$n.proxy" "$XDG_DATA_HOME/devvm/smolvm/vms/$n.ports."*
      [[ $n != devvm-img-* ]] || rm -rf "$XDG_DATA_HOME/devvm/smolvm/images/${n#devvm-img-}"
    else
      limactl delete -f "$n" >/dev/null 2>&1
    fi
    rm -f "$images/${n#devvm-img-}" "$vms/$n"
  done
  rm -f "$layers"/e2e*.sh "$layers"/e2e*.policy
  [[ "$(uname -s)" != Darwin ]] || security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a github >/dev/null 2>&1
  rm -f "$XDG_CONFIG_HOME/devvm/secrets/github" "$work/net-home/Library/Keychains"   # the link, never the folder
  [[ -z ${listener:-} ]] || kill "$listener" 2>/dev/null
  ps_unwatch
  [[ -z ${udp_listener:-} ]] || kill "$udp_listener" 2>/dev/null
  [[ -z ${fake:-} ]] || kill "$fake" 2>/dev/null
  [[ -z ${agent_pid:-} ]] || kill "$agent_pid" 2>/dev/null
  [[ -z ${user_agent_pid:-} ]] || kill "$user_agent_pid" 2>/dev/null
  pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock" 2>/dev/null   # devvm's own, in the sandbox
  rm -rf "$src" "$work/ssh" "$work/net-home" "$work/agent-home" "$work/agent-ssh" "$XDG_CONFIG_HOME/devvm/agent-keys" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/export.list" "$work/export" "$XDG_DATA_HOME/devvm/exports" "$work/nodocker" "$work/docker-calls" "$work/from-home" \
    "$work/dns-probe.sh" "$work/udp-probe.sh" "$work/probe.log" "$work/decisions" "$work/udp.log" "$work/nettop.csv" "$work/listener.auth" "$work/ps.log"
  # The project folders' .git is locked (macOS): unlocked to go.
  if [[ -d $projects ]]; then
    [[ "$(uname -s)" != Darwin ]] || chflags -R nouchg "$projects" 2>/dev/null
    rm -rf "$projects"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# Progress: each result shows [n/N], how long it took (since the previous check, so builds count),
# and the time left as the previous run measured it ($work/timings: "label<TAB>seconds"). A
# heartbeat every minute says what's happening during long steps.
timings=$work/timings-$(tr ' ' '-' <<<"$sel")   # per selection: they run different checks
: > "$timings.new"
total=$(wc -l 2>/dev/null < "$timings" | tr -d ' '); [[ ${total:-0} -gt 0 ]] || total="?"
started=$SECONDS ncheck=0
date +%s > "$work/mark"
mins() { printf '%dm%02ds' $(($1 / 60)) $(($1 % 60)); }
heartbeat() {
  local esc=$'\033' s f
  while sleep 60; do
    s=$(( $(date +%s) - $(cat "$work/mark") ))
    # The newest output: a check's, or devvm's own run log.
    # shellcheck disable=SC2012  # names without spaces
    f=$(ls -1t "$work/last.log" "$work/check.log" "$XDG_DATA_HOME"/devvm/logs/runs/*.log 2>/dev/null | head -1)
    printf '        ... %s since the last check: %s\n' "$(mins "$s")" \
      "$( [[ -f $f ]] && tail -1 "$f" | tr -d '\r' | sed "s/${esc}\[[0-9;?]*[A-Za-z]//g" | cut -c1-100)"
  done
}
heartbeat &
hb=$!
trap 'kill $hb 2>/dev/null; cleanup' EXIT

pass=0 fail=0 failed=()
# check LABEL CMD...: run CMD, count and print the result (and CMD's last output lines on failure).
check() {
  local label=$1 r dur left; shift
  if "$@" >"$work/check.log" 2>&1; then r=PASS; pass=$((pass + 1)); else r=FAIL; fail=$((fail + 1)); failed+=("$label"); fi
  ncheck=$((ncheck + 1))
  dur=$(( $(date +%s) - $(cat "$work/mark") )); date +%s > "$work/mark"
  printf '%s\t%s\n' "$label" "$dur" >> "$timings.new"
  left=$(awk -F'\t' -v l="$label" 'found { s += $2 } $1 == l { found = 1 } END { if (found) print s }' "$timings" 2>/dev/null)
  [[ $total == "?" ]] || (( ncheck <= total )) || total="?"   # more checks than last time
  echo "  $r  [$ncheck/$total] $label ($(mins "$dur"); $(mins $((SECONDS - started))) in${left:+, about $(mins "$left") left})"
  [[ $r == PASS ]] || tail -5 "$work/check.log" | sed 's/^/        /'
}
# skip LABEL WHY: a check this run has nothing for (counted, so runs compare their counts).
skip() {
  ncheck=$((ncheck + 1)) skipped=$((skipped + 1))
  printf '%s\t0\n' "$1" >> "$timings.new"
  echo "  SKIP  [$ncheck/$total] $1 ($2)"
}
skipped=0
section() { echo; echo "== $*"; }
quiet() { "$@" </dev/null >"$work/last.log" 2>&1; }
# keys_of IMAGE: the keys of the images recorded under that name, one per line.
keys_of() { grep -lx "name=$1" "$images"/* 2>/dev/null | while read -r f; do basename "$f"; done; }
# owned IMAGE: the images by that name are this run's, deleted at the end.
owned() { local k; for k in $(keys_of "$1"); do created+=("devvm-img-$k"); done; }
status_of() { limactl list --format '{{.Status}}' "$1" 2>/dev/null; }
export -f keys_of status_of; export images
# newproj NAME: a new, empty project folder, as devvm sees it (its physical path).
newproj() {
  [[ "$(uname -s)" != Darwin || ! -e "$projects/$1" ]] || chflags -R nouchg "$projects/$1"
  rm -rf "${projects:?}/$1"; mkdir -p "$projects/$1"; (cd "$projects/$1" && pwd -P)
}
# vm_of DIR: the VM devvm made for that folder, from its state.
vm_of() { grep -lx "dir=$1" "$vms"/* 2>/dev/null | head -1 | xargs basename 2>/dev/null; }
# launch DIR INPUT ARGS...: devvm ARGS in DIR, with INPUT for the shell it opens (which then ends);
# sets $vm and records it for cleanup. Its output is in $work/last.log.
launch() {
  local d=$1 in=$2; shift 2
  (cd "$d" && printf '%s\n' "$in" | "$D" "$@" >"$work/last.log" 2>&1)
  vm=$(vm_of "$d"); [[ -z $vm ]] || created+=("$vm")
}
in_vm() { limactl shell --workdir / "$1" bash -lc "$2" </dev/null; }

if $clean; then
  section "cleaning cached images"
  if [[ $backend == smolvm ]]; then
    # smolvm's images and their records, not Lima's.
    for n in $(smol_ls | awk '$1 ~ /^devvm-img-/ { print $1 }'); do SMOLVM_USAGE=0 smolvm machine delete --name "$n" -f >/dev/null 2>&1; done
    for k in "$XDG_DATA_HOME"/devvm/smolvm/images/*; do [[ ! -d $k ]] || rm -f "$images/$(basename "$k")"; done
    rm -rf "$XDG_DATA_HOME/devvm/smolvm/images"
  else
    for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
    rm -rf "$images" "$share"
  fi
fi

if want tools; then
# What a user gets: upstream's newest versions, each download with its checksum (rust: rustup checks),
# resolved by devvm's pinned mise, downloaded and checked on the first run.
section "shared tool versions"
march=arm64; [[ "$(uname -m)" != x86_64 ]] || march=x64   # the lock's platform: mise's name for the arch
ntools=$(awk -F' *= *' '/^\[tools\]/ { s = 1; next } /^\[/ { s = 0 } s && NF == 2' "$root/assets/shared-tools/mise.toml" | wc -l)
check "the shared tools resolve to upstream's newest, with checksums" bash -c "'$D' _refresh-tools </dev/null >'$work/last.log' 2>&1 &&
  (( \$(grep -c '^\[\[tools\.' '$lock') == $ntools )) &&
  (( \$(grep -A1 '\"platforms.linux-$march\"\]' '$lock' | grep -c '^checksum = \"sha256:') == $ntools - 1 ))"
fi

if want images; then
section "images"
check "devvm doctor finds everything it needs on this host" bash -c "'$D' doctor </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$('$D' doctor --fixes </dev/null) ]]"
# base stays cached between runs (--clean deletes it); the fixture layers' images are this run's.
check "build base" quiet env DEVVM_MESSAGES=names "$D" build base
# Its key: the build's, else the newest base from Ubuntu's image (from's bases are cached too).
kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
if [[ -n $kb ]] && want from; then
  features=$(tr '\n' ' ' < "$XDG_CONFIG_HOME/devvm/base-features")
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/last.log")
  check "base ran base.sh, then every feature" bash -c "
    for f in base $features; do grep -q \"^# \\[[0-9]*/[0-9]*\\] running_script script=\$f.sh vm=devvm-img-$kb\$\" '$runlog' || exit 1; done"
elif want from; then
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
[[ -n $kb ]] || kb=$(sed -n 's/^chain=base=\([0-9a-f]*\) e2efix=.*$/\1/p' "$images/$kf" 2>/dev/null)
check "base is a stopped image, recorded with Ubuntu's image and its C library" bash -c "
  [[ -n '$kb' && \$(status_of devvm-img-$kb) == Stopped ]] &&
  grep -qx 'chain=base=$kb' '$images/$kb' && grep -qx 'libc=gnu' '$images/$kb' && grep -qx 'from=' '$images/$kb' &&
  grep -qE '^digest=https://cloud-images\.ubuntu\.com/.*/release-[0-9]{8}/.*\.img sha256:[0-9a-f]{64}$' '$images/$kb'"
check "base mounts nothing from the host" bash -c "[[ -z \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' devvm-img-$kb) ]]"
check "devvm limactl runs Lima on devvm's Lima home, whatever LIMA_HOME says, with its exit code" bash -c "
  [[ -n '$kb' ]] && LIMA_HOME='$work/no-lima' '$D' limactl list -q </dev/null | grep -qx devvm-img-$kb &&
  ! '$D' limactl e2e-no-such-command </dev/null >/dev/null 2>&1"
check "building base again reuses it" bash -c "out=\$('$D' build base </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=base key=$kb)\" <<<\"\$out\""
check "its script ran in its image, cloned from base's" bash -c "
  [[ -n '$kf' && \$(status_of devvm-img-$kf) == Stopped ]] &&
  grep -qx 'chain=base=$kb e2efix=$kf' '$images/$kf' &&
  grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
echo "# edited" >> "$fixture"
check "editing it builds a new image; base and the old one stay" bash -c "'$D' build e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 2 ]] && [[ \$(status_of devvm-img-$kf) == Stopped ]] && [[ -f '$images/$kb' ]]"
owned e2efix
check "rebuild makes a new one and deletes the stale ones, not base" bash -c "'$D' rebuild e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 1 ]] && [[ \$(keys_of e2efix) != $kf ]] && [[ -z \$(status_of devvm-img-$kf) ]] &&
  [[ \$(status_of devvm-img-$kb) == Stopped ]]"
owned e2efix
kf=$(keys_of e2efix | head -1)
dg=$(sed -n 's/^digest=.* sha256:\([0-9a-f]\{12\}\)[0-9a-f]*$/\1/p' "$images/$kb" 2>/dev/null)
check "devvm images lists base and the fixture, current, with Ubuntu's release, digest and size" bash -c "
  out=\$('$D' images </dev/null 2>/dev/null) &&
  grep -qE '^base +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kb +current +[0-9.]+[KMGT] +lima$' <<<\"\$out\" &&
  grep -qE '^e2efix +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kf +current +[0-9.]+[KMGT] +lima$' <<<\"\$out\" &&
  grep -qE '^NAME +TOOL +VERSION +SIZE +VMS +STATE$' <<<\"\$out\""
check "images rm deletes an image by name, with its record, not base" bash -c "
  [[ -n '$kf' ]] && '$D' images rm e2efix </dev/null >/dev/null 2>&1 &&
  [[ -z \$(status_of devvm-img-$kf) ]] && [[ ! -e '$images/$kf' ]] && [[ \$(status_of devvm-img-$kb) == Stopped ]]"

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
  '$D' images rm '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(status_of devvm-img-$kx) ]]"
rm -f "$layers/e2efail.sh"

# Ctrl+C during a build: the run's names (DEVVM_MESSAGES) say which image it builds, its run log,
# and (in that log) when its script runs.
printf '#!/bin/bash\nsleep 600\n' > "$layers/e2eslow.sh"
DEVVM_MESSAGES=names "$D" build e2eslow </dev/null >"$work/slow.log" 2>&1 &
slow=$!
for _ in $(seq 300); do
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/slow.log")
  [[ -n $runlog ]] && grep -q '^# \[4/5\] running_script script=e2eslow.sh ' "$runlog" 2>/dev/null && break
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
fi

if want project; then
section "a base project"
p=$(newproj web); git -C "$p" init -q
# The shell devvm opens reads its commands from devvm's input: these, then it ends.
launch "$p" 'pwd; echo e2e-shell-ran' --no-network-policy --cpus 2 --memory 3 --disk 120 base
check "devvm creates the VM from base, and opens a shell in the folder" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx '$p' '$work/last.log' && grep -qx e2e-shell-ran '$work/last.log' &&
  grep -qx image=base '$vms/$vm' && grep -qx net=off '$vms/$vm' && grep -qx 'dir=$p' '$vms/$vm'"
check "it gets --cpus, --memory and --disk" in_vm "$vm" \
  '[[ $(nproc) == 2 ]] && (( $(awk "/^MemTotal:/ { print int(\$2 / 1048576 + 0.5) }" /proc/meminfo) == 3 )) && (( $(df -BG --output=size / | tail -1 | tr -dc 0-9) >= 115 ))'
check "devvm size shows it" bash -c "cd '$p' && [[ \$('$D' size 2>&1) == 'cpus=2 memory=3GiB disk=120GiB' ]]"
check "ls lists it, and no image" bash -c "'$D' ls >'$work/ls.out' 2>&1 &&
  grep -Eq '^$vm +Running +base +(on|off) +off +lima +$p\$' '$work/ls.out' && ! grep -q '^devvm-img-' '$work/ls.out'"
check "the folder is mounted read-write" bash -c "limactl shell --workdir / $vm bash -c 'echo from-vm > $p/vm.txt' </dev/null && grep -qx from-vm '$p/vm.txt'"
# Only the project folder and the shared tools: the VM's own mounts list holds nothing else (in a
# nested Linux test host, the guest's home has the host's path, so looking for files there proves
# nothing).
check "nothing else from the host is mounted" bash -c "[[ \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$p $share ' ]]"
check "SSH agent forwarding is off" bash -c "! limactl shell --workdir / $vm ssh-add -l </dev/null"
check "hostname is the folder" bash -c "[[ \$(limactl shell --workdir / $vm hostname </dev/null) == web ]]"
check "Exec's arguments, folder and input reach the guest as given" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2EExecQuoting\$' ./internal/backend/lima |
  grep -q -- '--- PASS: TestE2EExecQuoting'"

section "ssh by name"
# An ~/.ssh/config of its own, set up as the docs say, with a Host * that forwards the agent: yours
# is never read or changed. ssh gets no agent but a throwaway one.
sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc"
printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$XDG_CONFIG_HOME/devvm/ssh_config" > "$sc/config"
kh=$XDG_CONFIG_HOME/devvm/known_hosts
agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
check "the ssh config lists it by the folder's name" grep -q "^Host devvm-web $vm\$" "$XDG_CONFIG_HOME/devvm/ssh_config"
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
check "its sshd refuses remote forwarding (a port, a socket) and X11; a local forward works (the control)" bash -c "
  ! env -u SSH_AUTH_SOCK ssh -F '$sc/config' -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 devvm-web true </dev/null &&
  ! env -u SSH_AUTH_SOCK ssh -F '$sc/config' -o ExitOnForwardFailure=yes -R /tmp/devvm-e2e-\$\$.sock:'$sc/agent.sock' devvm-web true </dev/null &&
  limactl shell --workdir / $vm sudo sshd -T </dev/null | grep -qx 'x11forwarding no' &&
  limactl shell --workdir / $vm sudo sshd -T </dev/null | grep -qx 'allowtcpforwarding local' &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' -W 127.0.0.1:22 devvm-web </dev/null | head -c 4) == SSH- ]]"
check "the entry turns off X11, tunnels and adding keys to an agent, whatever Host * says" bash -c "
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n  Tunnel yes\n' '$XDG_CONFIG_HOME/devvm/ssh_config' > '$sc/hostile-config' &&
  g=\$(ssh -G -F '$sc/hostile-config' devvm-web) &&
  grep -qx 'forwardx11 no' <<<\"\$g\" && grep -qx 'forwardx11trusted no' <<<\"\$g\" && grep -qx 'addkeystoagent false' <<<\"\$g\" &&
  grep -qx 'tunnel false' <<<\"\$g\" && grep -qx 'permitremoteopen none' <<<\"\$g\" && grep -qx 'updatehostkeys false' <<<\"\$g\""

section ".git protection"
if [[ "$(uname -s)" != Darwin ]]; then
  check "off on Linux (no chflags), and says so" bash -c "grep -qx git_protect=off '$vms/$vm' &&
    grep -qF \"\$(m git_protect_needs_macos_new)\" '$work/last.log'"
else
  check "hooks and config are locked" bash -c "ls -lOd '$p/.git/config' | grep -q uchg && ls -lOd '$p/.git/hooks' | grep -q uchg &&
    grep -qx git_protect=on '$vms/$vm'"
  check "root in the VM can't add a hook" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x > $p/.git/hooks/pre-commit' </dev/null && [[ ! -e '$p/.git/hooks/pre-commit' ]]"
  check "root in the VM can't change the config" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x >> $p/.git/config' </dev/null"
  check "git in the VM can commit" in_vm "$vm" "cd $p && for i in 1 2 3 4 5 6 7 8 9 10; do git status >/dev/null 2>&1 && break; sleep 1; done &&
    git add -A && git -c user.email=e@e -c user.name=e commit -qm vm"
fi

section "an existing VM"
check "devvm stop stops it and drops its ssh config entry" bash -c "cd '$p' && '$D' stop </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(status_of $vm) == Stopped ]] && grep -qF \"\$(m stopped vm=$vm)\" '$work/last.log' &&
  ! grep -q '^Host devvm-web ' '$XDG_CONFIG_HOME/devvm/ssh_config'"
check "stopped, devvm starts it again" bash -c "[[ \$(status_of $vm) == Stopped ]] &&
  cd '$p' && printf 'echo e2e-again\n' | '$D' >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] && grep -qx e2e-again '$work/last.log'"
check "running, devvm opens it as it is" bash -c "cd '$p' && printf 'echo e2e-running\n' | '$D' 2>&1 | grep -qx e2e-running"
# devvm ssh and code with a home of their own, so the Include line is offered for an empty one (and,
# without a terminal, not added), and a stub code first on PATH: never your editor.
mkdir -p "$sc/home" "$sc/bin"
printf '#!/bin/sh\necho "$*" > %s/code-args\necho "${SSH_AUTH_SOCK-unset}" > %s/code-agent\n' "$sc" "$sc" > "$sc/bin/code"
chmod +x "$sc/bin/code"
check "devvm ssh starts a stopped VM and names it, without a shell; ssh reaches it" bash -c "cd '$p' &&
  HOME='$sc/home' '$D' stop </dev/null >/dev/null 2>&1 && [[ \$(status_of $vm) == Stopped ]] &&
  HOME='$sc/home' '$D' ssh </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | head -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | tail -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_include_yourself | head -1)\" '$work/last.log' && [[ ! -e '$sc/home/.ssh' ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "VS Code's settings in the VM: no port forwarding, no git sign-in relay" in_vm "$vm" \
  '[[ $(jq -c . ~/.vscode-server/data/Machine/settings.json) == "{\"remote.autoForwardPorts\":false,\"git.terminalAuthentication\":false,\"github.gitAuthentication\":false}" ]]'
in_vm "$vm" 'echo "{\"editor.fontSize\": 14, \"remote.autoForwardPorts\": true}" > ~/.vscode-server/data/Machine/settings.json' >/dev/null 2>&1
(cd "$p" && HOME=$sc/home "$D" ssh </dev/null >"$work/last.log" 2>&1)
check "changed back in the VM, devvm ssh turns them off again and keeps the others" in_vm "$vm" \
  'jq -e ".[\"editor.fontSize\"] == 14 and .[\"remote.autoForwardPorts\"] == false and .[\"github.gitAuthentication\"] == false" ~/.vscode-server/data/Machine/settings.json >/dev/null'
check "devvm code opens the folder over Remote-SSH (a stub code), with the agent devvm started with" bash -c "cd '$p' &&
  HOME='$sc/home' PATH='$sc/bin':\$PATH SSH_AUTH_SOCK='$sc/agent.sock' '$D' code </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(cat '$sc/code-args') == '--remote ssh-remote+devvm-web $p' && \$(cat '$sc/code-agent') == '$sc/agent.sock' ]]"
check "restart, and its new host keys are pinned" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "a running VM's new ports wait for its next start" bash -c "cd '$p' &&
  '$D' ports 47170-47173 2>&1 | grep -qF \"\$(m ports_next_start vm=$vm)\" && [[ \$('$D' ports 2>/dev/null) == all ]]"
check "so does its new size" bash -c "cd '$p' && '$D' size --cpus 3 2>&1 | grep -qF \"\$(m size_next_start vm=$vm)\" &&
  '$D' size 2>&1 >/dev/null | grep -qF \"\$(m size_pending size=cpus=3 vm=$vm)\""
check "and they apply on restart" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' ports 2>&1) == 47170-47173 && \$(limactl shell --workdir / $vm nproc </dev/null) == 3 &&
     \$('$D' size 2>&1) == 'cpus=3 memory=3GiB disk=120GiB' ]]"
check "a smaller disk is refused" bash -c "cd '$p' && ! '$D' size --disk 100 </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m disk_shrink have=120 vm=$vm want=100)\" '$work/last.log'"
# Listeners on each kind of address Lima matches differently (loopback, every address; IPv4 and 6), on
# listed ports (47170-47173) and unlisted ones (47174-47177). Their output goes nowhere, or the SSH
# session waits for them.
in_vm "$vm" 'p=47170; for r in 1 2; do for a in 127.0.0.1 ::1 0.0.0.0 ::; do
  setsid -f python3 -m http.server -b $a $p </dev/null >/dev/null 2>&1; p=$((p + 1)); done; done' >/dev/null 2>&1
check "only the listed ports reach this machine, whatever the address" bash -c \
  'for i in $(seq 20); do nc -z 127.0.0.1 47173 && break; sleep 1; done; sleep 3
  for p in 47170 47171 47172 47173; do nc -z 127.0.0.1 $p || exit 1; done
  for p in 47174 47175 47176 47177; do ! nc -z 127.0.0.1 $p || exit 1; done'
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
  check "git-protect waits for the VM to stop" bash -c "cd '$p' && printf 'exit\n' | '$D' >/dev/null 2>&1 &&
    [[ \$(status_of $vm) == Running ]] && '$D' git-protect off </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_running vm=$vm | head -1)\" && ls -lOd '$p/.git/config' | grep -q uchg"
  check "git-protect strict keeps host git status out of submodules" bash -c "limactl stop $vm >/dev/null 2>&1 && cd '$p' &&
    '$D' git-protect strict </dev/null >/dev/null 2>&1 && [[ \$(git config --file .git/config diff.ignoreSubmodules) == all &&
    \$('$D' git-protect 2>/dev/null) == strict ]] && ls -lOd '$p/.git/config' | grep -q uchg"
  check "git-protect off unlocks, and on locks again" bash -c "cd '$p' && '$D' git-protect off </dev/null >/dev/null 2>&1 &&
    ! ls -lOd '$p/.git/config' | grep -q uchg && ! git config --file .git/config diff.ignoreSubmodules &&
    '$D' git-protect on </dev/null >/dev/null 2>&1 && ls -lOd '$p/.git/config' | grep -q uchg && [[ \$('$D' git-protect 2>/dev/null) == on ]]"
else
  check "git-protect on is refused off macOS" bash -c "cd '$p' && '$D' git-protect on </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_on_needs_macos | head -1)\""
fi
check "rm deletes only this VM, with its state and ssh config entry, and unlocks" bash -c "cd '$p' && '$D' rm </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! grep -q '^Host devvm-web ' '$XDG_CONFIG_HOME/devvm/ssh_config' &&
  ! grep -q '^$vm ' '$kh' && { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$p/.git/config' | grep -q uchg; } &&
  [[ -z \$(ls '$XDG_DATA_HOME/devvm/exports' 2>/dev/null) ]]"

section "agent folders and the VM's setup"
# A home of its own: a fake agent config, history and git identity, and a fake token; nothing of
# yours is read.
ah=$work/agent-home; rm -rf "$ah"; mkdir -p "$ah/.claude/skills/e2e" "$ah/.claude/projects/p"; ah=$(cd "$ah" && pwd -P)
echo '# e2e skill' > "$ah/.claude/skills/e2e/SKILL.md"
echo '{"theme":"e2e-settings"}' > "$ah/.claude/settings.json"
echo '{"prompt":"e2e-history"}' > "$ah/.claude/projects/p/h.jsonl"
printf '[user]\n\tname = E2E User\n\temail = e2e@example.com\n' > "$ah/.gitconfig"
a=$(newproj agents)
(cd "$a" && HOME=$ah TZ=Europe/Paris CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake-plain quiet "$D" --no-network-policy --no-git-protect claude)
vm=$(vm_of "$a"); [[ -z $vm ]] || created+=("$vm")
check "an agent folder is mounted read-only at its host path, and linked into the VM's home" bash -c "
  limactl list --format '{{range .Config.Mounts}}{{.Location}}|{{.Writable}} {{end}}' $vm | grep -qF '$ah/.claude/skills|false ' &&
  ! limactl shell --workdir / $vm sudo touch '$ah/.claude/skills/x' </dev/null &&
  limactl shell --workdir / $vm bash -c 'grep -q e2e ~/.claude/skills/e2e/SKILL.md' </dev/null"
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
  HOME='$ah' '$D' export '$work/export/h.tar.gz' </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  tar -tzf '$work/export/h.tar.gz' > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list'"
check "rm exports its agents' history, without their logins, then deletes it" bash -c "cd '$a' && HOME='$ah' '$D' rm </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && f=\$(ls '$XDG_DATA_HOME'/devvm/exports/$vm-*.tar.gz) && tar -tzf \"\$f\" > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list'"

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
(cd "$g" && as_user quiet "$D" --no-network-policy --no-git-protect --ssh-agent base)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "--ssh-agent: the VM uses your agent's keys" bash -c "
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-user-key ]]"
check "ssh-agent add loads a key into devvm's own agent" bash -c "
  as_user '$D' ssh-agent add '$sa/devvm-key' </dev/null 2>&1 | grep -qF \"\$(m agent_holds keys=devvm-e2e-key)\""
check "ssh-agent devvm, then restart: the VM uses only that key, not yours" bash -c "cd '$g' &&
  as_user '$D' ssh-agent devvm </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "so does an editor connecting by name, whatever agent it has" bash -c "
  [[ \$(as_user ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "ssh-agent off, then restart: no agent in the VM" bash -c "cd '$g' &&
  as_user '$D' ssh-agent off </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  ! as_user limactl shell --workdir / $vm ssh-add -l </dev/null &&
  grep -A8 '^Host .* $vm\$' '$XDG_CONFIG_HOME/devvm/ssh_config' | grep -q '^  ForwardAgent no\$'"
check "rm deletes the SSH agent VM" bash -c "cd '$g' && as_user '$D' rm -f </dev/null >'$work/last.log' 2>&1 && ! limactl list -q | grep -qx $vm"
kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock"
fi

if want clone; then
section "--clone"
# A home of its own: no ~/.ssh/config, so nothing of yours is read or offered the Include line. A
# throwaway repo, with a branch besides main.
ch=$work/clone-home; rm -rf "$ch"; mkdir -p "$ch"
export GIT_CONFIG_GLOBAL=$ch/.gitconfig GIT_CONFIG_NOSYSTEM=1   # host git: none of your settings
c=$(newproj cloned)
git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$c" branch other
(cd "$c" && printf 'pwd; git branch --show-current\n' | HOME=$ch "$D" --clone --no-network-policy base >"$work/last.log" 2>&1)
vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
cgit="GIT_SSH_COMMAND='ssh -F $XDG_CONFIG_HOME/devvm/ssh_config' git -C '$c'"
check "the VM has its own clone, with every branch, only the shared tools mounted, and the shell opens in it" bash -c "
  [[ -n '$vm' && \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$share ' ]] &&
  [[ \$(limactl shell --workdir / $vm git -C '$c' branch --format='%(refname:short)' </dev/null | tr '\n' ' ') == 'main other ' ]] &&
  limactl shell --workdir / $vm git -C '$c' log --oneline </dev/null | grep -q one &&
  grep -qx '$c' '$work/last.log' && grep -qx main '$work/last.log' && grep -qx mode=clone '$vms/$vm'"
check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
check "nothing of .git is locked or watched" bash -c "! grep -q '^git_protect=' '$vms/$vm' &&
  { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$c/.git/config' | grep -q uchg; }"
in_vm "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "rm without a terminal refuses with unfetched commits, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_fetch remote=devvm | head -1)\" '$work/last.log' && limactl list -q | grep -qx $vm"
check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
in_vm "$vm" "cd '$c' && echo c > c" >/dev/null 2>&1
check "then rm refuses with uncommitted changes in the clone, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_delete | head -1)\" '$work/last.log' && grep -qx '?? c' '$work/last.log' && limactl list -q | grep -qx $vm"
check "rm -f deletes it and the remote fully; the fetched work stays as a local branch" bash -c "cd '$c' && HOME='$ch' '$D' rm -f </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! git -C '$c' remote | grep -q devvm &&
  [[ -z \$(git -C '$c' for-each-ref refs/remotes) ]] && git -C '$c' log --oneline saved/devvm/main | grep -q from-vm &&
  grep -qF \"\$(m saved_branches 'branches=\"saved/devvm/main\"' | head -1)\" '$work/last.log' && [[ ! -e '$ch/.ssh/config' ]]"
unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
fi

if want gc; then
section "gc"
# A folder named img-*: its VM is named like an image (devvm-img-keep-<hash>), and isn't one.
g=$(newproj img-keep)
launch "$g" exit --no-network-policy base
kg=$(sed -n 's/^chain=base=\([0-9a-f]*\)$/\1/p' "$vms/$vm" 2>/dev/null)
gfix=$layers/e2egc.sh
printf '#!/bin/bash\necho gc\n' > "$gfix"
check "a fixture layer, edited after its build, is stale" bash -c "'$D' build e2egc </dev/null >'$work/last.log' 2>&1 &&
  echo '# edited' >> '$gfix' && '$D' images </dev/null 2>/dev/null | grep -qE '^e2egc .* stale '"
owned e2egc
ko=$(keys_of e2egc | head -1)
# An old tool image no VM lists: unused only once there's a tools lock.
old=node-0.0.1-000000000000.erofs
[[ ! -f $lock ]] || { : > "$share/$old"; : > "$share/$old.sha256"; }
check "gc deletes the stale image and its record; base and the img-* project VM stay" bash -c "
  [[ -n '$vm' && -n '$kg' && -n '$ko' ]] && '$D' gc </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(status_of devvm-img-$ko) && ! -e '$images/$ko' ]] &&
  [[ \$(status_of devvm-img-$kg) == Stopped && -f '$images/$kg' ]] && [[ \$(status_of $vm) == Running ]]"
if [[ -f $lock ]]; then
  check "gc deletes a tool image no VM uses, with its hash" bash -c "[[ ! -e '$share/$old' && ! -e '$share/$old.sha256' ]]"
else
  skip "gc deletes a tool image no VM uses, with its hash" "no tools lock yet"
fi
check "gc again finds nothing to clean up" bash -c "'$D' gc </dev/null 2>&1 | grep -qxF \"==> \$(m nothing_to_clean)\""
rm -f "$gfix" "$share/$old" "$share/$old.sha256"
(cd "$g" && quiet "$D" rm -f)
fi

if want tools; then
section "shared tools"
t=$(newproj tools); echo '{}' > "$t/package.json"
# A package.json: node, detected. Its tool images stay cached in the share between runs (--clean
# deletes them), as base does.
launch "$t" exit --no-network-policy --no-git-protect
check "a node VM, with node's shared tools in its state and the share" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx image=node '$vms/$vm' && grep -qx libc=gnu '$vms/$vm' &&
  for f in \$(sed -n 's/^tools=//p' '$vms/$vm'); do [[ -f '$share'/\$f && -f '$share'/\$f.sha256 ]] || exit 1; done &&
  grep -q '^tools=node-$(lockv node)-.* pnpm-$(lockv pnpm)-.* bun-$(lockv bun)-' '$vms/$vm'"
check "node, npm, pnpm, bun and yarn work, node at the locked version" in_vm "$vm" \
  "[[ \$(node -v) == v$(lockv node) ]] && npm -v && pnpm -v && bun -v && yarn -v"
check "each tool is a read-only image from the share" in_vm "$vm" \
  "[[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]] && ! touch /opt/devvm/tools/node/$(lockv node)/x 2>/dev/null"
check "the share is read-only even for root" bash -c "limactl shell --workdir / $vm sudo touch /mnt/devvm-tools/x </dev/null 2>&1 | grep -q 'Read-only file system'"
check "npm i -g installs into the VM's home" in_vm "$vm" "npm i -g cowsay@1.6.0 >/dev/null 2>&1 && [[ \$(command -v cowsay) == \$HOME/.npm-global/bin/cowsay ]]"
# ssh with a command runs a non-login, non-interactive shell, as editors and agents' tools do.
check "a non-login shell finds the tools too" bash -c "ssh -F '$LIMA_HOME/$vm/ssh.config' lima-$vm 'node -v' </dev/null"
check "an .nvmrc picks its own version, installed into the VM" in_vm "$vm" "cd $t && echo 22.20.0 > .nvmrc && [[ \$(node -v) == v22.20.0 ]]; r=\$?; rm .nvmrc; exit \$r"
# Started without devvm (so no setup): the boot itself mounts them.
check "the tools are mounted at boot" bash -c "limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  limactl shell --workdir / $vm bash -lc 'node -v' </dev/null | grep -qx v$(lockv node)"
limactl stop "$vm" >/dev/null 2>&1   # two VMs at once starve a nested Linux host
files_before=$(find "$share" -name '*.erofs' | wc -l)
t2=$(newproj tools2); echo '{}' > "$t2/package.json"
launch "$t2" 'node -v' --no-network-policy --no-git-protect
check "a second node VM uses the same tool images: none built" bash -c "
  [[ -n '$vm' ]] && (( \$(find '$share' -name '*.erofs' | wc -l) == $files_before )) && grep -qx v$(lockv node) '$work/last.log'"
check "the tools build left no VM behind" bash -c "[[ -z \$(status_of devvm-tools-build) ]]"
# Upstream can publish mid-run (node's musl builds come days after a release): with the lock
# unchanged, the tools are the newest; changed, the VM may move (if one of its tools did).
check "devvm update: checks upstream; the newest unless the lock changed" bash -c "cd '$t2' &&
  before=\$(cksum < '$lock'); out=\$(printf 'exit\n' | '$D' update 2>&1)
  grep -qF \"\$(m tools_newest vm=$vm | head -1)\" <<<\"\$out\" ||
    { [[ \$(cksum < '$lock') != \"\$before\" ]] && grep -qF \"\$(m tools_switch vm=$vm | head -1)\" <<<\"\$out\"; }"
check "devvm remove takes node out of the VM" bash -c "cd '$t2' && printf 'exit\n' | '$D' remove node >'$work/last.log' 2>&1 &&
  grep -qx image=base '$vms/$vm' && ! grep -q '^tools=.*node' '$vms/$vm' &&
  limactl shell --workdir / $vm bash -lc '! command -v node' </dev/null"
fi

if want network; then
section "network policy"
# A claude+codex VM, so the claude token is bound to it: a fake one in devvm's environment (its
# proxy inherits it), a fake ~/.claude.json with an MCP server's header and a fake Codex login, in a
# home of its own: no real token or login is read.
nh=$work/net-home; rm -rf "$nh"; mkdir -p "$nh/.codex"
printf '%s\n' '{"mcpServers":{"e2e":{"type":"http","url":"https://api.github.com/devvm-e2e-mcp","headers":{"X-Devvm-E2e":"e2efake-mcp"}}}}' > "$nh/.claude.json"
echo '{"OPENAI_API_KEY":"sk-e2efake-codex","last_refresh":null}' > "$nh/.codex/auth.json"
cp "$nh/.claude.json" "$work/claude-before.json"; cp "$nh/.codex/auth.json" "$work/codex-before.json"
n=$(newproj net)
ps_watch
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" --network-policy --no-git-protect claude+codex >"$work/last.log" 2>&1)
ps_unwatch
vm=$(vm_of "$n"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
salt=$(sed -n 's/^proxy_salt=//p' "$vms/$vm" 2>/dev/null)
events=$XDG_DATA_HOME/devvm/logs/$vm.jsonl
# pgrep -f looks for [_]proxy: Linux's pgrep also matches the check's own shell, whose command line
# holds the pattern.
check "--network-policy: the VM runs behind its proxy, and the self-check passed" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ' >/dev/null"
# The control: the launch set the VM's environment (limactl edit, the password's stand-in in the
# run log), and the guest has the password.
check "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it" bash -c "
  key=\$(sed -n 's/^proxy_key=//p' '$vms/$vm') && [[ \${#key} -eq 32 ]] && [[ -s '$work/ps.log' ]] &&
  ! grep -qFf <(echo \"\$key\") '$work/ps.log' && ! ps -axww -o command= | grep -qFf <(echo \"\$key\") &&
  grep -l 'limactl edit .*devvm:devvm-proxy-password@' '$XDG_DATA_HOME'/devvm/logs/runs/*.log >/dev/null &&
  limactl shell --workdir / $vm bash -lc 'printenv HTTPS_PROXY' </dev/null | grep -qF \":\$key@\""
check "ssh devvm-net reaches it, confined, its host key checked" bash -c "
  grep -q '^Host devvm-net $vm\$' '$XDG_CONFIG_HOME/devvm/ssh_config' &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]]"
check "devvm ssh runs the self-check, which passes, and names it" bash -c "cd '$n' &&
  HOME='$nh' CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake '$D' ssh </dev/null >'$work/last.log' 2>&1 && grep -qx net_check=ok '$vms/$vm' &&
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
check "devvm log shows the block and the allowed connection" bash -c "cd '$n' &&
  '$D' log | grep -q '  BLOCKED  example.com:443  ' && '$D' log | grep -q '  allowed  github.com:443  rule '"
check "devvm log --blocked groups it, with the command that allows it" bash -c "cd '$n' &&
  '$D' log --blocked | grep -Eq '^[0-9TZ:-]+ +[0-9]+ +example\.com:443 +devvm policy allow example\.com\$'"
# The command devvm log suggests, run as it is: the same proxy allows the host at once, and after
# devvm policy rm blocks it again.
check "devvm policy allow applies at once to the running VM's proxy, and rm takes it back" bash -c "cd '$n' &&
  pid=\$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') && [[ -n \$pid ]] &&
  cmd=\$('$D' log --blocked | awk '\$3 == \"example.com:443\" { sub(/^.* devvm /, \"\"); print }') &&
  [[ \$cmd == 'policy allow example.com' ]] && '$D' \$cmd </dev/null >/dev/null &&
  '$D' policy | grep -qx '  example.com' &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://example.com' </dev/null &&
  '$D' policy rm example.com >/dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/' </dev/null) == 403 ]] &&
  [[ \$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') == \$pid ]]"
check "DNS fails at once, not after a timeout" in_vm "$vm" 'timeout 3 getent hosts example.com; [ $? -eq 2 ]'
# A service on the host's own loopback (a devvm _proxy, which answers anything with 407): unreachable
# directly, and refused by the VM's proxy (internal addresses need a rule naming them).
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
sleep 1
check "the host's loopback services are unreachable, directly or through the proxy" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18765/) == 407 ]] &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null http://192.168.5.2:18765/ </dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} http://192.168.5.2:18765/' </dev/null) == 403 ]] &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} --noproxy \"\" -x \"\$HTTP_PROXY\" http://127.0.0.1:18765/' </dev/null) == 403 ]]"
kill "$listener" 2>/dev/null; listener=""
check "the proxy refuses anything on the host without the VM's password" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:$pp http://devvm.proxy/blocked) == 407 ]]"
check "the token never reaches the VM" bash -c "
  ! limactl shell --workdir / $vm sudo grep -rqs -e e2efake /etc /home /root </dev/null &&
  ! limactl shell --workdir / $vm bash -lc 'env' </dev/null | grep -q e2efake"
check "devvm's CA is installed in the VM" bash -c "
  limactl shell --workdir / $vm cat /usr/local/share/ca-certificates/devvm-proxy.crt </dev/null | cmp -s - '$XDG_DATA_HOME/devvm/ca/ca.pem'"
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
check "devvm secret set saves a token, and devvm reads it back, never to a terminal" bash -c "
  echo ghp_E2EFAKE | HOME='$nh' '$D' secret set github --stdin >/dev/null && [[ \$(HOME='$nh' '$D' secret get github | cat) == ghp_E2EFAKE ]] &&
  { [[ \$(uname -s) != Darwin ]] || [[ \$(security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github -w) == ghp_E2EFAKE ]]; }"
check "secret use github: its proxy gets the binding at once, the VM its own placeholder on its next launch" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret use github >/dev/null && grep -q '\"name\":\"github\"' '$XDG_DATA_HOME/devvm/run/$vm.secrets.json' &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'echo \$GH_TOKEN' </dev/null) == devvm-proxy-managed-github-$salt ]] &&
  ! limactl shell --workdir / $vm bash -lc 'env; cat ~/.config/devvm/env' </dev/null | grep -q ghp_E2EFAKE &&
  HOME='$nh' '$D' secret ls | grep -Eq '^github +yes +$vm\$'"
check "the proxy puts the GitHub token in for its host" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.github.com/user -H \"Authorization: token \$GH_TOKEN\"' </dev/null &&
  grep '\"path\":\"/user\"' '$events' | grep -q '\"secrets\":\[\"github\"\]'"
check "secret unuse and rm take the binding and the token away at once" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret unuse github >/dev/null && ! grep -q '\"name\":\"github\"' '$XDG_DATA_HOME/devvm/run/$vm.secrets.json' &&
  HOME='$nh' '$D' secret rm github >/dev/null && ! HOME='$nh' '$D' secret get github >/dev/null 2>&1 &&
  { [[ \$(uname -s) != Darwin ]] || ! security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github >/dev/null 2>&1; }"
check "the self-check passes on the VM devvm started confined" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT= CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck'"
check "a VM started outside devvm is unenforced, and the self-check stops it" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT='net_unenforced_stop vm=$vm' CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck' && [[ \$(status_of '$vm') == Stopped ]] && grep -qx net_check=unenforced '$vms/$vm'"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$n' && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(status_of '$vm') == Running ]] && grep -qx net_check=ok '$vms/$vm'"
check "devvm stop stops it and its proxy at once, and devvm starts both again" bash -c "cd '$n' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ \$(status_of '$vm') == Stopped ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$(status_of '$vm') == Running ]] &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && grep -qx net_check=ok '$vms/$vm'"
check "ls shows its policy on, and unenforced once started outside devvm" bash -c "
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +on +lima ' &&
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +unenforced +lima '"
check "started outside devvm again: without a terminal, devvm stops it and opens nothing" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! printf 'echo e2e-opened\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -q e2e-opened '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
check "so does devvm ssh, which names nothing" bash -c "
  limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! HOME='$nh' '$D' ssh </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -qF \"\$(m ssh_host_info dir=$n host=devvm-net | head -1)\" '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
# Stopped by the self-check, it's still unenforced: its last self-check failed.
check "devvm network off: its next start is direct, without a proxy" bash -c "cd '$n' &&
  [[ \$('$D' network) == unenforced ]] && '$D' network off </dev/null >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$('$D' network) == off ]] &&
  [[ \$(status_of '$vm') == Running ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  limactl shell --workdir / $vm curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1 </dev/null"
check "devvm network on while it runs: unenforced until devvm restart starts it confined" bash -c "cd '$n' &&
  '$D' network on </dev/null >/dev/null 2>&1 && [[ \$('$D' network) == unenforced ]] &&
  printf 'exit\n' | HOME='$nh' '$D' restart >'$work/last.log' 2>&1 && [[ \$('$D' network) == on ]] &&
  grep -qx net_check=ok '$vms/$vm' && pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "devvm network off while it runs confined: launched, it keeps its proxy until it next starts" bash -c "cd '$n' &&
  '$D' network off </dev/null >/dev/null && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m net_off_next_start vm=$vm | head -1)\" '$work/last.log' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "rm -f deletes it, its proxy and its state" bash -c "cd '$n' && HOME='$nh' '$D' rm -f </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
fi

if want from; then
section "--from"
# Registry images need no Docker: these run with a docker that's installed but not running (it
# logs what devvm asked it, which may only be whether it has the image).
mkdir -p "$work/nodocker"; : > "$work/docker-calls"
printf '#!/bin/sh\necho "docker $*" >> "%s"\necho "Cannot connect to the Docker daemon" >&2; exit 1\n' \
  "$work/docker-calls" > "$work/nodocker/docker"
chmod 755 "$work/nodocker/docker"
nd() { PATH="$work/nodocker:$PATH" "$@"; }
[[ -f $lock ]] || quiet "$D" _refresh-tools   # the tools' versions, for lockv
# from_record REF: base's record for that --from.
from_record() { grep -lx "from=$1" "$images"/* 2>/dev/null | xargs grep -lx name=base 2>/dev/null | head -1; }
export -f from_record

f=$(newproj fromdocker); echo '{}' > "$f/package.json"
fh=$work/from-home; mkdir -p "$fh"   # the claude VM's: no agent config of yours
HOME=$fh nd launch "$f" exit --from node:24 --no-network-policy node+python+claude   # node:24 is Debian 12
check "a node+python+claude VM from node:24 (Debian 12) boots, its base keyed by the registry's digest" bash -c "
  [[ -n '$vm' ]] && limactl shell --workdir / '$vm' cat /etc/os-release </dev/null | grep -qx VERSION_CODENAME=bookworm &&
  grep -qx from=node:24 '$vms/$vm' && grep -qx libc=gnu '$vms/$vm' &&
  grep -qE '^digest=node@sha256:[0-9a-f]{64}$' \"\$(from_record node:24)\""
check "its node, claude and every feature work" in_vm "$vm" \
  'node --version && claude --version && eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
check "python, python3 and claude are the locked versions" in_vm "$vm" \
  "[[ \$(python --version) == 'Python $(lockv python)' && \$(python3 --version) == 'Python $(lockv python)' && \$(claude --version) == '$(lockv claude) '* ]]"
check "an interactive shell starts without errors" bash -c "out=\$(limactl shell --workdir / $vm bash -ic true </dev/null 2>&1) && ! grep -iE 'error|unknown option|not found' <<<\"\$out\""
(cd "$f" && HOME=$fh quiet "$D" rm -f)

# Alpine: apk, OpenRC and musl, so its own (musl) tool images.
al=$(newproj fromalpine); echo '{}' > "$al/package.json"
nd launch "$al" exit --from alpine:3 --no-network-policy
check "an Alpine VM runs node from musl tool images" bash -c "[[ -n '$vm' ]] && grep -qx libc=musl '$vms/$vm' && grep -qx libc=musl \"\$(from_record alpine:3)\""
check "and node, pnpm work there" in_vm "$vm" \
  "grep -q '^ID=alpine' /etc/os-release && [[ \$(node -v) == v$(lockv node) ]] && node -e 'require(\"os\")' && pnpm -v"
check "Docker runs in it (OpenRC)" in_vm "$vm" 'sudo docker run --rm hello-world | grep -q Hello'
check "and every feature" in_vm "$vm" 'eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
# As root: syslog's and acpid's pid files are root-only, so a user's rc-status calls them crashed.
check "no crashed services" in_vm "$vm" '[[ -z "$(sudo rc-status --crashed 2>/dev/null)" ]]'
(cd "$al" && quiet "$D" rm -f)

# Ubuntu's image has empty package lists, where apt 3.2 needs a second update to reach the snapshot.
check "base builds from ubuntu:26.04" nd quiet "$D" --from ubuntu:26.04 build base
check "none of them needed Docker" bash -c "! grep -v '^docker image inspect ' '$work/docker-calls'"
check "and the build VM and its temporary folders are gone" bash -c "
  ! limactl list -q 2>/dev/null | grep -qx devvm-oci-build && ! ls -d \"\${TMPDIR:-/tmp}\"/devvm-oci.* 2>/dev/null"
# An image only Docker has (built, never pushed) isn't checked: making one would change Docker's images.
fi

if want smolvm; then
# The smolvm backend (experimental): its native features, and a one-line refusal for the rest. Its
# machines share your smolvm on macOS: only those with the sandbox's label are looked at, and the
# others must be the same at the end.
S=(--backend smolvm)
me=$(id -un)
others_before=$(smol_others)
# in_smol VM CMD: CMD in a login shell as the VM's user, as devvm's shell has it.
in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -u "$me" -e "HOME=/home/$me.guest" -e "USER=$me" -- bash -lc "$2" </dev/null; }
# smol_home DIR: a home of a check's own that still has your smolvm's machines: on macOS smolvm keeps
# them under HOME, so its folders there are links to yours (rm -rf of DIR removes only the links).
smol_home() { mkdir -p "$1/Library/Application Support" "$1/Library/Caches"
  [[ "$(uname -s)" != Darwin ]] || for d in "Library/Application Support/smolvm" "Library/Caches/smolvm"; do
    [[ -e "$1/$d" ]] || ln -s "$HOME/$d" "$1/$d"; done; }
# smol_pid VM: the host process that runs it (the first line of its agent.pid).
smol_pid() { head -1 "$(SMOLVM_USAGE=0 smolvm machine data-dir --name "$1")/agent.pid" 2>/dev/null; }
export -f smol_pid
# smol_nameserver VM: the resolver the VM's resolv.conf names. smolvm's own network without a port
# forward is TSI, where the VM asks this machine's resolver itself; any forward, or an allow list, is
# virtio-net, where it asks smolvm's (the gateway, 100.96.0.1). design.md's DNS detection rests on it.
smol_nameserver() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf </dev/null; }
# smol_tsi VM: it asks this machine's first resolver (not the gateway).
smol_tsi() { local ns host; ns=$(smol_nameserver "$1"); host=$(awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf)
  echo "VM's: ${ns:-none}, this machine's: ${host:-none}"; [[ -n $ns && $ns != 100.96.0.1 && $ns == "$host" ]]; }
smol_vnet() { local ns; ns=$(smol_nameserver "$1"); echo "VM's: ${ns:-none}"; [[ $ns == 100.96.0.1 ]]; }
# smol_listens_only VM PORT...: the host process that runs it listens on each PORT (the control:
# what lsof sees), and on no other TCP port.
smol_listens_only() { local vm=$1 pid l p re=""; shift
  pid=$(smol_pid "$vm"); [[ $pid =~ ^[0-9]+$ ]] || { echo "no pid"; return 1; }
  l=$(lsof -nP -a -p "$pid" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u)
  echo "listening: ${l:-nothing}; want $*"
  for p in "$@"; do [[ $p =~ ^[0-9]+$ ]] && grep -q ":$p\$" <<<"$l" || return 1; re+="${re:+|}:$p\$"; done
  ! grep -qvE "$re" <<<"$l"; }
section "smolvm: shared tool versions"
check "the shared tools resolve to upstream's newest" quiet "$D" _refresh-tools

section "smolvm: images"
check "devvm doctor finds everything smolvm needs" bash -c "'$D' doctor --backend smolvm </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$('$D' doctor --backend smolvm --fixes </dev/null) ]]"
check "build base" quiet env DEVVM_MESSAGES=names "$D" "${S[@]}" build base
kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
# A cached base: the pack recorded as base.
[[ -n $kb ]] || kb=$(for k in "$XDG_DATA_HOME"/devvm/smolvm/images/*; do
  k=$(basename "$k"); grep -qx name=base "$images/$k" 2>/dev/null && echo "$k"; done | head -1)
packs=$XDG_DATA_HOME/devvm/smolvm/images
check "base is a pack, recorded with Ubuntu's container image by digest; its build machine is gone" bash -c "
  [[ -n '$kb' && -f '$packs/$kb/image.smolmachine' ]] && grep -qx libc=gnu '$images/$kb' &&
  grep -qE '^digest=docker\.io/library/ubuntu:[0-9.]+@sha256:[0-9a-f]{64}$' '$images/$kb' && [[ -z \$(smol_status devvm-img-$kb) ]]"
fixture=$layers/e2esmol.sh
token=e2e-smolvm-ran-$$-$(date +%s)
printf '#!/bin/bash\necho %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" > "$fixture"
check "a layer builds from base's pack, its script run as the VM's user" bash -c "'$D' ${S[*]} build e2esmol </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2esmol
kf=$(keys_of e2esmol | head -1)
check "it's a pack too, and devvm images lists both on smolvm" bash -c "[[ -n '$kf' && -f '$packs/$kf/image.smolmachine' ]] &&
  out=\$('$D' images </dev/null 2>/dev/null) && grep -qE '^base +ubuntu [0-9.]+ +[0-9a-f]{12} +$kb +current +[0-9.]+[KMGT] +smolvm$' <<<\"\$out\" &&
  grep -qE '^e2esmol +ubuntu [0-9.]+ +[0-9a-f]{12} +$kf +current +[0-9.]+[KMGT] +smolvm$' <<<\"\$out\""
echo "# edited" >> "$fixture"
check "rebuild makes a new one and deletes the stale one, not base" bash -c "'$D' ${S[*]} rebuild e2esmol </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2esmol | wc -l) -eq 1 && \$(keys_of e2esmol) != $kf && ! -e '$packs/$kf' && -f '$packs/$kb/image.smolmachine' ]]"
owned e2esmol
check "images rm deletes it" bash -c "'$D' ${S[*]} images rm e2esmol </dev/null >/dev/null 2>&1 && [[ -z \$(keys_of e2esmol) ]]"
rm -f "$fixture"
printf '#!/bin/bash\nexit 3\n' > "$layers/e2esmolfail.sh"
DEVVM_MESSAGES=names "$D" "${S[@]}" build e2esmolfail </dev/null >"$work/fail.log" 2>&1; rc=$?
kx=$(sed -n 's/^==> building_image image=e2esmolfail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
[[ -z $kx ]] || created+=("devvm-img-$kx")
check "a failed script keeps its machine, stopped, and says how to look inside" bash -c "
  [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: smolvm_script_failed script=e2esmolfail\.sh vm=devvm-img-$kx$' '$work/fail.log' &&
  [[ \$(smol_status devvm-img-$kx) == stopped ]] && '$D' ${S[*]} images rm '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(smol_status devvm-img-$kx) ]]"
rm -f "$layers/e2esmolfail.sh"

section "smolvm: a base project"
p=$(newproj smolweb); git -C "$p" init -q
launch "$p" 'pwd; id -un; echo e2e-shell-ran' "${S[@]}" --no-network-policy --ports 47180 --cpus 2 --memory 3 --disk 120 base
check "devvm creates the VM from base's pack, labelled, and opens a shell in the folder as you" bash -c "
  [[ -n '$vm' && \$(smol_status '$vm') == running ]] && grep -qx '$p' '$work/last.log' && grep -qx '$me' '$work/last.log' &&
  grep -qx e2e-shell-ran '$work/last.log' && grep -qx backend=smolvm '$vms/$vm' && grep -qx net=off '$vms/$vm'"
check "it gets --cpus, --memory and --disk" in_smol "$vm" \
  '[[ $(nproc) == 2 ]] && (( $(awk "/^MemTotal:/ { print int(\$2 / 1048576 + 0.5) }" /proc/meminfo) == 3 )) && (( $(df -BG --output=size / | tail -1 | tr -dc 0-9) >= 115 ))'
check "ls lists it on smolvm" bash -c "'$D' ls 2>/dev/null | grep -Eq '^$vm +Running +base +\S+ +off +smolvm +$p\$'"
check "the folder is mounted read-write, and what the VM writes is yours" bash -c "
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- bash -c \"\$1\" </dev/null; }
  in_smol 'echo from-vm > $p/vm.txt' && grep -qx from-vm '$p/vm.txt' && [[ \$(stat -f %Su '$p/vm.txt' 2>/dev/null || stat -c %U '$p/vm.txt') == '$me' ]]"
check "the shared tools are read-only, even for root" bash -c "! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- touch /mnt/devvm-tools/x </dev/null 2>/dev/null"
check "hostname is the folder" in_smol "$vm" '[[ $(hostname) == smolweb ]]'
check "Exec's arguments, folder, input and user reach the VM as given" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2EExecQuoting\$' ./internal/backend/smolvm |
  grep -q -- '--- PASS: TestE2EExecQuoting'"
check "Docker runs in it, started by devvm" in_smol "$vm" 'for i in $(seq 30); do sudo docker info >/dev/null 2>&1 && break; sleep 1; done; sudo docker run --rm hello-world | grep -q Hello'
check "SSH agent forwarding is off" in_smol "$vm" '[[ -z ${SSH_AUTH_SOCK:-} ]]'
# A listener on every address (Ubuntu's container image has perl, not python), left running.
SMOLVM_USAGE=0 smolvm machine exec --name "$vm" -d -- perl -MIO::Socket::INET -e \
  '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:47180", Listen => 5, ReuseAddr => 1) or die; while ($c = $s->accept) { close $c }' \
  </dev/null >/dev/null 2>&1
check "the listed port reaches this machine's loopback" bash -c 'for i in $(seq 20); do nc -z 127.0.0.1 47180 && exit 0; sleep 1; done; exit 1'
check "turning agent forwarding on is refused in one line" bash -c "cd '$p' &&
  ! DEVVM_MESSAGES=names '$D' ssh-agent on </dev/null 2>'$work/e' && grep -qx 'devvm: backend_lacks backend=smolvm feature=agent-toggle vm=$vm' '$work/e' &&
  grep -qx net=off '$vms/$vm'"
check "its process listens on its listed port (the control) and its ssh forward's, nothing else" smol_listens_only "$vm" 47180 "$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json")"
check "a running VM's new size and ports wait for its next start, and apply on restart" bash -c "cd '$p' &&
  '$D' size --cpus 3 </dev/null >/dev/null 2>&1 && '$D' ports add 47181 </dev/null >/dev/null 2>&1 &&
  printf 'nproc\n' | '$D' restart >'$work/last.log' 2>&1 && grep -qx 3 '$work/last.log' &&
  [[ \$('$D' ports 2>/dev/null) == 47180,47181 ]]"
check "a smaller disk is refused" bash -c "cd '$p' && ! DEVVM_MESSAGES=names '$D' size --disk 100 </dev/null >'$work/last.log' 2>&1 &&
  grep -q 'disk_shrink' '$work/last.log'"
check "devvm stop stops it, and devvm starts it again" bash -c "cd '$p' && '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ \$(smol_status $vm) == stopped ]] && printf 'echo again\n' | '$D' 2>&1 | grep -qx again && [[ \$(smol_status $vm) == running ]]"
check "rm deletes only this VM, with its state" bash -c "cd '$p' && '$D' rm -f </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status $vm) && ! -e '$vms/$vm' && ! -e '$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json' ]]"

section "smolvm: SSH agent forwarding"
g=$(newproj smolagent)
user_agent_pid=$(ssh-agent -a "$work/smol-agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
rm -f "$work/smol-key" "$work/smol-key.pub"; ssh-keygen -q -t ed25519 -N '' -C e2e-smolvm -f "$work/smol-key"
SSH_AUTH_SOCK=$work/smol-agent.sock ssh-add -q "$work/smol-key" 2>/dev/null
(cd "$g" && printf 'ssh-add -l\n' | SSH_AUTH_SOCK=$work/smol-agent.sock "$D" "${S[@]}" --no-network-policy --ports none --ssh-agent base >"$work/last.log" 2>&1)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "--ssh-agent: the VM uses the agent it was started with" bash -c "[[ -n '$vm' ]] && grep -q e2e-smolvm '$work/last.log'"
check "with its ssh forward, even with no ports of its own, smolvm's own network is virtio-net: the gateway's resolver" smol_vnet "$vm"
check "turning it off is refused" bash -c "cd '$g' && ! DEVVM_MESSAGES=names '$D' ssh-agent off </dev/null 2>'$work/e' &&
  grep -qx 'devvm: backend_lacks backend=smolvm feature=agent-toggle vm=$vm' '$work/e'"
(cd "$g" && quiet "$D" rm -f)
kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
rm -f "$work/smol-key" "$work/smol-key.pub" "$work/smol-agent.sock"

section "smolvm: ssh for editors"
# The emulation's guarantee (reviewed): each VM's own host keys, pinned; the VM's own key the only
# login, as the VM's user; no forwarding that carries the VM's connections out through this machine
# that devvm's entry lets through; agent forwarding as the entry says.
# An ~/.ssh/config of the section's own, set up as the docs say, with a Host * that forwards the agent:
# yours is never read or changed; ssh gets no agent but a throwaway one; a stub code.
UR=(env)   # an unreviewed emulation would add DEVVM_UNREVIEWED=backend:feature here
sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc/home" "$sc/bin"; smol_home "$sc/home"
dsc=$XDG_CONFIG_HOME/devvm/ssh_config kh=$XDG_CONFIG_HOME/devvm/known_hosts
printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$dsc" > "$sc/config"
agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
printf '#!/bin/sh\necho "$*" > %s/code-args\n' "$sc" > "$sc/bin/code"; chmod +x "$sc/bin/code"
# sshs ARGS...: ssh through the section's config, without an agent. smolssh_port VM: the port its
# entry names. smolssh_key VM: its pinned host key.
sshs() { env -u SSH_AUTH_SOCK ssh -F "$sc/config" "$@" </dev/null; }
smolssh_port() { awk -v h="$1" '$1 == "Host" { on = 0; for (i = 2; i <= NF; i++) if ($i == h) on = 1 } on && $1 == "Port" { print $2; exit }' "$dsc"; }
smolssh_key() { awk -v n="$1" '$1 == n { print $2, $3 }' "$kh"; }
# smolssh_loopback PORT: something listens on it, on the host's loopback alone.
smolssh_loopback() { local l; l=$(lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u)
  echo "$l"; [[ -n $l ]] && ! grep -vxE "127\.0\.0\.1:$1|\[::1\]:$1" <<<"$l"; }
# smolssh_pinned VM: the pinned key is the VM's own, its only one, ed25519.
smolssh_pinned() { local vmk
  vmk=$(in_smol "$1" 'cat /etc/ssh/ssh_host_*_key.pub' | awk '{ print $1, $2 }')
  echo "the VM's: $vmk"; echo "pinned: $(smolssh_key "$1")"
  [[ -n $vmk && $vmk == "$(smolssh_key "$1")" && $vmk == "ssh-ed25519 "* ]]; }
# smolssh_sshd VM: sshd's settings as it runs them (sshd -T on its config), as decided.
smolssh_sshd() { local t
  t=$(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- /usr/sbin/sshd -T -f /etc/devvm/ssh/sshd_config </dev/null) || return 1
  echo "$t" | grep -E '^(listenaddress|allowusers|permitrootlogin|authenticationmethods|passwordauthentication|kbdinteractiveauthentication|allowtcpforwarding|allowstreamlocalforwarding|permitlisten|gatewayports|x11forwarding|permittunnel|permituserenvironment|allowagentforwarding) '
  for want in 'listenaddress 0.0.0.0:22' "allowusers $me" 'permitrootlogin no' 'authenticationmethods publickey' \
    'passwordauthentication no' 'kbdinteractiveauthentication no' 'allowtcpforwarding local' 'allowstreamlocalforwarding local' \
    'permitlisten none' 'gatewayports no' 'x11forwarding no' 'permittunnel no' 'permituserenvironment no' 'allowagentforwarding yes'; do
    grep -qx "$want" <<<"$t" || { echo "not $want"; return 1; }
  done
  [[ $(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- stat -c '%U %a' /etc/devvm/ssh/sshd_config </dev/null) == 'root 600' ]]; }
# smolssh_nopass HOST: password and keyboard-interactive logins are refused, publickey alone offered.
smolssh_nopass() { local out r
  out=$(env -u SSH_AUTH_SOCK ssh -v -F "$sc/config" -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive \
    -o BatchMode=yes "$1" true </dev/null 2>&1); r=$?
  grep -E 'Authentications that can continue|denied' <<<"$out"; echo "rc=$r"
  (( r == 255 )) && grep -qE 'Authentications that can continue: publickey[[:space:]]*$' <<<"$out" &&
  ! grep -qE 'Authentications that can continue: .*(password|keyboard)' <<<"$out"; }
# smolssh_stopped VM DIR: devvm stop drops its entry, and nothing listens on its port after.
smolssh_stopped() { local p
  p=$(smolssh_port "$1"); [[ -n $p ]] || { echo "no port"; return 1; }
  (cd "$2" && "$D" stop </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  ! grep -q "$1" "$dsc" || { echo "still in the ssh config"; return 1; }
  for _ in 1 2 3 4 5; do nc -z 127.0.0.1 "$p" 2>/dev/null || return 0; sleep 1; done
  echo "port $p still listening"; return 1; }
# smolssh_onlykey VM HOST: another key, root and another user are refused; devvm's logs in (the control).
smolssh_onlykey() {
  rm -f "$sc/other" "$sc/other.pub"; ssh-keygen -q -t ed25519 -N '' -f "$sc/other"
  ! env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile="$kh" -o HostKeyAlias="$1" -o StrictHostKeyChecking=yes \
    -o BatchMode=yes -o IdentitiesOnly=yes -i "$sc/other" -p "$(smolssh_port "$1")" "$me@127.0.0.1" true </dev/null &&
  ! sshs -o BatchMode=yes -l root "$2" true && ! sshs -o BatchMode=yes -l nobody "$2" true && sshs -o BatchMode=yes "$2" true; }
# smolssh_forwarding HOST: devvm's sshd refuses remote forwarding (a port, a socket); a local forward,
# as editors use, works (the control).
smolssh_forwarding() {
  ! sshs -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 "$1" true &&
  ! sshs -o ExitOnForwardFailure=yes -R "/tmp/devvm-e2e-$$.sock:$sc/agent.sock" "$1" true &&
  [[ $(sshs -W 127.0.0.1:22 "$1" | head -c 4) == SSH- ]]; }
# smolssh_hostile VM HOST: root in the VM rewrites its sshd's config to allow remote forwarding, X11
# and tunnels (with xauth for X11), and restarts it. Through a config of the check's own whose Host *
# asks for X11 and to add keys to the agent, the session gets no display and the agent no key; the
# same asked on the command line gets both (the controls), and ssh -R now works (not covered: the
# entry can't refuse it). Then the agent is emptied; the next start restores sshd.
smolssh_hostile() { local hc=$sc/hostile-config d keys
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n' "$dsc" > "$hc"
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c 'apt-get update -q >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends xauth >/dev/null 2>&1 &&
    c=/etc/devvm/ssh/sshd_config && sed -i -e "s/^AllowTcpForwarding .*/AllowTcpForwarding yes/" -e "s/^AllowStreamLocalForwarding .*/AllowStreamLocalForwarding yes/" \
      -e "s/^X11Forwarding .*/X11Forwarding yes/" -e "s/^PermitTunnel .*/PermitTunnel yes/" -e "s/^PermitListen .*/PermitListen any/" "$c" &&
    kill "$(cat /run/devvm-sshd.pid)" && sleep 1 && /usr/sbin/sshd -f "$c" -E /var/log/devvm-sshd.log' </dev/null || { echo "no hostile sshd"; return 1; }
  sshh() { DISPLAY=:9 SSH_AUTH_SOCK=$sc/agent.sock ssh -F "$hc" -o BatchMode=yes "$@" </dev/null 2>/dev/null; }
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  d=$(sshh "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "through Host *: display $d, agent ${keys:-empty}"
  [[ $d == x=none && -z $keys ]] || return 1
  d=$(sshh -Y -o AddKeysToAgent=yes "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "asked on the command line: display $d, agent ${keys:-empty}"
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  [[ $d != x=none && -n $keys ]] || return 1
  sshh -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 "$2" true || { echo "ssh -R refused by the hostile sshd"; return 1; }; }
# smolssh_squatted VM DIR HOST: its recorded port, taken by another program while it's stopped: its
# next start forwards another, and ssh reaches it.
smolssh_squatted() { local p n sq
  p=$(smolssh_port "$1"); [[ -n $p ]] || { echo "no port"; return 1; }
  (cd "$2" && "$D" stop </dev/null >/dev/null 2>&1) || return 1
  perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:" . shift, Listen => 5, ReuseAddr => 1) or die; sleep 120' "$p" &
  sq=$!; sleep 0.5
  (cd "$2" && printf 'exit\n' | "${UR[@]}" "$D" >"$work/last.log" 2>&1); n=$(smolssh_port "$1")
  echo "squatted $p, now ${n:-none}"
  sshs -o BatchMode=yes "$3" true; local r=$?
  kill "$sq" 2>/dev/null; wait "$sq" 2>/dev/null
  [[ -n $n && $n != "$p" ]] && (( r == 0 )); }
# smolssh_nosshd VM DIR: root moves sshd away; devvm's next start warns smolvm_no_sshd, the VM runs
# with no entry, and devvm ssh says so; sshd put back, a start has ssh again (the control).
smolssh_nosshd() {
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- mv /usr/sbin/sshd /usr/sbin/sshd.away </dev/null || return 1
  (cd "$2" && printf 'exit\n' | "${UR[@]}" DEVVM_MESSAGES=names "$D" restart >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  if ! grep -qx "devvm: smolvm_no_sshd vm=$1" "$work/last.log" || [[ $(smol_status "$1") != running ]] || grep -q "$1" "$dsc"; then
    cat "$work/last.log"; return 1
  fi
  if (cd "$2" && "${UR[@]}" DEVVM_MESSAGES=names "$D" ssh </dev/null 2>"$work/e") || ! grep -qx "devvm: smolvm_no_sshd vm=$1" "$work/e"; then
    cat "$work/e"; return 1
  fi
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- mv /usr/sbin/sshd.away /usr/sbin/sshd </dev/null || return 1
  (cd "$2" && printf 'exit\n' | "${UR[@]}" "$D" restart >"$work/last.log" 2>&1) && grep -q "$1" "$dsc"; }
# smolssh_otherkey VM OTHER: OTHER's key doesn't log in to VM; VM's own does, the same way (the control).
smolssh_otherkey() {
  smolssh_with_key() { env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile="$kh" -o HostKeyAlias="$1" -o StrictHostKeyChecking=yes \
      -o BatchMode=yes -o IdentitiesOnly=yes -i "$XDG_DATA_HOME/devvm/smolvm/vms/$2.key" -p "$(smolssh_port "$1")" "$me@127.0.0.1" true </dev/null 2>/dev/null; }
  if smolssh_with_key "$1" "$2"; then echo "$2's key logged in to $1"; return 1; fi
  smolssh_with_key "$1" "$1" || { echo "its own key didn't"; return 1; }; }
q=$(newproj smolssh); git -C "$q" init -q
(cd "$q" && printf 'exit\n' | HOME=$sc/home "${UR[@]}" DEVVM_MESSAGES=names "$D" "${S[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm=$(vm_of "$q"); [[ -z $vm ]] || created+=("$vm")
check "the ssh config lists it by the folder's name, its sshd forwarded from the host's loopback" bash -c "
  [[ -n '$vm' ]] && ! grep -q 'backend_unreviewed' '$work/last.log' &&
  grep -q '^Host devvm-smolssh $vm\$' '$dsc'"
check "... on the host's loopback alone" smolssh_loopback "$(smolssh_port "$vm")"
check "with its ssh forward, smolvm's own network is virtio-net: the VM asks the gateway's resolver" smol_vnet "$vm"
check "its host key is pinned, as the VM has it: ed25519, its only one" smolssh_pinned "$vm"
check "ssh devvm-smolssh reaches it, as the VM's user, with the environment devvm's commands get" bash -c "
  out=\$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh 'id -un; hostname; echo \$BASH_ENV' </dev/null) &&
  [[ \$out == '$me'\$'\n'smolssh\$'\n'/etc/profile.d/devvm-tools.sh ]] || { echo \"\$out\"; false; }"
check "agent forwarding stays off, whatever Host * says" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -F '$sc/config' devvm-smolssh 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "... while ssh -A forwards it (the control)" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -A -F '$sc/config' devvm-smolssh 'test -S \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "a changed host key is refused" bash -c "
  cp '$kh' '$sc/kh' && ssh-keygen -q -t ed25519 -N '' -f '$sc/changed' &&
  { grep -v '^$vm ' '$sc/kh' || true; echo \"$vm \$(cut -d' ' -f1,2 '$sc/changed.pub')\"; } > '$kh' &&
  { env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh true </dev/null 2>'$sc/err'; r=\$?; cp '$sc/kh' '$kh'; } &&
  (( r == 255 )) && grep -q 'Host key verification failed' '$sc/err'"
check "password and keyboard-interactive logins are refused: publickey alone is offered" smolssh_nopass devvm-smolssh
check "only devvm's key logs in, as the VM's user: another key, root and another user are refused" smolssh_onlykey "$vm" devvm-smolssh
check "devvm's sshd refuses remote forwarding (a port, a socket); a local forward, as editors use, works" smolssh_forwarding devvm-smolssh
check "sshd runs devvm's settings, and its config is root's alone" smolssh_sshd "$vm"
check "an sshd root made hostile gets no display and no key through a Host * asking for them; asked on the command line, both (the controls), and ssh -R" smolssh_hostile "$vm" devvm-smolssh
check "devvm ssh names it; devvm code opens it over Remote-SSH (a stub code)" bash -c "cd '$q' &&
  HOME='$sc/home' ${UR[*]} '$D' ssh </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m ssh_host_info dir=$q host=devvm-smolssh | head -1)\" '$work/last.log' &&
  HOME='$sc/home' PATH='$sc/bin':\$PATH ${UR[*]} '$D' code </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(cat '$sc/code-args') == '--remote ssh-remote+devvm-smolssh $q' ]]"
k1=$(smolssh_key "$vm")
check "restart: new host keys, pinned again, and ssh reaches it" bash -c "cd '$q' &&
  printf 'exit\n' | ${UR[*]} '$D' restart >'$work/last.log' 2>&1 && [[ \$(smol_status '$vm') == running ]] &&
  k=\$(awk -v n='$vm' '\$1 == n { print \$2, \$3 }' '$kh') && [[ -n \$k && \$k != '$k1' ]] &&
  env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh true </dev/null"
q2=$(newproj smolssh2)
(cd "$q2" && printf 'exit\n' | "${UR[@]}" "$D" "${S[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm2=$(vm_of "$q2"); [[ -z $vm2 ]] || created+=("$vm2")
check "a VM made from the same image has host keys of its own, and a port of its own" bash -c "
  [[ -n '$vm2' ]] && a=\$(awk -v n='$vm' '\$1 == n { print \$3 }' '$kh') && b=\$(awk -v n='$vm2' '\$1 == n { print \$3 }' '$kh') &&
  [[ -n \$a && -n \$b && \$a != \$b ]] && env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh2 true </dev/null"
check "... and a key of its own: its key doesn't log in to the other VM, the other's own does (the control)" smolssh_otherkey "$vm" "$vm2"
check "an image without sshd (root removed it): the next start runs without ssh, warned, and the VM runs" smolssh_nosshd "$vm2" "$q2"
(cd "$q2" && quiet "${UR[@]}" "$D" rm -f)
check "its port taken while it's stopped: its next start forwards another, and ssh reaches it" smolssh_squatted "$vm" "$q" devvm-smolssh
check "devvm stop drops its entry, and nothing listens on its port" smolssh_stopped "$vm" "$q"
check "rm deletes it, its entry and its pinned keys" bash -c "cd '$q' && '$D' rm -f </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') ]] && ! grep -q '$vm' '$dsc' && ! grep -q '^$vm ' '$kh'"
kill "$agent_pid" 2>/dev/null; agent_pid=""

section "smolvm: --clone"
# A home of its own: no ~/.ssh/config, so nothing of yours is read or offered the Include line. A
# throwaway repo, with a branch besides main and a file it doesn't track.
ch=$work/clone-home; rm -rf "$ch"; mkdir -p "$ch"; smol_home "$ch"
export GIT_CONFIG_GLOBAL=$ch/.gitconfig GIT_CONFIG_NOSYSTEM=1   # host git: none of your settings
c=$(newproj smolcloned)
git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$c" branch other && echo host-only > "$c/untracked"
(cd "$c" && printf 'pwd; git branch --show-current\n' | HOME=$ch "${UR[@]}" "$D" "${S[@]}" --clone --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
cgit="GIT_SSH_COMMAND='ssh -F $dsc' git -C '$c'"
# smolclone_seeded VM: its own clone, every branch pushed in over ssh, nothing of the folder mounted.
smolclone_seeded() {
  [[ -n $1 ]] && grep -qx mode=clone "$vms/$1" && grep -qx "$c" "$work/last.log" && grep -qx main "$work/last.log" &&
  [[ $(in_smol "$1" "git -C '$c' branch --format='%(refname:short)'" | tr '\n' ' ') == 'main other ' ]] &&
  in_smol "$1" "git -C '$c' log --oneline" | grep -q one && in_smol "$1" "[ ! -e '$c/untracked' ]"; }
check "the VM has its own clone, every branch pushed in over ssh, nothing of the folder mounted, and the shell opens in it" smolclone_seeded "$vm"
check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
in_smol "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
check "pushing a branch into it works too" bash -c "git -C '$c' branch pushed other && $cgit push -q devvm pushed &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- git -C '$c' rev-parse -q --verify refs/heads/pushed </dev/null >/dev/null"
check "rm -f deletes it and the remote fully; the fetched work stays as a local branch" bash -c "cd '$c' && HOME='$ch' '$D' rm -f </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status '$vm') && ! -e '$vms/$vm' ]] && ! git -C '$c' remote | grep -q devvm &&
  git -C '$c' log --oneline saved/devvm/main | grep -q from-vm"
unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM

section "smolvm: network policy"
# The emulation's guarantee (reviewed): a VM with network policy reaches nothing but its proxy,
# through devvm's decider; DNS it's refused never leaves this machine; with the proxy gone nothing
# gets out, and smolvm won't start it without a decider; its policy applies as on Lima. A base VM:
# no agent, so no login is read. Your own HOME, as smolvm keeps its machines under it. U is what
# devvm runs with (an unreviewed emulation would add DEVVM_UNREVIEWED=backend:feature here). Its ssh
# is checked to open no way out.
U=(env)
# smol_root VM CMD: CMD as root in the VM. smol_block VM: the VM's object in smolvm's JSON.
smol_root() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- bash -c "$2" </dev/null; }
smol_block() { SMOLVM_USAGE=0 smolvm machine ls --json 2>/dev/null | awk -v n="\"name\": \"$1\"" '
  /^  \{/ { b = "" } { b = b $0 "\n" } /^  \}/ && index(b, n) { printf "%s", b }'; }
# smolnet_lists VM: its allow list is the gateway alone, its DNS a name that doesn't exist.
# smolnet_open VM: it has no allow list.
smolnet_lists() { local b; b=$(smol_block "$1" | tr -d ' \n')
  [[ $b == *'"allowed_cidrs":["100.96.0.1/32"]'* && $b == *'"allowed_hosts":["=devvm.invalid"]'* ]]; }
smolnet_open() { [[ -n $(smol_block "$1") ]] && ! smol_block "$1" | grep -q '"allowed_'; }
smolnet_closed() { smolnet_lists "$1" && smolnet_direct "$1"; }
# smolnet_direct VM: every direct connection fails, even as root: an address, a port of an allowed
# host, IPv6, and the host's loopback through the gateway (the listener, which answers 407).
smolnet_direct() {
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" &&
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1" &&
  ! smol_root "$1" "curl -6 --noproxy '*' -sS -m 5 -o /dev/null https://[2606:4700:4700::1111]" &&
  ! smol_root "$1" "timeout 5 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22 && head -c 4 <&3' | grep -q SSH-" &&
  [[ $(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/") != 407 ]]
}
# The DNS probe, run as root in the VM with a name prefix of this run's ($1): its resolver, and
# queries straight to the gateway's resolver and 1.1.1.1 (UDP, and TCP to 1.1.1.1) for TXT, ANY,
# AAAA, HTTPS, an EDNS0 A, a name under devvm.invalid and DEVVM.INVALID; last, devvm.invalid
# itself, which smolvm sends to this machine's resolver: the detector's positive control.
dns_probe='n=$1
# pkt NAME QTYPE [edns]: a query, as printf escapes.
pkt() { local q="" l t
  for l in ${1//./ }; do q+=$(printf "\\\\x%02x" "${#l}")$l; done
  t=$(printf "\\\\x%02x\\\\x%02x" $(($2 >> 8)) $(($2 & 255)))
  if [[ -n ${3:-} ]]; then
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x01$q\\x00$t\\x00\\x01\\x00\\x00\\x29\\x10\\x00\\x00\\x00\\x00\\x00\\x00\\x00"
  else
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x00$q\\x00$t\\x00\\x01"
  fi; }
send() { local p len; p=$(pkt "$@")
  printf "$p" > /dev/udp/100.96.0.1/53; printf "$p" > /dev/udp/1.1.1.1/53
  len=$(printf "$p" | wc -c)
  { printf "\\x00\\x$(printf %02x "$len")"; printf "$p"; } | timeout 3 bash -c "cat > /dev/tcp/1.1.1.1/53"; }
getent hosts "$n-a.example.com" >/dev/null; echo "resolver rc=$?"
send "$n-b.example.com" 16; send "$n-c.example.com" 255; send "$n-d.example.com" 28; send "$n-e.example.com" 65
send "$n-f.example.com" 1 edns; send "x.$n.devvm.invalid" 1; send DEVVM.INVALID 16
getent hosts "$n-g.example.org" >/dev/null; echo "resolver rc=$?"
sleep 1; getent hosts devvm.invalid >/dev/null; echo "control rc=$?"'
printf '%s\n' "$dns_probe" > "$work/dns-probe.sh"
# The UDP and ICMP probe, as root: datagrams to the host's loopback listener through the gateway, to
# 1.1.1.1 on 443 and 123, an NTP request to time.cloudflare.com (162.159.200.123) and its reply,
# and a ping to 1.1.1.1.
udp_probe='for t in 100.96.0.1/18766 1.1.1.1/443 1.1.1.1/123; do printf devvm-e2e-udp > /dev/udp/$t; done
if exec 3<>/dev/udp/162.159.200.123/123; then
  { printf "\\x1b"; head -c 47 /dev/zero; } >&3
  echo "ntp bytes=$(timeout 3 head -c 48 <&3 | wc -c | tr -d " ")"
else echo "ntp bytes=0"; fi
perl -MNet::Ping -e "\$p = Net::Ping->new(q(icmp), 2); print \$p->ping(q(1.1.1.1)) ? qq(ping reply\n) : qq(ping none\n)"'
printf '%s\n' "$udp_probe" > "$work/udp-probe.sh"
# smolnet_probe VM SCRIPT ARG: SCRIPT as root in the VM, its output in $work/probe.log, while nettop
# (macOS) watches the VM's smolvm process (the first line of its agent.pid) on this machine's
# non-loopback interfaces; prints the flows it had, one per line (closed ones are listed for a while
# too). smolvm's own record of its DNS decisions during the probe goes to $work/decisions.
smolnet_probe() {
  local pid csv=$work/nettop.csv np dec n0
  dec="$(SMOLVM_USAGE=0 smolvm machine data-dir --name "$1")/egress-decisions.jsonl"
  pid=$(head -1 "$(dirname "$dec")/agent.pid" 2>/dev/null)
  [[ $pid =~ ^[0-9]+$ ]] || return 1
  n0=$(cat "$dec" 2>/dev/null | wc -l)
  nettop -t external -L 14 -s 1 -n -x -p "$pid" > "$csv" 2>/dev/null &
  np=$!
  sleep 2
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -i -- bash -s -- "$3" < "$2" > "$work/probe.log" 2>&1
  wait "$np"
  tail -n +$((n0 + 1)) "$dec" 2>/dev/null | grep '"transport":"dns"' > "$work/decisions"
  # Rows: time,what,…: the process (smolvm-bin.PID), or a flow (udp4 LOCAL<->REMOTE).
  awk -F, 'index($2, "<->") { print $2 }' "$csv" | sort -u
}
# smolnet_no_leak VM: refused DNS doesn't leave this machine. No flow beyond loopback, and smolvm
# refused every probe name (each one seen) and let through only devvm.invalid: the control, which
# shows the detector sees a query that does go to this machine's resolver (nettop doesn't: smolvm
# asks it through the system's resolver).
smolnet_no_leak() {
  local f n=devvm-e2e-$$-$RANDOM name allowed
  f=$(smolnet_probe "$1" "$work/dns-probe.sh" "$n") || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"; cat "$work/decisions"
  [[ -z $f && $(grep -c '^resolver rc=2$' "$work/probe.log") == 2 ]] || return 1
  allowed=$(grep '"action":"allow"' "$work/decisions" | sed 's/.*"destination":"\([^"]*\)".*/\1/' | tr '[:upper:]' '[:lower:]' | sort -u)
  [[ $allowed == devvm.invalid ]] || { echo "allowed: $allowed"; return 1; }
  for name in "$n-a.example.com" "$n-b.example.com" "$n-c.example.com" "$n-d.example.com" "$n-e.example.com" \
    "$n-f.example.com" "x.$n.devvm.invalid" "$n-g.example.org"; do
    grep '"action":"deny"' "$work/decisions" | grep -qF "\"destination\":\"$name\"" || { echo "not seen: $name"; return 1; }
  done
}
# smol_raw NAME OPTION...: a machine of the run's own from base's pack, with the sandbox's label and
# those network options, started: no port forward, which every devvm VM has since its ssh. Deleted
# at the end, or by smol_raw_rm.
smol_raw() { local n=$1; shift; created+=("$n")
  SMOLVM_USAGE=0 smolvm machine create --name "$n" --label "devvm.data=$XDG_DATA_HOME/devvm" --from "$packs/$kb/image.smolmachine" \
    --cpus 1 --mem 1024 "$@" >/dev/null 2>&1 && SMOLVM_USAGE=0 smolvm machine start --name "$n" >/dev/null 2>&1; }
smol_raw_rm() { SMOLVM_USAGE=0 smolvm machine delete --name "$1" -f >/dev/null 2>&1; }
# smolnet_tsi_dns_seen: on smolvm's own network without a forward (TSI, smol_raw), the VM asks this
# machine's resolver, and the DNS probe's queries are seen to leave: the nettop control.
smolnet_tsi_dns_seen() { local n=devvm-e2e-tsi r
  smol_raw "$n" --net || { echo "no machine"; return 1; }
  smol_tsi "$n" && smolnet_dns_seen "$n"; r=$?
  smol_raw_rm "$n"; return "$r"; }
# smolnet_dns_seen VM: on smolvm's own network, the same probe's queries do leave (the control).
smolnet_dns_seen() { local f; f=$(smolnet_probe "$1" "$work/dns-probe.sh" "devvm-e2e-$$-$RANDOM") || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"; grep -q ':53$' <<<"$f"; }
# A UDP listener on the host's loopback, 127.0.0.1:18766: each datagram it gets is a line in $work/udp.log.
smolnet_udp_listen() { : > "$work/udp.log"
  perl -MIO::Socket::INET -e '$| = 1; $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:18766", Proto => "udp") or die;
    while ($s->recv($d, 2048)) { print "got $d\n" }' >> "$work/udp.log" 2>&1 &
  udp_listener=$!; sleep 0.5; }
# smolnet_udp_closed VM: no datagram reaches the host's loopback, NTP gets no answer, nor a ping, and
# the VM's process has no flow beyond loopback.
smolnet_udp_closed() { local f
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x) || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log" "$work/udp.log"
  [[ -z $f && ! -s $work/udp.log ]] && grep -qx 'ntp bytes=0' "$work/probe.log" && grep -qx 'ping none' "$work/probe.log"; }
# smolnet_udp_allowed_seen VM: a UDP allow for the NTP server, added live, lets the probe's request
# out and nettop sees it (the UDP probe's control under network policy); then it's taken away.
smolnet_udp_allowed_seen() {
  local f
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --allow-udp 162.159.200.123/32:123 >/dev/null 2>&1 || return 1
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x)
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --remove-allow-udp 162.159.200.123/32:123 >/dev/null 2>&1 || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"
  grep -q '<->162\.159\.200\.123:123$' <<<"$f"
}
# smolnet_udp_seen VM: on smolvm's own network, the probe's datagrams leave and nettop sees them
# (the control). bash's /dev/udp reads no reply on that network, so replies aren't counted.
smolnet_udp_seen() { local f
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x) || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"
  grep -q '<->162\.159\.200\.123:123$' <<<"$f" && grep -q '<->1\.1\.1\.1:443$' <<<"$f"; }
# smolnet_direct_open VM: on smolvm's own network the direct probes do get out (their control):
# 1.1.1.1, github's ssh banner, and IPv6 when this machine has it.
smolnet_direct_open() {
  smol_root "$1" "curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1" &&
  smol_root "$1" "timeout 10 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22 && head -c 4 <&3' | grep -q SSH-" || return 1
  if curl -6 -s -m 5 -o /dev/null 'https://[2606:4700:4700::1111]'; then
    smol_root "$1" "curl -6 --noproxy '*' -sS -m 10 -o /dev/null https://[2606:4700:4700::1111]"
  else echo "this machine has no IPv6: not checked"; fi
}
# smolnet_fake_decider MODE: a decider of nobody's on the VM's decider port, which answers garbage
# (garbage), closes without a verdict (silent), allows every connection direct (allow: verdict 0),
# or takes every one over and answers it itself, 418 (take: verdict 2, the fail-closed probes'
# positive control); its pid in $fake. It fails, naming the port, unless it listens within 10 s.
smolnet_fake_decider() {
  # Each answer's connection is shut for writing, then read to its end (2 s at most) before it's
  # closed: closing it with smolvm's bytes unread would reset it, losing the answer at times.
  perl -MIO::Socket::INET -e '$SIG{ALRM} = sub {}; $| = 1; $m = shift; $p = shift; $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:$p", Listen => 16, ReuseAddr => 1) or die "$@\n";
    while ($c = $s->accept) { sysread($c, $b, 4096); print $c "\x07garbage" if $m eq "garbage"; print $c "\x00" if $m eq "allow";
      print $c "\x02HTTP/1.1 418 squatter\r\nContent-Length: 0\r\nConnection: close\r\n\r\n" if $m eq "take"; $c->flush;
      shutdown($c, 1); alarm 2; 1 while sysread($c, $b, 4096); alarm 0; close $c }' "$1" "$dp" >/dev/null 2>"$work/fake.log" &
  fake=$!
  for _ in $(seq 20); do
    lsof -t -a -p "$fake" -iTCP:"$dp" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    kill -0 "$fake" 2>/dev/null || break
    sleep 0.5
  done
  echo "the fake decider ($1) isn't listening on 127.0.0.1:$dp: $(cat "$work/fake.log")"
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""; return 1; }
# smolnet_port_free PORT: nothing listens on PORT within 10 s, else it fails naming it.
smolnet_port_free() {
  for _ in $(seq 20); do lsof -t -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1 || return 0; sleep 0.5; done
  echo "port $1 is still listened on: $(lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>&1)"; return 1; }
# smolnet_nothing_out VM: nothing gets out: not through the proxy, not directly, not to the host's
# loopback listener through the gateway (which answers 407).
smolnet_nothing_out() {
  ! in_smol "$1" 'curl -fsS -m 10 -o /dev/null https://github.com' &&
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" &&
  [[ $(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/") != 407 ]]
}
# smolnet_fake_allow_seen VM: a decider of nobody's that allows the connection direct (verdict 0)
# still gets nothing to the host's loopback listener: smolvm refuses direct connections to the host
# (direct_target_policy). One that takes the connection over (verdict 2) answers it: the probe sees
# a fail-open when there is one (its positive control).
smolnet_fake_allow_seen() {
  local allow take
  smolnet_fake_decider allow || return 1
  allow=$(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/")
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
  smolnet_fake_decider take || return 1
  take=$(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/")
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
  [[ $allow != 407 && $take == 418 ]] || { echo "with a decider that allows: $allow (want no 407), that takes over: $take (want 418)"; return 1; }
}
# smolnet_fail_closed VM: with its proxy gone nothing gets out, nor with a decider that answers
# garbage or nothing; and smolvm won't start it without a decider.
smolnet_fail_closed() {
  local pid m
  pid=$(smolnet_proxy "$1") && [[ -n $pid ]] && kill "$pid" && smolnet_port_free "$dp" || return 1
  if smolnet_proxy "$1" >/dev/null || ! smolnet_nothing_out "$1"; then echo "with the proxy gone"; return 1; fi
  for m in garbage silent; do
    smolnet_fake_decider "$m" || return 1
    smolnet_nothing_out "$1"; local rc=$?
    kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
    (( rc == 0 )) || { echo "with a decider that answers $m"; return 1; }
  done
  # The control: a decider that allows everything lets the same probe through to the listener.
  smolnet_fake_allow_seen "$1" || return 1
  SMOLVM_USAGE=0 smolvm machine stop --name "$1" >/dev/null 2>&1 &&
  ! env -u SMOLVM_INTERCEPTOR_TOKEN SMOLVM_USAGE=0 smolvm machine start --name "$1" >"$work/e" 2>&1 && [[ $(smol_status "$1") != running ]]
}
# smolnet_widened VM DIR: its allow list widened while it runs (smolvm machine update): devvm shows
# it unenforced, and its next launch stops it; the one after starts it confined again.
smolnet_widened() {
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --allow-cidr 1.1.1.1/32 >/dev/null 2>&1 || return 1
  if ! "$D" ls | grep -Eq "^$1 +Running +\S+ +\S+ +unenforced +smolvm "; then "$D" ls; return 1; fi
  if (cd "$2" && printf 'echo e2e-opened\n' | "${U[@]}" DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1) ||
    ! grep -qx "devvm: net_unenforced_stop vm=$1" "$work/last.log" || [[ $(smol_status "$1") != stopped ]]; then
    cat "$work/last.log"; return 1
  fi
  (cd "$2" && printf 'exit\n' | "${U[@]}" "$D" >"$work/last.log" 2>&1) && [[ $(smol_status "$1") == running ]] &&
  grep -qx net_check=ok "$vms/$1" && smolnet_lists "$1"
}
# The proxy's lifeline-following process for VM, if it runs.
smolnet_proxy() { pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 " 2>/dev/null; }
# sshn ARGS...: ssh through devvm's ssh config alone, without an agent. smolnet_via VM HOST:PORT: an
# HTTP request through a local forward (ssh -W) from the VM to HOST:PORT, and the start of what comes
# back, in 15 s at most.
sshn() { env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -o BatchMode=yes "$@"; }
smolnet_via() { printf 'GET / HTTP/1.0\r\nHost: %s\r\n\r\n' "${2%:*}" |
  perl -e 'alarm 15; exec @ARGV' env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -W "$2" "$1" 2>/dev/null | head -c 12; }
smolnet_via_open() { local a; a=$(smolnet_via "$1" 1.1.1.1:80); echo "got: ${a:-nothing}"; [[ $a == HTTP/* ]]; }
# smolnet_ssh VM: ssh reaches it on its network policy's port; its sessions, a login's or not, get the
# proxy's settings, and an allowed host works through the proxy from one.
smolnet_ssh() { local key out
  key=$(sed -n 's/^proxy_key=//p' "$vms/$1")
  [[ -n $(smolssh_port "$1") && $(smolssh_port "$1") == "$(sed -n 's/^ssh_port=//p' "$vms/$1")" ]] || { echo "not its ssh_port"; return 1; }
  out=$(sshn "$1" 'printenv HTTPS_PROXY; printenv NO_PROXY; bash -lc "printenv https_proxy"' </dev/null)
  [[ $out == "http://devvm:$key@100.96.0.1:"*$'\n'"localhost,127.0.0.1,::1"$'\n'"http://devvm:$key@100.96.0.1:"* ]] || { echo "sessions: ${out//$key/KEY}"; return 1; }
  sshn "$1" 'curl -fsS -m 20 -o /dev/null https://github.com' </dev/null; }
# smolnet_ssh_closed VM: ssh opens no way out: a local forward to an outside host or to a service on
# the host's loopback (which answers 407) gets nothing back, remote forwarding is refused, and a
# direct connection from a session fails.
smolnet_ssh_closed() { local a b
  a=$(smolnet_via "$1" 1.1.1.1:80) b=$(smolnet_via "$1" 100.96.0.1:18765)
  echo "1.1.1.1:80: ${a:-nothing}; the host's loopback: ${b:-nothing}"
  [[ $a != HTTP/* && $b != HTTP/* ]] &&
  ! sshn -o ExitOnForwardFailure=yes -R 0:127.0.0.1:18765 "$1" true </dev/null &&
  ! sshn "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" </dev/null; }
# A service on the host's own loopback (a devvm _proxy, which answers anything with 407).
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
np=$(newproj smolnet)
ps_watch
(cd "$np" && printf 'exit\n' | "${U[@]}" DEVVM_MESSAGES=names "$D" "${S[@]}" --network-policy --ports none --no-git-protect base >"$work/last.log" 2>&1)
ps_unwatch
vm=$(vm_of "$np"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
dp=$(sed -n 's/^decider_port=//p' "$vms/$vm" 2>/dev/null)
check "--network-policy: the VM runs behind its proxy, which decides for it, and the self-check passed" bash -c "
  [[ -n '$vm' && -n '$dp' && \$(smol_status '$vm') == running ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  ! grep -q 'backend_unreviewed' '$work/last.log' &&
  pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm .* -decider 127.0.0.1:$dp -decider-to 100.96.0.1:$pp ' >/dev/null"
# The control: the guest has the password, from the VM's proxy file, in a login and in Docker's
# daemon, which devvm's services exec started with no login.
check "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it" bash -c "
  key=\$(sed -n 's/^proxy_key=//p' '$vms/$vm') && { [[ \${#key} -eq 32 ]] || { echo no key; false; }; } &&
  { [[ -s '$work/ps.log' ]] || { echo nothing sampled; false; }; } &&
  { ! grep -qFf <(echo \"\$key\") '$work/ps.log' || { echo in a process while devvm launched it; false; }; } &&
  { ! ps -axww -o command= | grep -qFf <(echo \"\$key\") || { echo in a process now; false; }; } &&
  { grep -qF \":\$key@\" '$XDG_DATA_HOME/devvm/smolvm/vms/$vm.proxy' || { echo no proxy file; false; }; } &&
  { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc 'printenv HTTPS_PROXY' </dev/null | grep -qF \":\$key@\" || { echo not in a login; false; }; } &&
  { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- sh -c 'tr \"\\\\0\" \"\\\\n\" < /proc/\$(cat /run/docker.pid)/environ' </dev/null |
    grep -qF \"https_proxy=http://devvm:\$key@\" || { echo not in the daemon devvm started; false; }; }"
check "its allow list is the host's gateway alone, and its DNS a name that doesn't exist" smolnet_lists "$vm"
check "... on virtio-net: it asks the gateway's resolver" smol_vnet "$vm"
check "an allowed host works, through the proxy" in_smol "$vm" "curl -fsS -m 20 -o /dev/null https://github.com"
check "another host is blocked by the proxy, and so is another port of an allowed one" bash -c "
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc \"\$1\" </dev/null; }
  [[ \$(in_smol 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]] &&
  in_smol 'curl -sS -m 20 -o /dev/null https://example.com' 2>&1 | grep -q 403 &&
  ! in_smol 'curl -fsS -m 20 -o /dev/null https://github.com:8443'"
check "direct connections fail, even as root: an address, other ports, IPv6, the host's loopback" smolnet_direct "$vm"
check "ssh reaches it on its port; its sessions, a login's or not, get the proxy's settings, and an allowed host works" smolnet_ssh "$vm"
check "ssh opens no way out: a local forward to 1.1.1.1 or the host's loopback gets nothing, remote forwarding is refused" smolnet_ssh_closed "$vm"
check "the host's loopback is refused through the proxy too (internal addresses need a rule)" bash -c "
  [[ \$(SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} http://100.96.0.1:18765/' </dev/null) == 403 ]]"
check "DNS fails at once, not after a timeout" in_smol "$vm" 'timeout 3 getent hosts example.com; [ $? -eq 2 ]'
if [[ "$(uname -s)" == Darwin ]]; then
  check "refused DNS doesn't leave this machine (TXT, ANY, AAAA, HTTPS, EDNS0, under devvm.invalid): smolvm refuses each, and only devvm.invalid, the control, goes to the resolver" smolnet_no_leak "$vm"
  smolnet_udp_listen
  check "UDP and ICMP don't get out: nothing reaches the host's loopback, NTP and ping get no answer" smolnet_udp_closed "$vm"
  # The ping has no control: smolvm's own network doesn't forward ICMP either.
  check "... while a UDP allow added live lets the probe's NTP request out (the control), until it's removed" smolnet_udp_allowed_seen "$vm"
else
  skip "refused DNS doesn't leave this machine (TXT, ANY, AAAA, HTTPS, EDNS0, under devvm.invalid): smolvm refuses each, and only devvm.invalid, the control, goes to the resolver" "nettop is macOS's"
  skip "UDP and ICMP don't get out: nothing reaches the host's loopback, NTP and ping get no answer" "nettop is macOS's"
  skip "... while a UDP allow added live lets the probe's NTP request out (the control), until it's removed" "nettop is macOS's"
fi
check "the decider's token is in no host process's arguments, and never reaches the VM" bash -c "
  tok=\$(sed -n 's/^decider_token=//p' '$vms/$vm') && [[ \${#tok} -eq 64 ]] &&
  ! ps -axww -o command= | grep -qFf <(echo \"\$tok\") &&
  ! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- bash -c \"grep -rqsF \$tok /etc /run /root /home /tmp /var/log\" </dev/null &&
  ! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- bash -c \"cat /proc/[0-9]*/environ 2>/dev/null | tr '\\\\0' '\\\\n' | grep -qF \$tok\" </dev/null"
check "devvm log shows the proxy's block and the direct connection to the host the decider refused" bash -c "cd '$np' &&
  '$D' log >'$work/e' 2>&1; grep -q '  BLOCKED  example.com:80  ' '$work/e' && grep -q '  BLOCKED  100.96.0.1:18765  ' '$work/e' ||
  { cat '$work/e'; false; }"
check "devvm policy allow applies at once to the running VM's proxy, and rm takes it back" bash -c "cd '$np' &&
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc \"\$1\" </dev/null; }
  pid=\$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') && [[ -n \$pid ]] &&
  ${U[*]} '$D' policy allow example.com </dev/null >/dev/null && in_smol 'curl -fsS -m 20 -o /dev/null https://example.com' &&
  ${U[*]} '$D' policy rm example.com </dev/null >/dev/null &&
  [[ \$(in_smol 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]] &&
  [[ \$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') == \$pid ]]"
check "its allow list widened while it runs: devvm shows it unenforced, stops it, then starts it confined again" smolnet_widened "$vm" "$np"
check "with its proxy gone nothing gets out (the host's loopback listener included), nor with a decider that answers garbage or nothing, while one that allows everything lets the probe through (the control); smolvm won't start it without a decider" \
  smolnet_fail_closed "$vm"
check "started outside devvm, with a decider of someone else's: devvm stops it and opens nothing" bash -c "
  SMOLVM_INTERCEPTOR_TOKEN=\$(openssl rand -hex 32) SMOLVM_USAGE=0 smolvm machine start --name '$vm' --mediated-egress --egress-interceptor 127.0.0.1:9 >/dev/null 2>&1 &&
  [[ \$(smol_status '$vm') == running ]] && cd '$np' &&
  ! printf 'echo e2e-opened\n' | ${U[*]} DEVVM_MESSAGES=names '$D' >'$work/last.log' 2>&1 &&
  grep -qx 'devvm: net_unenforced_stop vm=$vm' '$work/last.log' && ! grep -q e2e-opened '$work/last.log' && [[ \$(smol_status '$vm') == stopped ]]"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$np' &&
  printf 'exit\n' | ${U[*]} '$D' >'$work/last.log' 2>&1 && [[ \$(smol_status '$vm') == running ]] && grep -qx net_check=ok '$vms/$vm' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && [[ \$('$D' network 2>/dev/null) == on ]]"
check "devvm network off: its next start is smolvm's own network, without an allow list, a decider or a proxy" bash -c "cd '$np' &&
  ${U[*]} '$D' network off </dev/null >/dev/null 2>&1 && printf 'exit\n' | ${U[*]} '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' network) == off && \$(smol_status '$vm') == running ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1 </dev/null"
check "... with no allow list left" smolnet_open "$vm"
check "... where the direct probes get out (their control): 1.1.1.1, github's ssh, IPv6" smolnet_direct_open "$vm"
check "... and a local forward through ssh to 1.1.1.1 gets an answer (its control)" smolnet_via_open "$vm"
check "... on virtio-net, as its ssh forward has it" smol_vnet "$vm"
# A machine with a port forward (its ssh) is on virtio-net, where smolvm resolves through this
# machine's system resolver, which nettop can't tie to it: the DNS control runs on a machine without
# a forward (TSI), as the probe was checked before ssh.
if [[ "$(uname -s)" == Darwin ]]; then
  check "... and on TSI (a machine without a forward) the DNS probe's queries leave (its control)" smolnet_tsi_dns_seen
  check "... and the UDP probe's datagrams leave (its control)" smolnet_udp_seen "$vm"
else
  skip "... and on TSI (a machine without a forward) the DNS probe's queries leave (its control)" "nettop is macOS's"
  skip "... and the UDP probe's datagrams leave (its control)" "nettop is macOS's"
fi
check "devvm network on: devvm restart starts it confined again" bash -c "cd '$np' &&
  ${U[*]} '$D' network on </dev/null >/dev/null 2>&1 && printf 'exit\n' | ${U[*]} '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' network) == on ]] && grep -qx net_check=ok '$vms/$vm' && pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
check "... its allow list and DNS closed again, and direct connections fail" smolnet_closed "$vm"
check "rm -f deletes it, its proxy and its state" bash -c "cd '$np' && ${U[*]} '$D' rm -f </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status '$vm') && ! -e '$vms/$vm' ]] && sleep 4 && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
kill "$listener" 2>/dev/null; [[ -z ${udp_listener:-} ]] || kill "$udp_listener" 2>/dev/null; listener="" udp_listener=""
rm -f "$work/listener.auth"

section "smolvm: every port"
# The emulation's guarantee (reviewed): every port a VM listens on, on its loopback or all its
# addresses, IPv4 or IPv6, reaches this machine's loopback, and only its loopback; never a port this
# machine has on any address, one below 1024, or one devvm records for a VM, even while it's free;
# and forwarding opens the VM no way out (with network policy, nothing past its proxy).
# smolpa_serve VM HOST PORT: a server in the VM, as root, answering "vm-PORT" on HOST (an address of
# the VM's; :: takes IPv4 too), left running.
smolpa_serve() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -d -- perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
  $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1)) or die "$@";
  while ($c = $s->accept) { print $c "vm-$p\n"; close $c }' "$2" "$3" </dev/null >/dev/null 2>&1; }
# smolpa_answers PORT WANT: this machine's 127.0.0.1 and ::1 answer WANT on PORT, within 15 s.
smolpa_answers() { local a b; for _ in $(seq 15); do
    a=$(nc -w 2 127.0.0.1 "$1" </dev/null 2>/dev/null); b=$(nc -6 -w 2 ::1 "$1" </dev/null 2>/dev/null)
    [[ $a == "$2" && $b == "$2" ]] && return 0; sleep 1; done
  echo "127.0.0.1: ${a:-nothing}, ::1: ${b:-nothing}"; return 1; }
# smolpa_closed PORT: nothing answers on this machine's loopback on PORT, within 10 s.
smolpa_closed() { for _ in $(seq 10); do
    nc -z 127.0.0.1 "$1" 2>/dev/null || nc -6 -z ::1 "$1" 2>/dev/null || return 0; sleep 1; done
  echo "port $1 still answers"; return 1; }
# smolpa_forwarder VM: its forwarder's pid. smolpa_log VM: its forwarder's log.
smolpa_forwarder() { pgrep -f -- "[_]smolvm-ports -vm $1 " | head -1; }
smolpa_log() { cat "$XDG_DATA_HOME/devvm/smolvm/vms/$1.ports.log" 2>/dev/null; }
# smolpa_loopback VM PORT...: its forwarder listens on each PORT, and everything it listens on is on
# this machine's loopback alone.
smolpa_loopback() { local vm=$1 pid l p; shift
  pid=$(smolpa_forwarder "$vm"); [[ -n $pid ]] || { echo "no forwarder"; return 1; }
  l=$(lsof -nP -a -p "$pid" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u); echo "$l"
  for p in "$@"; do grep -qx "127\.0\.0\.1:$p" <<<"$l" && grep -qx "\[::1\]:$p" <<<"$l" || return 1; done
  ! grep -vE '^(127\.0\.0\.1|\[::1\]):[0-9]+$' <<<"$l"; }
# smolpa_master VM: its forwarder's ssh reads no configuration of yours, offers the VM's key alone,
# pins its host key and asks for no forwarding.
smolpa_master() { local m; m=$(pgrep -fl -- "ssh -F none -M .*HostKeyAlias=$1 " | head -1); echo "$m"
  [[ -n $m && $m == *"IdentityAgent=none"* && $m == *"StrictHostKeyChecking=yes"* && $m == *"ClearAllForwardings=yes"* &&
    $m == *"PermitRemoteOpen=none"* && $m == *"ForwardAgent=no"* ]] && ! grep -qE ' -[RLDAXY]( |$)' <<<"$m"; }
export -f smolpa_answers smolpa_closed smolpa_forwarder smolpa_log
pa=$(newproj smolall)
(cd "$pa" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${S[@]}" --no-network-policy --no-git-protect base >"$work/last.log" 2>&1)
vm=$(vm_of "$pa"); [[ -z $vm ]] || created+=("$vm")
check "the VM forwards every port (the default), with a forwarder of devvm's, none of smolvm's but its ssh's" bash -c "
  [[ -n '$vm' && \$(smol_status '$vm') == running ]] && ! grep -q backend_unreviewed '$work/last.log' &&
  ! grep -q smolvm_ports '$work/last.log' && [[ -n \$(smolpa_forwarder '$vm') ]] && smolpa_log '$vm' | grep -q \"forwarding $vm's ports\""
check "... its machine's process listening on its ssh forward alone" smol_listens_only "$vm" "$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json")"
check "... its ssh master reading no config of yours, the VM's key alone, its host key pinned, no forwarding asked for" smolpa_master "$vm"
smolpa_serve "$vm" 127.0.0.1 47391; smolpa_serve "$vm" ::1 47392; smolpa_serve "$vm" 0.0.0.0 47393; smolpa_serve "$vm" :: 47394
check "a server on the VM's loopback alone reaches this machine's, at 127.0.0.1 and ::1" smolpa_answers 47391 vm-47391
check "... one on its IPv6 loopback alone" smolpa_answers 47392 vm-47392
check "... one on all its IPv4 addresses, and one on all its addresses" bash -c "smolpa_answers 47393 vm-47393 && smolpa_answers 47394 vm-47394"
check "the forwarder listens on this machine's loopback alone" smolpa_loopback "$vm" 47391 47392 47393 47394
# This machine's own program, on every address, and the VM's server on the same port.
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:47395", Listen => 5, ReuseAddr => 1) or die;
  while ($c = $s->accept) { print $c "host-47395\n"; close $c }' </dev/null >/dev/null 2>&1 &
hostprog=$!
sleep 0.5
smolpa_serve "$vm" 127.0.0.1 47395; smolpa_serve "$vm" 0.0.0.0 1023
check "a port this machine has on any address stays its own, and one below 1024 isn't forwarded" bash -c "
  for _ in \$(seq 15); do smolpa_log '$vm' | grep -q 'port 47395: not forwarded' && break; sleep 1; done
  smolpa_log '$vm' | grep -q 'port 47395: not forwarded' && [[ \$(nc -w 2 127.0.0.1 47395 </dev/null) == host-47395 ]] &&
  ! smolpa_log '$vm' | grep -q 'port 1023' && smolpa_closed 1023"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
# smolpa_hostserve ADDR PORT: this machine's own program on ADDR (:: takes IPv4 too, as Node's and
# Go's servers do), answering "host-PORT", left running; its pid in hostprogs.
hostprogs=()
smolpa_hostserve() { perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
    $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1)) or die "$@";
    while ($c = $s->accept) { print $c "host-$p\n"; close $c }' "$1" "$2" </dev/null >/dev/null 2>&1 &
  hostprogs+=($!); }
# smolpa_hosts_kept: each of this machine's programs keeps its port, the VM's server on it isn't
# forwarded, and localhost never reaches two programs: on 127.0.0.1 alone (::1 stays closed), on
# ::1 alone (127.0.0.1 stays closed), and on every address, IPv4 and IPv6 at once.
smolpa_hosts_kept() { local vm=$1 p
  for _ in $(seq 15); do
    smolpa_log "$vm" | grep -q 'port 47403: not forwarded' && smolpa_log "$vm" | grep -q 'port 47402: not forwarded' && break; sleep 1; done
  smolpa_log "$vm" | grep -E 'port 4740[123]'
  for p in 47401 47402 47403; do smolpa_log "$vm" | grep -q "port $p: not forwarded" || { echo "port $p: no refusal logged"; return 1; }; done
  if [[ $(nc -w 2 127.0.0.1 47401 </dev/null) != host-47401 ]] || nc -6 -z ::1 47401 2>/dev/null; then
    echo "47401 not the host's alone"; return 1; fi
  if [[ $(nc -6 -w 2 ::1 47402 </dev/null) != host-47402 ]] || nc -z 127.0.0.1 47402 2>/dev/null; then
    echo "47402 not the host's alone"; return 1; fi
  [[ $(nc -w 2 127.0.0.1 47403 </dev/null) == host-47403 && $(nc -6 -w 2 ::1 47403 </dev/null) == host-47403 ]] ||
    { echo "47403 not the host's"; return 1; }; }
smolpa_hostserve 127.0.0.1 47401; smolpa_hostserve ::1 47402; smolpa_hostserve :: 47403
sleep 0.5
smolpa_serve "$vm" 0.0.0.0 47401; smolpa_serve "$vm" :: 47402; smolpa_serve "$vm" :: 47403
check "... so does one on its loopback alone, IPv4 or IPv6, and one on every address, IPv4 and IPv6 at once" smolpa_hosts_kept "$vm"
kill "${hostprogs[@]}" 2>/dev/null; wait "${hostprogs[@]}" 2>/dev/null; hostprogs=()
# smolpa_not_loopback VM: a server on the VM's own network address alone isn't forwarded, nor a
# port devvm records for another VM (its listed ports), free on this machine; one on the VM's
# loopback, started after them, is (the control).
smolpa_not_loopback() { local vm=$1 ip r=0
  ip=$(SMOLVM_USAGE=0 smolvm machine exec --name "$vm" -- perl -MIO::Socket::INET -e \
    'print IO::Socket::INET->new(PeerAddr => "100.96.0.1:9", Proto => "udp")->sockhost' </dev/null 2>/dev/null)
  [[ $ip =~ ^[0-9.]+$ && $ip != 127.* ]] || { echo "the VM's address: ${ip:-none}"; return 1; }
  printf 'net=off\nports=47406\n' > "$vms/devvm-e2e-reserved"
  smolpa_serve "$vm" "$ip" 47404; smolpa_serve "$vm" 0.0.0.0 47406; sleep 2; smolpa_serve "$vm" 127.0.0.1 47405
  smolpa_answers 47405 vm-47405 || { echo "the control isn't forwarded"; r=1; }
  smolpa_closed 47404 && smolpa_closed 47406 || r=1
  ! smolpa_log "$vm" | grep -E 'port (47404|47406)' || r=1
  rm -f "$vms/devvm-e2e-reserved"; return "$r"; }
check "a server on the VM's own address alone isn't forwarded, nor a port another VM lists, while it's free (a loopback server is, the control)" smolpa_not_loopback "$vm"
check "a server that stops: its forward goes; the others stay" bash -c "
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- pkill -f '0.0.0.0 47393' </dev/null &&
  smolpa_closed 47393 && smolpa_answers 47391 vm-47391"
check "devvm stop stops the forwarder, and nothing listens on its ports" bash -c "cd '$pa' && '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smolpa_forwarder '$vm') ]] && smolpa_closed 47391 && smolpa_closed 47394"
check "started again, it has a new forwarder, which forwards a server started then" bash -c "cd '$pa' &&
  printf 'exit\n' | '$D' >'$work/last.log' 2>&1 && [[ -n \$(smolpa_forwarder '$vm') ]] &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -d -- perl -MIO::Socket::IP -e '\$s = IO::Socket::IP->new(LocalHost => q(127.0.0.1), LocalPort => 47396, Listen => 5) or die; while (\$c = \$s->accept) { print \$c qq(vm-47396\n); close \$c }' </dev/null >/dev/null 2>&1 &&
  smolpa_answers 47396 vm-47396"
check "rm deletes it, its forwarder and its files" bash -c "cd '$pa' && '$D' rm -f </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') && -z \$(smolpa_forwarder '$vm') ]] && ! ls '$XDG_DATA_HOME/devvm/smolvm/vms/$vm'.ports.* 2>/dev/null && smolpa_closed 47396"

# With network policy: forwarding works, and opens the VM no way out; the forwarder never takes the
# VM's proxy or decider port, even with the proxy gone. A service on this machine's own loopback
# (which answers 407) for the direct probes.
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
pn=$(newproj smolallnet)
(cd "$pn" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${S[@]}" --network-policy --no-git-protect base >"$work/last.log" 2>&1)
vm=$(vm_of "$pn"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
dp=$(sed -n 's/^decider_port=//p' "$vms/$vm" 2>/dev/null)
smolpa_serve "$vm" 127.0.0.1 47397
check "with network policy, every port forwarded too, and the self-check passed" bash -c "
  [[ -n '$vm' && -n '$pp' && -n '$dp' ]] && grep -qx net_check=ok '$vms/$vm' && smolpa_answers 47397 vm-47397"
check "... direct connections still fail while it forwards, even as root: an address, other ports, IPv6, the host's loopback" smolnet_direct "$vm"
# smolpa_reserved VM: with its proxy and decider gone, the VM listens on their ports: neither is
# forwarded (nothing answers there); the next devvm starts the proxy on them again.
smolpa_reserved() {
  local pid
  pid=$(pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 ")
  [[ -n $pid ]] || { echo "no proxy"; return 1; }
  kill "$pid" || return 1
  smolpa_closed "$pp" || return 1
  smolpa_closed "$dp" || return 1
  smolpa_serve "$1" 0.0.0.0 "$pp"; smolpa_serve "$1" 0.0.0.0 "$dp"; smolpa_serve "$1" 127.0.0.1 47398
  smolpa_answers 47398 vm-47398 || { echo "the control: a port of its own isn't forwarded"; return 1; }
  if nc -z 127.0.0.1 "$pp" 2>/dev/null || nc -z 127.0.0.1 "$dp" 2>/dev/null || smolpa_log "$1" | grep -qE "port ($pp|$dp): forwarded"; then
    echo "the proxy's or decider's port was forwarded"; smolpa_log "$1" | tail -5; return 1
  fi
  (cd "$pn" && printf 'exit\n' | "$D" >"$work/last.log" 2>&1) && grep -qx net_check=ok "$vms/$1" &&
  pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 " >/dev/null; }
check "with its proxy and decider gone, the VM listening on their ports: neither is forwarded (a port of its own is, the control); devvm starts the proxy on them again" smolpa_reserved "$vm"
check "rm -f deletes it, its proxy and its forwarder" bash -c "cd '$pn' && '$D' rm -f </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') && -z \$(smolpa_forwarder '$vm') ]] && smolpa_closed 47397"
kill "$listener" 2>/dev/null; listener=""
rm -f "$work/listener.auth"

section "smolvm: shared tools"
t=$(newproj smoltools); echo '{}' > "$t/package.json"
launch "$t" 'node -v' "${S[@]}" --no-network-policy --ports none --no-git-protect
check "a node VM: its tools built uncompressed for smolvm, from the share, at the locked versions" bash -c "
  [[ -n '$vm' ]] && grep -qx image=node '$vms/$vm' && grep -qx v$(lockv node) '$work/last.log' &&
  for f in \$(sed -n 's/^tools=//p' '$vms/$vm'); do [[ -f '$share'/\$f && -f '$share'/\$f.sha256 ]] || exit 1; done"
check "node, npm, pnpm and bun work, each a read-only image" in_smol "$vm" \
  "npm -v && pnpm -v && bun -v && [[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]] && ! touch /opt/devvm/tools/node/$(lockv node)/x 2>/dev/null"
check "the tools build left no machine behind" bash -c "[[ -z \$(smol_status devvm-tools-build) ]]"
check "restarted, the tools are mounted again" bash -c "cd '$t' && printf 'node -v\n' | '$D' restart 2>&1 | grep -qx v$(lockv node)"
(cd "$t" && quiet "$D" rm -f)

check "no machine without the sandbox's label was touched" bash -c "[[ \"\$(smol_others)\" == '$others_before' ]]"
fi

# A script error can end a section early without failing a check: compare with the last complete run
# (same options). Fewer checks than last time fails the run, and the timings aren't kept.
short=""
if [[ $total != "?" ]] && (( ncheck < total )); then
  short="ran $ncheck checks, but the last complete run ran $total: a section ended early (look for errors above)"
else
  mv "$timings.new" "$timings"   # a complete run: the next one estimates from it
fi
echo
echo "e2e: $pass passed, $fail failed$( (( skipped == 0 )) || echo ", $skipped skipped") in $(mins $((SECONDS - started)))${failed[*]+: ${failed[*]}}"
[[ -z $short ]] || echo "e2e: FAILED: $short"
(( fail == 0 )) && [[ -z $short ]]
