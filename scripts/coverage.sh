#!/bin/bash
# Hold each package's test coverage to its baseline, in testdata/coverage/<os>.txt (darwin,
# linux), as each system runs other code and other tests.
#   scripts/coverage.sh check PERCENTS  fail when a package is more than a point below its baseline,
#                                       or has none
#   scripts/coverage.sh raise PERCENTS  raise each baseline a package went above, add new packages'
#                                       and drop gone ones'; a package more than a point below its
#                                       baseline fails it, and nothing is written
# A package within a point below its baseline passes and keeps it: the same tests can cover a
# little less from run to run (timing, the tools installed), and a baseline never goes down.
# COVERAGE_TOLERANCE sets that margin, in points (1 by default).
# PERCENTS is `go tool covdata percent`'s output, one line per package, such as
#   github.com/w3cj/devvm/internal/msg	coverage: 85.3% of statements
# scripts/test.sh makes it from the unit tests and the CLI tests together. The baseline has one
# line per package, its folder and its percentage; a baseline never goes down. A system with no
# baseline yet is said so and not checked. COVERAGE_BASELINE names another baseline (for tests).
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
baseline="${COVERAGE_BASELINE:-$root/testdata/coverage/$os.txt}"
module="github.com/w3cj/devvm"
tolerance="${COVERAGE_TOLERANCE:-1}"

usage() { echo "usage: scripts/coverage.sh check|raise PERCENTS" >&2; exit 2; }
[[ $# -eq 2 ]] || usage
mode=$1 percents=$2
[[ $mode == check || $mode == raise ]] || usage
[[ -f $percents ]] || { echo "coverage: no $percents" >&2; exit 1; }
if [[ ! -f $baseline && $mode == check ]]; then
  echo "coverage: no baseline for $os, so it isn't checked: run mise run coverage here to record one" >&2
  exit 0
fi
mkdir -p "$(dirname "$baseline")"
[[ -f $baseline ]] || : >"$baseline"

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# The packages' percentages as "folder percent" lines, sorted: the module's own is ".", and a
# package without statements is left out.
measured() {
  awk -v module="$module" '
    # A package without statements has no "coverage:" and no line end of its own, so the next
    # package can follow it on its line: the package is the word before "coverage:".
    {
      for (i = 2; i < NF; i++) if ($i == "coverage:") break
      if (i >= NF || $(i + 1) !~ /^[0-9.]+%$/) next
      pkg = $(i - 1)
      pct = $(i + 1)
      if (pkg == module) pkg = "."
      else if (index(pkg, module "/") == 1) pkg = substr(pkg, length(module) + 2)
      else next
      print pkg, substr(pct, 1, length(pct) - 1)
    }' "$percents" | LC_ALL=C sort
}

# compare reads the baseline, then the measured lines, and prints what each package does: "below
# PKG HAVE WANT" (more than the tolerance below), "new PKG HAVE", "gone PKG WANT", "up PKG HAVE
# WANT", or "same PKG WANT" (at its baseline or within the tolerance below it, so it keeps it).
compare() {
  awk -v tol="$tolerance" '
    FILENAME == ARGV[1] { if ($0 !~ /^#/ && NF == 2) want[$1] = $2; next }
    {
      have[$1] = $2
      if (!($1 in want)) print "new", $1, $2
      else if ($2 + tol < want[$1] + 0) print "below", $1, $2, want[$1]
      else if ($2 + 0 > want[$1] + 0) print "up", $1, $2, want[$1]
      else print "same", $1, want[$1]
    }
    END { for (p in want) if (!(p in have)) print "gone", p, want[p] }
  ' "$baseline" -
}

measured | compare >"$tmp"
status=0
while read -r what pkg a b; do
  case $what in
    below) echo "coverage: $pkg is at $a%, more than $tolerance below its baseline of $b%" >&2; status=1 ;;
    new) [[ $mode == raise ]] || { echo "coverage: $pkg has no baseline: run mise run coverage" >&2; status=1; } ;;
    gone) [[ $mode == raise ]] || { echo "coverage: $pkg has a baseline but wasn't measured: run mise run coverage" >&2; status=1; } ;;
  esac
done <"$tmp"
[[ $status -eq 0 ]] || exit 1
[[ $mode == raise ]] || exit 0

{
  echo "# Each package's test coverage, in percent: the unit tests and the CLI tests together."
  echo "# mise run test fails when a package drops below its line; mise run coverage raises them."
  awk '$1 != "gone" { print $2, $3 }' "$tmp" | LC_ALL=C sort
} >"$baseline"
while read -r what pkg a b; do
  case $what in
    up) echo "coverage: $pkg raised from $b% to $a%" ;;
    new) echo "coverage: $pkg added at $a%" ;;
    gone) echo "coverage: $pkg dropped, as it wasn't measured" ;;
  esac
done <"$tmp"
