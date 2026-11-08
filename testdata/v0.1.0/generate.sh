#!/bin/bash
# generate.sh: makes this folder's fixture again, from devvm 0.1.0 itself:
# a checkout of the v0.1.0 tag in a temporary folder, its CLI test
# harness (sandbox, stubs, test build) running generate.txtar with a
# release's version, then the files the run left copied here. Only ever
# run it against v0.1.0: a newer devvm's state goes in its own folder.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd -P)
repo=$(git -C "$here" rev-parse --show-toplevel)
tag=v0.1.0
work=$(mktemp -d /tmp/devvm-fixture.XXXXXX)
src=$work/src
cleanup() {
	git -C "$repo" worktree remove --force "$src" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

git -C "$repo" worktree add -q --detach "$src" "$tag"
# A release's version, as GoReleaser sets it, on top of the test build.
sed -i.orig 's|^const testBuildFlag = "\(.*\)"$|const testBuildFlag = "\1 -X github.com/devvmhq/devvm/internal/cli.version=0.1.0"|' "$src/internal/cli/script_test.go"
grep -q 'cli.version=0.1.0' "$src/internal/cli/script_test.go"
cp "$here/generate.txtar" "$src/testdata/script/fixture_v010.txtar"
(cd "$src" && mise trust -q . && CGO_ENABLED=0 mise exec -- go test ./internal/cli -run 'TestScript/^fixture_v010$' -count=1)

out=$src/fixture-out
dst=$here/$(cd "$src" && mise exec -- go env GOOS)
rm -rf "$dst"
mkdir "$dst"
cp "$out/root" "$dst/root"
# What its --json printed.
cp -R "$out/json" "$dst/json"
# devvm's folders, without the run logs (times and the sandbox's paths).
cp -R "$out/data" "$out/config" "$dst/"
rm -rf "$dst/data/devvm/logs" "$dst/data/devvm/vms/"*/logs
# The stubs' own state, so today's stubs know the VMs: Lima's home and
# the stub's state.json, without the VM's files (a clone's .git, which
# git can't keep in this repo).
cp -R "$out/lima" "$dst/lima"
mkdir "$dst/stub"
cp "$out/stub/state.json" "$dst/stub/state.json"
# Each file's and folder's mode, which git doesn't keep: the CLI test's
# fixture command sets them again.
case $(uname) in
Darwin) mode=(-f '%Lp %N') ;;
*) mode=(-c '%a %n') ;;
esac
(cd "$dst" && find data config lima stub json -print0 | xargs -0 stat "${mode[@]}") | LC_ALL=C sort -k2 >"$dst/modes"
echo "generated in $dst"
