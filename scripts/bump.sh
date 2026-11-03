#!/bin/bash
# Update the versions and checksums pinned in assets/ (layers/base.sh, layers/docker.sh, layers/nix.sh, features/*.sh,
# shared-tools/build.sh, oci/build.sh), internal/tools/mise.go (devvm's host mise) and
# internal/ubuntu/ubuntu.go (Ubuntu's container and cloud images).
#   scripts/bump.sh --check     show current and latest for every pin; change nothing
#   scripts/bump.sh [tool...]   update the given tools (default: all) and print a summary
# Tools: apt docker nix direnv nix-direnv mise rustup crane omb eza tealdeer
#   apt: base's package snapshot, Ubuntu's container image smolvm's base starts from, and
#        Ubuntu's cloud image Lima's base starts from
#   mise: base's, and the one devvm runs on the host to resolve the shared tools' versions
# The shared tools themselves (node, claude, ...) aren't pinned: devvm resolves them per user.
# Needs curl and jq, and gpgv for apt. Checksums come from upstream's published sums where there are
# any; otherwise they're computed from the download (said so in the output). Edited scripts change
# image keys, so the affected images rebuild on next use: test with `devvm build <image>`, then
# `devvm gc`.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
all="apt docker nix direnv nix-direnv mise rustup crane omb eza tealdeer"

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
# sha256 FILE: its SHA-256, with sha256sum (coreutils) or shasum (macOS's; Perl's, which Fedora
# lacks), whichever there is.
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -c1-64; }
sha_of() { fetch -o "$tmp/dl" "$1"; sha256 "$tmp/dl"; rm -f "$tmp/dl"; }

# get FILE VAR / set_var FILE VAR VALUE: a `VAR=value` line in a pinned script (a trailing comment
# stays); in a .go file, the quoted value of `const VAR = "…"` (get_const) or of a map entry
# `"VAR": "…",`.
get() { sed -n "s/^$2=\([^ ]*\).*/\1/p" "$root/$1"; }
get_const() { sed -n "s/^const $2 = \"\(.*\)\"$/\1/p" "$root/$1"; }
# valid VAR VALUE: VALUE comes from upstream and lands in a script that runs (some on the host), so
# only the shape a pin can have: a checksum, a commit, an image digest, a snapshot date or a version.
# Else exit.
valid() {
  local re='^[0-9A-Za-z][0-9A-Za-z.+~_-]*$'
  case $1 in
    *_sha256*) re='^[0-9a-f]{64}$' ;;
    *_commit) re='^[0-9a-f]{40}$' ;;
    *_digest) re='^sha256:[0-9a-f]{64}$' ;;
    *_release) re='^release-[0-9]{8}$' ;;
    apt_snapshot|*_built) re='^[0-9]{8}T[0-9]{6}Z$' ;;
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

# ubuntu_image sets ubuntu_digest, the digest of Ubuntu's container image (Docker Hub's ubuntu:26.04: its
# index, hashed here from what the registry sent), and ubuntu_built, when the newest of its linux/arm64
# and linux/amd64 images was built (their org.opencontainers.image.created, as YYYYMMDDTHHMMSSZ).
ubuntu_image() {
  local hub=https://registry-1.docker.io/v2/library/ubuntu auth a m c t
  auth="Authorization: Bearer $(fetch "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/ubuntu:pull" | jq -r .token)"
  fetch -H "$auth" -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json' \
    "$hub/manifests/26.04" > "$tmp/ubuntu-index"
  ubuntu_digest=sha256:$(sha256 "$tmp/ubuntu-index")
  ubuntu_built=""
  for a in arm64 amd64; do
    m=$(jq -r --arg a "$a" '[.manifests[] | select(.platform.os == "linux" and .platform.architecture == $a)][0].digest // ""' "$tmp/ubuntu-index")
    valid ubuntu_digest "$m"
    c=$(fetch -H "$auth" -H 'Accept: application/vnd.oci.image.manifest.v1+json' "$hub/manifests/$m" |
      jq -r '.annotations["org.opencontainers.image.created"] // ""')
    # 2026-10-25T10:30:32.309136+00:00 -> 20261025T103032Z; UTC only.
    t=$c
    if [[ $c =~ ^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]+)?(Z|\+00:00)$ ]]; then
      t=${BASH_REMATCH[1]}${BASH_REMATCH[2]}${BASH_REMATCH[3]}T${BASH_REMATCH[4]}${BASH_REMATCH[5]}${BASH_REMATCH[6]}Z
    fi
    valid ubuntu_built "$t"
    if [[ $t > $ubuntu_built ]]; then ubuntu_built=$t; fi
  done
}

