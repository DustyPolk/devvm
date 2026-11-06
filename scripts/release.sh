#!/bin/bash
# Build a release's files in dist/ with GoReleaser (.goreleaser.yaml), from git (only what's committed):
#   devvm-<version>-<os>-<arch>.tar.gz  the binary and the files it reads, for darwin and linux, arm64 and amd64
#   devvm-<version>.tar.gz              the source, which the Homebrew formula builds from
#   SHA256SUMS                          their checksums, which install.sh checks
#   SHA256SUMS.minisig                  SHA256SUMS signed with the release key (minisign), which install.sh checks
#   install.sh                          the install script, set to install this release and to trust the key
#   devvm.rb                            the Homebrew formula for w3cj/homebrew-tap
# Nothing is published. The version is VERSION's. The public key is packaging/minisign.pub (committed); the
# secret key is the maintainer's, at DEVVM_MINISIGN_KEY (default ~/.minisign/devvm.key), never in the repo.
#   scripts/release.sh          from the tag v<version>, which must exist and be signed (git tag -s)
#   scripts/release.sh --head   from HEAD, for testing (DEVVM_RELEASE_URL: where the source tarball will be;
#                               DEVVM_MINISIGN_KEY: a test key, else nothing is signed;
#                               DEVVM_MINISIGN_PUB: its public key, else packaging/minisign.pub)
# Needs goreleaser and minisign, pinned in mise.toml: mise exec -- scripts/release.sh.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
die() { echo "release: $*" >&2; exit 1; }

