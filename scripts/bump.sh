#!/bin/bash
# Update the versions and checksums pinned in assets/ (layers/base.sh, features/*.sh,
# shared-tools/build.sh, oci/build.sh) and internal/tools/mise.go (devvm's host mise).
#   scripts/bump.sh --check     show current and latest for every pin; change nothing
#   scripts/bump.sh [tool...]   update the given tools (default: all) and print a summary
# Tools: apt docker mise rustup crane omb eza tealdeer
#   mise: base's, and the one devvm runs on the host to resolve the shared tools' versions
# The shared tools themselves (node, claude, ...) aren't pinned: devvm resolves them per user.
# Needs curl and jq. Checksums come from upstream's published sums where there are any; otherwise
# they're computed from the download (said so in the output). Edited scripts change image keys, so
# the affected images rebuild on next use: test with `devvm build <image>`, then `devvm gc`.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
all="apt docker mise rustup crane omb eza tealdeer"

check=false
if [[ ${1:-} == --check ]]; then check=true; shift; fi
tools=${*:-$all}
for t in $tools; do [[ " $all " == *" $t "* ]] || { echo "bump: unknown tool '$t' (have: $all)" >&2; exit 1; }; done
for c in curl jq; do command -v "$c" >/dev/null || { echo "bump: needs $c" >&2; exit 1; }; done

summary=()
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fetch() { curl -fsSL --retry 3 "$@"; }
gh_latest() { fetch "https://api.github.com/repos/$1/releases/latest" | jq -r .tag_name; }
gh_head() { fetch "https://api.github.com/repos/$1/commits/${2:-master}" | jq -r .sha; }
sha_of() { fetch -o "$tmp/dl" "$1"; shasum -a 256 "$tmp/dl" | cut -c1-64; rm -f "$tmp/dl"; }

