#!/bin/bash
# Runs inside a project VM on every devvm launch, from a staging dir the host sends over:
#   home/        agent config files and credentials, copied into ~
#   links        paths (relative to the host home) mounted read-only at the host path; linked into ~
#   host_home    the host's home path
#   claude.json  host ~/.claude.json, merged in without its per-project history (with network
#                policy, MCP servers' env values and headers are placeholders)
#   env          exports (LANG, CLAUDE_CODE_OAUTH_TOKEN, GH_TOKEN: placeholders with network policy),
#                installed as ~/.config/devvm/env
#   ca.pem       the proxy's CA, when network policy is on
#   vm           DEVVM_* host settings applied to every VM: hostname, timezone, locale, git identity,
#                and DEVVM_PROXY (the proxy URL when network policy is on, else empty)
#   tools        the VM's shared tool images, "FILE TOOL VERSION" lines; tools.sh mounts them
set -euo pipefail
s=$(cd "$(dirname "$0")" && pwd)
host_home=$(cat "$s/host_home")

# shellcheck source=/dev/null
. "$s/vm"
systemd=false; command -v systemctl >/dev/null && systemd=true   # else OpenRC (Alpine)
if [[ -n "$DEVVM_HOSTNAME" && "$(hostname)" != "$DEVVM_HOSTNAME" ]]; then
  if $systemd; then sudo hostnamectl set-hostname "$DEVVM_HOSTNAME"
  else echo "$DEVVM_HOSTNAME" | sudo tee /etc/hostname >/dev/null; sudo hostname "$DEVVM_HOSTNAME"; fi
  grep -q "[[:space:]]$DEVVM_HOSTNAME\$" /etc/hosts || echo "127.0.1.1 $DEVVM_HOSTNAME" | sudo tee -a /etc/hosts >/dev/null
fi
if [[ -n "$DEVVM_TZ" ]]; then
  if $systemd; then
    [[ "$(timedatectl show -p Timezone --value)" == "$DEVVM_TZ" ]] ||
      sudo timedatectl set-timezone "$DEVVM_TZ" || echo "devvm: unknown timezone $DEVVM_TZ" >&2
  elif [[ -f /usr/share/zoneinfo/$DEVVM_TZ ]]; then
    [[ "$(readlink /etc/localtime)" == "/usr/share/zoneinfo/$DEVVM_TZ" ]] || sudo ln -sfn "/usr/share/zoneinfo/$DEVVM_TZ" /etc/localtime
  fi
fi
# musl (Alpine) has no locales to generate.
if [[ -n "$DEVVM_LANG" ]] && command -v locale-gen >/dev/null && ! locale -a | grep -qix "${DEVVM_LANG/UTF-8/utf8}"; then
  sudo locale-gen "$DEVVM_LANG" >/dev/null
fi
# Images build from a dated package snapshot (layers/base.sh); the VM's own apt uses the live archive.
[[ ! -f /etc/apt/apt.conf.d/50devvm-snapshot ]] || sudo rm -f /etc/apt/apt.conf.d/50devvm-snapshot

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
# "(devvm) " before the prompt. Themes (oh-my-bash's) rebuild PS1 before each prompt from
# PROMPT_COMMAND, and ~/.bashrc.d loads after them, so this hook runs last: an array entry on bash 5.1+
# (where oh-my-bash appends its own), else appended to the string (where oh-my-bash prepends).
mkdir -p ~/.bashrc.d
cat > ~/.bashrc.d/devvm-prompt.sh <<'EOF'
_devvm_prompt() { [[ $PS1 == '(devvm) '* ]] || PS1="(devvm) $PS1"; }
if (( BASH_VERSINFO[0] * 100 + BASH_VERSINFO[1] >= 501 )); then
  PROMPT_COMMAND+=(_devvm_prompt)
else
  PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND;}_devvm_prompt"
fi
EOF
# After root touches a directory on a virtiofs mount, the VM sees it as root's for a few seconds, and
# git refuses the repo ("dubious ownership"). The VM has one user, so git may trust every directory.
git config --global --get-all safe.directory | grep -qx '\*' || git config --global --add safe.directory '*'
[[ -z "$DEVVM_GIT_NAME" ]] || git config --global user.name "$DEVVM_GIT_NAME"
[[ -z "$DEVVM_GIT_EMAIL" ]] || git config --global user.email "$DEVVM_GIT_EMAIL"

[[ -d "$s/home" ]] && cp -a "$s/home/." ~/

while read -r rel; do
  [[ -n "$rel" && -e "$host_home/$rel" ]] || continue
  mkdir -p "$(dirname ~/"$rel")"
  [[ -L ~/"$rel" ]] || rm -rf ~/"$rel"
  ln -sfn "$host_home/$rel" ~/"$rel"
