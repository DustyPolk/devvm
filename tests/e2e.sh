#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images;
#                project makes a VM from base in a project folder and checks its mounts, ssh by
#                name with pinned host keys, .git protection and shell, then opening it again,
#                restart and rm, and an agent VM's folders and exported history; clone makes a --clone
#                VM of a throwaway repo and checks its clone, the remote, fetching, and rm's checks
#                for unfetched commits and uncommitted changes and the work it keeps; tools makes a node
#                VM and checks its shared tools, update and remove; network makes a claude VM with
#                network policy and checks it reaches only its allowlist, its token stays on the
#                host, and the self-check, on relaunch too
#     --from     every section, with from, and every base image feature: the longest. from makes VMs
#                from node:24 (Debian 12, with node, python and claude) and alpine:3 (musl tools,
#                Docker on OpenRC), each with every feature, and builds base from ubuntu:26.04, with a
#                docker that isn't running
#     --only     just these sections; SECTIONS below
#     --clean    delete the sandbox's cached images and tool images first (default: reuse them
#                between runs)
# The sandbox: Lima home ~/.lt-e2e-go (or $DEVVM_E2E_LIMA_HOME), state under /tmp/devvm-go-e2e. It refuses
# to run against ~/.lima or devvm's own Lima home, deletes only the VMs it created, and uses no real
# tokens. devvm is built from this checkout into the sandbox, with its files next to it.
set -uo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sections_all="images project clone tools network from"
usage() { echo "usage: tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]]  (sections: $sections_all)" >&2; exit 2; }
clean=false sel="images project clone tools network"
while (( $# )); do
  case $1 in
    --clean) clean=true ;;
    --from)  sel=$sections_all ;;
    --only)  [[ -n "${2:-}" ]] || usage; sel=$(tr ',' ' ' <<<"$2"); shift
             for s in $sel; do [[ " $sections_all " == *" $s "* ]] || usage; done ;;
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
mkdir -p "$LIMA_HOME"
for real in "$HOME/.lima" "$HOME/.local/share/devvm/lima"; do
  real=$(cd "$real" 2>/dev/null && pwd -P || echo "$real")
  [[ "$(cd "$LIMA_HOME" && pwd -P)" != "$real" ]] || { echo "e2e: refusing to use your real Lima home ($real)" >&2; exit 1; }
done

