#!/bin/bash
# Runs inside a project VM on every devvm launch, from a staging dir the host sends over:
#   tools.sh     installed as /usr/local/sbin/devvm-tools
#   tools        the VM's shared tool images, "FILE TOOL VERSION" lines; tools.sh mounts them
set -euo pipefail
s=$(cd "$(dirname "$0")" && pwd)

# Shared tools: mount this VM's images (at boot too, by devvm-tools.service), then tell mise which
# versions to use. /etc/mise is mise's system config: the user's own (~/.config/mise) comes first.
cmp -s "$s/tools.sh" /usr/local/sbin/devvm-tools 2>/dev/null || sudo install -D -m 755 "$s/tools.sh" /usr/local/sbin/devvm-tools
cmp -s "$s/tools" /etc/devvm/tools 2>/dev/null || sudo install -D -m 644 "$s/tools" /etc/devvm/tools
sudo /usr/local/sbin/devvm-tools
{
  echo '[tools]'
  while read -r _ tool version; do [[ -z $tool || $tool == rust ]] || echo "$tool = \"$version\""; done < "$s/tools"
  cat <<'T'

[settings]
experimental = true
shared_install_dirs = ["/opt/devvm/tools"]
# A project's version files choose versions, as their own tools would; a version that isn't shared
# installs into the VM on first use. Not go: go.mod's go line is a minimum, and Go's own GOTOOLCHAIN
# reads its toolchain line.
idiomatic_version_file_enable_tools = ["node", "python", "bun", "pnpm"]
auto_install = true
not_found_auto_install = true
python.compile = false
# mise points GOBIN into Go's install, which is read-only: go install goes to ~/go/bin.
go_set_gobin = false
# The project folder is the user's own: trust its mise.toml without asking.
trusted_config_paths = ["/"]
T
} > "$s/mise.toml"
cmp -s "$s/mise.toml" /etc/mise/config.toml 2>/dev/null || sudo install -D -m 644 "$s/mise.toml" /etc/mise/config.toml
# Links into tools this VM no longer has (devvm remove, devvm update).
for l in ~/.rustup/toolchains/* ~/.local/bin/claude; do
  [[ -L $l && ! -e $l && "$(readlink "$l")" == /opt/devvm/tools/* ]] && rm "$l"
done
# Rust: rustup in ~/.cargo (installed once, from the tool image) with the shared toolchain linked in.
# Other toolchains (rust-toolchain.toml) install next to it.
rust=$(awk '$2 == "rust" { print $3 }' "$s/tools")
if [[ -n $rust ]]; then
  [[ -x ~/.cargo/bin/rustup ]] ||
    /opt/devvm/tools/rust/"$rust"/rustup-init -y -q --no-modify-path --default-toolchain none >/dev/null
  mkdir -p ~/.rustup/toolchains
  t=~/.rustup/toolchains/$rust-$(uname -m)-unknown-linux-$(command -v apk >/dev/null && echo musl || echo gnu)
  [[ -L $t || -e $t ]] || ln -s /opt/devvm/tools/rust/"$rust"/toolchain "$t"
  ~/.cargo/bin/rustup default >/dev/null 2>&1 || ~/.cargo/bin/rustup default "$rust" >/dev/null
fi
# Go's telemetry off (it keeps the setting in ~/.config/go).
go=$(awk '$2 == "go" { print $3 }' "$s/tools")
if [[ -n $go && "$(cat ~/.config/go/telemetry/mode 2>/dev/null)" != off* ]]; then
  /opt/devvm/tools/go/"$go"/bin/go telemetry off
fi
# Claude Code checks for itself at ~/.local/bin/claude.
claude=$(awk '$2 == "claude" { print $3 }' "$s/tools")
if [[ -n $claude ]]; then mkdir -p ~/.local/bin; ln -sfn /opt/devvm/tools/claude/"$claude"/claude ~/.local/bin/claude; fi
mise reshim 2>/dev/null || true
# A command over ssh runs bash non-interactively, and bash then reads ~/.bashrc instead of BASH_ENV:
# its first line, before the "not interactive: return". Features (oh-my-bash) and users replace ~/.bashrc.
if [[ "$(head -1 ~/.bashrc 2>/dev/null)" != '. /etc/profile.d/devvm-tools.sh' ]]; then
  { echo '. /etc/profile.d/devvm-tools.sh'; cat ~/.bashrc 2>/dev/null || true; } > ~/.bashrc.tmp && mv ~/.bashrc.tmp ~/.bashrc
fi
