#!/bin/bash
# Run the unit and CLI tests with the race detector, then hold each package's coverage to its
# baseline (scripts/coverage.sh).
#   scripts/test.sh          fail when a package's coverage drops below its baseline (mise run test)
#   scripts/test.sh --raise  raise the baselines instead (mise run coverage)
# Tests get a throwaway HOME and XDG folders, so none can reach the real ones, no SSH agent, and
# no color settings of your own. Each package gets 30 minutes, not go test's 10, so the CLI tests
# finish on a busy machine instead of timing out. Coverage comes from each package's own tests and
# from the CLI tests' devvm, built with -cover for every package (CLI_GOCOVERDIR, a folder for each script), merged with
# go tool covdata. The baselines are in testdata/coverage/.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode=check
case ${1:-} in
  '') ;;
  --raise) mode=raise ;;
  *) echo "usage: scripts/test.sh [--raise]" >&2; exit 2 ;;
esac

GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" GOPATH="$(go env GOPATH)"
export GOCACHE GOMODCACHE GOPATH
home="$(mktemp -d "${TMPDIR:-/tmp}/devvm-test.XXXXXX")"
trap 'rm -rf "$home"' EXIT
cov="$home/coverage"
mkdir -p "$cov/unit" "$cov/cli" "$cov/merged"
unset SSH_AUTH_SOCK NO_COLOR FORCE_COLOR CLICOLOR_FORCE
cd "$root"
HOME="$home" XDG_DATA_HOME="$home/.local/share" XDG_CONFIG_HOME="$home/.config" XDG_CACHE_HOME="$home/.cache" \
  CLI_GOCOVERDIR="$cov/cli" \
  gotestsum -- -race -cover -timeout 30m ./... -args -test.gocoverdir="$cov/unit"
# The end-to-end runner's own tests, which need no VM: tests/e2e builds only with the e2e tag, and
# its TestE2E, the run itself, is left out.
HOME="$home" XDG_DATA_HOME="$home/.local/share" XDG_CONFIG_HOME="$home/.config" XDG_CACHE_HOME="$home/.cache" \
  gotestsum -- -race -tags e2e -run '^TestHarness' ./tests/e2e
# Each CLI test script writes its devvm's coverage to a folder of its own under $cov/cli, so
# devvm runs in parallel scripts don't race on one meta-data file. A script that never ran
# devvm leaves its folder empty, and it's left out.
inputs="$cov/unit"
for dir in "$cov/cli"/*/; do
  dir="${dir%/}"
  if [ -d "$dir" ] && [ -n "$(ls -A "$dir")" ]; then inputs="$inputs,$dir"; fi
done
go tool covdata merge -i="$inputs" -o="$cov/merged"
go tool covdata percent -i="$cov/merged" >"$cov/percents"
scripts/coverage.sh "$mode" "$cov/percents"