# devvm runs from a copy, built from this checkout, so the test can edit its files (and editing the
# repo mid-run is harmless).
src=$work/src
rm -rf "$src"; mkdir -p "$src" "$XDG_CONFIG_HOME/devvm/layers" "$XDG_DATA_HOME"
gocmd=(go); command -v go >/dev/null || gocmd=(mise exec -- go)
(cd "$root" && CGO_ENABLED=0 "${gocmd[@]}" build -o "$src/devvm" ./cmd/devvm) || { echo "e2e: couldn't build devvm" >&2; exit 1; }
cp "$root/base.yaml" "$src/"; cp -R "$root/layers" "$root/features" "$root/tools" "$root/guest" "$root/oci" "$src/"
# The features are in base's key, so only a run with from (which checks them) builds them; every
# other selection shares one smaller base.
if want from; then for f in "$root"/features/*.sh; do basename "$f" .sh; done > "$XDG_CONFIG_HOME/devvm/base-features"
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

created=()   # VMs this run made; the only ones it deletes
cleanup() {
  local n
  for n in ${created[@]+"${created[@]}"}; do
    limactl delete -f "$n" >/dev/null 2>&1
    rm -f "$images/${n#devvm-img-}" "$vms/$n"
  done
  rm -f "$layers"/e2e*.sh
  [[ "$(uname -s)" != Darwin ]] || security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a github >/dev/null 2>&1
  [[ -z ${listener:-} ]] || kill "$listener" 2>/dev/null
  [[ -z ${agent_pid:-} ]] || kill "$agent_pid" 2>/dev/null
  rm -rf "$src" "$work/ssh" "$work/net-home" "$work/agent-home" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/export.list" "$XDG_DATA_HOME/devvm/exports" "$work/nodocker" "$work/docker-calls" "$work/from-home"
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
  for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
  rm -rf "$images" "$share"
fi

if want tools; then
# What a user gets: upstream's newest versions, each download with its checksum (rust: rustup checks),
# resolved by devvm's pinned mise, downloaded and checked on the first run.
section "shared tool versions"
march=arm64; [[ "$(uname -m)" != x86_64 ]] || march=x64   # the lock's platform: mise's name for the arch
ntools=$(awk -F' *= *' '/^\[tools\]/ { s = 1; next } /^\[/ { s = 0 } s && NF == 2' "$src/tools/mise.toml" | wc -l)
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
printf '#!/bin/bash\nset -eu\necho %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" > "$fixture"
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

# A failed script keeps its image's VM, stopped and without a record, for inspection.
printf '#!/bin/bash\nexit 3\n' > "$layers/e2efail.sh"
DEVVM_MESSAGES=names "$D" build e2efail </dev/null >"$work/fail.log" 2>&1; rc=$?
kx=$(sed -n 's/^==> building_image image=e2efail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
[[ -z $kx ]] || created+=("devvm-img-$kx")
check "a failed script keeps its VM, stopped, and says how to look inside" bash -c "
  [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: script_failed lima_home=.* script=e2efail\.sh vm=devvm-img-$kx$' '$work/fail.log' &&
  [[ \$(status_of devvm-img-$kx) == Stopped ]] && [[ ! -e '$images/$kx' ]]"
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
launch "$p" 'pwd; echo e2e-shell-ran' --no-network-policy base
check "devvm creates the VM from base, and opens a shell in the folder" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx '$p' '$work/last.log' && grep -qx e2e-shell-ran '$work/last.log' &&
  grep -qx image=base '$vms/$vm' && grep -qx net=off '$vms/$vm' && grep -qx 'dir=$p' '$vms/$vm'"
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
check "stopped, devvm starts it again" bash -c "limactl stop $vm >/dev/null 2>&1 && [[ \$(status_of $vm) == Stopped ]] &&
  cd '$p' && printf 'echo e2e-again\n' | '$D' >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] && grep -qx e2e-again '$work/last.log'"
check "running, devvm opens it as it is" bash -c "cd '$p' && printf 'echo e2e-running\n' | '$D' 2>&1 | grep -qx e2e-running"
check "restart, and its new host keys are pinned" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
if [[ "$(uname -s)" == Darwin ]]; then
  check "a swapped .git is reported" bash -c "limactl shell --workdir / $vm bash -c 'cd $p && mv .git .git-old && git init -q' </dev/null;
    cd '$p' && printf 'exit\n' | '$D' 2>&1 | grep -qF \"\$(m git_replaced git=$p/.git | head -1)\""
  rm -rf "$p/.git"; mv "$p/.git-old" "$p/.git"
  # (virtiofs shows the VM the restored .git after a moment.) Locks are set while it's stopped.
  check "a submodule git folder the VM adds is reported, and locked" bash -c "limactl shell --workdir / $vm bash -c 'cd $p &&
      for i in 1 2 3 4 5 6 7 8 9 10; do git rev-parse >/dev/null 2>&1 && break; sleep 1; done && git init -q vmsub &&
      git -C vmsub -c user.email=e@e -c user.name=e commit -q --allow-empty -m s &&
      git update-index --add --cacheinfo 160000,\$(git -C vmsub rev-parse HEAD),vmsub' </dev/null &&
    limactl stop $vm >/dev/null 2>&1 && cd '$p' && printf 'exit\n' | '$D' 2>&1 | grep -qF \"\$(m git_embedded_new git=$p/vmsub/.git | head -1)\" &&
    ls -lOd '$p/vmsub/.git/config' | grep -q uchg"
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
check "rm exports its agents' history, without their logins, then deletes it" bash -c "cd '$a' && HOME='$ah' '$D' rm </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && f=\$(ls '$XDG_DATA_HOME'/devvm/exports/$vm-*.tar.gz) && tar -tzf \"\$f\" > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list'"
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
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" --network-policy --no-git-protect claude+codex >"$work/last.log" 2>&1)
vm=$(vm_of "$n"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
salt=$(sed -n 's/^proxy_salt=//p' "$vms/$vm" 2>/dev/null)
events=$XDG_DATA_HOME/devvm/logs/$vm.jsonl
# pgrep -f looks for [_]proxy: Linux's pgrep also matches the check's own shell, whose command line
# holds the pattern.
check "--network-policy: the VM runs behind its proxy, and the self-check passed" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ' >/dev/null"
check "ssh devvm-net reaches it, confined, its host key checked" bash -c "
  grep -q '^Host devvm-net $vm\$' '$XDG_CONFIG_HOME/devvm/ssh_config' &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]]"
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
if [[ "$(uname -s)" == Darwin ]]; then
  # The Keychain itself, under this run's service (a fake token, deleted at the end).
  security add-generic-password -U -s "$DEVVM_SECRET_SERVICE" -a github -w ghp_E2EFAKE >/dev/null 2>&1
  check "devvm reads a token from the Keychain, never to a terminal" bash -c "
    [[ \$('$D' secret get github | cat) == ghp_E2EFAKE ]]"
  security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a github >/dev/null 2>&1
fi
check "the self-check passes on the VM devvm started confined" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT= CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck'"
check "a VM started outside devvm is unenforced, and the self-check stops it" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT='net_unenforced_stop vm=$vm' CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck' && [[ \$(status_of '$vm') == Stopped ]] && grep -qx net_check=unenforced '$vms/$vm'"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$n' && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(status_of '$vm') == Running ]] && grep -qx net_check=ok '$vms/$vm'"
check "ls shows its policy on, and unenforced once started outside devvm" bash -c "
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +on +lima ' &&
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +unenforced +lima '"
check "started outside devvm again: without a terminal, devvm stops it and opens nothing" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! printf 'echo e2e-opened\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -q e2e-opened '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
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