v=$(tr -d '[:space:]' < VERSION)
[[ $v =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "VERSION isn't x.y.z: '$v'"
ref=v$v
[[ ${1:-} != --head ]] || ref=HEAD
if [[ $ref == HEAD ]]; then obj=HEAD; else obj=$(git rev-parse -q --verify "refs/tags/$ref") || die "no tag $ref: tag the release first (git tag -s $ref)"; fi
# A release comes from a tag the maintainer signed, not just any tag with the right name. The tag
# object checked is the one built from: its commit is resolved once, and used for everything below.
[[ $ref == HEAD ]] || git verify-tag "$obj" >/dev/null 2>&1 ||
  die "tag $ref isn't signed by a key git trusts (make it with git tag -s; see docs/content/docs/contributing/maintaining.md)"
# And by the release signer, not just any key git trusts: its fingerprint is pinned in this checkout's
# packaging/release-signer (one per line; # comments), never the tagged commit's, which the tag brings.
if [[ $ref != HEAD ]]; then
  signers=$root/packaging/release-signer
  [[ -s $signers ]] || die "no packaging/release-signer: write the tag-signing key's fingerprint there (see docs/content/docs/contributing/maintaining.md)"
  raw=$(git verify-tag --raw "$obj" 2>&1) || true
  # GPG: VALIDSIG <signing key> … <primary key>; SSH: "… key SHA256:…".
  fps=$(awk '$1 == "[GNUPG:]" && $2 == "VALIDSIG" { print $3; print $NF }' <<<"$raw"
        grep -oE 'SHA256:[A-Za-z0-9+/]{43}' <<<"$raw" || true)
  fps=$(sort -u <<<"$fps")
  ok=false
  while IFS= read -r fp; do
    [[ -n $fp ]] && grep -v '^#' "$signers" | tr -d ' \t' | grep -xiF "$fp" >/dev/null && ok=true
  done <<<"$fps"
  $ok || die "tag $ref is signed by $(paste -sd' ' - <<<"$fps"), not a key in packaging/release-signer"
fi
commit=$(git rev-parse -q --verify "$obj^{commit}") || die "no commit for $ref"
[[ "$(git show "$commit:VERSION" | tr -d '[:space:]')" == "$v" ]] || die "$ref's VERSION isn't $v"
pubkey=$(git show "$commit:packaging/minisign.pub" 2>/dev/null | sed -n 2p) || true
# A test key's public key, for HEAD only: a release verifies with the committed one.
if [[ $ref == HEAD && -n ${DEVVM_MINISIGN_PUB:-} ]]; then
  pubkey=$(sed -n 2p "$DEVVM_MINISIGN_PUB") || die "can't read $DEVVM_MINISIGN_PUB"
fi
key=${DEVVM_MINISIGN_KEY:-$HOME/.minisign/devvm.key}
sign=true
if [[ $ref == HEAD && -z ${DEVVM_MINISIGN_KEY:-} ]]; then sign=false
else
  [[ -n $pubkey ]] || die "no packaging/minisign.pub in $ref: create the release key first (see docs/content/docs/contributing/maintaining.md)"
  [[ -f $key ]] || die "no release key at $key (DEVVM_MINISIGN_KEY)"
  command -v minisign >/dev/null || die "needs minisign to sign the release (mise exec -- scripts/release.sh)"
fi
git cat-file -e "$commit:install.sh" 2>/dev/null || die "no install.sh in $ref"
command -v goreleaser >/dev/null || die "needs goreleaser (mise exec -- scripts/release.sh)"
url=${DEVVM_RELEASE_URL:-https://github.com/devvmhq/devvm/releases/download/v$v}

# GoReleaser builds a clone of the commit, so only what's committed goes in, whatever this checkout
# holds. The version is VERSION's, for HEAD too (DEVVM_VERSION, in .goreleaser.yaml's snapshot).
out=$root/dist
tmp=$(mktemp -d "${TMPDIR:-/tmp}/devvm-release.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
src=$tmp/src
{ git clone -q --no-checkout "$root" "$src" && git -C "$src" checkout -q --detach "$commit"; } ||
  die "couldn't check out $ref"
snap=()
if [[ $ref == HEAD ]]; then snap=(--snapshot); else export GORELEASER_CURRENT_TAG=$ref; fi
(cd "$src" && DEVVM_VERSION=$v goreleaser release --clean --skip=publish ${snap[@]+"${snap[@]}"}) || die "goreleaser failed"
rm -rf "$out"
mv "$src/dist" "$out"

# The built binary for this host says the release's version (GoReleaser's own default sets one devvm
# doesn't read).
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in arm64 | aarch64) arch=arm64 ;; x86_64) arch=amd64 ;; *) arch=none ;; esac
for bin in "$out"/devvm*_"${os}_$arch"*/devvm; do
  [[ -x $bin ]] || continue
  got=$("$bin" --version) || die "$bin --version failed"
  [[ $got == "$v" ]] || die "$bin --version says '$got', not $v"
done

tarball=devvm-$v.tar.gz
sha=$(awk -v f="$tarball" '$2 == f { print $1 }' "$out/SHA256SUMS")
[[ $sha =~ ^[0-9a-f]{64}$ ]] || die "SHA256SUMS has no $tarball"
if $sign; then
  minisign -Sm "$out/SHA256SUMS" -s "$key" -t "devvm $v" || die "couldn't sign SHA256SUMS"
  minisign -Vm "$out/SHA256SUMS" -P "$pubkey" >/dev/null || die "SHA256SUMS.minisig doesn't verify with packaging/minisign.pub"
else
  pubkey=""   # an unsigned test build: its install.sh doesn't expect a signature
fi

git show "$commit:install.sh" |
  awk -v v="$v" -v k="$pubkey" '
    /^release_version=""$/ { $0 = "release_version=\"" v "\""; n++ }
    /^minisign_key=""$/ { $0 = "minisign_key=\"" k "\""; m++ }
    { print } END { exit !(n == 1 && m == 1) }' \
  > "$out/install.sh" || die "install.sh needs one release_version=\"\" and one minisign_key=\"\" line"
chmod 755 "$out/install.sh"
git show "$commit:packaging/homebrew/devvm.rb" |
  awk -v u="$url/$tarball" -v s="$sha" '{ gsub("@URL@", u); gsub("@SHA256@", s); print }' \
  > "$out/devvm.rb"

echo "==> dist/ for devvm $v (from $ref):"
(cd "$out" && ls -l devvm-*.tar.gz SHA256SUMS* install.sh devvm.rb 2>/dev/null) || true
$sign || echo "(not signed: a test build without DEVVM_MINISIGN_KEY)"
echo "Attach the archives, SHA256SUMS, SHA256SUMS.minisig and install.sh to the GitHub release v$v; devvm.rb goes in w3cj/homebrew-tap/Formula/."
