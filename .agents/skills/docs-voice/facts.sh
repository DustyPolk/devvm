#!/usr/bin/env bash
# Compares a rewritten Markdown file's facts with a git revision's (HEAD by
# default): code spans, link targets, headings and numbers in prose. Prints
# what's only in the old version (-) or only in the new (+); nothing when
# they match. Code blocks are compared whole.
#
# A .toml file is compared as messages (locales/en.toml): each fact starts
# with its message's ID, so grep can pick a group of messages. A message's
# facts are its {{.fields}}, devvm's commands, code-like words (flags, paths,
# file names, NAME=value, ENV_VARS) and numbers.
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

# The facts of a message file (locales/en.toml) on stdin, each after its
# message's ID: the message itself, its {{.fields}}, devvm's commands (as the
# CLI reference names them), code-like words (flags, paths, file names,
# NAME=value, ENV_VARS) and numbers in its other and one strings.
message_facts() {
	local commands
	commands=$(find "$(git rev-parse --show-toplevel)/$docs/cli" -name 'devvm_*.md' |
		sed 's|.*/||; s|\.md$||' | tr '\n' ',')
	awk -v commands="$commands" '
		BEGIN { n = split(commands, c, ","); for (i = 1; i <= n; i++) command[c[i]] = 1 }
		function bare(w) { sub(/^[("'\''`]+/, "", w); sub(/[.,:;!?)"'\''`]+$/, "", w); return w }
		function code(w) {
			return w ~ /^--?[A-Za-z]/ || w ~ /[\/_=~@]/ || w ~ /[A-Za-z0-9]\.[A-Za-z]/ ||
				w ~ /^\.[A-Za-z]/ || w ~ /^[A-Z][A-Z0-9]{3,}$/ || w ~ /^\[.\/.\]$/
		}
		/^\[/ { id = substr($0, 2, length($0) - 2); print id ": message"; next }
		/^(other|one) = "/ {
			s = $0
			sub(/^(other|one) = "/, "", s)
			sub(/"[[:space:]]*$/, "", s)
			gsub(/\\n/, " ", s)
			gsub(/\\"/, "\"", s)
			while (match(s, /\{\{[^}]*\}\}/)) {
				print id ": field: " substr(s, RSTART, RLENGTH)
				s = substr(s, 1, RSTART - 1) "{}" substr(s, RSTART + RLENGTH)
			}
			n = split(s, w, " ")
			for (i = 1; i <= n; i++) {
				word = bare(w[i])
				if (word == "devvm") {
					best = ""; path = "devvm"
					for (j = i + 1; j <= i + 3 && j <= n; j++) {
						path = path "_" bare(w[j])
						if (path in command) best = path
					}
					if (best != "") { gsub(/_/, " ", best); print id ": command: " best }
				} else if (code(word)) {
					print id ": code: " word
				} else if (word ~ /^[0-9][0-9.,]*%?$/) {
					unit = bare(w[i + 1])
					if (unit ~ /^(KiB|MiB|GiB|KB|MB|GB|s|ms|minutes?|seconds?|days?)$/) word = word " " unit
					print id ": number: " word
				}
			}
		}' | LC_ALL=C sort
}

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

fn=facts
case $file in *.toml) fn=message_facts ;; esac
old=$(git show "$rev:./$file" | $fn 0)
new=$($fn 0 <"$file")
# Unique per side, counts ignored: a fact said once or twice is the same fact.
LC_ALL=C comm -3 <(printf '%s\n' "$old" | LC_ALL=C sort -u) <(printf '%s\n' "$new" | LC_ALL=C sort -u) |
	awk -F '\t' '$1 != "" { print "- " $1; next } { print "+ " $2 }'
