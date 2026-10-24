#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--only SECTION[,SECTION...]]
#     (default)  every section: minutes, more on a first run (base's build, and Ubuntu's cloud image
#                to download)
#     --only     just these sections; SECTIONS below
#     --clean    delete the sandbox's cached images first (default: reuse them between runs)
# The sandbox: Lima home ~/.lt-e2e-go (or $DEVVM_E2E_LIMA_HOME), state under /tmp/devvm-go-e2e. It refuses
# to run against ~/.lima or devvm's own Lima home, deletes only the VMs it created, and uses no real
# tokens. devvm is built from this checkout into the sandbox, with its files next to it.
set -uo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sections_all="images"
usage() { echo "usage: tests/e2e.sh [--clean] [--only SECTION[,SECTION...]]  (sections: $sections_all)" >&2; exit 2; }
clean=false sel=$sections_all
while (( $# )); do
  case $1 in
    --clean) clean=true ;;
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
export DEVVM_SECRET_SERVICE=devvm-e2e
unset DEVVM_MESSAGES DEVVM_VERBOSE DEVVM_UBUNTU_URL CLAUDE_CODE_OAUTH_TOKEN
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
cp "$root/base.yaml" "$src/"; cp -R "$root/layers" "$src/"
: > "$XDG_CONFIG_HOME/devvm/base-features"
D=$src/devvm
images=$XDG_DATA_HOME/devvm/images   # the image records, one per key
layers=$XDG_CONFIG_HOME/devvm/layers
# m NAME [name=value...]: a message's text, from devvm, so checks never repeat its wording. Exported
# for the checks' own shells.
m() { "$D" _msg "$@"; }
export -f m; export D

created=()   # VMs this run made; the only ones it deletes
cleanup() {
  local n
  for n in ${created[@]+"${created[@]}"}; do
    limactl delete -f "$n" >/dev/null 2>&1
    rm -f "$images/${n#devvm-img-}"
  done
  rm -f "$layers"/e2e*.sh
  rm -rf "$src"
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
section() { echo; echo "== $*"; }
quiet() { "$@" </dev/null >"$work/last.log" 2>&1; }
# keys_of IMAGE: the keys of the images recorded under that name, one per line.
keys_of() { grep -lx "name=$1" "$images"/* 2>/dev/null | while read -r f; do basename "$f"; done; }
# owned IMAGE: the images by that name are this run's, deleted at the end.
owned() { local k; for k in $(keys_of "$1"); do created+=("devvm-img-$k"); done; }
status_of() { limactl list --format '{{.Status}}' "$1" 2>/dev/null; }
export -f keys_of status_of; export images

if $clean; then
  section "cleaning cached images"
  for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
  rm -rf "$images"
fi

if want images; then
section "images"
# base stays cached between runs (--clean deletes it); the fixture layers' images are this run's.
check "build base" quiet "$D" build base
kb=$(keys_of base | head -1)
check "base is a stopped image, recorded with Ubuntu's image and its C library" bash -c "
  [[ -n '$kb' && \$(status_of devvm-img-$kb) == Stopped ]] &&
  grep -qx 'chain=base=$kb' '$images/$kb' && grep -qx 'libc=gnu' '$images/$kb' &&
  grep -qE '^digest=https://cloud-images\.ubuntu\.com/.*/release-[0-9]{8}/.*\.img sha256:[0-9a-f]{64}$' '$images/$kb'"
check "base mounts nothing from the host" bash -c "[[ -z \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' devvm-img-$kb) ]]"
check "building base again reuses it" bash -c "out=\$('$D' build base </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=base key=$kb)\" <<<\"\$out\""

fixture=$layers/e2efix.sh
token=e2e-fixture-ran-$$-$(date +%s)   # this run's: the run logs keep earlier runs'
printf '#!/bin/bash\nset -eu\necho %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" > "$fixture"
check "build the fixture layer on base" quiet "$D" build e2efix
owned e2efix
kf=$(keys_of e2efix | head -1)
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

# A script error can end a section early without failing a check: compare with the last complete run
# (same options). Fewer checks than last time fails the run, and the timings aren't kept.
short=""
if [[ $total != "?" ]] && (( ncheck < total )); then
  short="ran $ncheck checks, but the last complete run ran $total: a section ended early (look for errors above)"
else
  mv "$timings.new" "$timings"   # a complete run: the next one estimates from it
fi
echo
echo "e2e: $pass passed, $fail failed in $(mins $((SECONDS - started)))${failed[*]+: ${failed[*]}}"
[[ -z $short ]] || echo "e2e: FAILED: $short"
(( fail == 0 )) && [[ -z $short ]]
