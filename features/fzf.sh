#!/bin/bash
# fzf: fuzzy Ctrl-R history and Ctrl-T file search
set -euxo pipefail
if command -v apk >/dev/null; then sudo apk add --no-cache fzf
else sudo DEBIAN_FRONTEND=noninteractive apt-get install -y fzf; fi
# fzf 0.48+ prints its own bash setup; older ones (Debian 12 has 0.38) ship it as files.
if fzf --bash >/dev/null 2>&1; then
  echo 'eval "$(fzf --bash)"' > ~/.bashrc.d/fzf.sh
else
  f=/usr/share/doc/fzf/examples/key-bindings.bash
  [[ -f $f ]] || { echo "fzf: no 'fzf --bash' and no $f" >&2; exit 1; }
  printf '. %s\n' "$f" /usr/share/bash-completion/completions/fzf > ~/.bashrc.d/fzf.sh
fi
