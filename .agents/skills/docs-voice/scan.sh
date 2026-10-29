#!/usr/bin/env bash
# Flags likely writing tropes in Markdown prose (see tropes.md). Code blocks,
# code spans, link targets, shortcodes, HTML comments and front matter are
# skipped. Hits are candidates to judge, not errors.
#
#   scan.sh FILE...        a table: words, sentences, then hits per pattern
#   scan.sh -v FILE...     each hit: file:line: [pattern] the line as written
#   scan.sh -p NAME FILE.. -v for one pattern only
set -euo pipefail

verbose=0
only=
while [ $# -gt 0 ]; do
	case $1 in
	-v) verbose=1 ;;
	-p)
		verbose=1
		only=${2:?-p needs a pattern name}
		shift
		;;
	-h | --help)
		sed -n '2,8s/^# \{0,1\}//p' "$0"
		exit 0
		;;
	--) shift && break ;;
	*) break ;;
	esac
	shift
done
[ $# -gt 0 ] || {
	echo "usage: scan.sh [-v | -p NAME] FILE..." >&2
	exit 2
}

# name<TAB>flags<TAB>regex: flags w (whole words), i (any case), - (none).
patterns='paren	-	\([^)]*[a-z][^)]*\)
colon	-	[a-z)`*]: [a-z]
semicolon	-	;
boldlabel	-	^[[:space:]]*([-*]|[0-9]+\.) \*\*[^*]+\*\*
negative	wi	isn.t|aren.t|doesn.t|don.t|won.t|can.t|never|nothing|neither|none|no one
too	-	 too[,.;:)]
say	i	, say\)|\(say,|, say,
dash	-	—| --
aiword	wi	additionally|crucial|delve|delves|enhance|enhances|fostering|garner|intricate|intricacies|landscape|meticulous|meticulously|pivotal|robust|seamless|seamlessly|showcase|showcases|tapestry|testament|underscore|underscores|vibrant|leverage|leverages|streamline|streamlines|effortless|effortlessly|comprehensive|powerful|essential|ensure|ensures|ensuring
copula	wi	serves as|stands as|acts as|functions as|operates as|boasts|plays a [a-z]+ role
notbut	i	not (just|only|merely)|rather than|it.s not [^,.]*, it.s
ingtail	i	, (ensuring|highlighting|underscoring|emphasizing|reflecting|allowing|enabling|letting|making)
summary	i	^(in (short|summary|conclusion)|overall|in other words|that said|put simply)[ ,]
hedge	wi	generally|typically|usually|essentially|basically|simply|just|really|actually|of course
short	-	(^|[.!?] )(- |[0-9]+\. )?[^ .!?:;#|0-9-][^ .!?:;]*( [^ .!?:;]+){0,5}[.!?]( |$)'

# The prose of one file, line numbers kept: lines that aren't prose are blank.
prose() {
	awk '
		NR == 1 && /^---$/ { front = 1; print ""; next }
		front { if (/^---$/) front = 0; print ""; next }
		/^[[:space:]]*(```|~~~)/ { fence = !fence; print ""; next }
		fence { print ""; next }
		/^[[:space:]]*\|[[:space:]:-]*\|[[:space:]:|-]*$/ { print ""; next }
		{
			gsub(/<!--.*-->/, "")
			gsub(/\{\{[<%][^}]*[>%]\}\}/, "")
			gsub(/`[^`]*`/, "`x`")
			gsub(/\]\([^)]*\)/, "]")
			print
		}' "$1"
}

hits() { # flags regex: matching lines (-n) or matches (-o) from stdin
	local flags=$1 re=$2 mode=$3 opts=-E
	case $flags in *w*) opts="$opts -w" ;; esac
	case $flags in *i*) opts="$opts -i" ;; esac
	# shellcheck disable=SC2086 # opts are flags, split on purpose
	grep $opts "$mode" -- "$re" || true
}

if [ "$verbose" = 0 ]; then
	printf '%-44s %6s %5s' file words sents
	while IFS='	' read -r name _ _; do printf ' %5.5s' "$name"; done <<EOF
$patterns
EOF
	printf '\n'
fi

for f in "$@"; do
	text=$(prose "$f")
	if [ "$verbose" = 1 ]; then
		while IFS='	' read -r name flags re; do
			[ -z "$only" ] || [ "$only" = "$name" ] || continue
			printf '%s\n' "$text" | hits "$flags" "$re" -n |
				while IFS=: read -r n _; do
					printf '%s:%s: [%s] %s\n' "$f" "$n" "$name" "$(sed -n "${n}p" "$f")"
				done
		done <<EOF
$patterns
EOF
		continue
	fi
	words=$(printf '%s\n' "$text" | wc -w | tr -d ' ')
	sents=$(printf '%s\n' "$text" | grep -Eo '[a-z)`][.!?]( |$)' | wc -l | tr -d ' ')
	printf '%-44s %6s %5s' "$f" "$words" "$sents"
	while IFS='	' read -r name flags re; do
		printf ' %5s' "$(printf '%s\n' "$text" | hits "$flags" "$re" -o | wc -l | tr -d ' ')"
	done <<EOF
$patterns
EOF
	printf '\n'
done