# ubuntu_cloud DATE sets cloud_release, the newest dated release of Ubuntu's cloud image (Lima's base)
# before DATE (YYYYMMDD), and cloud_arm64 and cloud_amd64, its images' SHA-256s from its SHA256SUMS,
# once gpgv finds them signed by Ubuntu's cloud image key (assets/layers/ubuntu-cloud.gpg), in a home
# of its own so no other key counts. Else exit.
ubuntu_cloud() {
  local u=https://cloud-images.ubuntu.com/releases/resolute r
  command -v gpgv >/dev/null || { echo "bump: needs gpgv (brew install gnupg, or apt install gpgv)" >&2; exit 1; }
  fetch "$u/" > "$tmp/cloud-releases"
  cloud_release=""
  while read -r r; do
    if [[ ${r#release-} < $1 ]]; then cloud_release=$r; break; fi
  done < <(grep -o 'release-[0-9]\{8\}' "$tmp/cloud-releases" | sort -ru || true)
  if [[ -z $cloud_release ]]; then
    echo "bump: Ubuntu has no cloud image release before the snapshot's date $1" >&2
    exit 1
  fi
  valid cloud_release "$cloud_release"
  fetch "$u/$cloud_release/SHA256SUMS" > "$tmp/SHA256SUMS"
  fetch "$u/$cloud_release/SHA256SUMS.gpg" > "$tmp/SHA256SUMS.gpg"
  rm -rf "$tmp/gnupg"; mkdir -m 700 "$tmp/gnupg"
  if ! GNUPGHOME="$tmp/gnupg" gpgv --keyring "$root/assets/layers/ubuntu-cloud.gpg" \
      "$tmp/SHA256SUMS.gpg" "$tmp/SHA256SUMS" >"$tmp/gpgv.out" 2>&1; then
    cat "$tmp/gpgv.out" >&2
    echo "bump: Ubuntu's checksums for $cloud_release aren't signed by Ubuntu's cloud image key" >&2
    exit 1
  fi
  cloud_arm64=$(cloud_sum arm64); cloud_amd64=$(cloud_sum amd64)
}
# cloud_sum ARCH: the arch's image's checksum in the fetched SHA256SUMS.
cloud_sum() {
  awk -v f="ubuntu-26.04-server-cloudimg-$1.img" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/SHA256SUMS"
}

# The package snapshot base builds from, today's (UTC); Ubuntu's container image smolvm's base starts
# from, by digest; and Ubuntu's cloud image Lima's base starts from, by dated release and
# SHA-256. They move together, and only to images made before the snapshot: apt can't install what
# depends on the exact version of a package newer than the snapshot's.
bump_apt() {
  local f=assets/layers/base.sh g=internal/ubuntu/ubuntu.go cur new img rel want=false
  cur=$(get $f apt_snapshot); new=$(date -u +%Y%m%dT000000Z)
  img=$(get_const $g ContainerDigest); rel=$(get_const $g CloudRelease)
  ubuntu_image
  ubuntu_cloud "${new:0:8}"
  if report apt "$cur" "$new"; then want=true; fi
  if report ubuntu "${img:7:12}" "${ubuntu_digest:7:12}"; then want=true; fi
  if report ubuntu-cloud "$rel" "$cloud_release"; then want=true; fi
  $want || return 0
  if [[ ! $ubuntu_built < $new ]]; then
    echo "bump: Ubuntu's image was built at $ubuntu_built, after today's snapshot $new: bump apt tomorrow" >&2
    exit 1
  fi
  set_var $f apt_snapshot "$new"
  set_var $g ContainerDigest "$ubuntu_digest" ubuntu_digest
  set_var $g CloudRelease "$cloud_release" cloud_release
  set_var $g CloudSHA256Arm64 "$cloud_arm64" cloud_sha256_arm64
  set_var $g CloudSHA256Amd64 "$cloud_amd64" cloud_sha256_amd64
}

# Docker's packages: the newest of each in its repository for base's Ubuntu release, and its signing key.
bump_docker() {
  local f=assets/layers/docker.sh idx p var new sha
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

# Nix's release tarball for the nix layer: the version nixos.org's installer fetches, with the SHA-256s
# Nix publishes next to each tarball. Nix's newer releases come with another installer
# (artifacts.nixos.org): when nixos.org's no longer names a tarball, this says so, and the layer's pins
# change shape.
bump_nix() {
  local f=assets/layers/nix.sh cur new u
  cur=$(get $f nix_version)
  new=$(fetch https://nixos.org/nix/install | sed -n 's|^ *path=[0-9a-z]*/nix-\([0-9.]*\)-x86_64-linux\.tar\.xz$|\1|p' | head -1)
  [[ -n $new ]] || { echo "bump: nixos.org's installer names no nix-VERSION-x86_64-linux.tar.xz: see the layer's pins" >&2; exit 1; }
  report nix "$cur" "$new" || return 0
  u="https://releases.nixos.org/nix/nix-$new/nix-$new"
  pin3 $f nix "$new" "$(fetch "$u-aarch64-linux.tar.xz.sha256" | cut -c1-64)" "$(fetch "$u-x86_64-linux.tar.xz.sha256" | cut -c1-64)"
}

bump_direnv() {
  local f=assets/layers/nix.sh cur new u
  cur=$(get $f direnv_version); new=$(gh_latest direnv/direnv); new=${new#v}
  report direnv "$cur" "$new" || return 0
  echo "            (checksums computed from the downloads)"
  u="https://github.com/direnv/direnv/releases/download/v$new"
  pin3 $f direnv "$new" "$(sha_of "$u/direnv.linux-arm64")" "$(sha_of "$u/direnv.linux-amd64")"
}

# nix-direnv: one script (direnvrc) at the release's tag.
bump_nix-direnv() {
  local f=assets/layers/nix.sh cur new
  cur=$(get $f nix_direnv_version); new=$(gh_latest nix-community/nix-direnv); new=${new#v}
  report nix-direnv "$cur" "$new" || return 0
  echo "            (checksum computed from the download)"
  set_var $f nix_direnv_version "$new"
  set_var $f nix_direnv_sha256 "$(sha_of "https://raw.githubusercontent.com/nix-community/nix-direnv/$new/direnvrc")"
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
