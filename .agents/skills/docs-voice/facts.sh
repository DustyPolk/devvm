#!/usr/bin/env bash
# Compares a rewritten Markdown file's facts with a git revision's (HEAD by
# default): code spans, link targets, headings and numbers in prose. Prints
# what's only in the old version (-) or only in the new (+); nothing when
# they match. Code blocks are compared whole.
#
#   facts.sh FILE [REV]
set -euo pipefail

file=${1:?usage: facts.sh FILE [REV]}
rev=${2:-HEAD}

facts() {
	awk '
		/^[[:space:]]*(```|~~~)/ { fence = !fence; print "block: " $0; next }
		fence { print "block: " $0; next }
		/^#+ / { print "heading: " $0 }
		{
			line = $0
			while (match(line, /`[^`]+`/)) {
				print "code: " substr(line, RSTART, RLENGTH)
				line = substr(line, RSTART + RLENGTH)
			}
			line = $0
			while (match(line, /\]\([^)]+\)/)) {
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

old=$(git show "$rev:./$file" | facts)
new=$(facts <"$file")
# Unique per side, counts ignored: a fact said once or twice is the same fact.
LC_ALL=C comm -3 <(printf '%s\n' "$old" | LC_ALL=C sort -u) <(printf '%s\n' "$new" | LC_ALL=C sort -u) |
	awk -F '\t' '$1 != "" { print "- " $1; next } { print "+ " $2 }'