done < "$s/links"

# merge_claude_json VM HOST COPIED: host settings win, but keep the VM's own per-folder state
# (trust, allowed tools). MCP servers: each of the host's replaces the VM's by that name whole (no
# field of an older copy survives, such as a real header from before network policy), the ones
# devvm copied last time (COPIED, a JSON list) that the host no longer has go, and servers added
# in the VM stay.
merge_claude_json() {
  local vm=$1 host=$2 copied=$3 prev='[]'
  [[ ! -s $copied ]] || prev=$(cat "$copied")
  [[ -s $vm ]] || echo '{}' > "$vm"
  jq -s --argjson prev "$prev" '
    ((.[0] | del(.mcpServers)) * (.[1] | del(.projects, .mcpServers)))
    + {mcpServers: (((.[0].mcpServers // {}) | with_entries(select(.key as $k | $prev | index($k) | not)))
                    + (.[1].mcpServers // {}))}' "$vm" "$host" > "$vm.tmp"
  mkdir -p "$(dirname "$copied")"
  jq '.mcpServers // {} | keys' "$host" > "$copied.tmp" && mv "$copied.tmp" "$copied"
  mv "$vm.tmp" "$vm"
}
if [[ -f "$s/claude.json" ]]; then
  merge_claude_json ~/.claude.json "$s/claude.json" ~/.local/state/devvm/claude-mcp-copied.json
fi

# VS Code over Remote-SSH (devvm code): its settings for this machine turn off what reaches the host
# beyond the editor itself: forwarding ports the VM's programs open (to the host's localhost, which
# devvm ports doesn't cover) and answering git's credential prompts with the host's GitHub sign-in.
# The VM can change this file back; it's here so ordinary use is safe.
vsc=~/.vscode-server/data/Machine/settings.json
vsc_off='{"remote.autoForwardPorts": false, "git.terminalAuthentication": false, "github.gitAuthentication": false}'
mkdir -p "$(dirname "$vsc")"
if [[ ! -s $vsc ]]; then
  echo "$vsc_off" | jq . > "$vsc"
elif jq -e 'type == "object"' "$vsc" >/dev/null 2>&1; then
  jq --argjson off "$vsc_off" '. + $off' "$vsc" > "$vsc.tmp" && mv "$vsc.tmp" "$vsc"
else
  echo "devvm: $vsc isn't plain JSON, so VS Code's port forwarding and git sign-in stay as they are there" >&2
fi

# Codex has no env-var opt-out: append the off switches unless the (host-copied) config sets them.
if command -v codex >/dev/null; then
  mkdir -p ~/.codex && touch ~/.codex/config.toml
  grep -q '^\[analytics\]' ~/.codex/config.toml || printf '\n[analytics]\nenabled = false\n' >> ~/.codex/config.toml
  grep -q '^\[otel\]' ~/.codex/config.toml || printf '\n[otel]\nmetrics_exporter = "none"\n' >> ~/.codex/config.toml
fi

# Network policy (DEVVM_PROXY set): Lima's env: already puts the proxy variables in /etc/environment;
# point the tools that ignore them at the proxy too. When it's off, undo all of it.
proxy_block() {   # proxy_block FILE [CONTENT]: replace (or remove) devvm's block in FILE
  local f=$1
  [[ -f "$f" ]] || touch "$f"
  sed -i '/^# devvm-proxy begin/,/^# devvm-proxy end/d' "$f"
  [[ -z "${2:-}" ]] || printf '# devvm-proxy begin\n%s\n# devvm-proxy end\n' "$2" >> "$f"
}
docker_proxies() {   # docker_proxies CONFIG [sudo]: set (or remove) the proxy for containers
  local c=$1 tmp as=()
  [[ -z "${2:-}" ]] || as=(sudo)
  if ! ${as[@]+"${as[@]}"} test -f "$c"; then
    [[ -n "${DEVVM_PROXY:-}" ]] || return 0
    ${as[@]+"${as[@]}"} mkdir -p "$(dirname "$c")"
    echo '{}' | ${as[@]+"${as[@]}"} tee "$c" >/dev/null
  fi
  tmp=$(mktemp)
  if [[ -n "${DEVVM_PROXY:-}" ]]; then
    ${as[@]+"${as[@]}"} jq --arg p "$DEVVM_PROXY" \
      '.proxies.default = {httpProxy: $p, httpsProxy: $p, noProxy: "localhost,127.0.0.1"}' "$c" > "$tmp"
  else
    ${as[@]+"${as[@]}"} jq 'del(.proxies.default)' "$c" > "$tmp"
  fi
  ${as[@]+"${as[@]}"} cp "$tmp" "$c"
  rm -f "$tmp"
}
dropin=/etc/systemd/system/docker.service.d/devvm-proxy.conf
$systemd || dropin=/etc/conf.d/docker   # OpenRC: a block in Docker's own settings
# conf_block FILE [CONTENT]: replace (or remove) devvm's block in a root-owned FILE.
conf_block() {
  [[ -f $1 ]] || return 0
  sudo sed -i '/^# devvm-proxy begin/,/^# devvm-proxy end/d' "$1"
  [[ -z "${2:-}" ]] || printf '# devvm-proxy begin\n%s\n# devvm-proxy end\n' "$2" | sudo tee -a "$1" >/dev/null
}
before=$(sudo cat "$dropin" 2>/dev/null || true)
if [[ -n "${DEVVM_PROXY:-}" ]]; then
  echo 'Defaults env_keep += "http_proxy https_proxy no_proxy HTTP_PROXY HTTPS_PROXY NO_PROXY NODE_USE_ENV_PROXY"' |
    sudo tee /etc/sudoers.d/devvm-proxy >/dev/null
  sudo chmod 440 /etc/sudoers.d/devvm-proxy
  [[ ! -d /etc/apt/apt.conf.d ]] ||
    printf 'Acquire::http::Proxy "%s";\nAcquire::https::Proxy "%s";\n' "$DEVVM_PROXY" "$DEVVM_PROXY" |
      sudo tee /etc/apt/apt.conf.d/95devvm-proxy >/dev/null
  if ! $systemd; then
    # Without PAM (Alpine), shells don't read Lima's /etc/environment.
    printf 'export http_proxy=%s https_proxy=%s HTTP_PROXY=%s HTTPS_PROXY=%s no_proxy=localhost,127.0.0.1,::1 NO_PROXY=localhost,127.0.0.1,::1\n' \
      "$DEVVM_PROXY" "$DEVVM_PROXY" "$DEVVM_PROXY" "$DEVVM_PROXY" | sudo tee /etc/profile.d/devvm-proxy.sh >/dev/null
  fi
  if command -v dockerd >/dev/null; then
    sudo mkdir -p "$(dirname "$dropin")"
    if $systemd; then
      printf '[Service]\nEnvironment="HTTP_PROXY=%s" "HTTPS_PROXY=%s" "NO_PROXY=localhost,127.0.0.1"\n' \
        "$DEVVM_PROXY" "$DEVVM_PROXY" | sudo tee "$dropin" >/dev/null
    else
      conf_block "$dropin" "export HTTP_PROXY=$DEVVM_PROXY HTTPS_PROXY=$DEVVM_PROXY NO_PROXY=localhost,127.0.0.1"
    fi
  fi
  # Every SSH connection goes through the proxy too, checked against the allowlist and logged.
  # The proxy URL carries the VM's password (http://devvm:KEY@host:port), sent as Proxy-Authorization.
  hp=${DEVVM_PROXY#http://} auth=""
  if [[ $hp == *@* ]]; then auth=$(printf %s "${hp%%@*}" | base64 | tr -d '\n'); hp=${hp#*@}; fi
  { printf '#!/bin/bash\nexec 3<>/dev/tcp/%s/%s || exit 1\nauth=%q\n' "${hp%:*}" "${hp##*:}" "$auth"
    cat <<'SH'
# ssh's ProxyCommand with network policy: CONNECT HOST PORT through devvm's proxy. When the proxy
# refuses, its reason (with the host command that allows it) goes to stderr, where ssh shows it.
printf 'CONNECT %s:%s HTTP/1.1\r\nHost: %s:%s\r\nProxy-Authorization: Basic %s\r\n\r\n' "$1" "$2" "$1" "$2" "$auth" >&3
read -r _ code _ <&3
while IFS= read -r line <&3 && [[ -n ${line%$'\r'} ]]; do :; done
if [[ $code != 200 ]]; then
  read -r -t 5 msg <&3 && echo "$msg" >&2
  exit 1
fi
# Both directions. The reader replaces this script, so it ends when ssh signals its ProxyCommand on exit;
# the writer ends when ssh closes its input.
cat <&0 >&3 &
exec cat <&3
SH
  } | sudo tee /usr/local/bin/devvm-ssh-proxy >/dev/null
  sudo chmod 755 /usr/local/bin/devvm-ssh-proxy
  mkdir -p ~/.ssh
  proxy_block ~/.ssh/config "Host *
  ProxyCommand /usr/local/bin/devvm-ssh-proxy %h %p"
  # devvm-why: what the proxy blocked lately, and how to allow it.
  sudo tee /usr/local/bin/devvm-why >/dev/null <<'SH'
#!/bin/sh
# What this VM's network policy blocked recently, and the host command that allows it.
exec curl -sS --proxy "$HTTP_PROXY" http://devvm.proxy/blocked
SH
  sudo chmod 755 /usr/local/bin/devvm-why
  # Before each prompt: what was blocked since the last one (HTTPS clients don't show the proxy's
  # reason), and the host command that allows it. A hook like devvm-prompt.sh's.
  mkdir -p ~/.bashrc.d
  { printf '_devvm_proxy=%q\n' "$DEVVM_PROXY"
    cat <<'EOF'
_devvm_blocked() {
  local out
  out=$(curl -s --max-time 1 --proxy "$_devvm_proxy" "http://devvm.proxy/blocked/new?after=${_devvm_seen:-}") || return 0
  [[ ${out%%$'\n'*} =~ ^[0-9]+$ ]] || return 0
  _devvm_seen=${out%%$'\n'*}
  [[ $out != *$'\n'* ]] || printf '%s\n' "${out#*$'\n'}" >&2
}
if (( BASH_VERSINFO[0] * 100 + BASH_VERSINFO[1] >= 501 )); then
  PROMPT_COMMAND+=(_devvm_blocked)
else
  PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND;}_devvm_blocked"
fi
EOF
  } > ~/.bashrc.d/devvm-blocked.sh
else
  rm -f ~/.bashrc.d/devvm-blocked.sh
  sudo rm -f /etc/sudoers.d/devvm-proxy /etc/apt/apt.conf.d/95devvm-proxy /usr/local/bin/devvm-why \
    /usr/local/bin/devvm-ssh-proxy /etc/profile.d/devvm-proxy.sh
  if $systemd; then sudo rm -f "$dropin"; else conf_block "$dropin"; fi
  [[ ! -f ~/.ssh/config ]] || proxy_block ~/.ssh/config
fi
if command -v dockerd >/dev/null; then
  docker_proxies ~/.docker/config.json
  docker_proxies /root/.docker/config.json sudo
  if [[ "$before" != "$(sudo cat "$dropin" 2>/dev/null || true)" ]]; then
    if $systemd; then sudo systemctl daemon-reload && sudo systemctl restart docker
    else sudo rc-service docker restart >/dev/null; fi
  fi
fi

# The proxy's CA (network policy on): the proxy shows its own certificates for hosts where it puts
# tokens in. Trusted only in this VM.
crt=/usr/local/share/ca-certificates/devvm-proxy.crt
if [[ -f "$s/ca.pem" ]]; then
  if ! sudo cmp -s "$s/ca.pem" "$crt"; then
    sudo install -m 644 "$s/ca.pem" "$crt"
    # Its warnings (openssl rehash skipping the bundle file) are noise; a failure still stops this.
    sudo update-ca-certificates >/dev/null 2>&1
  fi
elif [[ -f "$crt" ]]; then
  sudo rm -f "$crt"
  sudo update-ca-certificates --fresh >/dev/null 2>&1
fi

# GitHub over HTTPS with the token from ~/.config/devvm/env (a placeholder with network policy).
if [[ -n "${DEVVM_GITHUB:-}" ]]; then
  git config --global credential.https://github.com.helper \
    '!f() { test "$1" = get || return 0; . ~/.config/devvm/env; printf "username=x-access-token\npassword=%s\n" "$GH_TOKEN"; }; f'
else
  git config --global --unset-all credential.https://github.com.helper 2>/dev/null || true
fi

# Tell agents why the network may fail, and what to do about it.
note="Network access in this VM goes through an allowlist (devvm network policy). If a download or request fails with a connection error or a 403 from the proxy, run \`devvm-why\` to see what was blocked; the user can allow it on the host with \`devvm policy allow <host>\`."
for f in ~/.claude/CLAUDE.md ~/.codex/AGENTS.md; do
  [[ -d "$(dirname "$f")" ]] || continue
  [[ -f "$f" ]] && sed -i '/^<!-- devvm-proxy -->/,/^<!-- \/devvm-proxy -->/d' "$f"
  [[ -n "${DEVVM_PROXY:-}" ]] || continue
  printf '\n<!-- devvm-proxy -->\n%s\n<!-- /devvm-proxy -->\n' "$note" >> "$f"
done

mkdir -p ~/.config/devvm
install -m 600 "$s/env" ~/.config/devvm/env
grep -qs 'devvm/env' ~/.profile || echo '[ -f ~/.config/devvm/env ] && . ~/.config/devvm/env' >> ~/.profile
