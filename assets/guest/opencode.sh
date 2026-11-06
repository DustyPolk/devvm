#!/bin/bash
# Brings opencode 2's logins in this VM in line with devvm's store, run in a login shell (opencode on
# PATH) by sync.sh on every launch and by devvm agent login --api-key. Its input is the store's
# opencode logins as opencode auth export prints them, with placeholders alone, under devvm's IDs
# (cred_devvm_PROVIDER). opencode 2 keeps its logins in its database, so once it has made one, an
# input that differs from the last this VM's database got, or one whose IDs the database no longer
# holds, is imported: devvm's entries of the last
# are logged out first, then each new one is imported on its own and made its provider's active
# credential, and the user's own credentials stay in the database. Until opencode has made its
# database there is nothing to import into, and nothing is recorded, so the first launch after it
# imports them all. Each provider whose import fails is printed as a line "devvm-opencode-failed
# PROVIDER", and then the script exits non-zero, recording nothing, so the next launch tries again;
# any other failure prints the command that failed to stderr.
#
# It runs in a login shell, whose ~/.bash_logout (Debian's clear_console, which fails without a
# terminal) would end it with its own status under set -e, so the work runs in a subshell and the
# shell itself exits with the subshell's status.
(
set -euo pipefail
trap 'echo "opencode.sh: line $LINENO: $BASH_COMMAND failed" >&2' ERR
new=$(cat)
last=~/.local/state/devvm/opencode-import.json
command -v opencode >/dev/null || exit 0
set -- ~/.local/share/opencode/opencode*.db
[[ -e $1 ]] || exit 0
# The record alone isn't enough: a database made again (deleted, or replaced by a history import)
# no longer holds devvm's entries, so every ID of the input must still be there. Only the IDs leave
# opencode auth export's output.
have=$(opencode auth export </dev/null 2>/dev/null | jq -r '.[].id' 2>/dev/null) || have=
missing=0
while read -r id; do
  grep -qxF -- "$id" <<<"$have" || missing=1
done < <(jq -r '.[].id' <<<"$new")
[[ "$new" != "$(cat "$last" 2>/dev/null)" || $missing == 1 ]] || exit 0
if [[ -f $last ]]; then
  { jq -r '.[] | "\(.integrationID) \(.id)"' "$last" 2>/dev/null || true; } | while read -r provider id; do
    opencode auth logout "$provider" "$id" </dev/null >/dev/null 2>&1 || true
  done
fi
# One entry at a time, so a refused one names its provider and the others still go in. The store's
# login is the one made last, so each provider then uses devvm's entry, as every VM does.
failed=0
while read -r entry; do
  provider=$(jq -r .integrationID <<<"$entry")
  if ! opencode auth import <<<"[$entry]" >&2; then
    echo "devvm-opencode-failed $provider"
    failed=1
    continue
  fi
  opencode auth switch "$provider" "$(jq -r .id <<<"$entry")" </dev/null >/dev/null 2>&1 || true
done < <(jq -c '.[]' <<<"$new")
[[ $failed == 0 ]] || exit 1
mkdir -p "${last%/*}"
printf '%s\n' "$new" > "$last.devvm"
mv -f "$last.devvm" "$last"
)
exit $?
