#!/usr/bin/env bash
# Compares a rewritten Markdown file's facts with a git revision's (HEAD by
# default): code spans, link targets, headings and numbers in prose. Prints
# what's only in the old version (-) or only in the new (+); nothing when
# they match. Code blocks are compared whole.
#
# --all compares the whole docs tree (docs/content/docs/**/*.md, except the
# generated cli/) at REV with the working tree, for changes that move text
# between pages. It compares code spans, numbers and code-block lines as
# multisets, so a fact said three times and then once shows as two fewer.
# It ignores front matter, headings and link targets, which move with pages.
# A summary line comes first; nothing is printed when the facts match.
#
#   facts.sh FILE [REV]
#   facts.sh --all [REV]
set -euo pipefail

usage='usage: facts.sh FILE [REV] | facts.sh --all [REV]'
file=${1:?$usage}
rev=${2:-HEAD}

facts() { # $1: 1 for --all's facts. Reads one Markdown file on stdin.
	awk -v all="$1" '
		all && NR == 1 && /^---$/ { front = 1; next }
		front { if (/^---$/) front = 0; next }
		/^[[:space:]]*(```|~~~)/ { fence = !fence; print "block: " $0; next }
		fence { print "block: " $0; next }
		/^#+ / { if (all) next; print "heading: " $0 }
		{
			line = $0
			while (match(line, /`[^`]+`/)) {
				print "code: " substr(line, RSTART, RLENGTH)
				line = substr(line, RSTART + RLENGTH)
			}
			line = $0
			while (!all && match(line, /\]\([^)]+\)/)) {
				print "link: " substr(line, RSTART + 2, RLENGTH - 3)
				line = substr(line, RSTART + RLENGTH)
			}
			line = $0
			gsub(/`[^`]+`/, "", line)
			gsub(/\]\([^)]+\)/, "]", line)
			while (match(line, /[0-9][0-9.,]*( ?(KiB|MiB|GiB|s|ms|%))?/)) {
				print "number: " substr(line, RSTART, RLENGTH)
				line = substr(line, RSTART + RLENGTH)
			}
		}' | LC_ALL=C sort
}

docs=docs/content/docs

# Every page's facts in one list, at a revision or (with no revision) in the
# working tree. Paths are relative to the repo's top folder.
tree_facts() {
	local page
	if [ $# -gt 0 ]; then
		git ls-tree -r --name-only "$1" -- "$docs" |
			while IFS= read -r page; do
				case $page in
				"$docs"/cli/*) ;;
				*.md) git show "$1:$page" | facts 1 ;;
				esac
			done
	else
		find "$docs" -name '*.md' ! -path "$docs/cli/*" |
			while IFS= read -r page; do facts 1 <"$page"; done
	fi | LC_ALL=C sort
}

if [ "$file" = --all ]; then
	cd "$(git rev-parse --show-toplevel)"
	git rev-parse --verify --quiet "$rev^{commit}" >/dev/null || {
		echo "facts.sh: no revision $rev" >&2
		exit 2
	}
	# comm keeps duplicates, so this compares the two lists as multisets.
	diff=$(LC_ALL=C comm -3 <(tree_facts "$rev") <(tree_facts) |
		awk -F '\t' '$1 != "" { print "- " $1; next } { print "+ " $2 }')
	[ -n "$diff" ] || exit 0
	printf '%s\n' "$diff" | awk -v rev="$rev" '
		/^-/ { old++ } /^\+/ { new++ }
		END { printf "%d only at %s, %d only in the working tree\n", old, rev, new }'
	# A fact repeated on one side prints once, with how many more copies.
	printf '%s\n' "$diff" | uniq -c |
		awk '{ n = $1; sub(/^ *[0-9]+ /, ""); print (n > 1 ? $0 " (" n "x)" : $0) }'
	exit 0
fi

old=$(git show "$rev:./$file" | facts 0)
new=$(facts 0 <"$file")
# Unique per side, counts ignored: a fact said once or twice is the same fact.
LC_ALL=C comm -3 <(printf '%s\n' "$old" | LC_ALL=C sort -u) <(printf '%s\n' "$new" | LC_ALL=C sort -u) |
	awk -F '\t' '$1 != "" { print "- " $1; next } { print "+ " $2 }'
