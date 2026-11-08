#!/bin/bash
# devvm-clip: xclip and wl-paste in a VM with clipboard images (devvm clipboard images), which devvm's
# setup links to it where the VM has neither. It answers what agents such as Claude Code and pi run
# when you paste: the types on the clipboard, and its image as a PNG, which it reads from devvm on
# the host at the address in /etc/devvm/clipboard. Anything else (text, copying, the primary
# selection) goes to the VM's own command when it has one, and fails otherwise.
conf=/etc/devvm/clipboard
name=$(basename "$0")
args=("$@")

# others: run the VM's own command with the arguments as given, else fail.
others() {
  local d
  for d in /usr/bin /bin; do
    [[ -x "$d/$name" ]] && exec "$d/$name" ${args[@]+"${args[@]}"}
  done
  echo "$name: in this VM it reads the image on your clipboard (image/png) alone" >&2
  exit 1
}

# ask REQUEST: devvm's answer, on stdout: the types (types) or the image (image/png); nothing when the
# clipboard has no image or devvm doesn't answer. A subshell, so a connection that fails ends it alone.
ask() (
  read -r addr port < "$conf" || exit 0
  exec 3<>"/dev/tcp/$addr/$port" || exit 0
  printf '%s\n' "$1" >&3
  cat <&3
) 2>/dev/null

# answer REQUEST MISSING: devvm's answer on stdout, or MISSING on stderr and exit 1 when it's empty.
answer() {
  local out
  out=$(mktemp) || exit 1
  ask "$1" > "$out"
  if [[ -s "$out" ]]; then
    cat "$out"
    rm -f "$out"
    exit 0
  fi
  rm -f "$out"
  echo "$2" >&2
  exit 1
}

case $name in
  xclip)
    out=false sel=clipboard target=""
    while [[ $# -gt 0 ]]; do
      case $1 in
        -o|-ou|-out) out=true ;;
        -t|-ta|-tar|-targ|-targe|-target) shift; target=${1:-} ;;
        -se|-sel|-sele|-selec|-select|-selecti|-selectio|-selection) shift; sel=${1:-} ;;
        *) others ;;
      esac
      shift
    done
    case $sel in c|cl|cli|clip|clipb|clipbo|clipboa|clipboar|clipboard) ;; *) others ;; esac
    $out || others
    case $target in
      TARGETS) answer types "Error: target TARGETS not available" ;;
      image/png) answer image/png "Error: target image/png not available" ;;
      *) others ;;
    esac
    ;;
  wl-paste)
    list=false type=""
    while [[ $# -gt 0 ]]; do
      case $1 in
        -l|--list-types) list=true ;;
        -n|--no-newline) ;;
        -t|--type) shift; type=${1:-} ;;
        --type=*) type=${1#--type=} ;;
        *) others ;;
      esac
      shift
    done
    if $list; then
      answer types "Nothing is copied"
    fi
    [[ $type == image/png ]] || others
    answer image/png "Clipboard content is not available as requested type \"image/png\""
    ;;
  *)
    others
    ;;
esac