# get FILE VAR / set_var FILE VAR VALUE: a `VAR=value` line in a pinned script (a trailing comment
# stays); in a .go file, the quoted value of `const VAR = "…"` or of a map entry `"VAR": "…",`.
get() { sed -n "s/^$2=\([^ ]*\).*/\1/p" "$root/$1"; }
# valid VAR VALUE: VALUE comes from upstream and lands in a script that runs (some on the host), so
# only the shape a pin can have: a checksum, a commit, a snapshot date or a version. Else exit.
valid() {
  local re='^[0-9A-Za-z][0-9A-Za-z.+~_-]*$'
  case $1 in
    *_sha256*) re='^[0-9a-f]{64}$' ;;
    *_commit) re='^[0-9a-f]{40}$' ;;
    apt_snapshot) re='^[0-9]{8}T[0-9]{6}Z$' ;;
  esac
  [[ $2 =~ $re ]] || { printf 'bump: upstream gave %s an unexpected value: %q\n' "$1" "$2" >&2; exit 1; }
}
# set_var stages the edit (checked as KIND, else as VAR); write_staged makes them all once every
# tool's values are in and checked, so a bad or missing one leaves every file as it was.
staged=()
set_var() { valid "${4:-$2}" "$3"; staged+=("$1" "$2" "$3"); }
write_staged() {
  local i
  for (( i = 0; i < ${#staged[@]}; i += 3 )); do
    # The value as it is (awk -v would read backslash escapes, sub() an &).
    if [[ ${staged[i]} == *.go ]]; then
      N=${staged[i + 1]} V=${staged[i + 2]} awk '
        { l = $0; sub(/^[ \t]*/, "", l) }
        index(l, "const " ENVIRON["N"] " = \"") == 1 || index(l, "\"" ENVIRON["N"] "\":") == 1 {
          e = length($0); while (substr($0, e, 1) != "\"") e--
          b = e - 1; while (substr($0, b, 1) != "\"") b--
          $0 = substr($0, 1, b) ENVIRON["V"] substr($0, e)
        }
        { print }' "$root/${staged[i]}" > "$tmp/edit"
    else
      N=${staged[i + 1]} V=${staged[i + 2]} awk 'index($0, ENVIRON["N"] "=") == 1 { s = index($0, " "); $0 = ENVIRON["N"] "=" ENVIRON["V"] (s ? substr($0, s) : "") } { print }' \
        "$root/${staged[i]}" > "$tmp/edit"
    fi
    cat "$tmp/edit" > "$root/${staged[i]}"
  done
}

# report TOOL CURRENT LATEST: print it; true when an update is wanted (not --check, and different).
report() {
  if [[ $2 == "$3" ]]; then printf '  %-9s %-14s up to date\n' "$1" "$2"; return 1; fi
  # The latest comes from upstream: no control characters on the terminal.
  printf '  %-9s %-14s -> %s\n' "$1" "$2" "$(LC_ALL=C tr -d '\000-\037\177' <<<"$3")"
  $check && return 1
  summary+=("$1: $2 -> $3")
}

# A version plus per-arch checksums in one script.
pin3() {   # pin3 FILE PREFIX VERSION SHA_ARM64 SHA_AMD64
  set_var "$1" "${2}_version" "$3"
  set_var "$1" "${2}_sha256_arm64" "$4"
  set_var "$1" "${2}_sha256_amd64" "$5"
}

# The package snapshot base builds from: today's (UTC).
bump_apt() {
  local f=assets/layers/base.sh cur new
  cur=$(get $f apt_snapshot); new=$(date -u +%Y%m%dT000000Z)
  report apt "$cur" "$new" || return 0
  set_var $f apt_snapshot "$new"
}

# Docker's packages: the newest of each in its repository for base's Ubuntu release, and its signing key.
bump_docker() {
  local f=assets/layers/base.sh idx p var new sha
  idx=$(fetch https://download.docker.com/linux/ubuntu/dists/resolute/stable/binary-arm64/Packages)
  for p in docker-ce:docker containerd.io:containerd docker-buildx-plugin:buildx \
      docker-compose-plugin:compose docker-model-plugin:model; do
    var=${p#*:}_version
    # "5:29.9.0-1~ubuntu.26.04~resolute" -> 29.9.0
    new=$(awk -v p="${p%%:*}" '/^Package:/ { n = $2 } /^Version:/ && n == p { v = $2; sub(/^[0-9]+:/, "", v); sub(/-[^-]*$/, "", v); print v }' <<<"$idx" |
      sort -V | tail -1)
    if report "${p#*:}" "$(get $f "$var")" "$new"; then set_var $f "$var" "$new"; fi
  done
  sha=$(sha_of https://download.docker.com/linux/ubuntu/gpg)
  if report docker-gpg "$(get $f docker_gpg_sha256 | cut -c1-12)" "$(cut -c1-12 <<<"$sha")"; then
    echo "            (Docker's signing key changed: check it before committing)"
    set_var $f docker_gpg_sha256 "$sha"
  fi
}

# mise in base, and on the host (devvm's host mise, the same version: it writes the lock base's mise
# installs from), from the release's published checksums.
bump_mise() {
  local f=assets/layers/base.sh cur new sums
  cur=$(get $f mise_version); new=$(gh_latest jdx/mise); new=${new#v}
  report mise "$cur" "$new" || return 0
  sums=$(fetch "https://github.com/jdx/mise/releases/download/v$new/SHASUMS256.txt")
  pin3 $f mise "$new" "$(awk '$2 ~ /-linux-arm64$/ { print $1 }' <<<"$sums")" \
    "$(awk '$2 ~ /-linux-x64$/ { print $1 }' <<<"$sums")"
  set_var $f mise_sha256_arm64_musl "$(awk '$2 ~ /-linux-arm64-musl$/ { print $1 }' <<<"$sums")"
  set_var $f mise_sha256_amd64_musl "$(awk '$2 ~ /-linux-x64-musl$/ { print $1 }' <<<"$sums")"
  local g=internal/tools/mise.go os a
  set_var $g miseVersion "$new" mise_version
  for os in macos linux; do
    for a in arm64 x64; do
      set_var $g "$os-$a" "$(awk -v s="-$os-$a" '$2 ~ s "$" { print $1 }' <<<"$sums")" mise_sha256
    done
  done
}

bump_rustup() {
  local f=assets/shared-tools/build.sh cur new u
  cur=$(get $f rustup_version)
  new=$(fetch https://static.rust-lang.org/rustup/release-stable.toml | sed -n "s/^version = '\(.*\)'/\1/p")
  report rustup "$cur" "$new" || return 0
  u="https://static.rust-lang.org/rustup/archive/$new"
  pin3 $f rustup "$new" "$(fetch "$u/aarch64-unknown-linux-gnu/rustup-init.sha256" | cut -c1-64)" \
    "$(fetch "$u/x86_64-unknown-linux-gnu/rustup-init.sha256" | cut -c1-64)"
  set_var $f rustup_sha256_arm64_musl "$(fetch "$u/aarch64-unknown-linux-musl/rustup-init.sha256" | cut -c1-64)"
  set_var $f rustup_sha256_amd64_musl "$(fetch "$u/x86_64-unknown-linux-musl/rustup-init.sha256" | cut -c1-64)"
}

bump_eza() {
  local f=assets/features/cli-tools.sh cur new u
  cur=$(get $f eza_version); new=$(gh_latest eza-community/eza); new=${new#v}
  report eza "$cur" "$new" || return 0
  echo "            (checksums computed from the downloads)"
  u="https://github.com/eza-community/eza/releases/download/v$new"
  pin3 $f eza "$new" "$(sha_of "$u/eza_aarch64-unknown-linux-gnu.tar.gz")" "$(sha_of "$u/eza_x86_64-unknown-linux-musl.tar.gz")"
}

bump_tealdeer() {   # publishes a .sha256 next to each binary
  local f=assets/features/cli-tools.sh cur new u
  cur=$(get $f tealdeer_version); new=$(gh_latest tealdeer-rs/tealdeer); new=${new#v}
  report tealdeer "$cur" "$new" || return 0
  u="https://github.com/tealdeer-rs/tealdeer/releases/download/v$new"
  pin3 $f tealdeer "$new" "$(fetch "$u/tealdeer-linux-aarch64-musl.sha256" | cut -c1-64)" \
    "$(fetch "$u/tealdeer-linux-x86_64-musl.sha256" | cut -c1-64)"
}

# crane (go-containerregistry), which --from pulls images with, from the release's checksums.txt.
bump_crane() {
  local f=assets/oci/build.sh cur new sums
  cur=$(get $f crane_version); new=$(gh_latest google/go-containerregistry); new=${new#v}
  report crane "$cur" "$new" || return 0
  sums=$(fetch "https://github.com/google/go-containerregistry/releases/download/v$new/checksums.txt")
  pin3 $f crane "$new" "$(awk '$2 == "go-containerregistry_Linux_arm64.tar.gz" { print $1 }' <<<"$sums")" \
    "$(awk '$2 == "go-containerregistry_Linux_x86_64.tar.gz" { print $1 }' <<<"$sums")"
}

bump_omb() {
  local f=assets/features/omb.sh cur new
  cur=$(get $f omb_commit); new=$(gh_head ohmybash/oh-my-bash)
  report omb "${cur:0:12}" "${new:0:12}" || return 0
  set_var $f omb_commit "$new"
}

for t in $tools; do "bump_$t"; done
write_staged

$check && exit 0
if (( ${#summary[@]} == 0 )); then echo "==> everything is up to date"; exit 0; fi
echo
echo "Bump pinned versions"
echo
printf -- '- %s\n' "${summary[@]}"
echo
echo "==> the edited images and tools rebuild on next use. Test: devvm build <image>; then devvm gc"
