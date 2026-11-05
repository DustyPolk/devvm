#!/bin/bash
# Claude Code's state for one project folder in a VM, sourced by sync.sh. Claude Code keeps a folder's
# sessions in ~/.claude/projects/SLUG, SLUG its path as devvm computes it (claudeSlug), and its trust
# and MCP servers under the path itself in ~/.claude.json's "projects". Only the VM's own files are
# changed, as the VM's user.

# claude_json_project_move FILE FROM TO: FILE's "projects" entry for FROM becomes TO's, unless TO has
# one of its own, which then stays as it is, as does FROM's. A FILE that isn't JSON stays as it is.
claude_json_project_move() {
  local f=$1 from=$2 to=$3
  [[ -s $f && ! -L $f ]] || return 0
  if jq --arg o "$from" --arg n "$to" '
      if (.projects | type) == "object" and (.projects | has($o)) and (.projects | has($n) | not)
      then .projects[$n] = .projects[$o] | del(.projects[$o]) else . end' "$f" > "$f.tmp"; then
    mv "$f.tmp" "$f"
  else
    rm -f "$f.tmp"
    echo "devvm: $f isn't plain JSON, so its settings for $from stay under that path" >&2
  fi
}

# claude_project_moved FROM TO FROM_SLUG TO_SLUG: Claude Code's sessions and settings of the project
# folder that was at FROM, and is at TO now, follow it. The sessions' folder is renamed, or, when TO
# has sessions of its own, each of FROM's that TO's folder doesn't have moves into it. Links are left
# as they are. Running it again changes nothing.
claude_project_moved() {
  local from=$1 to=$2 old=$HOME/.claude/projects/$3 new=$HOME/.claude/projects/$4 e
  if [[ -d $old && ! -L $old ]]; then
    if [[ ! -e $new && ! -L $new ]]; then
      mv "$old" "$new"
    elif [[ -d $new && ! -L $new ]]; then
      for e in "$old"/* "$old"/.[!.]* "$old"/..?*; do
        [[ -e $e || -L $e ]] || continue
        [[ -e $new/${e##*/} || -L $new/${e##*/} ]] || mv "$e" "$new/"
      done
      rmdir "$old" 2>/dev/null || true
    fi
  fi
  claude_json_project_move "$HOME/.claude.json" "$from" "$to"
}
