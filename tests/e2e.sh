#!/bin/bash
# End-to-end tests with real VMs, in a sandbox. Hosted CI can't run nested VMs on macOS.
#   tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...]]
#     (default)  every section but from: minutes, more on a first run (base's and the tools' builds, and
#                Ubuntu's cloud image to download). images builds, rebuilds and interrupts images;
#                project makes a VM from base in a project folder, opens it from a subfolder in that
#                subfolder, and checks its mounts, a file watcher in it (the VM's edits seen, none
#                of this machine's: Lima's mounts pass on no file events), no Docker, every
#                port through devvm's forwarder (never a port a program here has, nor a server on a
#                link's address), ssh by
#                name with pinned host keys, .git protection and shell, then devvm stop, opening it
#                again, restart, devvm size, devvm ports (a running VM's at once, never a listed port a
#                program here has, a bad list a usage error), a mistyped command refused with a suggestion,
#                devvm git-protect and rm, a VM from a devcontainer.json (its ports, variables, commands
#                once and at each start, a changed file, an image on this machine's registry refused), an agent VM's folders and history (devvm history export, rm's export), and SSH agent forwarding
#                (a throwaway agent's, and devvm's own with a throwaway key); clone makes a --clone
#                VM of a throwaway repo and checks its clone, the remote, fetching, and rm's checks
#                for unfetched commits and uncommitted changes and the work it keeps; gc deletes a stale
#                image and an unused tool image, keeping base and a VM named like an image; tools makes a node
#                VM and checks its shared tools, devvm tools, tools update, rm and add, and devvm IMAGE
#                refused on it; network makes a claude VM with
#                network policy and checks it reaches only its allowlist, devvm network, its log and rules, its tokens
#                stay on the host (devvm secret too), and the self-check, on relaunch too; devvm stop
#                stops its proxy; nix builds the nix layer behind a company proxy (no proxy left in its
#                image), makes a node+nix VM of a flake project with network policy and checks Nix's
#                pins, store and trust, nix-daemon's proxy, nix run nixpkgs#hello through the shared Nix
#                cache (fetched by the host, a VM's writes and other requests refused, a second VM's
#                hits), sudo nix, the flake's dev shell through direnv before the shared node, a blocked
#                download named, and network off (no shared cache); link makes two small targets and
#                a client that forwards every port, and checks links between them, each check a row
#                of the design note's threat check: names alone (never the client's localhost, never
#                forwarded to this machine), two targets on one port, port 80, publish, unpublish,
#                add and rm on a running client, the sockets, the 256-connection cap, a program
#                holding a link's address, root loosening the client's sshd, a stopped and restarted
#                target, the target's rm and a VM made again, the three in a group (joined, a port
#                published to it, leaving, a VM refused whose hostname is a member's name), network policy on both and the policy rule's refusals,
#                a group's all-or-none refusals, the log, and nothing left behind (in the --from tier, an
#                Alpine client's link to port 80 too). --backend smolvm and docker run it too
#     --only desktop
#                base with the desktop feature, and a VM with network policy whose desktop devvm
#                desktop serves on this machine's loopback alone: a wrong password refused, the
#                password let in, the VM's server on its user's own socket and no TCP port, the
#                clipboard off, the password in no file, log or process's arguments, no way out of the
#                VM; root's own server in its place (no password) still behind devvm's, the viewer's
#                clipboard never reaching it; --clipboard
#     --from     every section, with from, and every base image feature: the longest. from makes VMs
#                from node:24 (Debian 12, with node, python, the docker and nix layers and claude) and
#                alpine:3 (musl tools, the docker layer on OpenRC, the nix layer's daemon an OpenRC service,
#                through the proxy once network policy is on), each with every feature, and builds base from ubuntu:26.04, with a
#                docker that isn't running
#     --only     just these sections; SECTIONS below
#     --clean    delete the sandbox's cached images and tool images first (default: reuse them
#                between runs)
#     --backend smolvm
#                the smolvm backend's own section instead (experimental): base and a layer built as
#                packs, a project VM's mounts, a file watcher in it seeing this machine's edits (no
#                deletions), size, listed ports through devvm's forwarder (a server on the VM's loopback reaching this machine, none published by smolvm, never one a
#                program here has, a running VM's changed at once), no Docker
#                without the docker layer, agent forwarding, a node VM's
#                shared tools, agent toggling's emulation (your agent, devvm's or none from each
#                start, devvm history export's too, none reaching no agent and adding to none, a VM set to
#                devvm's or none never reaching yours, one started by smolvm machine start from your
#                shell stopped), ssh for editors' emulation (pinned
#                host keys of each VM's own, its own key alone, no forwarding out even past a hostile
#                sshd, devvm host and code, an alpine:3 VM, a --clone VM's push and fetch), network policy's emulation
#                (a docker VM: nothing but the proxy, Docker's pulls through it, DNS kept on this
#                machine, failing closed, the policy, ssh opening no way out), every port's (servers on the VM's loopback or all its addresses
#                reach this machine's loopback alone, never a port of this machine's or devvm's, no
#                way out with network policy), the nix layer (nix-daemon started after each start,
#                with the VM's proxy and then without, nix run nixpkgs#hello, a sandboxed build),
#                links between VMs (the link section's checks, port 80 on smolvm's kernel too),
#                and that no machine without the sandbox's label is touched. Its machines share your
#                smolvm (macOS): it deletes only those it made, with the sandbox's label.
#     --backend docker [--engine linux|desktop]
#                the Docker backend's own section instead (experimental), state under
#                /tmp/devvm-go-e2e-docker. On Linux, on this machine's engine: base and a layer built by
#                committing containers, confined (no host loopback-only server reached, one on every
#                address reached), a failed build kept, a project VM's mounts, a file watcher in it
#                seeing this machine's edits (on Desktop, no deletions), its files the host user's
#                (uid and gid), its home a volume of its own, size, no Docker inside, what it lacks
#                (a disk size, the nix layer) refused in one line, agent folders added and removed (its container made again from a
#                commit, its disk kept), a node VM's shared tools linked from Docker's tools volume;
#                the docker layer under Sysbox (an emulation, reviewed; refused on an engine without
#                it, on Docker Desktop saying to use a VM backend): a user namespace of its own, every
#                capability in it, the folder's files yours (root's set-user-id file root's, named at
#                the next launch), its engine's own volumes, its own network alone (containers on
#                Docker's default network and another out of reach even as root, and this machine's
#                loopback even routed to, the internet not; IPv6 turned on reaches this machine's
#                servers), Docker running in it after each start, no network policy, ls saying so;
#                refused for a VM that exists, which stays as it was;
#                ssh for editors', ports' and agent forwarding's emulations (nothing published or
#                listening, pinned host keys of each VM's own, its own key alone, no forwarding out even
#                past a hostile sshd, a VM started outside devvm, devvm host and code, every port and
#                listed ports through devvm's forwarder, a running VM's changed at once, never a port
#                of this machine's programs or another VM's, the agent in ssh sessions alone, a --clone VM); network policy's
#                emulation (the VM without NET_RAW on an internal network of its own, with
#                inhibit_ipv4, beside its relay alone, by ID; nothing but its proxy reached even as
#                root, no outside name, no DNS query leaving its network (a capture on the engine's
#                own network, with a control), no IPv6 it can use; the relay devvm's Linux build in an
#                empty image, read only, no capabilities, its password in no process's arguments, a
#                token kept out of the VM and its relay, its proxy following its container,
#                fail-closed without it, a network added, a relay replaced or a start outside devvm
#                unenforced and stopped, another container on its network refused, network off and
#                on, ping without NET_RAW); links between VMs (the link section's checks), on each
#                engine.
#                On macOS, the same on a Linux engine in a Lima VM e2e keeps (Lima home
#                ~/.lt-e2e-go-docker, VM dockerhost: this checkout read-only, devvm built for it here,
#                Sysbox installed, pinned),
#                then on Docker Desktop, with devvm's Linux build beside devvm: base, a layer, an
#                Alpine base and the tools built there through a proxy of their own (the internet
#                reached, this machine's loopback not, no NET_RAW), a project
#                VM, an agent VM, a node VM, the emulations and network policy. --engine runs one of
#                the two. Docker Desktop is yours: only what has the sandbox's label is touched, the
#                rest must be the same at the end (but the images bases start from, which devvm pulls
#                by digest and Docker keeps), and everything with the label is deleted then. --clean
#                deletes the Linux engine's images first.
# The sandbox: Lima home ~/.lt-e2e-go (or $DEVVM_E2E_LIMA_HOME), state under /tmp/devvm-go-e2e. It refuses
# to run against ~/.lima or devvm's own Lima home, deletes only the VMs it created, and uses no real
# tokens. devvm is built from this checkout into the sandbox, with its files next to it (or copied from
# $DEVVM_E2E_BIN, a test build of it, where there's no Go: the docker host VM).
set -uo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sections_all="images project clone gc tools network desktop nix from link"
usage() { echo "usage: tests/e2e.sh [--clean] [--from | --only SECTION[,SECTION...] | --backend smolvm | --backend docker [--engine linux|desktop]]  (sections: $sections_all)" >&2; exit 2; }
clean=false sel="images project clone gc tools network nix link" backend=lima engines=""
while (( $# )); do
  case $1 in
    --clean) clean=true ;;
    --from)  sel=$sections_all ;;
    --only)  [[ -n "${2:-}" ]] || usage; sel=$(tr ',' ' ' <<<"$2"); shift
             for s in $sel; do [[ " $sections_all " == *" $s "* ]] || usage; done ;;
    --backend) case ${2:-} in smolvm|docker) backend=$2 sel=$2 ;; *) usage ;; esac; shift ;;
    --engine) [[ ${2:-} == linux || ${2:-} == desktop ]] || usage; engines=$2; shift ;;
    *) usage ;;
  esac
  shift
done
[[ -z $engines || $backend == docker ]] || usage
# OpenSSL first on PATH, before anything is built: the CA checks' HTTPS server is openssl s_server,
# and LibreSSL's (macOS's /usr/bin/openssl) cuts the VM's OpenSSL client off mid-request.
ossl=$(openssl version 2>/dev/null)
if [[ $ossl != OpenSSL* ]]; then
  fix='brew install openssl@3, then put $(brew --prefix openssl@3)/bin first on PATH'
  [[ "$(uname -s)" == Darwin ]] || fix="install your distribution's openssl package"
  echo "e2e: openssl is ${ossl:-missing}, and e2e needs OpenSSL: $fix" >&2; exit 1
fi
# The Docker engines checked: this machine's on Linux; on macOS a Linux one in a VM, then Desktop's.
if [[ $backend == docker && -z $engines ]]; then
  if [[ "$(uname -s)" == Darwin ]]; then engines="linux desktop"; else engines=linux; fi
fi
want() { [[ " $sel " == *" $1 "* ]]; }

# Its own Lima home (other sandboxes' images aren't its own), for devvm and for its own limactl calls.
export DEVVM_LIMA_HOME=${DEVVM_E2E_LIMA_HOME:-$HOME/.lt-e2e-go}
export LIMA_HOME=$DEVVM_LIMA_HOME
work=/tmp/devvm-go-e2e
# The Docker backend's: apart, so it runs beside the others. The same path in the docker host VM, so
# what the Linux engine's run built carries the label Docker Desktop's run looks for.
[[ $backend != docker ]] || work=/tmp/devvm-go-e2e-docker
export XDG_DATA_HOME=$work/data XDG_CONFIG_HOME=$work/config
export DEVVM_SECRET_SERVICE=devvm-go-e2e   # its Keychain items: fake tokens only, deleted at the end
unset DEVVM_MESSAGES DEVVM_UBUNTU_URL DEVVM_HOST_MISE CLAUDE_CODE_OAUTH_TOKEN DEVVM_FROM
unset SSH_AUTH_SOCK   # your SSH agent is never reached: checks that need one start their own
unset FORCE_COLOR CLICOLOR_FORCE   # devvm's output stays plain where the checks read it
mkdir -p "$LIMA_HOME"
for real in "$HOME/.lima" "$HOME/.local/share/devvm/lima"; do
  real=$(cd "$real" 2>/dev/null && pwd -P || echo "$real")
  [[ "$(cd "$LIMA_HOME" && pwd -P)" != "$real" ]] || { echo "e2e: refusing to use your real Lima home ($real)" >&2; exit 1; }
done

# devvm is built from this checkout into the sandbox, its files embedded (so editing the repo mid-run
# is harmless).
src=$work/src
rm -rf "$src"; mkdir -p "$src" "$XDG_CONFIG_HOME/devvm/layers" "$XDG_DATA_HOME"
gocmd=(go); command -v go >/dev/null || gocmd=(mise exec -- go)
# A test build: DEVVM_UNREVIEWED and the other test overrides count in it
# (never in a release).
testbuild=(build -ldflags "-X github.com/w3cj/devvm/internal/testbuild.testBuild=yes")
if [[ -n ${DEVVM_E2E_BIN:-} ]]; then
  cp "$DEVVM_E2E_BIN" "$src/devvm" || { echo "e2e: no devvm at $DEVVM_E2E_BIN" >&2; exit 1; }
else
  (cd "$root" && CGO_ENABLED=0 "${gocmd[@]}" "${testbuild[@]}" -o "$src/devvm" ./cmd/devvm) || { echo "e2e: couldn't build devvm" >&2; exit 1; }
  # On macOS, devvm's Linux build beside it, as a release has it: Docker's network policy relays and
  # Docker Desktop's builds run it.
  if [[ $backend == docker && "$(uname -s)" == Darwin ]]; then
    a=$(cd "$root" && "${gocmd[@]}" env GOARCH)
    (cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=$a "${gocmd[@]}" "${testbuild[@]}" -o "$src/devvm-linux-$a" ./cmd/devvm) ||
      { echo "e2e: couldn't build devvm for Linux" >&2; exit 1; }
  fi
fi
# The features are in base's key, so only a run with from (which checks them) builds them all; desktop,
# and the smolvm and Docker backends' sections, which check devvm desktop, build desktop alone; every
# other selection shares one smaller base.
if want from; then for f in "$root"/assets/features/*.sh; do basename "$f" .sh; done > "$XDG_CONFIG_HOME/devvm/base-features"
elif want desktop || [[ $backend != lima ]]; then echo desktop > "$XDG_CONFIG_HOME/devvm/base-features"
else : > "$XDG_CONFIG_HOME/devvm/base-features"; fi
D=$src/devvm
# The digest of Ubuntu's container image smolvm's and Docker's base start from, as devvm pins it.
ubuntu_pin=$(sed -n 's/^const ContainerDigest = "\(sha256:[0-9a-f]\{64\}\)"$/\1/p' "$root/internal/ubuntu/ubuntu.go")
# The dated release of Ubuntu's cloud image Lima's base starts from, and its images' SHA-256s.
cloud_pin() { sed -n "s/^const $1 = \"\([0-9a-z-]*\)\"$/\1/p" "$root/internal/ubuntu/ubuntu.go"; }
cloud_release=$(cloud_pin CloudRelease)
cloud_sums="$(cloud_pin CloudSHA256Arm64)|$(cloud_pin CloudSHA256Amd64)"
images=$XDG_DATA_HOME/devvm/images   # the image records, one per key
layers=$XDG_CONFIG_HOME/devvm/layers
vms=$XDG_DATA_HOME/devvm/vms         # the VMs' state, one per VM
projects=$work/p                     # the project folders, this run's
share=$XDG_DATA_HOME/devvm/tools     # the shared tool images
lock=$XDG_DATA_HOME/devvm/mise.lock  # the shared tools' versions, resolved at the start of each run
export DEVVM_NO_TOOL_CHECK=1         # and not again mid-run
# lockv TOOL: its version in the lock.
lockv() { awk -v t="[[tools.$1]]" '$0 == t { f = 1; next } f && /^version = / { gsub(/"/, "", $3); print $3; exit }' "$lock"; }
# m NAME [name=value...]: a message's text, from devvm, so checks never repeat its wording. Exported
# for the checks' own shells.
m() { "$D" _msg "$@"; }
export -f m; export D

# smol_ls: the sandbox's smolvm machines, "NAME STATE" lines: those with its label. smol_others: every
# other machine smolvm has, "NAME LABEL" (yours, on macOS). smolvm's JSON has each machine's keys
# sorted, labels before name and state.
smol_json() { SMOLVM_USAGE=0 smolvm machine ls --json 2>/dev/null | awk '
  /^  \{/ { lab = ""; n = ""; s = "" }
  /^      "devvm\.data": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); lab = v }
  /^    "name": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); n = v }
  /^    "state": / { v = $0; sub(/^[^:]*: "/, "", v); sub(/",?$/, "", v); s = v }
  /^  \}/ { print n "\t" s "\t" lab }'; }
smol_ls() { smol_json | awk -F'\t' -v want="$XDG_DATA_HOME/devvm" '$3 == want { print $1, $2 }'; }
smol_others() { smol_json | awk -F'\t' -v want="$XDG_DATA_HOME/devvm" '$3 != want { print $1, $3 }'; }
smol_status() { smol_ls | awk -v n="$1" '$1 == n { print $2 }'; }
export -f smol_json smol_ls smol_others smol_status
# The Docker backend's: only what has the sandbox's label ($dlabel). dns is its names' hash of the data
# folder (devvm-$dns:KEY images, devvm-img-KEY-$dns build containers).
dlabel=label=devvm.data=$XDG_DATA_HOME/devvm
dns=$(printf %s "$XDG_DATA_HOME/devvm" | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-8)
# d_state NAME: the state of the sandbox's container by that name (running, exited, created), "" for none.
d_state() { docker container ls -a --filter "$dlabel" --format '{{.Names}} {{.State}}' 2>/dev/null | awk -v n="$1" '$1 == n { print $2 }'; }
# d_vol NAME, d_img REF: the sandbox's volume, or image, by that name is there.
d_vol() { docker volume ls -q --filter "$dlabel" 2>/dev/null | grep -qx "$1"; }
d_img() { docker image ls --filter "$dlabel" --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep -qx "$1"; }
# d_net NAME: the sandbox's network by that name is there.
d_net() { docker network ls -q --filter "$dlabel" --filter "name=^$1\$" 2>/dev/null | grep -q .; }
# d_rm NAME: the sandbox's container by that name, its relay, network, volumes and commits.
d_rm() { local n=$1 v i
  [[ -z $(d_state "$n") ]] || docker container rm -f -v "$n" >/dev/null 2>&1
  [[ -z $(d_state "$n-proxy") ]] || docker container rm -f -v "$n-proxy" >/dev/null 2>&1
  ! d_net "$n-net" || docker network rm "$n-net" >/dev/null 2>&1
  for v in "$n-home" "$n-clone"; do ! d_vol "$v" || docker volume rm "$v" >/dev/null 2>&1; done
  for i in $(docker image ls --filter "$dlabel" --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep "^devvm-$dns:$n-"); do
    docker image rm "$i" >/dev/null 2>&1; done; }
# d_purge: everything with the sandbox's label: containers, then networks, volumes and images.
d_purge() { local x
  for x in $(docker container ls -a -q --filter "$dlabel" 2>/dev/null); do docker container rm -f -v "$x" >/dev/null 2>&1; done
  for x in $(docker network ls -q --filter "$dlabel" 2>/dev/null); do docker network rm "$x" >/dev/null 2>&1; done
  for x in $(docker volume ls -q --filter "$dlabel" 2>/dev/null); do docker volume rm "$x" >/dev/null 2>&1; done
  for x in $(docker image ls --filter "$dlabel" --format '{{.Repository}}:{{.Tag}}' 2>/dev/null); do docker image rm "$x" >/dev/null 2>&1; done; }
# d_ours: how many containers, networks, volumes and images have the sandbox's label.
d_ours() { { docker container ls -a -q --filter "$dlabel"; docker network ls -q --filter "$dlabel"; docker volume ls -q --filter "$dlabel"
  docker image ls -q --filter "$dlabel"; } 2>/dev/null | grep -c .; }
# d_others: what Docker has without the sandbox's label: containers (ID and name), images (ID),
# volumes and networks (name and ID). Yours, on Docker Desktop: the same at the end.
d_others() {
  { docker container ls -a --format '{{.ID}} {{.Names}} {{.Label "devvm.data"}}' | awk -v w="$XDG_DATA_HOME/devvm" '$3 != w { print "container", $1, $2 }'
    comm -23 <(docker image ls -a -q --no-trunc | sort -u) <(docker image ls -a -q --no-trunc --filter "$dlabel" | sort -u) | sed 's/^/image /'
    docker volume ls --format '{{.Name}} {{.Label "devvm.data"}}' | awk -v w="$XDG_DATA_HOME/devvm" '$2 != w { print "volume", $1 }'
    docker network ls --no-trunc --format '{{.ID}} {{.Name}} {{.Label "devvm.data"}}' | awk -v w="$XDG_DATA_HOME/devvm" '$3 != w { print "network", $1, $2 }'
  } 2>/dev/null | sort; }
export -f d_state d_vol d_img d_ours d_net; export dlabel dns
d_desktop=false   # set once Docker Desktop's checks start: cleanup then deletes all with the label there
d_servers=()      # the build confinement check's servers
created=()   # VMs this run made; the only ones it deletes
# ps_watch: every host process's command line, sampled every 0.1 s into $work/ps.log until
# ps_unwatch, for checking a secret was in none while devvm ran.
watcher=""
ps_watch() {
  : > "$work/ps.log"
  (while :; do ps -axww -o command= >> "$work/ps.log" 2>/dev/null; sleep 0.1; done) &
  watcher=$!
}
ps_unwatch() { [[ -z $watcher ]] || { kill "$watcher" 2>/dev/null; wait "$watcher" 2>/dev/null; watcher=""; }; }
cleanup() {
  local n
  for n in ${created[@]+"${created[@]}"}; do
    if [[ $backend == smolvm ]]; then
      # Only a machine with the sandbox's label, and the image's pack.
      [[ -z $(smol_status "$n") ]] || SMOLVM_USAGE=0 smolvm machine delete --name "$n" -f >/dev/null 2>&1
      rm -f "$XDG_DATA_HOME/devvm/smolvm/vms/$n.json" "$XDG_DATA_HOME/devvm/smolvm/vms/$n.proxy" "$XDG_DATA_HOME/devvm/smolvm/vms/$n.ports."*
      [[ $n != devvm-img-* ]] || rm -rf "$XDG_DATA_HOME/devvm/smolvm/images/${n#devvm-img-}"
    elif [[ $backend == docker ]]; then
      # Only what has the sandbox's label: the VM's container, volumes and commits; an image's tag and
      # build container.
      if [[ $n == devvm-img-* ]]; then
        d_rm "$n-$dns"; ! d_img "devvm-$dns:${n#devvm-img-}" || docker image rm "devvm-$dns:${n#devvm-img-}" >/dev/null 2>&1
      else d_rm "$n"; rm -f "$XDG_DATA_HOME/devvm/docker/vms/$n.json"; fi
    else
      limactl delete -f "$n" >/dev/null 2>&1
    fi
    rm -f "$images/${n#devvm-img-}" "$vms/$n"
  done
  rm -f "$layers"/e2e*.sh "$layers"/e2e*.policy
  ! $d_desktop || d_purge
  for n in ${d_cap:-} ${d_squat:-}; do [[ -z $(d_state "$n") ]] || docker container rm -f "$n" >/dev/null 2>&1; done
  for n in ${d_servers[@]+"${d_servers[@]}"}; do kill "$n" 2>/dev/null; done
  [[ "$(uname -s)" != Darwin ]] || security delete-generic-password -s "$DEVVM_SECRET_SERVICE" -a github >/dev/null 2>&1
  rm -f "$XDG_CONFIG_HOME/devvm/secrets/github" "$work/net-home/Library/Keychains"   # the link, never the folder
  [[ -z ${listener:-} ]] || kill "$listener" 2>/dev/null
  [[ -z ${corp:-} ]] || kill "$corp" 2>/dev/null   # the company proxy's stand-in
  [[ -z ${hostprog:-} ]] || kill "$hostprog" 2>/dev/null
  [[ -z ${ca_server:-} ]] || kill "$ca_server" 2>/dev/null
  [[ -z ${cad:-} ]] || rm -rf "$cad" "$cafolder"   # the run's own CA, and devvm's CA folder in the sandbox
  ps_unwatch
  [[ -z ${udp_listener:-} ]] || kill "$udp_listener" 2>/dev/null
  [[ -z ${fake:-} ]] || kill "$fake" 2>/dev/null
  [[ -z ${desk:-} ]] || kill "$desk" 2>/dev/null   # a devvm desktop
  [[ -z ${agent_pid:-} ]] || kill "$agent_pid" 2>/dev/null
  [[ -z ${user_agent_pid:-} ]] || kill "$user_agent_pid" 2>/dev/null
  pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock" 2>/dev/null   # devvm's own, in the sandbox
  pkill -f "[_]docker-ports -vm .* -key $XDG_DATA_HOME/" 2>/dev/null       # Docker VMs' forwarders, the sandbox's
  pkill -f "[_]lima-ports -vm .* -lifeline $DEVVM_LIMA_HOME/" 2>/dev/null  # Lima VMs' forwarders, the sandbox's
  pkill -f "[_]proxy -listen .* -listen-unix $XDG_DATA_HOME/devvm/docker/" 2>/dev/null   # Docker VMs' and builds' proxies
  rm -rf "$src" "$work/ssh" "$work/net-home" "$work/agent-home" "$work/agent-ssh" "$XDG_CONFIG_HOME/devvm/agent-keys" "$work/clone-home" "$work/claude-before.json" "$work/codex-before.json" \
    "$work/export.list" "$work/export" "$XDG_DATA_HOME/devvm/exports" "$work/nodocker" "$work/docker-calls" "$work/from-home" \
    "$work/dns-probe.sh" "$work/d-dns-probe.sh" "$work/udp-probe.sh" "$work/probe.log" "$work/decisions" "$work/udp.log" "$work/nettop.csv" "$work/listener.auth" "$work/ps.log" "$work/lima-bypass" \
    "$work/corp.auth" "$work/corp.policy" "$work/corp.jsonl" "$work/corp.log"
  # The project folders' .git is locked (macOS): unlocked to go.
  if [[ -d $projects ]]; then
    [[ "$(uname -s)" != Darwin ]] || chflags -R nouchg "$projects" 2>/dev/null
    rm -rf "$projects"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# Progress: each result shows [n/N], how long it took (since the previous check, so builds count),
# and the time left as the previous run measured it ($work/timings: "label<TAB>seconds"). A
# heartbeat every minute says what's happening during long steps.
# Per selection and Docker engines: they run different checks (on macOS, the Linux engine's are timed by
# its own run, in the docker host VM).
timings=$work/timings-$(tr ' ' '-' <<<"$sel${engines:+ $engines}")
: > "$timings.new"
total=$(wc -l 2>/dev/null < "$timings" | tr -d ' '); [[ ${total:-0} -gt 0 ]] || total="?"
started=$SECONDS ncheck=0
date +%s > "$work/mark"
mins() { printf '%dm%02ds' $(($1 / 60)) $(($1 % 60)); }
heartbeat() {
  local esc=$'\033' s f
  while sleep 60; do
    s=$(( $(date +%s) - $(cat "$work/mark") ))
    # The newest output: a check's, or devvm's own run log.
    # shellcheck disable=SC2012  # names without spaces
    f=$(ls -1t "$work/last.log" "$work/check.log" "$XDG_DATA_HOME"/devvm/logs/runs/*.log 2>/dev/null | head -1)
    printf '        ... %s since the last check: %s\n' "$(mins "$s")" \
      "$( [[ -f $f ]] && tail -1 "$f" | tr -d '\r' | sed "s/${esc}\[[0-9;?]*[A-Za-z]//g" | cut -c1-100)"
  done
}
heartbeat &
hb=$!
trap 'kill $hb 2>/dev/null; cleanup' EXIT

pass=0 fail=0 failed=()
# check LABEL CMD...: run CMD, count and print the result (and CMD's last output lines on failure).
check() {
  local label=$1 r dur left; shift
  if "$@" >"$work/check.log" 2>&1; then r=PASS; pass=$((pass + 1)); else r=FAIL; fail=$((fail + 1)); failed+=("$label"); fi
  ncheck=$((ncheck + 1))
  dur=$(( $(date +%s) - $(cat "$work/mark") )); date +%s > "$work/mark"
  printf '%s\t%s\n' "$label" "$dur" >> "$timings.new"
  left=$(awk -F'\t' -v l="$label" 'found { s += $2 } $1 == l { found = 1 } END { if (found) print s }' "$timings" 2>/dev/null)
  [[ $total == "?" ]] || (( ncheck <= total )) || total="?"   # more checks than last time
  echo "  $r  [$ncheck/$total] $label ($(mins "$dur"); $(mins $((SECONDS - started))) in${left:+, about $(mins "$left") left})"
  [[ $r == PASS ]] || tail -5 "$work/check.log" | sed 's/^/        /'
}
# skip LABEL WHY: a check this run has nothing for (counted, so runs compare their counts).
skip() {
  ncheck=$((ncheck + 1)) skipped=$((skipped + 1))
  printf '%s\t0\n' "$1" >> "$timings.new"
  echo "  SKIP  [$ncheck/$total] $1 ($2)"
}
skipped=0
# rfwd_links HOST SSH...: the VM's sshd, reached as HOST by the command SSH..., binds a remote forward
# only on 127.77.0.0/16, the addresses of links: ssh's default (localhost), 127.0.0.1, 0.0.0.0, ::1,
# localhost and another range are refused, as is a remote socket forward, and 127.77.0.250 binds
# (the control). A safeguard for an honest VM: the boundary is the host's ssh.
rfwd_links() { local h=$1 a; shift
  for a in 0:127.0.0.1:9 47613:127.0.0.1:9 127.0.0.1:47613:127.0.0.1:9 0.0.0.0:47613:127.0.0.1:9 \
    '[::1]:47613:127.0.0.1:9' localhost:47613:127.0.0.1:9 127.78.0.2:47613:127.0.0.1:9 "/tmp/devvm-e2e-$$.sock:$sc/agent.sock"; do
    ! "$@" -o ExitOnForwardFailure=yes -R "$a" "$h" true </dev/null 2>/dev/null || { echo "-R $a bound"; return 1; }
  done
  "$@" -o ExitOnForwardFailure=yes -R 127.77.0.250:47613:127.0.0.1:9 "$h" true </dev/null || { echo "-R 127.77.0.250 refused"; return 1; }; }
export -f rfwd_links
section() { echo; echo "== $*"; }
quiet() { "$@" </dev/null >"$work/last.log" 2>&1; }
# keys_of IMAGE: the keys of the images recorded under that name, one per line.
keys_of() { grep -lx "name=$1" "$images"/* 2>/dev/null | while read -r f; do basename "$f"; done; }
# owned IMAGE: the images by that name are this run's, deleted at the end.
owned() { local k; for k in $(keys_of "$1"); do created+=("devvm-img-$k"); done; }
status_of() { limactl list --format '{{.Status}}' "$1" 2>/dev/null; }
export -f keys_of status_of; export images
# newproj NAME: a new, empty project folder, as devvm sees it (its physical path).
newproj() {
  [[ "$(uname -s)" != Darwin || ! -e "$projects/$1" ]] || chflags -R nouchg "$projects/$1"
  rm -rf "${projects:?}/$1"; mkdir -p "$projects/$1"; (cd "$projects/$1" && pwd -P)
}
# vm_of DIR: the VM devvm made for that folder, from its state.
vm_of() { grep -lx "dir=$1" "$vms"/* 2>/dev/null | head -1 | xargs basename 2>/dev/null; }
# launch DIR INPUT ARGS...: devvm ARGS in DIR, with INPUT for the shell it opens (which then ends);
# sets $vm and records it for cleanup. Its output is in $work/last.log.
launch() {
  local d=$1 in=$2; shift 2
  (cd "$d" && printf '%s\n' "$in" | "$D" "$@" >"$work/last.log" 2>&1)
  vm=$(vm_of "$d"); [[ -z $vm ]] || created+=("$vm")
}
in_vm() { limactl shell --workdir / "$1" bash -lc "$2" </dev/null; }
# corp_start: a company proxy of this run's ($corp): a devvm _proxy on 127.0.0.1:18766 with a
# password of its own ($work/corp.auth), allowing any host, its events in $work/corp.jsonl;
# $corpurl is its address with the password. corp_stop ends it.
corp_start() {
  corppw=e2e-corp-pw-$$-$(date +%s)
  echo "$corppw" > "$work/corp.auth"; printf '**\n' > "$work/corp.policy"; : > "$work/corp.jsonl"
  "$D" _proxy -listen 127.0.0.1:18766 -vm e2e-corp -auth-file "$work/corp.auth" -policy "$work/corp.policy" \
    -log "$work/corp.jsonl" >"$work/corp.log" 2>&1 &
  corp=$!
  sleep 1
  corpurl="http://devvm:$corppw@127.0.0.1:18766"
}
corp_stop() { [[ -z ${corp:-} ]] || kill "$corp" 2>/dev/null; corp=""; }
# e2e_ca: a throwaway CA of this run's in $cad (ca.pem), and a server certificate it signed for
# host.lima.internal (srv.pem, srv.key): never a certificate of yours. ca_serve PORT: an HTTPS server
# on this machine with it, on every address ($ca_server). $cafolder is devvm's CA folder.
cad=$work/e2e-ca cafolder=$XDG_CONFIG_HOME/devvm/ca ca_server=""
e2e_ca() {
  [[ ! -f $cad/srv.pem ]] || return 0
  rm -rf "$cad"; mkdir -p "$cad"
  printf '%s\n' '[req]' 'distinguished_name = dn' 'prompt = no' '[dn]' 'CN = devvm e2e CA' \
    '[ca]' 'basicConstraints = critical,CA:TRUE' 'keyUsage = critical,keyCertSign,cRLSign' 'subjectKeyIdentifier = hash' \
    '[srv]' 'basicConstraints = CA:FALSE' 'subjectAltName = DNS:host.lima.internal,DNS:localhost,IP:127.0.0.1' \
    'extendedKeyUsage = serverAuth' 'subjectKeyIdentifier = hash' 'authorityKeyIdentifier = keyid' > "$cad/openssl.cnf"
  openssl req -x509 -newkey rsa:2048 -nodes -days 2 -config "$cad/openssl.cnf" -extensions ca \
    -keyout "$cad/ca.key" -out "$cad/ca.pem" >/dev/null 2>&1 &&
  openssl req -new -newkey rsa:2048 -nodes -config "$cad/openssl.cnf" -subj /CN=host.lima.internal \
    -keyout "$cad/srv.key" -out "$cad/srv.csr" >/dev/null 2>&1 &&
  openssl x509 -req -days 2 -in "$cad/srv.csr" -CA "$cad/ca.pem" -CAkey "$cad/ca.key" -CAcreateserial \
    -extfile "$cad/openssl.cnf" -extensions srv -out "$cad/srv.pem" >/dev/null 2>&1
}
ca_serve() {
  [[ -z $ca_server ]] || return 0
  # -www reads nothing from its input.
  openssl s_server -accept "$1" -cert "$cad/srv.pem" -key "$cad/srv.key" -www -quiet </dev/null >/dev/null 2>&1 &
  ca_server=$!; sleep 0.5
}
# ca_stop: ends ca_serve's server once a section is done with it, so its port is free for later
# sections' checks (the network section's VM server has 47195 too).
ca_stop() { [[ -z $ca_server ]] || { kill "$ca_server" 2>/dev/null; wait "$ca_server" 2>/dev/null; }; ca_server=""; }
# ca_gone VM: the running VM has none of devvm's CA files, and the run's CA isn't in its system's
# bundle: what an image keeps.
ca_gone() {
  limactl shell --workdir / "$1" sh -c '! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 &&
    [ ! -e /etc/ssl/devvm.pem ] && [ ! -e /var/lib/devvm-ca ] && ! grep -qF "$1" /etc/ssl/certs/ca-certificates.crt' \
    _ "$(sed -n 4p "$cad/ca.pem")" </dev/null
}
export -f ca_gone; export cad
# host_port_kept DIR VM PORT CONTROL SERVE: a program on this machine has PORT, which VM lists, on
# every address (with SO_REUSEADDR, as most servers do, so its ended connections don't hold the port
# on Linux once it's gone): the VM's next start leaves it out, warned, and with servers in the
# VM on PORT and CONTROL (SERVE VM PORTS...), PORT's localhost connections still reach the program
# and CONTROL's the VM. Once the program is gone, the next start forwards PORT again.
host_port_kept() { local hp out r=1
  (cd "$1" && "$D" stop </dev/null >/dev/null 2>&1) || { echo "stop failed"; return 1; }
  perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalPort => shift, Listen => 5, ReuseAddr => 1) or die;
    while ($c = $s->accept) { print $c "host-program\n"; close $c }' "$3" &
  hp=$!; sleep 0.5
  (cd "$1" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1)
  if grep -qx "devvm: ports_taken count=1 ports=$3 vm=$2" "$work/last.log"; then
    "$5" "$2" "$3" "$4"
    for _ in $(seq 20); do nc -z 127.0.0.1 "$4" && break; sleep 1; done; sleep 3
    out=$(nc -w 2 127.0.0.1 "$3" </dev/null)
    echo "control $4: $(nc -z 127.0.0.1 "$4" && echo forwarded); $3 answered by: ${out:-nothing}"
    nc -z 127.0.0.1 "$4" && [[ $out == host-program ]] && r=0
  else cat "$work/last.log"; fi
  kill "$hp" 2>/dev/null; wait "$hp" 2>/dev/null
  (( r == 0 )) || return 1
  (cd "$1" && "$D" stop </dev/null >/dev/null 2>&1 && printf 'exit\n' | DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1)
  ! grep -q ports_taken "$work/last.log" || { cat "$work/last.log"; return 1; }
  "$5" "$2" "$3"
  for _ in $(seq 20); do nc -z 127.0.0.1 "$3" && return 0; sleep 1; done
  echo "$3 not forwarded again"; return 1; }
# lima_serve VM PORTS...: a server in the VM on each port, every address, left running.
lima_serve() { local p; for p in "${@:2}"; do in_vm "$1" "setsid -f python3 -m http.server -b 0.0.0.0 $p </dev/null >/dev/null 2>&1"; done; }
# vm_say VM ADDR PORT WORD: a server in a Lima VM on ADDR:PORT alone that answers each connection with
# WORD and a newline (pkill -f '[d]evvm-e2e-say' ends it).
vm_say() {
  in_vm "$1" "setsid -f python3 -c 'import socket,sys; s=socket.socket(); s.bind((sys.argv[1],int(sys.argv[2]))); s.listen(5); [(lambda c: (c.sendall(sys.argv[3].encode()+bytes([10])), c.close()))(s.accept()[0]) for _ in iter(int,1)]' $2 $3 $4 devvm-e2e-say </dev/null >/dev/null 2>&1"
}
# lima_boot VM: its boot's ID.
lima_boot() { in_vm "$1" 'cat /proc/sys/kernel/random/boot_id'; }
# ports_live DIR VM PORT SERVE BOOT: VM runs, listing ports without PORT, with a server in it on
# PORT (SERVE VM PORT), which doesn't reach this machine; devvm ports add PORT says the VM forwards
# its new ports and the server reaches this machine at once; devvm ports rm PORT closes it again.
# The VM is never restarted: BOOT VM (its boot or start) is the same after.
ports_live() { local b0 b1 now
  b0=$("$5" "$2"); "$4" "$2" "$3"; sleep 3
  ! nc -z 127.0.0.1 "$3" || { echo "$3 reached before it's listed"; return 1; }
  (cd "$1" && "$D" ports add "$3" </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  now=$(cd "$1" && "$D" ports 2>/dev/null)
  grep -qF "$(m ports_forwarding ports="$now" vm="$2")" "$work/last.log" || { cat "$work/last.log"; return 1; }
  for _ in $(seq 20); do nc -z 127.0.0.1 "$3" && break; sleep 1; done
  nc -z 127.0.0.1 "$3" || { echo "$3 not forwarded once added"; return 1; }
  (cd "$1" && "$D" ports rm "$3" </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  for _ in $(seq 15); do nc -z 127.0.0.1 "$3" || break; sleep 1; done
  ! nc -z 127.0.0.1 "$3" || { echo "$3 still forwarded once removed"; return 1; }
  b1=$("$5" "$2"); echo "boot or start: ${b0:-none} before, ${b1:-none} after"
  [[ -n $b0 && $b0 == "$b1" ]]; }
# lima_listens tcp|udp PORT: Lima's own processes (limactl's hostagent) have PORT on this machine.
lima_listens() {
  if [[ "$(uname -s)" == Darwin ]]; then
    if [[ $1 == tcp ]]; then lsof -a -nP -c limactl -iTCP:"$2" -sTCP:LISTEN >/dev/null 2>&1; else lsof -a -nP -c limactl -iUDP:"$2" >/dev/null 2>&1; fi
  else
    ss -Hlnp -"${1:0:1}" "sport = :$2" 2>/dev/null | grep -q '"limactl"'
  fi
}
export -f lima_listens
# loopback_only PORT: something on this machine listens on TCP PORT, and only on 127.0.0.1 or ::1.
loopback_only() { local addrs
  if [[ "$(uname -s)" == Darwin ]]; then
    addrs=$(lsof -nP -iTCP:"$1" -sTCP:LISTEN -F n 2>/dev/null | sed -n 's/^n//p')
  else
    addrs=$(ss -Hltn "sport = :$1" 2>/dev/null | awk '{print $4}')
  fi
  [[ -n $addrs ]] && ! grep -qvE "^(127\.0\.0\.1|\[::1\]):$1\$" <<<"$addrs"
}
export -f loopback_only
# desktop_serve DIR [ARGS...]: devvm desktop --no-open ARGS in DIR, in the background ($desk, its
# output in $work/desktop.log), until it says where the VM's desktop is: its address ($desk_addr,
# 127.0.0.1:PORT), port ($desk_port) and password ($desk_pw). desktop_end ends it with Ctrl+C.
desk=""
desktop_serve() {
  local d=$1 i; shift
  desk_addr="" desk_port="" desk_pw=""
  (cd "$d" && DEVVM_MESSAGES=names exec "$D" desktop --no-open "$@" </dev/null >"$work/desktop.log" 2>&1) &
  desk=$!
  for i in $(seq 240); do
    grep -q '^desktop_ready ' "$work/desktop.log" && break
    kill -0 "$desk" 2>/dev/null || break
    sleep 1
  done
  desk_addr=$(sed -n 's/^desktop_ready address=\([^ ]*\) .*/\1/p' "$work/desktop.log")
  desk_pw=$(sed -n 's/^desktop_ready .*password=\([a-z0-9]*\) .*/\1/p' "$work/desktop.log")
  desk_port=${desk_addr##*:}
  [[ -n $desk_addr && -n $desk_pw ]]
}
desktop_end() { [[ -z $desk ]] || { kill -INT "$desk" 2>/dev/null; wait "$desk" 2>/dev/null; desk=""; }; }
# desktop_view [VAR=VALUE...]: TestE2EDesktop on the desktop devvm desktop serves: a wrong password
# refused, the password let in (with DEVVM_E2E_CLIPBOARD, a clipboard and a key event sent).
desktop_view() {
  (cd "$root" && env DEVVM_E2E_DESKTOP="$desk_addr" DEVVM_E2E_PASSWORD="$desk_pw" "$@" CGO_ENABLED=0 \
    "${gocmd[@]}" test -count=1 -v -run '^TestE2EDesktop$' ./internal/desktop) >"$work/view.log" 2>&1
  cat "$work/view.log"; grep -q -- '--- PASS: TestE2EDesktop' "$work/view.log"
}
# desktop_in_vm: what's checked in a VM whose desktop runs, as its user: its Xvnc has its socket in a
# folder of the user's own (0700), the socket 0600, none of its sockets listening on TCP, and the
# clipboard off.
desktop_in_vm='set -e
d=/run/devvm-desktop pid=$(cat /run/devvm-desktop/xvnc.pid)
[ "$(cat "/proc/$pid/comm")" = Xvnc ]
[ "$(stat -c %a "$d")" = 700 ] && [ "$(stat -c %a "$d/vnc.sock")" = 600 ] && [ "$(stat -c %U "$d/vnc.sock")" = "$(id -un)" ]
for f in "/proc/$pid/fd/"*; do
  l=$(readlink "$f" 2>/dev/null) || continue
  case $l in socket:*) i=${l#socket:[}; i=${i%]}
    if awk -v i="$i" "\$4 == \"0A\" && \$10 == i { f = 1 } END { exit !f }" /proc/net/tcp /proc/net/tcp6 2>/dev/null; then
      echo "Xvnc listens on TCP"; exit 1; fi ;;
  esac
done
for p in AcceptCutText SendCutText; do [ "$(DISPLAY=:1 XAUTHORITY="$HOME/.Xauthority" vncconfig -get "$p")" = off ]; done'
# desktop_secret: the password, read from the input, is in no file of the VM's (run as root there),
# its fixed folders searched. Never on a command line: sudo would log it in the VM's journal.
desktop_secret='read -r pw && [ -n "$pw" ] && ! grep -rlsF -D skip -- "$pw" /home /root /etc /run /tmp /var/log /usr/local'
# watch_sees RUN VM DIR BY [NESTED [DELETES [FILES]]]: a watcher in the VM on DIR and a subfolder, as a dev
# server's (RUN VM CMD runs CMD there: in_vm, in_smol, in_d), sees a file changed in each, one made
# and one deleted, by this machine (BY host) or the VM (BY vm), each within 5 s; with NESTED no,
# never the subfolder's (9p's limit); with DELETES no, not the deletion (smolvm's and Docker
# Desktop's limit); with FILES yes, it watches the two changed files too, and a change counts when
# the file's own watch sees it (smolvm's limit: a folder's watch sees a change to a file in it with
# no name, unless the change comes as the file's creation). The watcher is perl (every base has it) calling inotify itself, so the
# test VM needs nothing added. It prints its events, and writes them to a file in a folder of
# DIR's it doesn't watch too, as smolvm's exec passes output on only at the end, and 9p shows this
# machine a file the VM writes only later; they're in $work/watch.out after. watch_misses RUN VM
# DIR: the watcher sees none of this machine's (Lima's limit: polling).
watch_pl='use strict; use IO::Handle;
my %nr = (x86_64 => [294, 254], aarch64 => [26, 27]);
chomp(my $m = qx(uname -m));
my ($init, $add) = @{$nr{$m} or die "no inotify syscalls for $m\n"};
my $fd = syscall($init, 0); $fd >= 0 or die "inotify_init1: $!\n";
my $log = shift @ARGV; open(my $lf, ">>", $log) or die "$log: $!\n";
sub out { print STDOUT @_; print $lf @_ }
my ($top, %rel) = ($ARGV[0]);
for my $dir (@ARGV) {
  my $wd = syscall($add, $fd, $dir, 0x2 | 0x4 | 0x8 | 0x40 | 0x80 | 0x100 | 0x200);
  $wd >= 0 or die "inotify_add_watch $dir: $!\n";
  $rel{$wd} = $dir eq $top ? "" : substr($dir, length($top) + 1) . "/";
}
open(my $in, "<&=", $fd) or die "inotify fd: $!\n";
STDOUT->autoflush(1); $lf->autoflush(1); alarm 120;
my %name = (0x2 => "MODIFY", 0x4 => "ATTRIB", 0x8 => "CLOSE_WRITE", 0x40 => "MOVED_FROM", 0x80 => "MOVED_TO", 0x100 => "CREATE", 0x200 => "DELETE");
out("ready\n");
while (sysread($in, my $buf, 65536)) {
  for (my $i = 0; $i + 16 <= length $buf; ) {
    my ($wd, $mask, undef, $len) = unpack("iIII", substr($buf, $i, 16));
    my $f = $rel{$wd} . unpack("Z*", substr($buf, $i + 16, $len));
    for my $bit (sort { $a <=> $b } keys %name) { out("$name{$bit} $f\n") if $mask & $bit }
    $i += 16 + $len;
  }
}'
# watch_events: what the watcher printed and wrote so far, each line once.
watch_events() { cat "$watch_log" "$work/watch.stream" 2>/dev/null | awk '!seen[$0]++'; }
watch_saw() { local end=$((SECONDS + 5))
  while (( SECONDS <= end )); do watch_events | grep -qE "^$1\$" && return 0; sleep 0.2; done
  echo "nothing for $1 within 5 s" >> "$work/watch.missed"; return 1; }
# watch_do RUN VM BY CMD: CMD on this machine (BY host) or in the VM.
watch_do() { if [[ $3 == host ]]; then bash -c "$4"; else "$1" "$2" "$4" >/dev/null; fi; }
watch_sees() { local w r=1 files="" own=""
  watch_log=$3/.e2e-watch/events
  mkdir -p "$3/e2e-sub" "$3/.e2e-watch" && printf '%s\n' "$watch_pl" > "$3/.e2e-watch/watch.pl" && : > "$watch_log" &&
    echo before > "$3/watched.txt" && echo before > "$3/e2e-sub/nested.txt" || return 1
  : > "$work/watch.missed"
  # FILES yes: the files' own watches print their events as "MODIFY watched.txt/".
  if [[ ${7:-no} == yes ]]; then files=" '$3/watched.txt' '$3/e2e-sub/nested.txt'" own='/?'; fi
  # The last check deleted .e2e-watch, and the VM can go on seeing it as gone for a while after
  # this machine writes it again (Lima's virtiofs, right after the check before): the VM waits up
  # to 20 s for the script, then runs it, which fails naming the file if it's still missing.
  "$1" "$2" "for _ in \$(seq 100); do [ -s '$3/.e2e-watch/watch.pl' ] && break; sleep 0.2; done
    LC_ALL=C exec perl '$3/.e2e-watch/watch.pl' '$watch_log' '$3' '$3/e2e-sub'$files" > "$work/watch.stream" 2>&1 &
  w=$!
  for _ in $(seq 60); do watch_events | grep -qx ready && break; sleep 0.5; done
  if watch_events | grep -qx ready; then
    r=0   # each one tried, so the output shows them all
    # A change may come as a new file (smolvm's): either way, a watcher looks at the file again.
    watch_do "$1" "$2" "$4" "echo after >> '$3/watched.txt'"; watch_saw "(MODIFY|CLOSE_WRITE|CREATE) watched.txt$own" || r=1
    watch_do "$1" "$2" "$4" "echo after >> '$3/e2e-sub/nested.txt'"
    if [[ ${5:-yes} == yes ]]; then watch_saw "(MODIFY|CLOSE_WRITE|CREATE) e2e-sub/nested.txt$own" || r=1; fi
    watch_do "$1" "$2" "$4" "echo made > '$3/made.txt'"; watch_saw '(CREATE|MOVED_TO) made.txt' || r=1
    watch_do "$1" "$2" "$4" "rm '$3/made.txt'"
    if [[ ${6:-yes} == yes ]]; then watch_saw '(DELETE|MOVED_FROM) made.txt' || r=1
    else sleep 5; ! watch_events | grep -qE '^(DELETE|MOVED_FROM) made.txt$' || { echo "the deletion was seen" >> "$work/watch.missed"; r=1; }; fi
    # NESTED no: the subfolder's change is never seen, even after the rest.
    if [[ ${5:-yes} == no ]] && watch_events | grep -q ' e2e-sub/'; then
      echo "the subfolder's change was seen" >> "$work/watch.missed"; r=1
    fi
  fi
  "$1" "$2" "pkill -f '[.]e2e-watch/watch[.]pl'" >/dev/null 2>&1
  kill "$w" 2>/dev/null; wait "$w" 2>/dev/null
  watch_events > "$work/watch.out"
  echo "the watcher saw:"; cat "$work/watch.out" "$work/watch.missed"   # what it missed last
  rm -rf "$3/.e2e-watch" "$3/watched.txt" "$3/made.txt" "$3/e2e-sub"
  return "$r"; }
watch_misses() { watch_sees "$1" "$2" "$3" host; grep -qx ready "$work/watch.out" && ! grep -qvx ready "$work/watch.out"; }

# Links between VMs: link_section's checks, the same on each backend, each the end-to-end proof of a
# row of the design note's threat check (internals/design.md, "Links between VMs"). lk_be is the
# backend they run on, lk_cli devvm's flag for it.
lk_be=lima lk_cli=()
# lk_pl: the VMs' helper, /var/tmp/e2e-link.pl (perl: every base has it). serve ADDR PORT WORD
# answers each connection with an HTTP response whose body is WORD; hold ADDR PORT keeps every
# connection it accepts open (each writes its pid to /var/tmp/e2e-link-ADDR-PORT.pid); conns HOST
# PORT N opens N connections, all at once but the last, 5 s after the others, and prints those the far
# end closed (a burst past the backlog of the link's socket on macOS, 128, which devvm _links accepts
# whole before it starts their relays);
# listeners prints each TCP listener as "PROCESS ADDR:PORT" (as root, to see every process's).
lk_pl='use strict; use warnings; use IO::Socket::INET; use IO::Select;
my $mode = shift // "";
sub v4 { join ".", reverse map { hex } $_[0] =~ /(..)/g }
sub addr { my $a = uc shift;
  return v4($a) if length $a == 8;
  return "[::]" if $a =~ /^0{32}$/;
  return "[::1]" if $a eq "00000000000000000000000001000000";
  return "[::ffff:" . v4($1) . "]" if $a =~ /^0{16}FFFF0000(.{8})$/;
  return "[$a]"; }
if ($mode eq "serve" or $mode eq "hold") {
  my ($host, $port, $word) = @ARGV;
  my $s = IO::Socket::INET->new(LocalAddr => $host, LocalPort => $port, Listen => 1024, ReuseAddr => 1) or die "$host:$port: $!\n";
  open(my $pf, ">", "/var/tmp/e2e-link-$host-$port.pid") or die "$!\n"; print $pf "$$\n"; close $pf;
  my @held;
  while (my $c = $s->accept) {
    if ($mode eq "hold") { push @held, $c; next }
    my $sel = IO::Select->new($c); my $buf = "";
    while ($buf !~ /\n\r?\n/ and $sel->can_read(2)) { sysread($c, $buf, 4096, length $buf) or last }
    my $body = "$word\n";
    print $c "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nContent-Length: " . length($body) . "\r\nConnection: close\r\n\r\n$body";
    close $c;
  }
} elsif ($mode eq "conns") {
  my ($host, $port, $n) = @ARGV; my @c;
  for my $i (1 .. $n) {
    sleep 5 if $i == $n;
    push @c, IO::Socket::INET->new(PeerAddr => $host, PeerPort => $port, Timeout => 10) || die "connection $i: $!\n";
  }
  sleep 5;
  my @closed;
  for my $i (0 .. $#c) { push @closed, $i + 1 if IO::Select->new($c[$i])->can_read(0) and !sysread($c[$i], my $x, 1) }
  print "closed: @closed\n";
} elsif ($mode eq "listeners") {
  my %own;
  for my $d (glob "/proc/[0-9]*") {
    open(my $f, "<", "$d/comm") or next; my $n = <$f> // "?"; chomp $n;
    opendir(my $fd, "$d/fd") or next;
    for my $e (readdir $fd) { my $l = readlink("$d/fd/$e") // next; $own{$1} = $n if $l =~ /^socket:\[(\d+)\]$/ }
  }
  for my $t ("tcp", "tcp6") {
    open(my $h, "<", "/proc/net/$t") or next; my $head = <$h>;
    while (my $line = <$h>) {
      my @f = split " ", $line; next unless $f[3] eq "0A";
      my ($a, $p) = split /:/, $f[1];
      printf "%s %s:%d\n", $own{$f[9]} // "?", addr($a), hex $p;
    }
  }
} else { die "e2e-link.pl: serve, hold, conns or listeners\n" }'
# lk_user VM CMD: CMD in a login shell as the VM's user, on lk_be; lk_root VM CMD: as root; lk_bg VM
# CMD: as root, left running; lk_put VM FILE: the input, as FILE in the VM.
lk_user() { case $lk_be in lima) in_vm "$1" "$2" ;; smolvm) in_smol "$1" "$2" ;; *) in_d "$1" "$2" ;; esac; }
lk_root() { case $lk_be in
  lima) limactl shell --workdir / "$1" sudo sh -c "$2" </dev/null ;;
  smolvm) SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c "$2" </dev/null ;;
  *) docker exec -u 0 "$1" sh -c "$2" </dev/null ;; esac; }
lk_bg() { case $lk_be in
  lima) lk_root "$1" "setsid -f $2 </dev/null >/dev/null 2>&1" ;;
  smolvm) SMOLVM_USAGE=0 smolvm machine exec --name "$1" -d -- sh -c "$2" </dev/null >/dev/null 2>&1 ;;
  *) docker exec -d -u 0 "$1" sh -c "$2" </dev/null >/dev/null 2>&1 ;; esac; }
lk_put() { case $lk_be in
  lima) limactl shell --workdir / "$1" sudo sh -c 'cat > "$1"' _ "$2" ;;
  smolvm) SMOLVM_USAGE=0 smolvm machine exec --name "$1" -i -- sh -c 'cat > "$1"' _ "$2" ;;
  *) docker exec -i -u 0 "$1" sh -c 'cat > "$1"' _ "$2" ;; esac; }
# lk_running VM: it runs, as its backend says.
lk_running() { case $lk_be in
  lima) [[ $(status_of "$1") == Running ]] ;; smolvm) [[ $(smol_status "$1") == running ]] ;; *) [[ $(d_state "$1") == running ]] ;; esac; }
# lk_until SECONDS CMD...: CMD, once a second until it passes, for up to SECONDS; lk_not CMD...: CMD fails.
lk_until() { local n=$1 i; shift; for ((i = 1; i < n; i++)); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done; "$@"; }
lk_not() { ! "$@"; }
# lk_start VM serve|hold ADDR PORT [WORD]: the helper's server in the VM, as root, once it listens;
# lk_end VM ADDR PORT ends it. lk_serve VM WORD PORTS...: the helper written to the VM, and a server
# on its 127.0.0.1 on each port, answering WORD-PORT.
lk_start() { local f=/var/tmp/e2e-link-$3-$4.pid
  if ! { lk_root "$1" "rm -f $f" && lk_bg "$1" "perl /var/tmp/e2e-link.pl $2 $3 $4 ${5:-}" && lk_until 20 lk_root "$1" "test -s $f"; }; then
    echo "the helper's $2 on $3:$4 in $1 didn't start in 20s"; return 1
  fi; }
lk_end() { lk_root "$1" "f=/var/tmp/e2e-link-$2-$3.pid; [ ! -s \$f ] || kill \$(cat \$f); rm -f \$f"; }
lk_serve() { local vm=$1 w=$2 p; shift 2
  printf '%s\n' "$lk_pl" | lk_put "$vm" /var/tmp/e2e-link.pl || return 1
  for p in "$@"; do lk_start "$vm" serve 127.0.0.1 "$p" "$w-$p" || return 1; done; }
# lk_raw VM HOST PORT: what a raw TCP client in the VM (bash's /dev/tcp) gets back for a request: the
# body's line. lk_gets VM HOST PORT WORD: it's WORD. lk_curls VM URL WORD: curl gets WORD, through the
# VM's proxy settings when it has them. lk_resolves VM NAME: the name resolves in the VM.
lk_raw() { lk_user "$1" "exec 3<>/dev/tcp/$2/$3 && printf 'GET / HTTP/1.0\r\n\r\n' >&3 && timeout 10 cat <&3 | tail -n 1"; }
lk_gets() { local a; a=$(lk_raw "$1" "$2" "$3" 2>/dev/null); echo "$2:$3 answered: ${a:-nothing}"; [[ $a == "$4" ]]; }
lk_curls() { local a; a=$(lk_user "$1" "curl -fsS --max-time 10 $2" 2>/dev/null); echo "$2 answered: ${a:-nothing}"; [[ $a == "$3" ]]; }
lk_resolves() { lk_user "$1" "getent hosts $2" >/dev/null 2>&1; }
# lk_dv DIR ARGS...: devvm ARGS in DIR, messages by name, its output in $work/last.log; lk_up DIR
# [ARGS...]: the same with a shell that ends at once. lk_said LINE: the output has that line.
lk_dv() { local d=$1; shift; (cd "$d" && DEVVM_MESSAGES=names "$D" "$@" </dev/null >"$work/last.log" 2>&1); }
lk_up() { local d=$1; shift; (cd "$d" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "$@" >"$work/last.log" 2>&1); }
# lk_shows DIR REGEX ARGS...: devvm ARGS' output in DIR has a line matching REGEX (read whole first:
# grep -q ending a pipe early fails it, under pipefail).
lk_shows() { local o; o=$(cd "$1" && "$D" "${@:3}" 2>/dev/null); grep -Eq -- "$2" <<<"$o"; }
lk_said() { grep -qxF -- "$1" "$work/last.log" || { echo "not said: $1"; tail -5 "$work/last.log"; return 1; }; }
# lk_ssh ARGS...: ssh through devvm's ssh config, without an agent.
lk_ssh() { env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -o BatchMode=yes "$@" </dev/null; }
# lk_addr VM NAME: the address its links named NAME listen on, from its state. lk_hash PARTS...: the
# hash devvm names a link session's files by. lk_sock VM ADDR PORT: a link's socket; lk_ctl VM: the
# session's control socket. lk_mode PATH: its mode, in octal.
lk_addr() { sed -n 's/^links=//p' "$vms/$1" 2>/dev/null | tr ' ' '\n' | awk -F, -v n="$2" '$1 == n { print $5; exit }'; }
lk_hash() { local s=$1; shift
  { printf '%s' "$s"; for s in "$@"; do printf '\0%s' "$s"; done; } | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16; }
lk_sock() { echo "$XDG_DATA_HOME/devvm/run/link-$(lk_hash "$1" socket "$2" "$3").sock"; }
lk_ctl() { echo "$XDG_DATA_HOME/devvm/run/link-$(lk_hash "$1" session).ctl"; }
lk_mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
# lk_session VM: the pid of its devvm _links; lk_master VM: of that session's ssh master to it.
# lk_desc PIDS...: them and their descendants, comma-separated.
lk_session() { local p; p=$(pgrep -f -- "[_]links $1\$" | head -1); [[ -n $p ]] && echo "$p"; }
lk_master() { local p; p=$(pgrep -f -- "[-]M -S $(lk_ctl "$1") " | head -1); [[ -n $p ]] && echo "$p"; }
lk_desc() { local p c; for p in "$@"; do echo "$p"; for c in $(pgrep -P "$p" 2>/dev/null); do lk_desc "$c"; done; done; }
# lk_sshport VM: the host loopback port devvm's ssh reaches it on ("" through a ProxyCommand: Docker).
lk_sshport() { local p; p=$(sed -n 's/^ssh_port=//p' "$vms/$1" 2>/dev/null)
  [[ -n $p ]] || p=$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$1.json" 2>/dev/null)
  echo "$p"; }
# lk_host_tcp PIDS: those processes' TCP sockets on this machine, "LOCAL (LISTEN)" or "LOCAL->PEER (STATE)".
lk_host_tcp() {
  if [[ "$(uname -s)" == Darwin ]]; then lsof -nP -a -p "$1" -iTCP 2>/dev/null | awk 'NR > 1 { print $9, $10 }'
  else ss -Htanp 2>/dev/null | grep -E "pid=($(tr , '|' <<<"$1"))," | awk '{ print ($1 == "LISTEN" ? $4 : $4 "->" $5), "(" $1 ")" }'; fi; }
# lk_host_clean CLIENT VMS...: CLIENT's link session and every process it started listen on no TCP
# port of this machine's, and connect only to the ssh ports of VMS on its loopback.
lk_host_clean() { local pid t peer v ok bad=""; pid=$(lk_session "$1") || { echo "no link session"; return 1; }; shift
  t=$(lk_host_tcp "$(lk_desc "$pid" | paste -sd, -)"); echo "the link session's TCP: $(paste -sd ' ' - <<<"${t:-none}")"
  ! grep -q LISTEN <<<"$t" || return 1
  while read -r peer; do
    [[ -n $peer ]] || continue
    ok=no; for v in "$@"; do [[ $peer != "127.0.0.1:$(lk_sshport "$v")" ]] || ok=yes; done; [[ $ok == yes ]] || bad+=" $peer"
  done < <(grep -- '->' <<<"$t" | awk '{ sub(/^.*->/, "", $1); print $1 }')
  [[ -z $bad ]] || { echo "connected to:$bad"; return 1; }; }
# lk_listens VM: its TCP listeners, as the helper prints them, read as root and as its user: root in a
# container may not read the processes of the user's (no CAP_SYS_PTRACE), whose sshd has the links'.
# On Docker neither can: each connection's sshd (sshd -i) isn't dumpable, so a listener no process
# shows is counted as sshd's ("sshd?"), which only makes lk_vm_clean stricter. lk_vm_clean VM
# PORTS...: its sshd listens only on its own port (22) and links' addresses, and on each PORT sshd
# listens on a link's address and nothing listens anywhere else (127.0.0.1, ::1, every address).
lk_listens() {
  { lk_root "$1" 'perl /var/tmp/e2e-link.pl listeners'; lk_user "$1" 'perl /var/tmp/e2e-link.pl listeners'; } 2>/dev/null |
    awk -v d="$lk_be" '!($2 in n) || n[$2] == "?" { n[$2] = $1 } END { for (a in n) print (d == "docker" && n[a] == "?" ? "sshd?" : n[a]), a }'; }
lk_vm_clean() { local vm=$1 l p bad; shift
  l=$(lk_listens "$vm") || { echo "no listeners read"; return 1; }
  bad=$(awk '$1 ~ /^sshd/ && $2 !~ /:22$/ && $2 !~ /^127\.77\./' <<<"$l")
  [[ -z $bad ]] || { echo "sshd also listens on: $bad"; return 1; }
  for p in "$@"; do
    grep -qE "^sshd[^ ]* 127\.77\.[0-9]+\.[0-9]+:$p\$" <<<"$l" || { echo "no link listens on $p: $(grep ":$p\$" <<<"$l")"; return 1; }
    bad=$(grep ":$p\$" <<<"$l" | grep -v ' 127\.77\.')
    [[ -z $bad ]] || { echo "on $p: $bad"; return 1; }
  done; }
# lk_event VM EVENT NAME PORT [TEXT]: its event log has that event for the link NAME:PORT, with TEXT.
lk_event() { grep -F "\"event\":\"$2\",\"host\":\"$3\",\"port\":$4," "$XDG_DATA_HOME/devvm/logs/$1.jsonl" 2>/dev/null | grep -qF -- "${5:-}"; }
# lk_set_net VM on|off: its state's network policy set by hand, as nothing devvm runs would: the rule
# broken under devvm, which checks it again on every connection and at each launch.
lk_set_net() { local s; s=$(sed "s/^net=.*/net=$2/" "$vms/$1") && printf '%s\n' "$s" > "$vms/$1"; }
# lk_hostile VM [gateway]: root in the client loosens its sshd (any listen address, all forwarding;
# with gateway, GatewayPorts yes too) and makes it read that; lk_honest VM puts it back.
lk_hostile_sh='if [ -f /etc/devvm/ssh/sshd_config ]; then c=/etc/devvm/ssh/sshd_config
  [ -f $c.e2e ] || cp $c $c.e2e
  sed -i -e "s/^PermitListen .*/PermitListen any/" -e "s/^AllowTcpForwarding .*/AllowTcpForwarding yes/" $c
  [ "$h" != gateway ] || sed -i "s/^GatewayPorts .*/GatewayPorts yes/" $c
  [ ! -s /run/devvm-sshd.pid ] || kill -HUP "$(cat /run/devvm-sshd.pid)"
else { echo "PermitListen any"; echo "AllowTcpForwarding yes"; [ "$h" != gateway ] || echo "GatewayPorts yes"; } > /etc/ssh/sshd_config.d/00-aaa-e2e.conf
  systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || rc-service sshd reload 2>/dev/null || true
fi'
lk_honest_sh='if [ -f /etc/devvm/ssh/sshd_config.e2e ]; then mv /etc/devvm/ssh/sshd_config.e2e /etc/devvm/ssh/sshd_config
  [ ! -s /run/devvm-sshd.pid ] || kill -HUP "$(cat /run/devvm-sshd.pid)"
else rm -f /etc/ssh/sshd_config.d/00-aaa-e2e.conf
  systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || rc-service sshd reload 2>/dev/null || true
fi'
lk_hostile() { lk_root "$1" "h=${2:-}; $lk_hostile_sh"; }
lk_honest() { lk_root "$1" "$lk_honest_sh"; }
# lk_reconnect VM NAME PORT WORD: its link session's connection ended (its master killed), so the
# session makes a new one, to sshd as it is now; the link NAME:PORT answers WORD again through it.
lk_reconnect() { local m; m=$(lk_master "$1"); [[ -n $m ]] || { echo "no master"; return 1; }
  kill "$m"; lk_until 30 lk_new_master "$1" "$m" && lk_until 30 lk_gets "$1" "$2" "$3" "$4"; }
lk_new_master() { local n; n=$(lk_master "$1"); [[ -n $n && $n != "$2" ]]; }

# link_section BACKEND PREFIX: the link checks on BACKEND (lima, smolvm, docker), the folders named
# from PREFIX: PREFIXdb and PREFIXapi the targets, PREFIXapp the client. Three small VMs without
# network policy, then network policy on the client and the first target; ports 47240-47249, and 80.
link_section() {
  lk_be=$1 lk_x=$2 lk_cli=(); [[ $lk_be == lima ]] || lk_cli=(--backend "$lk_be")
  sc=${sc:-$work/ssh}   # rfwd_links's socket forward names a path there
  lk_p1=$(newproj "${lk_x}db") lk_p2=$(newproj "${lk_x}api") lk_pc=$(newproj "${lk_x}app")
  local small=(--no-network-policy --no-git-protect --cpus 2 --memory 2)
  launch "$lk_p1" exit ${lk_cli[@]+"${lk_cli[@]}"} "${small[@]}" --ports none base; lk_v1=$vm
  launch "$lk_p2" exit ${lk_cli[@]+"${lk_cli[@]}"} "${small[@]}" --ports none base; lk_v2=$vm
  launch "$lk_pc" exit ${lk_cli[@]+"${lk_cli[@]}"} "${small[@]}" --ports all base; lk_vc=$vm
  lk_w1=${lk_x}db lk_w2=${lk_x}api lk_n2=${lk_x}api   # the targets' servers' words; the second's link name
  check "two targets that forward no port and a client that forwards every port, without network policy" lk_made
  lk_serve "$lk_v1" "$lk_w1" 47240 47242 47243 47244 47249 80 >/dev/null 2>&1
  lk_start "$lk_v1" hold 127.0.0.1 47241 >/dev/null 2>&1
  lk_serve "$lk_v2" "$lk_w2" 47240 80 >/dev/null 2>&1
  printf '%s\n' "$lk_pl" | lk_put "$lk_vc" /var/tmp/e2e-link.pl >/dev/null 2>&1
  check "sshd -T in every VM: remote forwards on 127.77.*:* alone, at the address asked, no socket forwards, tunnels or X11" \
    lk_sshd_t "$lk_v1" "$lk_v2" "$lk_vc"
  if [[ $lk_be == lima ]]; then
    check "Lima's own rules ignore every port of the client's" bash -c "
      [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $lk_vc) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]]"
  fi
  check "devvm link publish offers ports, and lists them, with \"version\": 1 in its --json" lk_publish
  check "a port the target doesn't publish is refused by devvm link add" lk_unpublished_add
  check "devvm link add on the running client applies within seconds: NAME:PORT and NAME.devvm.internal:PORT, by curl and a raw TCP client" lk_add_applies
  check "--as NAME names the link alone: the target's folder name doesn't resolve in the client" lk_as_alone
  check "two targets publishing the same port are each reached by their own name" lk_two
  check "a port below 1024 (80) is linked on the same port, sshd listening on it as the VM's user" lk_low
  check "devvm link lists each link, with its target, folder and status, and \"version\": 1 in its --json" lk_list
  check "devvm network on is refused for a client linked to a target without network policy" lk_on_refused
  check "a published port that isn't linked is refused: nothing listens for it in the client" lk_not_linked
  check "after devvm link unpublish, the link's next connection is refused and logged, and the clients are named" lk_unpublish
  check "devvm link rm on the running client applies within seconds: its listener goes" lk_rm_applies
  check "nothing from a link listens on the client's 127.0.0.1, ::1 or every address; its sshd listens only on its own port and the links' addresses" \
    lk_vm_clean "$lk_vc" 47240 80
  check "the client forwards every port, and no link is forwarded to this machine" lk_not_forwarded
  check "a server of the client's own on its 127.0.0.1 with a link's port runs and reaches this machine as usual (the control); the link still reaches its target" lk_own_server
  check "each link's end on this machine is a socket of mode 600 in a folder of mode 700, and no TCP port of this machine's carries it" lk_sockets
  check "ssh -R into the client binds nothing on 0.0.0.0, 127.0.0.1, ::1, localhost, another range or a socket; on 127.77.0.0/16 it does (the control)" \
    rfwd_links "$lk_vc" lk_ssh
  check "a program in the client on a link's address and port first fails that link alone (link_port_taken); another link works" lk_port_taken
  check "256 connections at once, past the socket's backlog, all carried; a 257th is refused, and devvm network log shows link_limit" lk_cap
  check "root in the client lets sshd bind any address: the link session still asks only for its links' addresses, and the link works" lk_hostile_listen
  check "... with GatewayPorts yes too: the link session still listens on no TCP port here and connects only to the VMs' sshd, and the link works" lk_hostile_gateway
  echo "  recorded: $(cat "$work/link-gateway.log" 2>/dev/null)"; rm -f "$work/link-gateway.log"
  check "sshd put back, the session's next connection binds the links' addresses alone again" lk_honest_again
  check "devvm network log shows the links' connections and refusals without network policy" lk_log_off
  check "a stopped target: the connection fails, devvm network log shows link_target_stopped, the launch warns, and the target stays stopped" lk_stopped
  check "restarted, it works again, with its new host key" lk_restarted
  check "devvm rm of the target removes the client's link, names the client, and its sockets go; the client's session runs on" lk_rm_target
  check "a VM made again in that folder isn't linked" lk_made_again
  # Groups: the three VMs, none with network policy, in one group.
  lk_g=${lk_x}g
  check "three VMs join a group, the first making it, the trust said on each join: each reaches the ports the others publish by name" gp_join
  check "devvm group lists the three, each with its name, folder and published ports, and \"version\": 1 in its --json" gp_show
  check "a port a member publishes reaches the others' links at once" gp_publish
  check "in a group: devvm network on is refused, so is joining another group, and devvm link rm of the group's link" gp_refused
  check "devvm group leave removes only the group's links, in it and the others, naming them; the user's link stays; the last one's leaving removes the group" gp_leave
  check "a join is refused when a member's name is the joiner's own hostname, whatever name it joins as" gp_hostname
  check "devvm network on for a target while it runs, then for the client, restarted behind its proxy" lk_policy_on
  check "the target's network policy on but not yet enforced (devvm ls: unenforced): the link's next connection is refused and logged, and devvm link add and the client's launch warn" lk_policy_unenforced
  check "the target restarted behind its proxy" lk_policy_target
  check "with network policy, the client reaches NAME:PORT and NAME.devvm.internal:PORT by curl through its proxy settings and a raw TCP client; a direct connection fails (the control)" lk_policy_reach
  check "a link added while the client with network policy runs: curl through its proxy reaches it at once, by GET and CONNECT, no link name is in NO_PROXY, and nothing restarts" lk_proxy_added
  check "the client's proxy reaches only the links on its own list: not a name or port it hasn't linked, a name doubled or an address; the target's proxy doesn't reach the client's links" lk_proxy_own
  check "devvm network log shows the links' connections with network policy" lk_log_on
  check "a client with network policy is refused by devvm link add for a target without it" lk_mismatch_add
  check "devvm network off is refused for a target that a client with network policy links to" lk_off_refused
  check "the rule broken under devvm: the link's next connection is refused, and the client's launch warns and runs on without it" lk_policy_broken
  check "the rule restored, the client's next launch has the link again" lk_policy_back
  if [[ $lk_be == lima ]] && want from; then
    check "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" lk_alpine
  elif [[ $lk_be == lima ]]; then
    skip "a port below 1024 on Lima's Alpine VMs: an Alpine client reaches its link to port 80 on the same port" "Alpine's image comes from the from section"
  fi
  check "a group is all or none: a VM without network policy is refused by a group with it, and one with it by a group without; two with it reach each other through their group's links" gp_policy
  check "devvm rm of the client leaves no link session, socket, pid file, log or link state" lk_rm_client
  (cd "$lk_p1" && quiet "$D" rm --force --no-export)
  (cd "$lk_p2" && quiet "$D" rm --force --no-export)
}
lk_made() { local v
  for v in "$lk_v1" "$lk_v2" "$lk_vc"; do
    if ! { [[ -n $v ]] && lk_running "$v" && grep -qx net=off "$vms/$v"; }; then echo "${v:-a VM} wasn't made"; return 1; fi; done
  grep -qx ports=none "$vms/$lk_v1" && grep -qx ports=none "$vms/$lk_v2" && ! grep -q '^ports=' "$vms/$lk_vc"; }
lk_sshd_t() { local vm t want
  for vm in "$@"; do
    t=$(lk_root "$vm" 's=$(command -v sshd || echo /usr/sbin/sshd); c=/etc/devvm/ssh/sshd_config; if [ -f $c ]; then $s -T -f $c; else $s -T; fi') ||
      { echo "$vm: no sshd -T"; return 1; }
    for want in 'allowtcpforwarding yes' 'gatewayports clientspecified' 'permitlisten 127.77.*:*' 'allowstreamlocalforwarding local' \
      'permittunnel no' 'x11forwarding no'; do
      grep -qxF "$want" <<<"$t" || { echo "$vm: not $want"; return 1; }
    done
  done; }
lk_publish() {
  # 47243 on its own, as devvm link unpublish takes ports as they're listed.
  lk_dv "$lk_p1" link publish 80,47240-47242,47244 && lk_said "==> published ports=80,47240-47242,47244 vm=$lk_v1" &&
    lk_dv "$lk_p1" link publish 47243 && lk_said "==> published ports=80,47240-47242,47244,47243 vm=$lk_v1" &&
    lk_dv "$lk_p2" link publish 47240,80 && lk_said "==> published ports=47240,80 vm=$lk_v2" || return 1
  [[ $(cd "$lk_p1" && "$D" link publish 2>/dev/null) == 80,47240-47242,47244,47243 ]] &&
    lk_shows "$lk_p1" '^  "version": 1,$' link publish --json &&
    lk_shows "$lk_p1" '^  "ports": "80,47240-47242,47244,47243"$' link publish --json; }
lk_unpublished_add() {
  ! lk_dv "$lk_pc" link add "${lk_x}db" 47249 && lk_said "devvm: link_not_published port=47249 vm=$lk_v1" && ! grep -q '^links=' "$vms/$lk_vc"; }
lk_add_applies() { local s=$SECONDS
  lk_dv "$lk_pc" link add "${lk_x}db" 47240 --as db && lk_said "==> link_added name=db port=47240 target=$lk_v1 vm=$lk_vc" || return 1
  lk_until 15 lk_curls "$lk_vc" http://db:47240/ "$lk_w1-47240" || return 1
  echo "reached $((SECONDS - s)) s after devvm link add began"
  lk_curls "$lk_vc" http://db.devvm.internal:47240/ "$lk_w1-47240" && lk_gets "$lk_vc" db 47240 "$lk_w1-47240" &&
    lk_gets "$lk_vc" db.devvm.internal 47240 "$lk_w1-47240" && [[ $(lk_addr "$lk_vc" db) == 127.77.0.1 ]]; }
lk_as_alone() { lk_resolves "$lk_vc" db && lk_resolves "$lk_vc" db.devvm.internal && ! lk_resolves "$lk_vc" "${lk_x}db" &&
  ! lk_resolves "$lk_vc" "${lk_x}db.devvm.internal"; }
lk_two() {
  lk_dv "$lk_pc" link add "$lk_n2" 47240 && lk_said "==> link_added name=$lk_n2 port=47240 target=$lk_v2 vm=$lk_vc" || return 1
  lk_until 15 lk_gets "$lk_vc" "$lk_n2" 47240 "$lk_w2-47240" && lk_gets "$lk_vc" db 47240 "$lk_w1-47240" &&
    [[ $(lk_addr "$lk_vc" "$lk_n2") == 127.77.0.2 ]]; }
lk_low() {
  lk_dv "$lk_pc" link add "$lk_n2" 80 && lk_said "==> link_added name=$lk_n2 port=80 target=$lk_v2 vm=$lk_vc" || return 1
  echo "the client's ip_unprivileged_port_start: $(lk_root "$lk_vc" 'cat /proc/sys/net/ipv4/ip_unprivileged_port_start')"
  lk_until 15 lk_gets "$lk_vc" "$lk_n2" 80 "$lk_w2-80" && grep -qE '^sshd[^ ]* 127\.77\.0\.2:80$' <<<"$(lk_listens "$lk_vc")"; }
lk_list() { local l j
  l=$(cd "$lk_pc" && "$D" link 2>/dev/null); echo "$l"
  grep -Eq "^db +47240 +$lk_v1 +$lk_p1 +Running +yes\$" <<<"$l" && grep -Eq "^$lk_n2 +80 +$lk_v2 +$lk_p2 +Running +yes\$" <<<"$l" || return 1
  j=$(cd "$lk_pc" && "$D" link --json 2>/dev/null)
  grep -qxF '  "version": 1,' <<<"$j" && grep -qxF '      "name": "db",' <<<"$j" && grep -qxF "      \"target\": \"$lk_v2\"," <<<"$j"; }
lk_on_refused() { local r=0
  lk_dv "$lk_pc" network on && r=1
  cat "$work/last.log"
  (( r == 0 )) && grep -Eq "^devvm: network_on_linked targets=.*$lk_v1.* vm=$lk_vc\$" "$work/last.log" &&
    grep -Eq "^devvm: network_on_linked targets=.*$lk_v2.* vm=$lk_vc\$" "$work/last.log" && grep -qx net=off "$vms/$lk_vc"; }
lk_not_linked() { local a; a=$(lk_raw "$lk_vc" db 47244 2>&1); echo "db:47244: ${a:-nothing}"
  ! lk_raw "$lk_vc" db 47244 >/dev/null 2>&1; }
lk_unpublish() {
  lk_dv "$lk_pc" link add "${lk_x}db" 47243 || { tail -5 "$work/last.log"; return 1; }
  lk_until 15 lk_gets "$lk_vc" db 47243 "$lk_w1-47243" || return 1
  lk_dv "$lk_p1" link unpublish 47243 && lk_said "==> unpublished ports=47243 vm=$lk_v1" && lk_said "unpublished_linked vm=$lk_v1 vms=$lk_vc" || return 1
  lk_gets "$lk_vc" db 47243 "" && lk_until 5 lk_event "$lk_vc" link_refused db 47243 '"reason":"unpublished"' &&
    lk_shows "$lk_pc" "^db +47243 +$lk_v1 +$lk_p1 +Running +unpublished\$" link; }
lk_rm_applies() { local s=$SECONDS
  lk_dv "$lk_pc" link rm db 47243 && lk_said "==> link_removed name=db port=47243 vm=$lk_vc" || return 1
  lk_until 15 lk_not lk_listening "$lk_vc" 127.77.0.1:47243 || return 1
  echo "gone $((SECONDS - s)) s after devvm link rm began"
  ! lk_raw "$lk_vc" db 47243 >/dev/null 2>&1 && [[ ! -e $(lk_sock "$lk_vc" 127.77.0.1 47243) ]]; }
lk_listening() { grep -q " $2\$" <<<"$(lk_listens "$1")"; }
lk_not_forwarded() { sleep 3; lk_listening "$lk_vc" 127.77.0.1:47240 && ! nc -z 127.0.0.1 47240 && ! nc -z ::1 47240; }
lk_own_server() { local h
  lk_start "$lk_vc" serve 127.0.0.1 47240 own || return 1
  for _ in $(seq 20); do h=$(curl -s --max-time 5 http://127.0.0.1:47240/ 2>/dev/null); [[ $h != own ]] || break; sleep 1; done
  echo "this machine's 127.0.0.1:47240 answered: ${h:-nothing}"
  [[ $h == own ]] && lk_curls "$lk_vc" http://127.0.0.1:47240/ own && lk_gets "$lk_vc" db 47240 "$lk_w1-47240" &&
    lk_end "$lk_vc" 127.0.0.1 47240; }
lk_sockets() { local s r=0 dir=$XDG_DATA_HOME/devvm/run
  for s in "$(lk_sock "$lk_vc" 127.77.0.1 47240)" "$(lk_sock "$lk_vc" 127.77.0.2 47240)" "$(lk_sock "$lk_vc" 127.77.0.2 80)"; do
    echo "$s: $(lk_mode "$s" 2>/dev/null)"; [[ -S $s && $(lk_mode "$s") == 600 ]] || r=1
  done
  echo "$dir: $(lk_mode "$dir")"; [[ $(lk_mode "$dir") == 700 ]] || r=1
  (( r == 0 )) && lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
lk_port_taken() {
  lk_start "$lk_vc" serve 127.77.0.1 47242 squatter || return 1
  lk_dv "$lk_pc" link add "${lk_x}db" 47242 && lk_said "devvm: link_port_taken name=db port=47242 vm=$lk_vc" || return 1
  lk_gets "$lk_vc" db 47242 squatter && lk_gets "$lk_vc" db 47240 "$lk_w1-47240" && lk_end "$lk_vc" 127.77.0.1 47242 &&
    lk_dv "$lk_pc" link rm db 47242; }
lk_cap() { local out s
  lk_dv "$lk_pc" link add "${lk_x}db" 47241 || { cat "$work/last.log"; return 1; }
  out=$(lk_user "$lk_vc" 'LC_ALL=C perl /var/tmp/e2e-link.pl conns db 47241 257' 2>&1 | grep -E '^(closed:|connection )'); echo "$out"
  echo "events for db:47241: $(grep -F '"host":"db","port":47241,' "$XDG_DATA_HOME/devvm/logs/$lk_vc.jsonl" | sed 's/.*"event":"\([a-z_]*\)".*/\1/' | sort | uniq -c | paste -sd ' ' -)"
  echo "the session's log: $(tail -3 "$XDG_DATA_HOME/devvm/logs/$lk_vc.links.log" | paste -sd '|' -)"
  [[ $out == "closed: 257" ]] && lk_event "$lk_vc" link_limit db 47241 '"reason":"256"' &&
    lk_shows "$lk_pc" "  REFUSED  db:47241  256 connections carried already\$" network log -n 100000 || return 1
  # Their relays end once the target's server lets them go.
  lk_end "$lk_v1" 127.0.0.1 47241 && s=$(lk_session "$lk_vc") &&
    lk_until 60 lk_few "$s" && lk_dv "$lk_pc" link rm db 47241; }
lk_few() { (( $(lk_desc "$1" | wc -l) < 20 )); }
lk_hostile_listen() {
  # Without the link to port 80, which GatewayPorts yes would put on the client's every address, and
  # its forwarder on this machine's port 80.
  lk_dv "$lk_pc" link rm "$lk_n2" 80 && lk_hostile "$lk_vc" || return 1
  # The control: sshd now binds an address it refused.
  lk_ssh -o ExitOnForwardFailure=yes -R 127.0.0.1:47248:127.0.0.1:9 "$lk_vc" true || { echo "sshd still refuses 127.0.0.1"; return 1; }
  lk_reconnect "$lk_vc" db 47240 "$lk_w1-47240" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" 47240 "$lk_w2-47240" &&
    lk_vm_clean "$lk_vc" 47240 && lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
# With GatewayPorts yes, sshd binds every remote forward on the client's every address, whatever the
# host asked: db's on 0.0.0.0:47240, so the second target's on the same port can't bind. What it shows
# is printed; root in the client can put a relay of its own there as well.
lk_hostile_gateway() {
  lk_hostile "$lk_vc" gateway || return 1
  lk_reconnect "$lk_vc" db 47240 "$lk_w1-47240" || return 1
  echo "the client's listeners on 47240: $(lk_listens "$lk_vc" | grep -E ':47240$' | paste -sd ' ' -)" | tee "$work/link-gateway.log"
  lk_host_clean "$lk_vc" "$lk_vc" "$lk_v1" "$lk_v2"; }
lk_honest_again() {
  lk_honest "$lk_vc" && lk_reconnect "$lk_vc" db 47240 "$lk_w1-47240" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" 47240 "$lk_w2-47240" &&
    lk_vm_clean "$lk_vc" 47240 && lk_sshd_t "$lk_vc"; }
lk_log_off() { local l
  l=$(cd "$lk_pc" && "$D" network log -n 100000 2>/dev/null)
  grep -E 'REFUSED|:80 ' <<<"$l" | sort -u -k2 | tail -5
  grep -Eq "  link +db:47240  to $lk_v1\$" <<<"$l" && grep -Eq "  link +$lk_n2:80  to $lk_v2\$" <<<"$l" &&
    grep -Eq "  REFUSED  db:47243  to $lk_v1: unpublished\$" <<<"$l" && lk_event "$lk_vc" link db 47240 "\"target\":\"$lk_v1\""; }
lk_stopped() {
  lk_dv "$lk_p2" stop && lk_until 30 lk_not lk_running "$lk_v2" || return 1
  lk_gets "$lk_vc" "$lk_n2" 47240 "" && lk_until 5 lk_event "$lk_vc" link_target_stopped "$lk_n2" 47240 || return 1
  lk_up "$lk_pc" && lk_said "devvm: link_target_stopped target=$lk_v2 vm=$lk_vc" || return 1
  sleep 5; lk_shows "$lk_pc" "^$lk_n2 +47240 +$lk_v2 +$lk_p2 +Stopped +yes\$" link &&
    lk_shows "$lk_pc" "  REFUSED  $lk_n2:47240  to $lk_v2: it isn't running\$" network log -n 100000 &&
    ! lk_running "$lk_v2"; }
lk_restarted() { local k0 k1
  k0=$(grep "^$lk_v2 " "$XDG_CONFIG_HOME/devvm/known_hosts" | sort)
  if ! { lk_up "$lk_p2" && lk_running "$lk_v2" && lk_serve "$lk_v2" "$lk_w2" 47240 80; }; then tail -5 "$work/last.log"; return 1; fi
  k1=$(grep "^$lk_v2 " "$XDG_CONFIG_HOME/devvm/known_hosts" | sort)
  lk_until 20 lk_gets "$lk_vc" "$lk_n2" 47240 "$lk_w2-47240" || return 1
  # Lima keeps a VM's host keys; smolvm and Docker give it new ones at each start.
  if [[ $lk_be == lima ]]; then echo "host key kept: $([[ $k0 == "$k1" ]] && echo yes || echo no)"
  else [[ -n $k1 && $k0 != "$k1" ]] || { echo "the same host key"; return 1; }; fi; }
lk_rm_target() { local s
  s=$(lk_session "$lk_vc")
  lk_dv "$lk_p2" rm --force --no-export && lk_said "==> links_target_gone count=1 vm=$lk_v2 vms=$lk_vc" || return 1
  ! grep "^links=" "$vms/$lk_vc" | grep -qF "$lk_v2" && lk_until 15 lk_not lk_resolves "$lk_vc" "$lk_n2" &&
    lk_until 15 lk_not test -e "$(lk_sock "$lk_vc" 127.77.0.2 47240)" && [[ ! -e $(lk_sock "$lk_vc" 127.77.0.2 80) ]] &&
    [[ -n $s && $(lk_session "$lk_vc") == "$s" ]] && lk_gets "$lk_vc" db 47240 "$lk_w1-47240"; }
lk_made_again() { local id0
  id0=$(grep "^links=" "$vms/$lk_vc")
  launch "$lk_p2" exit ${lk_cli[@]+"${lk_cli[@]}"} --no-network-policy --no-git-protect --cpus 2 --memory 2 --ports none base
  if ! { [[ $vm == "$lk_v2" ]] && lk_running "$lk_v2"; }; then echo "not made again: ${vm:-none}"; return 1; fi
  ! lk_shows "$lk_pc" "$lk_v2" link && ! lk_resolves "$lk_vc" "$lk_n2" &&
    [[ $(grep "^links=" "$vms/$lk_vc") == "$id0" ]] && lk_dv "$lk_p2" link publish 47240 && lk_serve "$lk_v2" "$lk_w2" 47240; }
# The target gets network policy first, while it runs: until its next start it runs outside its
# proxy, its servers still up, and the client's link to it is refused then, and warned of by
# devvm link add and the client's launch (lk_policy_unenforced).
lk_policy_on() {
  if ! { lk_dv "$lk_p1" network on && grep -qx net=on "$vms/$lk_v1"; }; then echo "$lk_v1:"; tail -5 "$work/last.log"; return 1; fi
  if ! { lk_dv "$lk_pc" network on && lk_up "$lk_pc" restart && grep -qx net=on "$vms/$lk_vc" && grep -qx net_check=ok "$vms/$lk_vc"; }; then
    echo "$lk_vc:"; tail -5 "$work/last.log"; return 1
  fi; }
lk_policy_unenforced() {
  lk_shows "$lk_p1" "^$lk_v1 +Running .* unenforced " ls || { echo "devvm ls doesn't show it unenforced"; return 1; }
  lk_root "$lk_v1" "test -s /var/tmp/e2e-link-127.0.0.1-47240.pid" || { echo "the target's server isn't up"; return 1; }
  lk_gets "$lk_vc" db 47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db 47240 '"reason":"unenforced"' || return 1
  # devvm link add and the client's launch warn of it, refusing nothing.
  lk_dv "$lk_pc" link add "${lk_x}db" 47240 && lk_said "==> link_exists name=db port=47240 vm=$lk_vc" &&
    lk_said "devvm: link_target_unenforced target=$lk_v1 vm=$lk_vc" || return 1
  lk_up "$lk_pc" && lk_said "devvm: link_target_unenforced target=$lk_v1 vm=$lk_vc"; }
lk_policy_target() {
  if ! { lk_up "$lk_p1" restart && grep -qx net_check=ok "$vms/$lk_v1"; }; then echo "$lk_v1:"; tail -5 "$work/last.log"; return 1; fi
  lk_serve "$lk_v1" "$lk_w1" 47240 80; }
# The client got its link before its restart with network policy; lk_proxy_added adds one while it
# runs.
lk_policy_reach() {
  lk_user "$lk_vc" '[ -n "$HTTP_PROXY" ]' || { echo "no proxy settings"; return 1; }
  lk_until 20 lk_curls "$lk_vc" http://db:47240/ "$lk_w1-47240" && lk_curls "$lk_vc" http://db.devvm.internal:47240/ "$lk_w1-47240" &&
    lk_gets "$lk_vc" db 47240 "$lk_w1-47240" && ! lk_user "$lk_vc" "curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1/" >/dev/null 2>&1; }
# lk_via VM URL [-p]: the status curl in VM gets for URL through the VM's own proxy, whatever its
# NO_PROXY says (-p: by CONNECT, its answer's status), and the body's line on stdout's next line.
# lk_boot VM: its boot and its init's start, which a restart of the VM changes on every backend.
lk_via() { local w='%{http_code}'; [[ ${3:-} != -p ]] || w='%{http_connect}'
  lk_user "$1" "curl -s -m 10 --noproxy '' -x \"\$HTTP_PROXY\" ${3:-} -w '$w\\n' $2 2>/dev/null | tail -2 | tac 2>/dev/null || true" 2>/dev/null; }
lk_boot() { lk_root "$1" 'cat /proc/sys/kernel/random/boot_id; cut -d" " -f22 /proc/1/stat'; }
# lk_via_gets VM URL WORD [-p]: through the VM's proxy, URL answers 200 with WORD.
lk_via_gets() { local a; a=$(lk_via "$1" "$2" "${4:-}"); echo "$2${4:+ ($4)}: $(paste -sd ' ' - <<<"$a")"
  [[ $(head -1 <<<"$a") == 200 && $(sed -n 2p <<<"$a") == "$3" ]]; }
lk_proxy_added() { local b0 b1 k0 k1 n
  b0=$(lk_boot "$lk_vc") k0=$(grep "^$lk_vc " "$XDG_CONFIG_HOME/devvm/known_hosts" | sort)
  n=$(lk_user "$lk_vc" 'printf "%s|%s" "$NO_PROXY" "$no_proxy"'); echo "NO_PROXY|no_proxy: $n"
  [[ $n == "localhost,127.0.0.1,::1|localhost,127.0.0.1,::1" ]] || return 1
  lk_start "$lk_v1" serve 127.0.0.1 47242 "$lk_w1-47242" &&
    lk_dv "$lk_pc" link add "${lk_x}db" 47242 --as db && lk_said "==> link_added name=db port=47242 target=$lk_v1 vm=$lk_vc" || return 1
  lk_until 15 lk_via_gets "$lk_vc" http://db:47242/ "$lk_w1-47242" &&
    lk_via_gets "$lk_vc" http://db.devvm.internal:47242/ "$lk_w1-47242" && lk_via_gets "$lk_vc" http://db:47242/ "$lk_w1-47242" -p &&
    lk_curls "$lk_vc" http://db:47242/ "$lk_w1-47242" && lk_gets "$lk_vc" db 47242 "$lk_w1-47242" || return 1
  b1=$(lk_boot "$lk_vc") k1=$(grep "^$lk_vc " "$XDG_CONFIG_HOME/devvm/known_hosts" | sort)
  echo "boot and init before: $(paste -sd ' ' - <<<"$b0"), after: $(paste -sd ' ' - <<<"$b1")"
  [[ -n $b0 && $b0 == "$b1" && $k0 == "$k1" ]] && [[ $(lk_user "$lk_vc" 'printf "%s" "$NO_PROXY"') == localhost,127.0.0.1,::1 ]] &&
    lk_event "$lk_vc" link db 47242 "\"target\":\"$lk_v1\""; }
lk_proxy_own() { local u c l0 l1 log=$XDG_DATA_HOME/devvm/logs/$lk_vc.jsonl
  l0=$(grep -c '"event":"link' "$log" 2>/dev/null)
  for u in "http://${lk_x}db:47240/" http://db:47244/ http://db:47243/ http://db.devvm.internal.devvm.internal:47240/ \
    http://devvm.internal:47240/ http://127.77.0.1:47240/; do
    c=$(lk_via "$lk_vc" "$u" | head -1); echo "$u: $c"; [[ $c == 403 ]] || return 1
    c=$(lk_via "$lk_vc" "$u" -p | head -1); echo "$u (CONNECT): $c"; [[ $c == 403 ]] || return 1
  done
  l1=$(grep -c '"event":"link' "$log" 2>/dev/null); echo "link events: $l0 before, $l1 after"
  # The target has network policy and no links: the client's link names are nothing to its proxy.
  c=$(lk_via "$lk_v1" http://db:47240/ | head -1); echo "the target's proxy, http://db:47240/: $c"; [[ $c == 403 ]] || return 1
  c=$(lk_via "$lk_v1" http://db:47240/ -p | head -1); echo "  (CONNECT): $c"
  [[ $c == 403 && $l0 == "$l1" ]] && lk_via_gets "$lk_vc" http://db:47240/ "$lk_w1-47240"; }
lk_log_on() { local t0 last
  t0=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  lk_gets "$lk_vc" db 47240 "$lk_w1-47240" && grep -qx net=on "$vms/$lk_vc" || return 1
  last=$(cat "$XDG_DATA_HOME/devvm/logs/$lk_vc.jsonl".1 "$XDG_DATA_HOME/devvm/logs/$lk_vc.jsonl" 2>/dev/null |
    grep -F '"event":"link","host":"db","port":47240,' | tail -1)
  echo "the last: ${last:-none}, since $t0"
  [[ $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") > "$t0" || $(sed -n 's/^{"time":"\([^"]*\)".*/\1/p' <<<"$last") == "$t0" ]] &&
    lk_shows "$lk_pc" "  link +db:47240  to $lk_v1\$" network log -n 100000; }
lk_mismatch_add() {
  ! lk_dv "$lk_pc" link add "$lk_n2" 47240 && lk_said "devvm: link_policy_mismatch target=$lk_v2 vm=$lk_vc" && ! lk_resolves "$lk_vc" "$lk_n2"; }
lk_off_refused() {
  ! lk_dv "$lk_p1" network off && lk_said "devvm: network_off_linked vm=$lk_v1 vms=$lk_vc" && grep -qx net=on "$vms/$lk_v1"; }
lk_policy_broken() {
  lk_set_net "$lk_v1" off || return 1
  lk_gets "$lk_vc" db 47240 "" && lk_until 5 lk_event "$lk_vc" link_refused db 47240 '"reason":"policy"' &&
    lk_up "$lk_pc" && lk_said "devvm: link_policy_mismatch target=$lk_v1 vm=$lk_vc" && lk_running "$lk_vc" &&
    lk_until 10 lk_not lk_resolves "$lk_vc" db; }
lk_policy_back() {
  lk_set_net "$lk_v1" on && lk_up "$lk_pc" && ! grep -q link_policy_mismatch "$work/last.log" &&
    lk_until 10 lk_resolves "$lk_vc" db && lk_until 20 lk_curls "$lk_vc" http://db:47240/ "$lk_w1-47240"; }
lk_alpine() { local pa va r=0
  pa=$(newproj "${lk_x}alp")
  nd launch "$pa" exit --from alpine:3 --no-network-policy --no-git-protect --cpus 2 --memory 2 --ports none base; va=$vm
  if ! { [[ -n $va ]] && grep -qx libc=musl "$vms/$va"; }; then echo "no Alpine VM"; tail -5 "$work/last.log"; return 1; fi
  lk_dv "$pa" link add "${lk_x}db" 80 && lk_said "==> link_added name=${lk_x}db port=80 target=$lk_v1 vm=$va" || r=1
  (( r == 0 )) && echo "its ip_unprivileged_port_start: $(lk_root "$va" 'cat /proc/sys/net/ipv4/ip_unprivileged_port_start')" &&
    lk_until 20 lk_gets "$va" "${lk_x}db" 80 "$lk_w1-80" || r=1
  (cd "$pa" && quiet "$D" rm --force --no-export)
  return "$r"; }
# Groups, on links: the client publishes a port of its own first, so each of the three has one the
# others reach. The client's own link to the first target, db, names its group links too.
gp_join() { local n1=${lk_x}db na=${lk_x}app
  lk_start "$lk_vc" serve 127.0.0.1 47246 "$na-47246" && lk_dv "$lk_pc" link publish 47246 || return 1
  lk_dv "$lk_pc" group join "$lk_g" && lk_said "==> group_made group=$lk_g vm=$lk_vc" && lk_said "group_trust group=$lk_g" || return 1
  lk_dv "$lk_p1" group join "$lk_g" && lk_said "==> group_joined group=$lk_g vm=$lk_v1" && lk_said "group_trust group=$lk_g" &&
    lk_said "==> group_links_added group=$lk_g links=db:80,47241-47242,47244 vm=$lk_vc" &&
    lk_said "==> group_links_added group=$lk_g links=$na:47246 vm=$lk_v1" || return 1
  lk_dv "$lk_p2" group join "$lk_g" && lk_said "==> group_joined group=$lk_g vm=$lk_v2" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:47240 vm=$lk_v1" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:47240 vm=$lk_vc" &&
    lk_said "==> group_links_added group=$lk_g links=$na:47246, $n1:80,47240-47242,47244 vm=$lk_v2" || return 1
  lk_until 15 lk_gets "$lk_v2" "$n1" 47240 "$lk_w1-47240" && lk_until 15 lk_gets "$lk_v2" "$na" 47246 "$na-47246" &&
    lk_until 15 lk_gets "$lk_v1" "$lk_n2" 47240 "$lk_w2-47240" && lk_until 15 lk_gets "$lk_v1" "$na" 47246 "$na-47246" &&
    lk_until 15 lk_gets "$lk_vc" "$lk_n2" 47240 "$lk_w2-47240" && lk_until 15 lk_gets "$lk_vc" db 47244 "$lk_w1-47244" &&
    lk_gets "$lk_vc" db 47240 "$lk_w1-47240" && lk_shows "$lk_pc" "^$lk_n2 +47240 +$lk_v2 +$lk_p2 +Running +yes\$" link; }
gp_show() { local l
  l=$(cd "$lk_pc" && "$D" group 2>/dev/null); echo "$l"
  grep -Eq "^${lk_x}db +$lk_v1 +$lk_p1 +80,47240-47242,47244\$" <<<"$l" && grep -Eq "^$lk_n2 +$lk_v2 +$lk_p2 +47240\$" <<<"$l" &&
    grep -Eq "^${lk_x}app +$lk_vc +$lk_pc +47246\$" <<<"$l" && lk_shows "$lk_pc" '^  "version": 1,$' group --json &&
    lk_shows "$lk_p2" "^  \"group\": \"$lk_g\",\$" group --json; }
gp_publish() {
  lk_start "$lk_v2" serve 127.0.0.1 47245 "$lk_w2-47245" && lk_dv "$lk_p2" link publish 47245 &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:47245 vm=$lk_v1" &&
    lk_said "==> group_links_added group=$lk_g links=$lk_n2:47245 vm=$lk_vc" || return 1
  lk_until 15 lk_gets "$lk_v1" "$lk_n2" 47245 "$lk_w2-47245" && lk_until 15 lk_gets "$lk_vc" "$lk_n2" 47245 "$lk_w2-47245"; }
# Only a member is refused: outside the group, network on would turn the second target's policy on,
# failing the checks after this one that need it off.
gp_refused() {
  grep -qx "group=$lk_g" "$vms/$lk_v2" || { echo "$lk_v2 isn't in $lk_g: network on not tried"; return 1; }
  ! lk_dv "$lk_p2" network on &&lk_said "devvm: group_policy_mixed group=$lk_g vm=$lk_v2" && grep -qx net=off "$vms/$lk_v2" &&
    ! lk_dv "$lk_p2" group join "${lk_x}other" && lk_said "devvm: group_other group=$lk_g vm=$lk_v2" &&
    ! lk_dv "$lk_pc" link rm "$lk_n2" 47240 && lk_said "devvm: link_from_group group=$lk_g name=$lk_n2 port=47240 vm=$lk_vc"; }
# Each leaves, whatever comes of the checks, so the sections after run without a group.
gp_leave() { local r=0
  lk_dv "$lk_p2" group leave && lk_said "==> group_left group=$lk_g vm=$lk_v2" &&
    lk_said "==> group_links_removed group=$lk_g links=$lk_n2:47240,47245 vm=$lk_v1" &&
    lk_said "==> group_links_removed group=$lk_g links=$lk_n2:47240,47245 vm=$lk_vc" &&
    lk_said "==> group_links_removed group=$lk_g links=${lk_x}app:47246, ${lk_x}db:80,47240-47242,47244 vm=$lk_v2" || r=1
  (( r == 0 )) && lk_until 15 lk_not lk_resolves "$lk_v1" "$lk_n2" && lk_until 15 lk_not lk_resolves "$lk_vc" "$lk_n2" &&
    lk_until 15 lk_not lk_resolves "$lk_v2" "${lk_x}db" || r=1
  lk_dv "$lk_p1" group leave || r=1
  lk_dv "$lk_pc" group leave && lk_said "==> group_gone group=$lk_g" || r=1
  # The client's own link to db stays; the group's to db's other ports go.
  (( r == 0 )) && lk_gets "$lk_vc" db 47240 "$lk_w1-47240" && lk_until 15 lk_not lk_listening "$lk_vc" 127.77.0.1:47244 &&
    lk_shows "$lk_pc" "^db +47240 +$lk_v1 " link && ! lk_shows "$lk_pc" "^db +47244 " link || r=1
  lk_dv "$lk_p2" link unpublish 47245; lk_end "$lk_v2" 127.0.0.1 47245
  lk_dv "$lk_pc" link unpublish 47246; lk_end "$lk_vc" 127.0.0.1 47246
  return "$r"; }
# The client alone in a group as db, the first target's folder part: the target is refused, as
# itself or another name, and stays out. The client leaves whatever comes of the checks.
gp_hostname() { local r=0 g=${lk_x}h n1=${lk_x}db
  lk_dv "$lk_pc" group join "$g" --as "$n1" && lk_said "==> group_made group=$g vm=$lk_vc" || r=1
  (( r == 0 )) && ! lk_dv "$lk_p1" group join "$g" && lk_said "devvm: group_name_hostname group=$g member=$lk_vc name=$n1 vm=$lk_v1" &&
    ! lk_dv "$lk_p1" group join "$g" --as "${n1}2" && lk_said "devvm: group_name_hostname group=$g member=$lk_vc name=$n1 vm=$lk_v1" &&
    ! grep -q "^group=$g" "$vms/$lk_v1" || r=1
  lk_dv "$lk_pc" group leave && lk_said "==> group_gone group=$g" || r=1
  return "$r"; }
# All or none, with the client and the first target with network policy and the second without.
gp_policy() { local r=0 on=${lk_x}on off=${lk_x}off
  lk_dv "$lk_pc" group join "$on" && lk_said "==> group_made group=$on vm=$lk_vc" || r=1
  (( r == 0 )) && ! lk_dv "$lk_p2" group join "$on" && lk_said "devvm: group_policy_on group=$on vm=$lk_v2" || r=1
  (( r == 0 )) && lk_dv "$lk_p2" group join "$off" && ! lk_dv "$lk_p1" group join "$off" &&
    lk_said "devvm: group_policy_off group=$off vm=$lk_v1" && ! grep -q "^group=$off" "$vms/$lk_v1" || r=1
  # The client's own links to db are 47240 and 47242 (lk_proxy_added).
  (( r == 0 )) && lk_dv "$lk_p1" group join "$on" && lk_said "==> group_links_added group=$on links=db:80,47241,47244 vm=$lk_vc" &&
    lk_until 15 lk_gets "$lk_vc" db 80 "$lk_w1-80" && lk_curls "$lk_vc" http://db:80/ "$lk_w1-80" || r=1
  lk_dv "$lk_p1" group leave; lk_dv "$lk_pc" group leave; lk_dv "$lk_p2" group leave
  (( r == 0 )) && ! grep -q '^group=.' "$vms/$lk_v1" "$vms/$lk_vc" "$vms/$lk_v2"; }
lk_rm_client() {
  lk_dv "$lk_pc" rm --force --no-export || { tail -5 "$work/last.log"; return 1; }
  lk_until 10 lk_not lk_session "$lk_vc" || { echo "its session runs"; return 1; }
  [[ -z $(lk_session "$lk_vc") ]] && ! ls "$XDG_DATA_HOME/devvm/run/"link-* >/dev/null 2>&1 &&
    [[ ! -e $XDG_DATA_HOME/devvm/run/$lk_vc.links.pid && ! -e $vms/$lk_vc ]] && ! ls "$XDG_DATA_HOME/devvm/logs/$lk_vc".* >/dev/null 2>&1 &&
    ! grep -l "^links=.*$lk_vc" "$vms"/* >/dev/null 2>&1; }

if $clean; then
  section "cleaning cached images"
  if [[ $backend == smolvm ]]; then
    # smolvm's images and their records, not Lima's.
    for n in $(smol_ls | awk '$1 ~ /^devvm-img-/ { print $1 }'); do SMOLVM_USAGE=0 smolvm machine delete --name "$n" -f >/dev/null 2>&1; done
    for k in "$XDG_DATA_HOME"/devvm/smolvm/images/*; do [[ ! -d $k ]] || rm -f "$images/$(basename "$k")"; done
    rm -rf "$XDG_DATA_HOME/devvm/smolvm/images"
  elif [[ $backend == docker ]]; then
    # This engine's, with the sandbox's label (Docker Desktop's are deleted after each run anyway).
    [[ " $engines " != *" linux "* || "$(uname -s)" == Darwin ]] || d_purge
    rm -rf "$images" "$share" "$XDG_DATA_HOME/devvm/docker"
  else
    for n in $(limactl list -q 2>/dev/null | grep '^devvm-img-'); do limactl delete -f "$n" >/dev/null 2>&1; done
    rm -rf "$images" "$share"
  fi
fi

if want tools; then
# What a user gets: upstream's newest versions, each download with its checksum (rust: rustup checks),
# resolved by devvm's pinned mise, downloaded and checked on the first run.
section "shared tool versions"
march=arm64; [[ "$(uname -m)" != x86_64 ]] || march=x64   # the lock's platform: mise's name for the arch
ntools=$(awk -F' *= *' '/^\[tools\]/ { s = 1; next } /^\[/ { s = 0 } s && NF == 2' "$root/assets/shared-tools/mise.toml" | wc -l)
check "the shared tools resolve to upstream's newest, with checksums" bash -c "'$D' _refresh-tools </dev/null >'$work/last.log' 2>&1 &&
  (( \$(grep -c '^\[\[tools\.' '$lock') == $ntools )) &&
  (( \$(grep -A1 '\"platforms.linux-$march\"\]' '$lock' | grep -c '^checksum = \"sha256:') == $ntools - 1 ))"
fi

if want images; then
section "images"
check "devvm doctor finds everything it needs on this host" bash -c "'$D' doctor </dev/null >'$work/last.log' 2>&1 &&
  '$D' doctor --json </dev/null | grep -qxF '  \"problems\": [],'"
# base stays cached between runs (--clean deletes it); the fixture layers' images are this run's.
check "build base" quiet env DEVVM_MESSAGES=names "$D" build base
# Its key: the build's, else the newest base from Ubuntu's image (from's bases are cached too).
kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
if [[ -n $kb ]] && want from; then
  features=$(tr '\n' ' ' < "$XDG_CONFIG_HOME/devvm/base-features")
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/last.log")
  check "base ran base.sh, then every feature" bash -c "
    for f in base $features; do grep -q \"^# \\[[0-9]*/[0-9]*\\] running_script script=\$f.sh vm=devvm-img-$kb\$\" '$runlog' || exit 1; done"
elif want from; then
  skip "base ran base.sh, then every feature" "base is cached; from checks the features"
fi
fixture=$layers/e2efix.sh
token=e2e-fixture-ran-$$-$(date +%s)   # this run's: the run logs keep earlier runs'
rm -f "$fixture" "${fixture%.sh}.policy"
check "layer new writes a starter layer" quiet "$D" layer new e2efix
printf 'echo %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" >> "$fixture"
check "build the fixture layer on base" quiet "$D" build e2efix
owned e2efix
kf=$(keys_of e2efix | head -1)
# A cached base's key: the one the fixture was built on (other selections' bases, with other
# features, are cached too, and may be newer).
[[ -n $kb ]] || kb=$(sed -n 's/^chain=base=\([0-9a-f]*\) e2efix=.*$/\1/p' "$images/$kf" 2>/dev/null)
check "base is a stopped image, recorded with Ubuntu's pinned cloud image and its C library" bash -c "
  [[ -n '$kb' && \$(status_of devvm-img-$kb) == Stopped ]] &&
  grep -qx 'chain=base=$kb' '$images/$kb' && grep -qx 'libc=gnu' '$images/$kb' && grep -qx 'from=' '$images/$kb' &&
  [[ -n '$cloud_release' ]] &&
  grep -qE '^digest=https://cloud-images\.ubuntu\.com/.*/$cloud_release/.*\.img sha256:($cloud_sums)$' '$images/$kb'"
check "base mounts nothing from the host" bash -c "[[ -z \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' devvm-img-$kb) ]]"
check "devvm limactl runs Lima on devvm's Lima home, whatever LIMA_HOME says, with its exit code" bash -c "
  [[ -n '$kb' ]] && LIMA_HOME='$work/no-lima' '$D' limactl list -q </dev/null | grep -qx devvm-img-$kb &&
  ! '$D' limactl e2e-no-such-command </dev/null >/dev/null 2>&1"
check "building base again reuses it" bash -c "out=\$('$D' build base </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=base key=$kb)\" <<<\"\$out\""
check "its script ran in its image, cloned from base's" bash -c "
  [[ -n '$kf' && \$(status_of devvm-img-$kf) == Stopped ]] &&
  grep -qx 'chain=base=$kb e2efix=$kf' '$images/$kf' &&
  grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
echo "# edited" >> "$fixture"
check "editing it builds a new image; base and the old one stay" bash -c "'$D' build e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 2 ]] && [[ \$(status_of devvm-img-$kf) == Stopped ]] && [[ -f '$images/$kb' ]]"
owned e2efix
check "rebuild makes a new one and deletes the stale ones, not base" bash -c "'$D' rebuild --force e2efix </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2efix | wc -l) -eq 1 ]] && [[ \$(keys_of e2efix) != $kf ]] && [[ -z \$(status_of devvm-img-$kf) ]] &&
  [[ \$(status_of devvm-img-$kb) == Stopped ]]"
owned e2efix
kf=$(keys_of e2efix | head -1)
dg=$(sed -n 's/^digest=.* sha256:\([0-9a-f]\{12\}\)[0-9a-f]*$/\1/p' "$images/$kb" 2>/dev/null)
check "devvm images lists base and the fixture, current, with Ubuntu's release, digest and size" bash -c "
  out=\$('$D' images </dev/null 2>/dev/null) &&
  grep -qE '^base +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kb +current +[0-9.]+[KMGT] +lima$' <<<\"\$out\" &&
  grep -qE '^e2efix +ubuntu [0-9.]+ \([0-9]{8}\) +$dg +$kf +current +[0-9.]+[KMGT] +lima$' <<<\"\$out\" &&
  grep -qE '^NAME +TOOL +VERSION +BACKEND +LIBC +SIZE +VMS +STATE$' <<<\"\$out\""
check "images rm deletes an image by name, with its record, not base" bash -c "
  [[ -n '$kf' ]] && '$D' images rm --force e2efix </dev/null >/dev/null 2>&1 &&
  [[ -z \$(status_of devvm-img-$kf) ]] && [[ ! -e '$images/$kf' ]] && [[ \$(status_of devvm-img-$kb) == Stopped ]]"

# The user's CAs: a build trusts them while its scripts run, with the variables for tools that don't
# read the system's store, and its image keeps none of them, so its key is the same without them. A
# copy of the image, booted without devvm (no setup), shows what the image holds.
e2e_ca
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
catoken=e2e-ca-trusted-$$-$(date +%s)
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt ]]' \
  'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /etc/ssl/devvm.pem' "$catoken" > "$layers/e2eca.sh"
check "a build trusts the CAs in devvm's CA folder while its scripts run" bash -c "'$D' build e2eca </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$catoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2eca
kc=$(keys_of e2eca | head -1)
rm -rf "$cafolder"
check "its key is the same without them: nothing to build" bash -c "[[ -n '$kc' ]] && out=\$('$D' build e2eca </dev/null 2>&1) &&
  ! grep -qF \"\$(m building_image image=e2eca key=$kc)\" <<<\"\$out\" && [[ \$(keys_of e2eca) == '$kc' ]]"
created+=(devvm-e2e-cacheck)
check "its image keeps none of them" bash -c "[[ -n '$kc' ]] &&
  limactl clone --tty=false --mount-none devvm-img-$kc devvm-e2e-cacheck >/dev/null 2>&1 &&
  limactl start --tty=false devvm-e2e-cacheck >/dev/null 2>&1 && ca_gone devvm-e2e-cacheck"
limactl delete -f devvm-e2e-cacheck >/dev/null 2>&1
rm -f "$layers/e2eca.sh"

# Java's cacerts: ca-certificates-java's hook adds the CAs a build trusts (debian:devvm-extra-N.pem)
# to it, and it's told of each one devvm takes out again, so the image's cacerts keeps none. A
# project VM on that image gets the CA in cacerts at its launch, and drops it at the launch after
# it's removed from devvm's CA folder.
e2e_ca
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
jtoken=e2e-java-ca-$$-$(date +%s)
jlist='keytool -list -cacerts -storepass changeit 2>/dev/null | grep -i "^debian:devvm-extra-"'
jnone="keytool -list -cacerts -storepass changeit >/dev/null 2>&1 && ! { $jlist; }"
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends default-jre-headless ca-certificates-java >/dev/null' \
  "$jlist" "$jtoken" > "$layers/e2ejava.sh"
check "a layer that installs Java has the CAs in Java's cacerts while it builds" bash -c "'$D' build e2ejava </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$jtoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2ejava
kj=$(keys_of e2ejava | head -1)
created+=(devvm-e2e-javacheck)
check "its image's cacerts keeps none of them" bash -c "[[ -n '$kj' ]] &&
  limactl clone --tty=false --mount-none devvm-img-$kj devvm-e2e-javacheck >/dev/null 2>&1 &&
  limactl start --tty=false devvm-e2e-javacheck >/dev/null 2>&1 &&
  limactl shell --workdir / devvm-e2e-javacheck bash -c '$jnone' </dev/null"
limactl delete -f devvm-e2e-javacheck >/dev/null 2>&1
if [[ -n $kj ]]; then
  pj=$(newproj e2ejava)
  launch "$pj" 'exit' --no-network-policy e2ejava
  check "a project VM with Java has the CA in cacerts" bash -c "[[ -n '$vm' ]] &&
    limactl shell --workdir / '$vm' bash -c '$jlist' </dev/null"
  rm -rf "$cafolder"
  check "removed from devvm's CA folder, it's gone from cacerts from the VM's next launch" bash -c "[[ -n '$vm' ]] &&
    cd '$pj' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
    limactl shell --workdir / '$vm' bash -c '$jnone' </dev/null"
  [[ -z $vm ]] || (cd "$pj" && "$D" rm --force --no-export </dev/null >/dev/null 2>&1)
else
  skip "a project VM with Java has the CA in cacerts" "the Java layer didn't build"
  skip "removed from devvm's CA folder, it's gone from cacerts from the VM's next launch" "the Java layer didn't build"
fi
rm -rf "$cafolder"
rm -f "$layers/e2ejava.sh"

# The host's proxy: a build goes through a proxy of devvm's for it alone, which alone has the
# host's settings. The build's commands get only that proxy's address with a password of the
# build's, sudo keeping them, and its image keeps none of it (Lima writes the settings it
# propagates into the guest's /etc/environment). The host's proxy is a closed port with a password
# of this run's, and NO_PROXY=* keeps every tool off the proxies, as the layer needs no network.
# Its disk and Lima's files for it are read whole, free space included.
pxpass=e2e-proxy-pw-$$-$(date +%s)
pxtoken=e2e-proxy-seen-$$-$(date +%s)
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  '[[ $HTTPS_PROXY == http://devvm:*@192.168.5.2:* && $http_proxy == "$HTTPS_PROXY" && $(sudo printenv HTTPS_PROXY) == "$HTTPS_PROXY" ]]' \
  '[[ $HTTPS_PROXY != *e2e-proxy-pw-* && $NO_PROXY == "*" ]]' \
  "$pxtoken" > "$layers/e2eproxy.sh"
check "a build's commands get a proxy of devvm's for the build, never the host's own settings" bash -c "
  env HTTPS_PROXY='http://corp:$pxpass@127.0.0.1:9' HTTP_PROXY='http://corp:$pxpass@127.0.0.1:9' NO_PROXY='*' \
    '$D' build e2eproxy </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$pxtoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log &&
  ! ps -axww -o command= | grep -q -- '[_]proxy -listen 127.0.0.1:0 -vm devvm-img-' &&
  [[ -z \$(ls -A '$XDG_DATA_HOME/devvm/run/builds' 2>/dev/null) ]]"
owned e2eproxy
kp=$(keys_of e2eproxy | head -1)
check "its image keeps none of them: not its disk, Lima's files for it or devvm's logs" bash -c "
  [[ -n '$kp' && -d '$LIMA_HOME/devvm-img-$kp' ]] &&
  ! LC_ALL=C grep -rqaF -D skip '$pxpass' '$LIMA_HOME/devvm-img-$kp' '$XDG_DATA_HOME/devvm/logs'"
rm -f "$layers/e2eproxy.sh"

# A failed script keeps its image's VM, stopped and without a record, for inspection.
printf '#!/bin/bash\nexit 3\n' > "$layers/e2efail.sh"
DEVVM_MESSAGES=names "$D" build e2efail </dev/null >"$work/fail.log" 2>&1; rc=$?
kx=$(sed -n 's/^==> building_image image=e2efail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
[[ -z $kx ]] || created+=("devvm-img-$kx")
check "a failed script keeps its VM, stopped, and says how to look inside" bash -c "
  [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: script_failed script=e2efail\.sh vm=devvm-img-$kx$' '$work/fail.log' &&
  [[ \$(status_of devvm-img-$kx) == Stopped ]] && [[ ! -e '$images/$kx' ]]"
check "devvm images lists it as unfinished, and images rm deletes it by its key" bash -c "
  [[ -n '$kx' ]] && '$D' images </dev/null 2>/dev/null | grep -qE '^- +- +- +$kx +unfinished +' &&
  '$D' images rm --force '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(status_of devvm-img-$kx) ]]"
rm -f "$layers/e2efail.sh"

# Ctrl+C during a build: the run's names (DEVVM_MESSAGES) say which image it builds, its run log,
# and (in that log) when its script runs.
printf '#!/bin/bash\nsleep 600\n' > "$layers/e2eslow.sh"
DEVVM_MESSAGES=names "$D" build e2eslow </dev/null >"$work/slow.log" 2>&1 &
slow=$!
for _ in $(seq 300); do
  runlog=$(sed -n 's/^ *log_details file=//p' "$work/slow.log")
  [[ -n $runlog ]] && grep -q '^# \[4/5\] running_script script=e2eslow.sh ' "$runlog" 2>/dev/null && break
  sleep 1
done
ks=$(sed -n 's/^==> building_image image=e2eslow key=\([0-9a-f]*\)$/\1/p' "$work/slow.log")
[[ -z $ks ]] || created+=("devvm-img-$ks")
kill -INT "$slow" 2>/dev/null
wait "$slow"; rc=$?
check "Ctrl+C during a build deletes the unfinished image, and exits 130" bash -c "
  [[ -n '$ks' && $rc -eq 130 ]] && grep -qx '==> interrupted_image vm=devvm-img-$ks' '$work/slow.log' &&
  [[ -z \$(status_of devvm-img-$ks) ]] && [[ ! -e '$images/$ks' ]] && [[ \$(status_of devvm-img-$kb) == Stopped ]]"
rm -f "$fixture" "$layers/e2eslow.sh"
fi

if want project; then
section "a base project"
p=$(newproj web); git -C "$p" init -q
# The shell devvm opens reads its commands from devvm's input: these, then it ends.
launch "$p" 'pwd; echo e2e-shell-ran' --no-network-policy --cpus 2 --memory 3 --disk 120 base
# The launch's output, for the .git protection checks: the checks before them overwrite last.log.
cp "$work/last.log" "$work/web-launch.log"
check "devvm creates the VM from base, and opens a shell in the folder" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx '$p' '$work/last.log' && grep -qx e2e-shell-ran '$work/last.log' &&
  grep -qx image=base '$vms/$vm' && grep -qx net=off '$vms/$vm' && grep -qx 'dir=$p' '$vms/$vm'"
# Without a terminal, devvm in a subfolder opens the folder's VM there, says so, and makes none.
mkdir -p "$p/src/lib"
check "devvm in a subfolder opens the folder's VM in the subfolder" bash -c "
  cd '$p/src/lib' && printf 'pwd\n' | '$D' >'$work/last.log' 2>&1 &&
  grep -qx '$p/src/lib' '$work/last.log' && grep -qF \"\$(m using_parent_vm dir='$p' vm='$vm')\" '$work/last.log' &&
  ! grep -qE '^dir=$p/src(/lib)?\$' '$vms'/*"
check "it gets --cpus, --memory and --disk" in_vm "$vm" \
  '[[ $(nproc) == 2 ]] && (( $(awk "/^MemTotal:/ { print int(\$2 / 1048576 + 0.5) }" /proc/meminfo) == 3 )) && (( $(df -BG --output=size / | tail -1 | tr -dc 0-9) >= 115 ))'
check "devvm size shows it" bash -c "cd '$p' && [[ \$('$D' size 2>&1) == 'cpus=2 memory=3GiB disk=120GiB' ]]"
check "ls lists it, and no image" bash -c "'$D' ls >'$work/ls.out' 2>&1 &&
  grep -Eq '^$vm +Running +base +(on|off) +off +- +lima +$p\$' '$work/ls.out' && ! grep -q '^devvm-img-' '$work/ls.out'"
check "ls --json and size --json show it" bash -c "'$D' ls --json 2>/dev/null | grep -qxF '      \"name\": \"$vm\",' &&
  cd '$p' && '$D' size --json 2>/dev/null | grep -qxF '  \"disk_gib\": 120'"
check "the folder is mounted read-write" bash -c "limactl shell --workdir / $vm bash -c 'echo from-vm > $p/vm.txt' </dev/null && grep -qx from-vm '$p/vm.txt'"
# On Linux hosts the mount is 9p, whose subfolders' events reach no watcher, even the VM's own.
if [[ "$(uname -s)" == Darwin ]]; then
  check "a watcher in the VM sees files changed, made and deleted in the VM" watch_sees in_vm "$vm" "$p" vm
else
  check "a watcher in the VM sees files changed, made and deleted in the VM, but not in a subfolder (9p)" watch_sees in_vm "$vm" "$p" vm no
fi
check "... but none changed, made or deleted on this machine (Lima's mounts pass on no file events: polling)" watch_misses in_vm "$vm" "$p"
# Only the project folder and the shared tools: the VM's own mounts list holds nothing else (in a
# nested Linux test host, the guest's home has the host's path, so looking for files there proves
# nothing).
check "nothing else from the host is mounted" bash -c "[[ \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$p $share ' ]]"
check "SSH agent forwarding is off" bash -c "! limactl shell --workdir / $vm ssh-add -l </dev/null"
check "without the docker layer, it has no Docker" in_vm "$vm" '! command -v dockerd && ! command -v docker && ! systemctl cat docker.service >/dev/null 2>&1'
check "hostname is the folder" bash -c "[[ \$(limactl shell --workdir / $vm hostname </dev/null) == web ]]"
# Every port (the default), through devvm's forwarder over Lima's ssh, Lima's own forwards off: a
# program on this machine on every address of a port (with SO_REUSEADDR, as most servers do) keeps
# its localhost connections while the VM listens on it too; a VM server on another port reaches this
# machine (the control).
check "Lima forwards nothing itself; devvm's forwarder runs, with the host key devvm pinned" bash -c "
  [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $vm) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]] &&
  pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null &&
  grep -qxF \"\$(cut -d' ' -f2- '$XDG_DATA_HOME/devvm/lima-vms/$vm.ports.known_hosts' | sed 's/^/$vm /')\" '$XDG_CONFIG_HOME/devvm/known_hosts'"
# Without network policy too: so no other VM's forwarder takes it before Lima binds it.
check "its ssh port is the one its state records, on this machine's loopback alone" bash -c "
  sp=\$(sed -n 's/^ssh_port=//p' '$vms/$vm') && [[ -n \$sp ]] && ! grep -qx net=on '$vms/$vm' &&
  [[ \$(limactl list --format '{{.SSHLocalPort}}' $vm) == \"\$sp\" ]] && loopback_only \"\$sp\""
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalPort => shift, Listen => 5, ReuseAddr => 1) or die;
  while ($c = $s->accept) { print $c "host-program\n"; close $c }' 47161 &
hostprog=$!; sleep 0.5
lima_serve "$vm" 47160 47161
in_vm "$vm" "setsid -f python3 -c 'import socket,time; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((\"0.0.0.0\",47162)); time.sleep(600)' devvm-e2e-udp </dev/null >/dev/null 2>&1" >/dev/null 2>&1
check "a VM server on a port reaches this machine" bash -c 'for i in $(seq 20); do nc -z 127.0.0.1 47160 && exit 0; sleep 1; done; exit 1'
check "through devvm's forwarder alone: Lima's hostagent has none of its ports, TCP or UDP" bash -c "sleep 3; nc -z 127.0.0.1 47160 &&
  ! lima_listens tcp 47160 && ! lima_listens udp 47162 && pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null"
check "the forward listens on this machine's loopback alone" loopback_only 47160
check "a port a program here has on every address keeps its localhost connections while the VM listens on it" bash -c "sleep 3
  out=\$(nc -w 2 127.0.0.1 47161 </dev/null); echo \"47161 answered by: \${out:-nothing}\"; [[ \$out == host-program ]] &&
  grep -q 'port 47161: not forwarded' '$XDG_DATA_HOME/devvm/lima-vms/$vm.ports.log'"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
# A server on a link's address (127.77.0.0/16) is never forwarded, while the VM's own on 127.0.0.1
# with the same port is (the control).
vm_say "$vm" 127.77.0.1 47163 link
vm_say "$vm" 127.77.0.1 47164 link
vm_say "$vm" 127.0.0.1 47164 own
check "a server on a link's address isn't forwarded, the VM's own on 127.0.0.1 with its port is" bash -c "
  limactl shell --workdir / $vm ss -Hltn 'sport = :47163' </dev/null | grep -q '127\.77\.0\.1:47163' &&
  for i in \$(seq 20); do [[ \$(nc -w 2 127.0.0.1 47164 </dev/null) == own ]] && break; sleep 1; done &&
  [[ \$(nc -w 2 127.0.0.1 47164 </dev/null) == own ]] && sleep 3 && ! nc -z 127.0.0.1 47163 && ! nc -z ::1 47163 &&
  ! grep -q 'port 47163' '$XDG_DATA_HOME/devvm/lima-vms/$vm.ports.log'"
in_vm "$vm" 'pkill -f "[h]ttp.server -b 0.0.0.0 4716"; pkill -f "[d]evvm-e2e-udp"; pkill -f "[d]evvm-e2e-say"' >/dev/null 2>&1
check "Exec's arguments, folder and input reach the guest as given" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2EExecQuoting\$' ./internal/backend/lima |
  grep -q -- '--- PASS: TestE2EExecQuoting'"

section "a company's CA"
# The run's CA, in devvm's CA folder: the running VM trusts it from its next launch, in its system's
# store, so an HTTPS server here it signed is trusted; a file there that isn't a certificate fails
# the launch; emptied, the next launch takes it out again.
e2e_ca; ca_serve 47195
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
check "a CA in devvm's CA folder is trusted in the VM from its next launch" bash -c "cd '$p' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm sh -c 'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /usr/local/share/ca-certificates/devvm-extra-1.crt &&
    cmp -s /etc/ssl/devvm.pem /usr/local/share/ca-certificates/devvm-extra-1.crt && curl -sSf -o /dev/null https://host.lima.internal:47195/' </dev/null"
check "the VM's environment points tools that don't read the system's store at it" in_vm "$vm" \
  '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt && $REQUESTS_CA_BUNDLE == $SSL_CERT_FILE && $PIP_CERT == $SSL_CERT_FILE ]]'
printf 'not a certificate\n' > "$cafolder/notes.pem"
check "a file there that isn't a certificate fails the launch, naming it" bash -c "cd '$p' &&
  ! printf 'exit\n' | DEVVM_MESSAGES=names '$D' >'$work/last.log' 2>&1 &&
  grep -qx 'devvm: ca_not_certificate file=$cafolder/notes.pem' '$work/last.log'"
rm -rf "$cafolder"
check "emptied, the VM trusts it no more from its next launch" bash -c "cd '$p' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm sh -c '! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 && [ ! -s /etc/ssl/devvm.pem ] &&
    ! curl -sSf -o /dev/null https://host.lima.internal:47195/' </dev/null"
ca_stop

section "ssh by name"
# An ~/.ssh/config of its own, set up as the docs say, with a Host * that forwards the agent: yours
# is never read or changed. ssh gets no agent but a throwaway one.
sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc"
printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$XDG_CONFIG_HOME/devvm/ssh_config" > "$sc/config"
kh=$XDG_CONFIG_HOME/devvm/known_hosts
agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
check "the ssh config lists it by the folder's name" grep -q "^Host devvm-web $vm\$" "$XDG_CONFIG_HOME/devvm/ssh_config"
check "its host keys are pinned, as the VM has them" bash -c "
  [[ \$(grep -c '^$vm ' '$kh') -gt 0 && \$(grep -c '^$vm ' '$kh') == \$(limactl shell --workdir / $vm sh -c 'cat /etc/ssh/ssh_host_*_key.pub' </dev/null | wc -l | tr -d ' ') ]]"
check "ssh devvm-web reaches it" bash -c "[[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "agent forwarding stays off, whatever Host * says" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -F '$sc/config' devvm-web 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "a changed host key is refused" bash -c "
  cp '$kh' '$sc/kh' && ssh-keygen -q -t ed25519 -N '' -f '$sc/other' &&
  { grep -v '^$vm ' '$sc/kh' || true; echo \"$vm \$(cut -d' ' -f1,2 '$sc/other.pub')\"; } > '$kh' &&
  { env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web true </dev/null 2>'$sc/err'; r=\$?; cp '$sc/kh' '$kh'; } &&
  (( r == 255 )) && grep -q 'Host key verification failed' '$sc/err'"
# lima_sshd VM HOST: its sshd binds remote forwards only on links' addresses (rfwd_links) and refuses
# X11, as sshd -T shows; a local forward works (the control).
lima_sshd() { local t want
  rfwd_links "$2" env -u SSH_AUTH_SOCK ssh -F "$sc/config" || return 1
  t=$(limactl shell --workdir / "$1" sudo sshd -T </dev/null) || return 1
  for want in 'x11forwarding no' 'allowtcpforwarding yes' 'allowstreamlocalforwarding local' 'gatewayports clientspecified' \
    'permitlisten 127.77.*:*' 'permittunnel no'; do
    grep -qxF "$want" <<<"$t" || { echo "not $want"; return 1; }
  done
  [[ $(env -u SSH_AUTH_SOCK ssh -F "$sc/config" -W 127.0.0.1:22 "$2" </dev/null | head -c 4) == SSH- ]]; }
check "its sshd binds remote forwards only on 127.77.0.0/16, refuses socket forwards and X11; a local forward works (the control)" lima_sshd "$vm" devvm-web
check "the entry turns off X11, tunnels and adding keys to an agent, whatever Host * says" bash -c "
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n  Tunnel yes\n' '$XDG_CONFIG_HOME/devvm/ssh_config' > '$sc/hostile-config' &&
  g=\$(ssh -G -F '$sc/hostile-config' devvm-web) &&
  grep -qx 'forwardx11 no' <<<\"\$g\" && grep -qx 'forwardx11trusted no' <<<\"\$g\" && grep -qx 'addkeystoagent false' <<<\"\$g\" &&
  grep -qx 'tunnel false' <<<\"\$g\" && grep -qx 'permitremoteopen none' <<<\"\$g\" && grep -qx 'updatehostkeys false' <<<\"\$g\""

section ".git protection"
if [[ "$(uname -s)" != Darwin ]]; then
  check "off on Linux (no chflags), and says so" bash -c "grep -qx git_protect=off '$vms/$vm' &&
    grep -qF \"\$(m git_protect_needs_macos_new)\" '$work/web-launch.log'"
else
  check "hooks and config are locked" bash -c "ls -lOd '$p/.git/config' | grep -q uchg && ls -lOd '$p/.git/hooks' | grep -q uchg &&
    grep -qx git_protect=on '$vms/$vm'"
  check "root in the VM can't add a hook" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x > $p/.git/hooks/pre-commit' </dev/null && [[ ! -e '$p/.git/hooks/pre-commit' ]]"
  check "root in the VM can't change the config" bash -c "! limactl shell --workdir / $vm sudo sh -c 'echo x >> $p/.git/config' </dev/null"
  check "git in the VM can commit" in_vm "$vm" "cd $p && for i in 1 2 3 4 5 6 7 8 9 10; do git status >/dev/null 2>&1 && break; sleep 1; done &&
    git add -A && git -c user.email=e@e -c user.name=e commit -qm vm"
fi

section "an existing VM"
check "devvm stop stops it and its port forwarder, and drops its ssh config entry" bash -c "cd '$p' && '$D' stop </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(status_of $vm) == Stopped ]] && grep -qF \"\$(m stopped vm=$vm)\" '$work/last.log' && ! pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null &&
  ! grep -q '^Host devvm-web ' '$XDG_CONFIG_HOME/devvm/ssh_config'"
# A program here that took its ssh port while it was stopped, on 127.0.0.1 alone and accepting
# nothing (SO_REUSEADDR, as the port's last connections may still be in TIME_WAIT): the VM gets a
# new one, recorded, and the program keeps its own.
old_sp=$(sed -n 's/^ssh_port=//p' "$vms/$vm")
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "127.0.0.1", LocalPort => shift, Listen => 1, ReuseAddr => 1) or die "$!\n"; sleep 600' "$old_sp" &
hostprog=$!; sleep 0.5
check "stopped, devvm starts it again, on a new ssh port when a program here has its own" bash -c "[[ -n '$old_sp' && \$(status_of $vm) == Stopped ]] && kill -0 '$hostprog' &&
  cd '$p' && printf 'echo e2e-again\n' | '$D' >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] && grep -qx e2e-again '$work/last.log' &&
  sp=\$(sed -n 's/^ssh_port=//p' '$vms/$vm') && [[ -n \$sp && \$sp != '$old_sp' ]] &&
  [[ \$(limactl list --format '{{.SSHLocalPort}}' $vm) == \"\$sp\" ]] && kill -0 '$hostprog'"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
check "running, devvm opens it as it is" bash -c "cd '$p' && printf 'echo e2e-running\n' | '$D' 2>&1 | grep -qx e2e-running"
# devvm host and code with a home of their own, so the Include line is offered for an empty one (and,
# without a terminal, not added), and a stub code first on PATH: never your editor.
mkdir -p "$sc/home" "$sc/bin"
printf '#!/bin/sh\necho "$*" > %s/code-args\necho "${SSH_AUTH_SOCK-unset}" > %s/code-agent\n' "$sc" "$sc" > "$sc/bin/code"
chmod +x "$sc/bin/code"
check "devvm host starts a stopped VM and names it, without a shell; ssh reaches it" bash -c "cd '$p' &&
  HOME='$sc/home' '$D' stop </dev/null >/dev/null 2>&1 && [[ \$(status_of $vm) == Stopped ]] &&
  HOME='$sc/home' '$D' host </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | head -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_host_info dir=$p host=devvm-web | tail -1)\" '$work/last.log' &&
  grep -qF \"\$(m ssh_include_yourself | head -1)\" '$work/last.log' && [[ ! -e '$sc/home/.ssh' ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "VS Code's settings in the VM: no port forwarding, no git sign-in relay" in_vm "$vm" \
  '[[ $(jq -c . ~/.vscode-server/data/Machine/settings.json) == "{\"remote.autoForwardPorts\":false,\"git.terminalAuthentication\":false,\"github.gitAuthentication\":false}" ]]'
in_vm "$vm" 'echo "{\"editor.fontSize\": 14, \"remote.autoForwardPorts\": true}" > ~/.vscode-server/data/Machine/settings.json' >/dev/null 2>&1
(cd "$p" && HOME=$sc/home "$D" host </dev/null >"$work/last.log" 2>&1)
check "changed back in the VM, devvm host turns them off again and keeps the others" in_vm "$vm" \
  'jq -e ".[\"editor.fontSize\"] == 14 and .[\"remote.autoForwardPorts\"] == false and .[\"github.gitAuthentication\"] == false" ~/.vscode-server/data/Machine/settings.json >/dev/null'
check "devvm code opens the folder over Remote-SSH (a stub code), with the agent devvm started with" bash -c "cd '$p' &&
  HOME='$sc/home' PATH='$sc/bin':\$PATH SSH_AUTH_SOCK='$sc/agent.sock' '$D' code </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(cat '$sc/code-args') == '--remote ssh-remote+devvm-web $p' && \$(cat '$sc/code-agent') == '$sc/agent.sock' ]]"
check "restart, and its new host keys are pinned" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-web hostname </dev/null) == web ]]"
check "a running VM's new ports apply at once" bash -c "cd '$p' &&
  '$D' ports 47170-47173 2>&1 | grep -qF \"\$(m ports_forwarding ports=47170-47173 vm=$vm)\" && [[ \$('$D' ports 2>/dev/null) == 47170-47173 ]] &&
  ps -axww -o command= | grep -- '[_]lima-ports -vm $vm ' | grep -q -- ' -only 47170-47173\$'"
check "a bad port list is a usage error, exit 2, naming devvm ports --help, and the VM's ports stay" bash -c "cd '$p' &&
  { '$D' ports 47170-x </dev/null >'$work/last.log' 2>&1; [[ \$? == 2 ]]; } &&
  grep -qxF \"\$(m usage_help 'command=devvm ports')\" '$work/last.log' && [[ \$('$D' ports 2>/dev/null) == 47170-47173 ]]"
check "a word that's neither a command nor an image is a usage error suggesting the command, and the VM runs on" bash -c "cd '$p' &&
  { '$D' stoop </dev/null >'$work/last.log' 2>&1; [[ \$? == 2 ]]; } &&
  grep -qF \"\$(m no_command_or_image_command command=stop word=stoop)\" '$work/last.log' && [[ \$(status_of $vm) == Running ]]"
check "a port added to a running VM reaches this machine at once, and once removed doesn't, the VM never restarted" ports_live "$p" "$vm" 47178 lima_serve lima_boot
in_vm "$vm" 'pkill -f "[h]ttp.server -b 0.0.0.0 47178"' >/dev/null 2>&1
check "its new size waits for its next start" bash -c "cd '$p' && '$D' size --cpus 3 2>&1 | grep -qF \"\$(m size_next_start vm=$vm)\" &&
  '$D' size 2>&1 >/dev/null | grep -qF \"\$(m size_pending size=cpus=3 vm=$vm)\""
check "and applies on restart, its ports kept" bash -c "cd '$p' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' ports 2>&1) == 47170-47173 && \$(limactl shell --workdir / $vm nproc </dev/null) == 3 &&
     \$('$D' size 2>&1) == 'cpus=3 memory=3GiB disk=120GiB' ]]"
check "a smaller disk is refused" bash -c "cd '$p' && ! '$D' size --disk 100 </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m disk_shrink have=120 vm=$vm want=100)\" '$work/last.log'"
# Listeners on each kind of address Lima matches differently (loopback, every address; IPv4 and 6), on
# listed ports (47170-47173) and unlisted ones (47174-47177). Their output goes nowhere, or the SSH
# session waits for them.
in_vm "$vm" 'p=47170; for r in 1 2; do for a in 127.0.0.1 ::1 0.0.0.0 ::; do
  setsid -f python3 -m http.server -b $a $p </dev/null >/dev/null 2>&1; p=$((p + 1)); done; done' >/dev/null 2>&1
check "only the listed ports reach this machine, whatever the address" bash -c \
  'for i in $(seq 20); do nc -z 127.0.0.1 47173 && break; sleep 1; done; sleep 3
  for p in 47170 47171 47172 47173; do nc -z 127.0.0.1 $p || exit 1; done
  for p in 47174 47175 47176 47177; do ! nc -z 127.0.0.1 $p || exit 1; done'
check "a listed port a program on this machine has, on every address, stays its own until it's freed" host_port_kept "$p" "$vm" 47170 47171 lima_serve
if [[ "$(uname -s)" == Darwin ]]; then
  check "a swapped .git is reported" bash -c "limactl shell --workdir / $vm bash -c 'cd $p && mv .git .git-old && git init -q' </dev/null;
    cd '$p' && printf 'exit\n' | '$D' 2>&1 | grep -qF \"\$(m git_replaced git=$p/.git | head -1)\""
  rm -rf "$p/.git"; mv "$p/.git-old" "$p/.git"
  # (virtiofs shows the VM the restored .git after a moment.) Locks are set while it's stopped:
  # devvm stop sets them.
  check "a submodule git folder the VM adds is reported, and locked by devvm stop" bash -c "limactl shell --workdir / $vm bash -c 'cd $p &&
      for i in 1 2 3 4 5 6 7 8 9 10; do git rev-parse >/dev/null 2>&1 && break; sleep 1; done && git init -q vmsub &&
      git -C vmsub -c user.email=e@e -c user.name=e commit -q --allow-empty -m s &&
      git update-index --add --cacheinfo 160000,\$(git -C vmsub rev-parse HEAD),vmsub' </dev/null &&
    cd '$p' && '$D' stop </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m git_embedded_new git=$p/vmsub/.git | head -1)\" '$work/last.log' &&
    [[ \$(status_of $vm) == Stopped ]] && ls -lOd '$p/vmsub/.git/config' | grep -q uchg"
  check "git-protect on a running VM waits for it to stop, and devvm stop applies it" bash -c "cd '$p' &&
    printf 'exit\n' | '$D' >/dev/null 2>&1 && [[ \$(status_of $vm) == Running ]] && '$D' git-protect off </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_next_stop mode=off vm=$vm | head -1)\" && ls -lOd '$p/.git/config' | grep -q uchg &&
    '$D' stop </dev/null >/dev/null 2>&1 && ! ls -lOd '$p/.git/config' | grep -q uchg &&
    [[ \$('$D' git-protect 2>/dev/null) == off ]] && printf 'exit\n' | '$D' >/dev/null 2>&1 && [[ \$(status_of $vm) == Running ]]"
  check "stopped outside devvm, its port forwarder ends with it (its lifeline)" bash -c "limactl stop $vm >/dev/null 2>&1 &&
    for i in \$(seq 10); do pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null || exit 0; sleep 1; done; exit 1"
  check "git-protect strict keeps host git status out of submodules" bash -c "limactl stop $vm >/dev/null 2>&1; cd '$p' &&
    '$D' git-protect strict </dev/null >/dev/null 2>&1 && [[ \$(git config --file .git/config diff.ignoreSubmodules) == all &&
    \$('$D' git-protect 2>/dev/null) == strict ]] && ls -lOd '$p/.git/config' | grep -q uchg"
  check "git-protect off unlocks, and on locks again" bash -c "cd '$p' && '$D' git-protect off </dev/null >/dev/null 2>&1 &&
    ! ls -lOd '$p/.git/config' | grep -q uchg && ! git config --file .git/config diff.ignoreSubmodules &&
    '$D' git-protect on </dev/null >/dev/null 2>&1 && ls -lOd '$p/.git/config' | grep -q uchg && [[ \$('$D' git-protect 2>/dev/null) == on ]]"
else
  check "git-protect on is refused off macOS" bash -c "cd '$p' && '$D' git-protect on </dev/null 2>&1 |
    grep -qF \"\$(m git_protect_on_needs_macos | head -1)\""
fi
check "rm deletes only this VM, with its state and ssh config entry, and unlocks" bash -c "cd '$p' && '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! grep -q '^Host devvm-web ' '$XDG_CONFIG_HOME/devvm/ssh_config' &&
  ! grep -q '^$vm ' '$kh' && { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$p/.git/config' | grep -q uchg; } &&
  ! ls '$XDG_DATA_HOME/devvm/lima-vms/$vm'.* >/dev/null 2>&1 && ! pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null &&
  [[ -z \$(ls '$XDG_DATA_HOME/devvm/exports' 2>/dev/null) ]]"

section "a devcontainer.json"
# Its forwardPorts beyond the user's ports said, not forwarded (the default is none for this VM), its
# variables in the shell and its commands, its postCreateCommand once, its postStartCommand at each
# start; nothing written to the folder, and a changed file only warned about.
dc=$(newproj dc); mkdir -p "$dc/.devcontainer"
printf '%s\n' '{' '  // e2e' '  "name": "dc",' '  "forwardPorts": [47180],' \
  '  "containerEnv": {"E2E_DC": "dc-${containerWorkspaceFolderBasename}"},' \
  '  "postCreateCommand": "echo \"$E2E_DC $(pwd)\" >> ~/dc-created",' \
  '  "postStartCommand": ["sh", "-c", "echo started >> ~/dc-started"],' '}' > "$dc/.devcontainer/devcontainer.json"
cp "$dc/.devcontainer/devcontainer.json" "$work/dc.json"
quiet "$D" ports -g none
launch "$dc" 'echo "dc-env=$E2E_DC"' --no-network-policy base
"$D" ports -g all </dev/null >/dev/null 2>&1
check "devvm makes the VM as its devcontainer.json says, and says so" bash -c "[[ -n '$vm' && \$(status_of '$vm') == Running ]] &&
  grep -qF \"\$(m devcontainer_using file=.devcontainer/devcontainer.json vm=$vm)\" '$work/last.log' &&
  grep -Eqx 'devcontainer=[0-9a-f]{64} \.devcontainer/devcontainer\.json' '$vms/$vm' && grep -qx ports=none '$vms/$vm'"
check "its forwardPorts don't widen your ports: said, not added" \
  grep -qF "$(m devcontainer_ports_not_forwarded file=.devcontainer/devcontainer.json ports=47180)" "$work/last.log"
check "its shell has the file's variables" grep -qx dc-env=dc-dc "$work/last.log"
check "postCreateCommand ran once, in the VM, in the folder, with them" in_vm "$vm" "[[ \$(cat ~/dc-created) == 'dc-dc $dc' ]]"
check "postStartCommand ran at its start" in_vm "$vm" '[[ $(cat ~/dc-started) == started ]]'
check "nothing was written to the project folder" bash -c "[[ \$(ls -A '$dc') == .devcontainer && \$(ls -A '$dc/.devcontainer') == devcontainer.json ]] &&
  cmp -s '$dc/.devcontainer/devcontainer.json' '$work/dc.json'"
in_vm "$vm" 'setsid -f python3 -m http.server -b 0.0.0.0 47180 </dev/null >/dev/null 2>&1; setsid -f python3 -m http.server -b 0.0.0.0 47181 </dev/null >/dev/null 2>&1' >/dev/null 2>&1
check "... and don't reach this machine" bash -c \
  'sleep 5; ! nc -z 127.0.0.1 47180 && ! nc -z 127.0.0.1 47181'
check "launched again, running: nothing runs again" bash -c "cd '$dc' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm bash -c '[[ \$(wc -l < ~/dc-created) == 1 && \$(wc -l < ~/dc-started) == 1 ]]' </dev/null"
check "restarted: postStartCommand runs again, postCreateCommand doesn't" bash -c "cd '$dc' && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  limactl shell --workdir / $vm bash -c '[[ \$(wc -l < ~/dc-created) == 1 && \$(wc -l < ~/dc-started) == 2 ]]' </dev/null"
printf '%s\n' '{"forwardPorts": [47182], "postCreateCommand": "touch ~/dc-changed"}' > "$dc/.devcontainer/devcontainer.json"
check "a changed file is warned about, and changes nothing" bash -c "cd '$dc' && printf 'exit\n' | '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m devcontainer_changed file=.devcontainer/devcontainer.json vm=$vm | head -1)\" '$work/last.log' &&
  grep -qx ports=none '$vms/$vm' && limactl shell --workdir / $vm bash -c '[[ ! -e ~/dc-changed ]]' </dev/null"
check "rm deletes it" bash -c "cd '$dc' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 && ! limactl list -q | grep -qx $vm"
printf '%s\n' '{"image": "localhost:5000/app:1"}' > "$dc/.devcontainer/devcontainer.json"
check "an image on this machine's registry is refused, nothing made" bash -c "cd '$dc' && ! '$D' --no-network-policy base </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m devcontainer_image_not_public file=.devcontainer/devcontainer.json from=localhost:5000/app:1)\" '$work/last.log' &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]]"

section "agent folders and the VM's setup"
# A home of its own: a fake agent config, history and git identity, and a fake token; nothing of
# yours is read.
ah=$work/agent-home; rm -rf "$ah"; mkdir -p "$ah/.claude/skills/e2e" "$ah/.claude/projects/p"; ah=$(cd "$ah" && pwd -P)
echo '# e2e skill' > "$ah/.claude/skills/e2e/SKILL.md"
echo '{"theme":"e2e-settings"}' > "$ah/.claude/settings.json"
echo '{"prompt":"e2e-history"}' > "$ah/.claude/projects/p/h.jsonl"
printf '[user]\n\tname = E2E User\n\temail = e2e@example.com\n' > "$ah/.gitconfig"
a=$(newproj agents)
(cd "$a" && HOME=$ah TZ=Europe/Paris CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake-plain quiet "$D" --no-network-policy --no-git-protect claude)
vm=$(vm_of "$a"); [[ -z $vm ]] || created+=("$vm")
check "an agent folder is mounted read-only at its host path, and linked into the VM's home" bash -c "
  limactl list --format '{{range .Config.Mounts}}{{.Location}}|{{.Writable}} {{end}}' $vm | grep -qF '$ah/.claude/skills|false ' &&
  ! limactl shell --workdir / $vm sudo touch '$ah/.claude/skills/x' </dev/null &&
  limactl shell --workdir / $vm bash -c 'grep -q e2e ~/.claude/skills/e2e/SKILL.md' </dev/null"
check "its settings are copied in, its history never" bash -c "
  limactl shell --workdir / $vm bash -c 'grep -q e2e-settings ~/.claude/settings.json && [[ ! -e ~/.claude/projects/p ]]' </dev/null &&
  ! limactl shell --workdir / $vm sudo grep -rqs e2e-history /home /root /etc </dev/null"
check "the VM gets the host's git identity, timezone and its hostname" in_vm "$vm" \
  "[[ \$(git config --global user.name) == 'E2E User' && \$(git config --global user.email) == e2e@example.com &&
     \$(timedatectl show -p Timezone --value) == Europe/Paris && \$(hostname) == agents ]]"
check "without network policy, the Claude token itself" in_vm "$vm" '[[ $CLAUDE_CODE_OAUTH_TOKEN == sk-ant-oat01-e2efake-plain ]]'
mkdir -p "$ah/.claude/agents"
check "a new agent folder is noticed on launch, and waits for a restart" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' 2>&1 |
  grep -qF \"\$(m mounts_next_start vm=$vm | head -1)\""
check "restart mounts it" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 &&
  limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm | grep -qF '$ah/.claude/agents ' &&
  limactl shell --workdir / $vm ls '$ah/.claude/agents' </dev/null"
rm -rf "$ah/.claude/skills"
check "a removed one is unmounted, and the VM starts" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 &&
  ! limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm | grep -q skills && [[ \$(status_of $vm) == Running ]]"
in_vm "$vm" 'mkdir -p ~/.claude/projects/e2e && echo e2e-transcript > ~/.claude/projects/e2e/t.jsonl && echo secret > ~/.claude/.credentials.json'
limactl stop "$vm" >/dev/null 2>&1
check "export starts the stopped VM and saves its agents' history to a file, without their logins" bash -c "cd '$a' &&
  HOME='$ah' '$D' history export '$work/export/h.tar.gz' </dev/null >'$work/last.log' 2>&1 && [[ \$(status_of $vm) == Running ]] &&
  tar -tzf '$work/export/h.tar.gz' > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list'"
check "rm exports its agents' history, without their logins, only yours to read, then deletes it" bash -c "ex='$XDG_DATA_HOME/devvm/exports' &&
  mkdir -p \"\$ex\" && chmod 755 \"\$ex\" && cd '$a' && HOME='$ah' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && f=\$(ls \"\$ex\"/$vm-*.tar.gz) && tar -tzf \"\$f\" > '$work/export.list' &&
  grep -qx '\.claude/projects/e2e/t\.jsonl' '$work/export.list' && ! grep -q 'credentials' '$work/export.list' &&
  [[ \$(stat -c %a \"\$ex\" 2>/dev/null || stat -f %Lp \"\$ex\") == 700 ]] && [[ \$(stat -c %a \"\$f\" 2>/dev/null || stat -f %Lp \"\$f\") == 600 ]]"

section "SSH agents"
# A home of its own, without keys. A throwaway agent with a throwaway key stands for yours; devvm's
# own agent (its socket in the sandbox's data folder) gets another.
sa=$work/agent-ssh; rm -rf "$sa"; mkdir -p "$sa/home"; sa=$(cd "$sa" && pwd -P)
ssh-keygen -q -t ed25519 -N '' -C devvm-e2e-user-key -f "$sa/user-key"
ssh-keygen -q -t ed25519 -N '' -C devvm-e2e-key -f "$sa/devvm-key"
user_agent_pid=$(ssh-agent -a "$sa/user.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
SSH_AUTH_SOCK=$sa/user.sock ssh-add -q "$sa/user-key" 2>/dev/null
# as_user CMD...: CMD with that home and agent.
as_user() { HOME=$sa/home SSH_AUTH_SOCK=$sa/user.sock "$@"; }
export -f as_user; export sa
g=$(newproj agentvm)
(cd "$g" && as_user quiet "$D" --no-network-policy --no-git-protect --ssh-agent base)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "--ssh-agent: the VM uses your agent's keys" bash -c "
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-user-key ]]"
check "ssh-agent add loads a key into devvm's own agent" bash -c "
  as_user '$D' ssh-agent add '$sa/devvm-key' </dev/null 2>&1 | grep -qF \"\$(m agent_holds keys=devvm-e2e-key)\""
check "ssh-agent devvm while it runs: after a launch, devvm's shell and ssh by name still use yours, until a restart" bash -c "cd '$g' &&
  as_user '$D' ssh-agent devvm </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' >>'$work/last.log' 2>&1 &&
  [[ \$(as_user ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-user-key ]] &&
  [[ \$(printf 'ssh-add -l\nexit\n' | as_user '$D' 2>/dev/null | awk '/SHA256:/ { print \$3 }') == devvm-e2e-user-key ]]"
check "ssh-agent devvm, then restart: the VM uses only that key, not yours" bash -c "cd '$g' &&
  as_user '$D' ssh-agent devvm </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  [[ \$(as_user limactl shell --workdir / $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "so does an editor connecting by name, whatever agent it has" bash -c "
  [[ \$(as_user ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' $vm ssh-add -l </dev/null | awk '{ print \$3 }') == devvm-e2e-key ]]"
check "ssh-agent off, then restart: no agent in the VM" bash -c "cd '$g' &&
  as_user '$D' ssh-agent off </dev/null >'$work/last.log' 2>&1 && printf 'exit\n' | as_user '$D' restart >>'$work/last.log' 2>&1 &&
  ! as_user limactl shell --workdir / $vm ssh-add -l </dev/null &&
  grep -A8 '^Host .* $vm\$' '$XDG_CONFIG_HOME/devvm/ssh_config' | grep -q '^  ForwardAgent no\$'"
check "ssh-agent rm takes the key out of devvm's own agent and its record, and leaves the key file" bash -c "cd '$g' &&
  as_user '$D' ssh-agent </dev/null 2>/dev/null | grep -q devvm-e2e-key &&
  as_user '$D' ssh-agent rm '$sa/devvm-key' </dev/null >'$work/last.log' 2>&1 &&
  ! SSH_AUTH_SOCK='$XDG_DATA_HOME/devvm/agent.sock' ssh-add -l </dev/null | grep -q devvm-e2e-key &&
  ! grep -q devvm-key '$XDG_CONFIG_HOME/devvm/agent-keys' && [[ -f '$sa/devvm-key' ]] &&
  [[ \$(as_user '$D' ssh-agent </dev/null 2>/dev/null) == off ]]"
check "rm deletes the SSH agent VM" bash -c "cd '$g' && as_user '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 && ! limactl list -q | grep -qx $vm"
kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
pkill -f "[s]sh-agent -a $XDG_DATA_HOME/devvm/agent.sock"
fi

if want clone; then
section "--clone"
# A home of its own: no ~/.ssh/config, so nothing of yours is read or offered the Include line. A
# throwaway repo, with a branch besides main.
ch=$work/clone-home; rm -rf "$ch"; mkdir -p "$ch"
export GIT_CONFIG_GLOBAL=$ch/.gitconfig GIT_CONFIG_NOSYSTEM=1   # host git: none of your settings
c=$(newproj cloned)
git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$c" branch other
(cd "$c" && printf 'pwd; git branch --show-current\n' | HOME=$ch "$D" --clone --no-network-policy base >"$work/last.log" 2>&1)
vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
cgit="GIT_SSH_COMMAND='ssh -F $XDG_CONFIG_HOME/devvm/ssh_config' git -C '$c'"
check "the VM has its own clone, with every branch, only the shared tools mounted, and the shell opens in it" bash -c "
  [[ -n '$vm' && \$(limactl list --format '{{range .Config.Mounts}}{{.Location}} {{end}}' $vm) == '$share ' ]] &&
  [[ \$(limactl shell --workdir / $vm git -C '$c' branch --format='%(refname:short)' </dev/null | tr '\n' ' ') == 'main other ' ]] &&
  limactl shell --workdir / $vm git -C '$c' log --oneline </dev/null | grep -q one &&
  grep -qx '$c' '$work/last.log' && grep -qx main '$work/last.log' && grep -qx mode=clone '$vms/$vm'"
check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
check "nothing of .git is locked or watched" bash -c "! grep -q '^git_protect=' '$vms/$vm' &&
  { [[ \$(uname -s) != Darwin ]] || ! ls -lOd '$c/.git/config' | grep -q uchg; }"
in_vm "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "rm without a terminal refuses with unfetched commits, even with --force, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_fetch remote=devvm | head -1)\" '$work/last.log' && limactl list -q | grep -qx $vm"
check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
in_vm "$vm" "cd '$c' && echo c > c" >/dev/null 2>&1
check "then rm refuses with uncommitted changes in the clone, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m no_terminal_delete | head -1)\" '$work/last.log' && grep -qx '?? c' '$work/last.log' && limactl list -q | grep -qx $vm"
in_vm "$vm" "cd '$c' && rm c && git worktree add -q .worktrees/a && mkdir -p .git/info && echo .worktrees/ >> .git/info/exclude && echo w > .worktrees/a/w" >/dev/null 2>&1
check "then rm refuses with uncommitted changes in a worktree in the clone, shown under its path, and keeps it" bash -c "cd '$c' && ! HOME='$ch' '$D' rm --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m clone_worktree_dirty path='$c/.worktrees/a' | head -1)\" '$work/last.log' && grep -qx '?? w' '$work/last.log' &&
  ! grep -qF \"\$(m clone_dirty | head -1)\" '$work/last.log' &&
  grep -qF \"\$(m no_terminal_delete | head -1)\" '$work/last.log' && limactl list -q | grep -qx $vm"
check "rm --no-clone-check deletes it unchecked after a warning, the worktree's change with it, and the remote fully; the fetched work stays as a local branch" bash -c "cd '$c' && HOME='$ch' '$D' rm --force --no-export --no-clone-check </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m clone_check_skipped vm=$vm | head -1)\" '$work/last.log' && ! grep -qF \"\$(m clone_worktree_dirty path='$c/.worktrees/a' | head -1)\" '$work/last.log' &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! git -C '$c' remote | grep -q devvm &&
  [[ -z \$(git -C '$c' for-each-ref refs/remotes) ]] && git -C '$c' log --oneline saved/devvm/main | grep -q from-vm &&
  grep -qF \"\$(m saved_branches 'branches=\"saved/devvm/main\"' | head -1)\" '$work/last.log' && [[ ! -e '$ch/.ssh/config' ]]"
unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
fi

if want gc; then
section "gc"
# A folder named img-*: its VM is named like an image (devvm-img-keep-<hash>), and isn't one.
g=$(newproj img-keep)
launch "$g" exit --no-network-policy base
kg=$(sed -n 's/^chain=base=\([0-9a-f]*\)$/\1/p' "$vms/$vm" 2>/dev/null)
gfix=$layers/e2egc.sh
printf '#!/bin/bash\necho gc\n' > "$gfix"
check "a fixture layer, edited after its build, is stale" bash -c "'$D' build e2egc </dev/null >'$work/last.log' 2>&1 &&
  echo '# edited' >> '$gfix' && '$D' images </dev/null 2>/dev/null | grep -qE '^e2egc .* stale '"
owned e2egc
ko=$(keys_of e2egc | head -1)
# An old tool image no VM lists: unused only once there's a tools lock.
old=node-0.0.1-000000000000.erofs
[[ ! -f $lock ]] || { : > "$share/$old"; : > "$share/$old.sha256"; }
check "gc deletes the stale image and its record; base and the img-* project VM stay" bash -c "
  [[ -n '$vm' && -n '$kg' && -n '$ko' ]] && '$D' gc --force </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(status_of devvm-img-$ko) && ! -e '$images/$ko' ]] &&
  [[ \$(status_of devvm-img-$kg) == Stopped && -f '$images/$kg' ]] && [[ \$(status_of $vm) == Running ]]"
if [[ -f $lock ]]; then
  check "gc deletes a tool image no VM uses, with its hash" bash -c "[[ ! -e '$share/$old' && ! -e '$share/$old.sha256' ]]"
else
  skip "gc deletes a tool image no VM uses, with its hash" "no tools lock yet"
fi
check "gc again finds nothing to clean up" bash -c "'$D' gc </dev/null 2>&1 | grep -qxF \"==> \$(m nothing_to_clean)\""
check "space lists the Lima VM and images, and changes nothing" bash -c "'$D' space </dev/null 2>/dev/null >'$work/last.log' &&
  grep -qE '^VMs +lima +[0-9]+ ' '$work/last.log' && grep -qE '^images +lima +[0-9]+ ' '$work/last.log' &&
  [[ \$(status_of $vm) == Running && -f '$vms/$vm' ]]"
# The img-* VM deleted outside devvm, its state an hour old: gc --orphans forgets what devvm kept.
quiet limactl delete -f "$vm"
touch -t 202001010000 "$vms/$vm"
check "gc --orphans forgets a VM limactl deleted, and nothing of base's" bash -c "'$D' gc --orphans --force </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m forgot_orphan vm=$vm)\" '$work/last.log' && [[ ! -e '$vms/$vm' ]] &&
  ! ls '$XDG_DATA_HOME/devvm/lima-vms/$vm'.* >/dev/null 2>&1 && [[ \$(status_of devvm-img-$kg) == Stopped && -f '$images/$kg' ]]"
check "retention sets a setting, shows it, and KEY=off turns it off again" bash -c "
  '$D' retention orphans.age=30d </dev/null >'$work/last.log' 2>&1 && '$D' retention </dev/null 2>/dev/null | grep -qx 'orphans.age=30d' &&
  '$D' retention orphans.age=off </dev/null >>'$work/last.log' 2>&1 && '$D' retention </dev/null 2>/dev/null | grep -qx 'orphans.age=off'"
rm -f "$gfix" "$share/$old" "$share/$old.sha256"
(cd "$g" && quiet "$D" rm --force --no-export)
fi

if want tools; then
section "shared tools"
t=$(newproj tools); echo '{}' > "$t/package.json"
# A package.json: node, detected. Its tool images stay cached in the share between runs (--clean
# deletes them), as base does.
launch "$t" exit --no-network-policy --no-git-protect
check "a node VM, with node's shared tools in its state and the share" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx image=node '$vms/$vm' && grep -qx libc=gnu '$vms/$vm' &&
  for f in \$(sed -n 's/^tools=//p' '$vms/$vm'); do [[ -f '$share'/\$f && -f '$share'/\$f.sha256 ]] || exit 1; done &&
  grep -q '^tools=node-$(lockv node)-.* pnpm-$(lockv pnpm)-.* bun-$(lockv bun)-' '$vms/$vm'"
check "node, npm, pnpm, bun and yarn work, node at the locked version" in_vm "$vm" \
  "[[ \$(node -v) == v$(lockv node) ]] && npm -v && pnpm -v && bun -v && yarn -v"
check "each tool is a read-only image from the share" in_vm "$vm" \
  "[[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]] && ! touch /opt/devvm/tools/node/$(lockv node)/x 2>/dev/null"
check "the share is read-only even for root" bash -c "limactl shell --workdir / $vm sudo touch /mnt/devvm-tools/x </dev/null 2>&1 | grep -q 'Read-only file system'"
check "npm i -g installs into the VM's home" in_vm "$vm" "npm i -g cowsay@1.6.0 >/dev/null 2>&1 && [[ \$(command -v cowsay) == \$HOME/.npm-global/bin/cowsay ]]"
# Node reads its own CA list, and NODE_EXTRA_CA_CERTS: a CA in devvm's CA folder, from the VM's
# next launch, makes an HTTPS server here it signed trusted.
e2e_ca; ca_serve 47195
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
(cd "$t" && printf 'exit\n' | "$D" >"$work/last.log" 2>&1)
check "node trusts a CA in devvm's CA folder" in_vm "$vm" \
  'node -e "fetch(\"https://host.lima.internal:47195/\").then(r => process.exit(r.ok ? 0 : 1), e => { console.error(e); process.exit(1) })"'
rm -rf "$cafolder"; ca_stop
# ssh with a command runs a non-login, non-interactive shell, as editors and agents' tools do.
check "a non-login shell finds the tools too" bash -c "ssh -F '$LIMA_HOME/$vm/ssh.config' lima-$vm 'node -v' </dev/null"
# The checks above pass with a download of the locked version too: these tell the share's from one.
# Before the .nvmrc check, which installs into ~/.local/share/mise/installs on purpose.
for tool in node pnpm bun; do
  check "$tool runs from the share, not a download" in_vm "$vm" \
    "p=\$(readlink -f \"\$(mise which $tool)\") && [[ \$p == /opt/devvm/tools/$tool/$(lockv "$tool")/* ]] || { echo \"\$p\"; false; }"
done
check "mise installed none of node, pnpm and bun into the VM" in_vm "$vm" \
  "[[ ! -e ~/.local/share/mise/installs/node && ! -e ~/.local/share/mise/installs/pnpm && ! -e ~/.local/share/mise/installs/bun ]]"
check "an .nvmrc picks its own version, installed into the VM" in_vm "$vm" "cd $t && echo 22.20.0 > .nvmrc && [[ \$(node -v) == v22.20.0 ]]; r=\$?; rm .nvmrc; exit \$r"
check "with the .nvmrc gone, node runs from the share again" in_vm "$vm" \
  "for d in $t ~; do p=\$(cd \"\$d\" && readlink -f \"\$(mise which node)\") && [[ \$p == /opt/devvm/tools/node/$(lockv node)/* ]] || { echo \"\$d: \$p\"; exit 1; }; done"
# Started without devvm (so no setup): the boot itself mounts them.
check "the tools are mounted at boot" bash -c "limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  limactl shell --workdir / $vm bash -lc 'node -v' </dev/null | grep -qx v$(lockv node)"
limactl stop "$vm" >/dev/null 2>&1   # two VMs at once starve a nested Linux host
# A tool image gone from the share stops the launch, named, and mise downloads nothing in its place;
# back in the share, the next launch mounts it.
nodef=$(sed -n 's/^tools=//p' "$vms/$vm" | tr ' ' '\n' | grep '^node-')
mv "$share/$nodef" "$work/$nodef"
check "a tool missing from the share stops the launch, named, and isn't downloaded" bash -c "cd '$t' &&
  ! printf 'node -v\n' | '$D' --no-network-policy --no-git-protect >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m tools_unmounted 'tools=node $(lockv node)' vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -qx v$(lockv node) '$work/last.log' &&
  limactl shell --workdir / $vm bash -lc '! mountpoint -q /opt/devvm/tools/node/$(lockv node) && [[ ! -e ~/.local/share/mise/installs/node/$(lockv node) ]]' </dev/null"
mv "$work/$nodef" "$share/$nodef"
check "back in the share, the next launch mounts it" bash -c "cd '$t' &&
  printf 'node -v\n' | '$D' --no-network-policy --no-git-protect >'$work/last.log' 2>&1 && grep -qx v$(lockv node) '$work/last.log' &&
  limactl shell --workdir / $vm bash -lc '[[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]]' </dev/null"
limactl stop "$vm" >/dev/null 2>&1
files_before=$(find "$share" -name '*.erofs' | wc -l)
t2=$(newproj tools2); echo '{}' > "$t2/package.json"
launch "$t2" 'node -v' --no-network-policy --no-git-protect
check "a second node VM uses the same tool images: none built" bash -c "
  [[ -n '$vm' ]] && (( \$(find '$share' -name '*.erofs' | wc -l) == $files_before )) && grep -qx v$(lockv node) '$work/last.log'"
check "the tools build left no VM behind" bash -c "[[ -z \$(status_of devvm-tools-build) ]]"
# Upstream can publish mid-run (node's musl builds come days after a release): with the lock
# unchanged, the tools are the newest; changed, the VM may move (if one of its tools did).
check "devvm tools lists node, pnpm and bun, each at the lock's version" bash -c "cd '$t2' && '$D' tools </dev/null >'$work/last.log' 2>&1 &&
  grep -qE '^node +$(lockv node) +- +-$' '$work/last.log' && grep -qE '^pnpm +$(lockv pnpm) +- +-$' '$work/last.log' &&
  grep -qE '^bun +$(lockv bun) +- +-$' '$work/last.log' || { cat '$work/last.log'; false; }"
check "devvm tools update: checks upstream; the newest unless the lock changed" bash -c "cd '$t2' &&
  before=\$(cksum < '$lock'); out=\$(printf 'exit\n' | '$D' tools update 2>&1)
  grep -qF \"\$(m tools_newest vm=$vm | head -1)\" <<<\"\$out\" ||
    { [[ \$(cksum < '$lock') != \"\$before\" ]] && grep -qF \"\$(m tools_switch vm=$vm | head -1)\" <<<\"\$out\"; }"
check "devvm tools rm takes node out of the VM" bash -c "cd '$t2' && printf 'exit\n' | '$D' tools rm node >'$work/last.log' 2>&1 &&
  grep -qx image=base '$vms/$vm' && ! grep -q '^tools=.*node' '$vms/$vm' &&
  limactl shell --workdir / $vm bash -lc '! command -v node' </dev/null"
check "devvm node on it adds nothing: refused in one line, naming devvm tools add" bash -c "cd '$t2' &&
  ! DEVVM_MESSAGES=names '$D' node </dev/null >'$work/last.log' 2>&1; cat '$work/last.log'
  [[ \$(grep -c '^devvm: ' '$work/last.log') -eq 1 ]] && grep -qx 'devvm: vm_lacks_tools tools=node vm=$vm' '$work/last.log' &&
  grep -qx image=base '$vms/$vm'"
check "devvm tools add node mounts it again, nothing built" bash -c "cd '$t2' && n=\$(find '$share' -name '*.erofs' | wc -l) &&
  printf 'node -v\nexit\n' | '$D' tools add node >'$work/last.log' 2>&1 && grep -qx v$(lockv node) '$work/last.log' &&
  grep -qx image=node '$vms/$vm' && (( \$(find '$share' -name '*.erofs' | wc -l) == n ))"
fi

if want network; then
section "network policy"
# A claude+codex VM, so the claude token is bound to it: a fake one in devvm's environment (its
# proxy inherits it), a fake ~/.claude.json with an MCP server's header and a fake Codex login, in a
# home of its own: no real token or login is read.
nh=$work/net-home; rm -rf "$nh"; mkdir -p "$nh/.codex"
printf '%s\n' '{"mcpServers":{"e2e":{"type":"http","url":"https://api.github.com/devvm-e2e-mcp","headers":{"X-Devvm-E2e":"e2efake-mcp"}}}}' > "$nh/.claude.json"
echo '{"OPENAI_API_KEY":"sk-e2efake-codex","last_refresh":null}' > "$nh/.codex/auth.json"
cp "$nh/.claude.json" "$work/claude-before.json"; cp "$nh/.codex/auth.json" "$work/codex-before.json"
n=$(newproj net)
ps_watch
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" --network-policy --no-git-protect claude+codex >"$work/last.log" 2>&1)
ps_unwatch
vm=$(vm_of "$n"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
salt=$(sed -n 's/^proxy_salt=//p' "$vms/$vm" 2>/dev/null)
events=$XDG_DATA_HOME/devvm/logs/$vm.jsonl
# pgrep -f looks for [_]proxy: Linux's pgrep also matches the check's own shell, whose command line
# holds the pattern.
check "--network-policy: the VM runs behind its proxy, and the self-check passed" bash -c "
  [[ -n '$vm' && \$(status_of '$vm') == Running ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ' >/dev/null"
# The control: the launch set the VM's environment (limactl edit, the password's stand-in in the
# run log), and the guest has the password.
check "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it" bash -c "
  key=\$(sed -n 's/^proxy_key=//p' '$vms/$vm') && [[ \${#key} -eq 32 ]] && [[ -s '$work/ps.log' ]] &&
  ! grep -qFf <(echo \"\$key\") '$work/ps.log' && ! ps -axww -o command= | grep -qFf <(echo \"\$key\") &&
  grep -l 'limactl edit .*devvm:devvm-proxy-password@' '$XDG_DATA_HOME'/devvm/logs/runs/*.log >/dev/null &&
  limactl shell --workdir / $vm bash -lc 'printenv HTTPS_PROXY' </dev/null | grep -qF \":\$key@\""
check "ssh devvm-net reaches it, confined, its host key checked" bash -c "
  grep -q '^Host devvm-net $vm\$' '$XDG_CONFIG_HOME/devvm/ssh_config' &&
  [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]]"
check "devvm host runs the self-check, which passes, and names it" bash -c "cd '$n' &&
  HOME='$nh' CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake '$D' host </dev/null >'$work/last.log' 2>&1 && grep -qx net_check=ok '$vms/$vm' &&
  grep -qF \"\$(m ssh_host_info dir=$n host=devvm-net | head -1)\" '$work/last.log'"
check "direct connections fail, even as root: an address, other ports" bash -c "
  ! limactl shell --workdir / $vm sudo curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm sudo curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm sudo timeout 5 bash -c 'exec 3<>/dev/tcp/1.1.1.1/53' </dev/null &&
  ! limactl shell --workdir / $vm sudo timeout 5 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22' </dev/null"
check "an allowed host works, through the proxy" in_vm "$vm" "curl -fsS -m 20 -o /dev/null https://github.com"
check "another host is blocked by the proxy, and so is another port of an allowed one" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://example.com' </dev/null 2>&1 | grep -q 403 &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/' </dev/null) == 403 ]] &&
  ! limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://github.com:8443' </dev/null"
check "devvm network log shows the block and the allowed connection" bash -c "cd '$n' &&
  '$D' network log | grep -q '  BLOCKED  example.com:443  ' && '$D' network log | grep -q '  allowed  github.com:443  rule '"
check "devvm network log --blocked groups it, with the command that allows it" bash -c "cd '$n' &&
  '$D' network log --blocked | grep -Eq '^[0-9TZ:-]+ +[0-9]+ +example\.com:443 +devvm network allow example\.com\$'"
# The command devvm network log suggests, run as it is: the same proxy allows the host at once, and after
# devvm network rm blocks it again.
check "devvm network allow applies at once to the running VM's proxy, devvm network shows it after on, and rm takes it back" bash -c "cd '$n' &&
  pid=\$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') && [[ -n \$pid ]] &&
  cmd=\$('$D' network log --blocked | awk '\$3 == \"example.com:443\" { sub(/^.* devvm /, \"\"); print }') &&
  [[ \$cmd == 'network allow example.com' ]] && '$D' \$cmd </dev/null >/dev/null &&
  '$D' network >'$work/e' && [[ \$(head -1 '$work/e') == on ]] && grep -qx '  example.com' '$work/e' &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://example.com' </dev/null &&
  '$D' network rm example.com >/dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/' </dev/null) == 403 ]] &&
  [[ \$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') == \$pid ]]"
# A line that reads as a deny but isn't a rule: the running proxy denies everything, refused with no
# allow command, devvm network rm warns of it, and once it's gone the allowed host works again.
# The VM's own file is put back whatever happens, so the checks after this one see its rules.
check "a deny that isn't a rule makes the running proxy refuse everything until it's gone" bash -c "cd '$n' &&
  cp '$vms/$vm.policy' '$work/vm.policy' && echo '! example.org' >> '$vms/$vm.policy' &&
  { [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 20 -o /dev/null -w %{http_code} http://github.com/' </dev/null) == 403 ]] &&
    '$D' network log | grep -q '  BLOCKED  github.com:80  .*reads as a deny' &&
    '$D' network log --blocked | grep -Eq ' github\.com:80 +-\$' &&
    DEVVM_MESSAGES=names '$D' network rm nothere.test </dev/null 2>&1 | grep -q '^devvm: policy_broken_deny '; ok=\$?; } ;
  cp '$work/vm.policy' '$vms/$vm.policy' && [[ \${ok:-1} == 0 ]] &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 20 -o /dev/null https://github.com' </dev/null"
check "DNS fails at once, not after a timeout" in_vm "$vm" 'timeout 3 getent hosts example.com; [ $? -eq 2 ]'
# Lima's network's DNS (192.168.5.3) and the host's address on it, asked directly, as root could:
# a refusal or no answer passes, an address for example.com fails.
lima_dns_asked=$(cat <<'EOF'
python3 - <<'PY'
import socket, sys
q = bytes.fromhex("123401000001000000000000076578616d706c6503636f6d0000010001")
for h in ("192.168.5.3", "192.168.5.2"):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(3)
    try:
        s.sendto(q, (h, 53))
        d = s.recv(512)
    except OSError as e:
        print(h, e)
        continue
    print(h, "rcode", d[3] & 15, "answers", int.from_bytes(d[6:8], "big"))
    if d[3] & 15 == 0 and int.from_bytes(d[6:8], "big") > 0:
        sys.exit("example.com answered by " + h)
PY
EOF
)
check "... and Lima's network's DNS addresses, asked directly, answer nothing" in_vm "$vm" "$lima_dns_asked"
# A service on the host's own loopback (a devvm _proxy, which answers anything with 407): unreachable
# directly, and refused by the VM's proxy (internal addresses need a rule naming them).
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
sleep 1
check "the host's loopback services are unreachable, directly or through the proxy" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18765/) == 407 ]] &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null http://192.168.5.2:18765/ </dev/null &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} http://192.168.5.2:18765/' </dev/null) == 403 ]] &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} --noproxy \"\" -x \"\$HTTP_PROXY\" http://127.0.0.1:18765/' </dev/null) == 403 ]]"
kill "$listener" 2>/dev/null; listener=""
# A company proxy: a devvm _proxy of this run's, with a password of its own, allowing any host. The
# VM's next launch with HTTPS_PROXY and HTTP_PROXY in devvm's environment starts its proxy again
# through it, which sends an allowed host there by name, and refuses one the VM's policy doesn't
# allow before it gets there. The launch after, without them, starts it again going directly.
corp_start
check "behind a company proxy, an allowed host goes through it by name, and a blocked one never reaches it" bash -c "
  (cd '$n' && printf 'exit\n' | HOME='$nh' CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake HTTPS_PROXY='$corpurl' HTTP_PROXY='$corpurl' \
    '$D' >'$work/last.log' 2>&1) &&
  grep -q -- ' -upstream ' <(ps -axww -o command= | grep -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 30 -o /dev/null https://github.com' </dev/null &&
  grep -q '\"event\":\"tunnel\",\"host\":\"github.com\",\"port\":443' '$work/corp.jsonl' &&
  grep '\"host\":\"github.com\"' '$events' | grep -q '\"upstream\":\"127.0.0.1:18766\"' &&
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://example.com' </dev/null 2>&1 | grep -q 403 &&
  ! grep -q '\"host\":\"example.com\"' '$work/corp.jsonl'"
# The password is read from its file, never written into the check: its own arguments would hold it.
check "the company proxy's password is in no process's arguments, log or VM" bash -c "
  ! ps -axww -o command= | grep -qFf '$work/corp.auth' &&
  ! grep -rqFf '$work/corp.auth' '$XDG_DATA_HOME/devvm/logs' &&
  ! limactl shell --workdir / $vm sudo grep -rqsFf /dev/stdin /etc /home /root <'$work/corp.auth' &&
  ! limactl shell --workdir / $vm bash -lc env </dev/null | grep -qFf '$work/corp.auth'"
check "without them, the next launch starts the VM's proxy again, going directly" bash -c "
  (cd '$n' && printf 'exit\n' | HOME='$nh' CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake env -u HTTPS_PROXY -u HTTP_PROXY -u https_proxy -u http_proxy \
    '$D' >'$work/last.log' 2>&1) &&
  ! grep -q -- ' -upstream ' <(ps -axww -o command= | grep -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') &&
  n0=\$(grep -c . '$work/corp.jsonl') &&
  limactl shell --workdir / $vm bash -lc 'curl -fsS -m 30 -o /dev/null https://github.com' </dev/null &&
  [[ \$(grep -c . '$work/corp.jsonl') == \"\$n0\" ]]"
corp_stop
check "the proxy refuses anything on the host without the VM's password" bash -c "
  [[ \$(curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:$pp http://devvm.proxy/blocked) == 407 ]]"
check "the token never reaches the VM" bash -c "
  ! limactl shell --workdir / $vm sudo grep -rqs -e e2efake /etc /home /root </dev/null &&
  ! limactl shell --workdir / $vm bash -lc 'env' </dev/null | grep -q e2efake"
check "devvm's CA is installed in the VM" bash -c "
  limactl shell --workdir / $vm cat /usr/local/share/ca-certificates/devvm-proxy.crt </dev/null | cmp -s - '$XDG_DATA_HOME/devvm/ca/ca.pem'"
check "the VM gets its own placeholder, not the token" in_vm "$vm" "[[ \$CLAUDE_CODE_OAUTH_TOKEN == sk-ant-oat01-devvm-proxy-managed-$salt ]]"
check "the VM's Codex and Claude Code (MCP) logins hold only placeholders" bash -c "
  ! limactl shell --workdir / $vm bash -c 'cat ~/.codex/auth.json ~/.claude.json' </dev/null | grep -e e2efake &&
  limactl shell --workdir / $vm bash -c 'cat ~/.codex/auth.json' </dev/null | grep -q devvm-proxy-managed"
# Requests verify the proxy's certificates: no -k.
check "the proxy puts the token in for its host" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.anthropic.com/v1/models -H \"Authorization: Bearer \$CLAUDE_CODE_OAUTH_TOKEN\"' </dev/null &&
  grep '\"path\":\"/v1/models\"' '$events' | grep -q '\"secrets\":\[\"claude\"\]'"
check "but not for another VM's placeholder" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.anthropic.com/devvm-e2e-other -H \"Authorization: Bearer sk-ant-oat01-devvm-proxy-managed-$(printf '0%.0s' {1..32})\"' </dev/null &&
  grep -q '/devvm-e2e-other' '$events' && ! grep '/devvm-e2e-other' '$events' | grep -q secrets"
check "and an MCP server's header for its host, from the VM's copy of the login" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.github.com/devvm-e2e-mcp -H \"X-Devvm-E2e: \$(jq -r .mcpServers.e2e.headers[\\\"X-Devvm-E2e\\\"] ~/.claude.json)\"' </dev/null &&
  grep '/devvm-e2e-mcp' '$events' | grep -q '\"secrets\":\[\"claude-mcp-'"
check "and Codex's API key, from the VM's copy of its login" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.openai.com/v1/models -H \"Authorization: Bearer \$(jq -r .OPENAI_API_KEY ~/.codex/auth.json)\"' </dev/null &&
  grep '\"path\":\"/v1/models\"' '$events' | grep -q '\"secrets\":\[\"codex-key\"\]'"
check "the host's login files are unchanged" bash -c "cmp -s '$nh/.codex/auth.json' '$work/codex-before.json' && cmp -s '$nh/.claude.json' '$work/claude-before.json'"
# devvm secret with a fake GitHub token: in the Keychain under this run's service on macOS, else in
# the sandbox's config folder; deleted at the end. macOS finds the Keychain through HOME, so the
# VM's home links the Keychains folder (the link alone goes at the end).
if [[ "$(uname -s)" == Darwin ]]; then mkdir -p "$nh/Library" && ln -s "$HOME/Library/Keychains" "$nh/Library/Keychains"; fi
check "devvm secret set saves a token, and devvm reads it back, never to a terminal" bash -c "
  echo ghp_E2EFAKE | HOME='$nh' '$D' secret set github --stdin >/dev/null && [[ \$(HOME='$nh' '$D' secret get github | cat) == ghp_E2EFAKE ]] &&
  { [[ \$(uname -s) != Darwin ]] || [[ \$(security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github -w) == ghp_E2EFAKE ]]; }"
check "secret use github: its proxy gets the binding at once, the VM its own placeholder on its next launch" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret use github >/dev/null && grep -q '\"name\":\"github\"' '$XDG_DATA_HOME/devvm/run/$vm.secrets.json' &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(limactl shell --workdir / $vm bash -lc 'echo \$GH_TOKEN' </dev/null) == devvm-proxy-managed-github-$salt ]] &&
  ! limactl shell --workdir / $vm bash -lc 'env; cat ~/.config/devvm/env' </dev/null | grep -q ghp_E2EFAKE &&
  HOME='$nh' '$D' secret | grep -Eq '^github +yes +$vm\$'"
check "the proxy puts the GitHub token in for its host" bash -c "
  limactl shell --workdir / $vm bash -lc 'curl -sS -m 20 -o /dev/null https://api.github.com/user -H \"Authorization: token \$GH_TOKEN\"' </dev/null &&
  grep '\"path\":\"/user\"' '$events' | grep -q '\"secrets\":\[\"github\"\]'"
check "secret unuse and rm take the binding and the token away at once" bash -c "cd '$n' &&
  HOME='$nh' '$D' secret unuse github >/dev/null && ! grep -q '\"name\":\"github\"' '$XDG_DATA_HOME/devvm/run/$vm.secrets.json' &&
  HOME='$nh' '$D' secret rm --force github >/dev/null && ! HOME='$nh' '$D' secret get github >/dev/null 2>&1 &&
  { [[ \$(uname -s) != Darwin ]] || ! security find-generic-password -s '$DEVVM_SECRET_SERVICE' -a github >/dev/null 2>&1; }"
check "the self-check passes on the VM devvm started confined" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT= CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck'"
check "a VM started outside devvm is unenforced, and the self-check stops it" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$root' &&
  DEVVM_E2E_VM=$vm DEVVM_E2E_WANT='net_unenforced_stop vm=$vm' CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2ESelfCheck\$' ./internal/netpolicy |
  grep -q -- '--- PASS: TestE2ESelfCheck' && [[ \$(status_of '$vm') == Stopped ]] && grep -qx net_check=unenforced '$vms/$vm'"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$n' && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  [[ \$(status_of '$vm') == Running ]] && grep -qx net_check=ok '$vms/$vm'"
check "devvm stop stops it and its proxy at once, and devvm starts both again" bash -c "cd '$n' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ \$(status_of '$vm') == Stopped ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$(status_of '$vm') == Running ]] &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && grep -qx net_check=ok '$vms/$vm'"
# macOS: the hostagent's Seatbelt profile denies it every bind but its ssh port's, so Lima forwards
# nothing even without its ignore rules. A limactl first on PATH puts Lima's own rule (forward
# everything) back for one start, inside the sandbox devvm starts limactl in: Lima tries the guest's
# ports (its log says so), the sandbox refuses them, and devvm's forwarder has them.
if [[ "$(uname -s)" == Darwin ]]; then
  sp=$(sed -n 's/^ssh_port=//p' "$vms/$vm"); ha=$DEVVM_LIMA_HOME/$vm/ha.stderr.log
  # ha_listens: the hostagent's TCP listeners and UDP sockets, one per line.
  ha_listens() { lsof -nP -a -p "$(cat "$DEVVM_LIMA_HOME/$vm/ha.pid")" -iTCP -sTCP:LISTEN -iUDP 2>/dev/null | awk 'NR > 1 { print $8, $9 }'; }
  export -f ha_listens in_vm; export vm sp ha
  check "the hostagent's profile denies every bind but its ssh port's (TCP) and its own unix sockets'; it has no other" bash -c "
    sb='$XDG_DATA_HOME/devvm/run/$vm.sb' && [[ -n '$sp' && \$(grep -c network-bind \"\$sb\") == 3 ]] &&
    grep -qxF '(deny network-bind)' \"\$sb\" && grep -qxF '(allow network-bind (local tcp \"localhost:$sp\"))' \"\$sb\" &&
    grep -qxF \"(allow network-bind (local unix-socket (subpath \\\"\$(cd '$DEVVM_LIMA_HOME/$vm' && pwd -P)\\\")))\" \"\$sb\" &&
    bind='import socket; socket.socket().bind((\"127.0.0.1\", 47197))' && python3 -c \"\$bind\" &&
    ! /usr/bin/sandbox-exec -f \"\$sb\" python3 -c \"\$bind\" 2>/dev/null &&
    udp='import socket; socket.socket(socket.AF_INET, socket.SOCK_DGRAM).bind((\"127.0.0.1\", $sp))' && python3 -c \"\$udp\" &&
    ! /usr/bin/sandbox-exec -f \"\$sb\" python3 -c \"\$udp\" 2>/dev/null &&
    l=\$(ha_listens) && echo \"hostagent's: \$l\" && [[ \$l == 'TCP 127.0.0.1:$sp' ]] &&
    grep -A1 '^hostResolver:' '$DEVVM_LIMA_HOME/$vm/lima.yaml' | grep -q 'enabled: false'"
  lima_real=$(command -v limactl); mkdir -p "$work/lima-bypass"
  cat > "$work/lima-bypass/limactl" <<EOF
#!/bin/bash
# e2e: Lima's own rule for this start, set where limactl start runs: inside devvm's sandbox.
if [[ \$1 == start ]]; then "$lima_real" edit --tty=false --set '.portForwards = []' "\${@: -1}" >/dev/null 2>&1 || exit 1; fi
exec "$lima_real" "\$@"
EOF
  chmod 755 "$work/lima-bypass/limactl"
  check "with Lima's own forwards on, the sandbox refuses the hostagent a VM server's port, TCP or UDP" bash -c "cd '$n' &&
    HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 &&
    printf 'exit\n' | PATH='$work/lima-bypass':\$PATH HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$(status_of '$vm') == Running ]] &&
    grep -qx 'portForwards: \[\]' '$DEVVM_LIMA_HOME/$vm/lima.yaml' && [[ \$(sed -n 's/^ssh_port=//p' '$vms/$vm') == '$sp' ]] &&
    in_vm '$vm' \"setsid -f python3 -m http.server -b 0.0.0.0 47195 </dev/null >/dev/null 2>&1; setsid -f python3 -c 'import socket,time; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((\\\"0.0.0.0\\\",47196)); time.sleep(600)' devvm-e2e-udp </dev/null >/dev/null 2>&1\" &&
    refused() { grep -q 'listen tcp 127.0.0.1:47195: bind: operation not permitted' \"\$ha\" &&
      grep -Eq 'listen udp [0-9.]+:47196: bind: operation not permitted' \"\$ha\"; } &&
    { for i in \$(seq 20); do refused && break; sleep 1; done; refused; } && grep -q 'Forwarding TCP from 0.0.0.0:47195' \"\$ha\" &&
    ! lima_listens tcp 47195 && ! lima_listens udp 47196 && l=\$(ha_listens) && echo \"hostagent's: \$l\" && [[ \$l == 'TCP 127.0.0.1:$sp' ]]"
  check "... and the VM works: ssh, devvm's forwarder (on loopback alone), the proxy, the self-check" bash -c "
    for i in \$(seq 20); do nc -z 127.0.0.1 47195 && break; sleep 1; done; nc -z 127.0.0.1 47195 &&
    { loopback_only 47195 || { echo 'not on loopback alone:'; lsof -nP -iTCP:47195 -sTCP:LISTEN; false; }; } &&
    { pgrep -f -- '[_]lima-ports -vm $vm ' >/dev/null || { echo 'no forwarder'; false; }; } &&
    { ! lima_listens tcp 47195 || { echo 'limactl listens'; false; }; } &&
    { [[ \$(env -u SSH_AUTH_SOCK ssh -F '$XDG_CONFIG_HOME/devvm/ssh_config' devvm-net hostname </dev/null) == net ]] || { echo 'ssh failed'; false; }; } &&
    { in_vm '$vm' 'curl -fsS -m 20 -o /dev/null https://github.com' || { echo 'the proxy failed'; false; }; } &&
    { grep -qx net_check=ok '$vms/$vm' || { echo 'no self-check:'; grep '^net' '$vms/$vm'; false; }; }"
  in_vm "$vm" 'pkill -f "[h]ttp.server -b 0.0.0.0 4719"; pkill -f "[d]evvm-e2e-udp"' >/dev/null 2>&1
  check "the next start puts Lima's ignore rules back" bash -c "cd '$n' &&
    HOME='$nh' '$D' stop </dev/null >/dev/null 2>&1 && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
    [[ \$(limactl list --format '{{range .Config.PortForwards}}{{.GuestIP}}={{.Ignore}} {{end}}' $vm) == '127.0.0.1=true 0.0.0.0=true ::=true ' ]] &&
    grep -qx net_check=ok '$vms/$vm'"
else
  skip "the hostagent's profile denies every bind but its ssh port's (TCP) and its own unix sockets'; it has no other" "Seatbelt is macOS's"
  skip "with Lima's own forwards on, the sandbox refuses the hostagent a VM server's port, TCP or UDP" "Seatbelt is macOS's"
  skip "... and the VM works: ssh, devvm's forwarder (on loopback alone), the proxy, the self-check" "Seatbelt is macOS's"
  skip "the next start puts Lima's ignore rules back" "Seatbelt is macOS's"
fi
# Ctrl+C while Lima's start waits for the VM, its hostagent running it: devvm stops the VM and its
# proxy, so it never runs without the record of its confined start, and ls shows its policy on.
(cd "$n" && HOME=$nh "$D" stop </dev/null >/dev/null 2>&1)
(cd "$n" && exec env HOME="$nh" CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake DEVVM_MESSAGES=names "$D" </dev/null >"$work/int.log" 2>&1) &
ip=$!
for _ in $(seq 300); do [[ -S $DEVVM_LIMA_HOME/$vm/ha.sock ]] && break; sleep 0.2; done
kill -INT "$ip" 2>/dev/null
wait "$ip"; rc=$?
check "Ctrl+C during its start stops it and its proxy, and ls shows its policy on" bash -c "
  [[ $rc -eq 130 ]] && grep -qx '==> interrupted_start vm=$vm' '$work/int.log' && [[ \$(status_of '$vm') == Stopped ]] &&
  ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  '$D' ls | grep -Eq '^$vm +Stopped +[a-z+]*claude[a-z+]* +off +on +- +lima '"
(cd "$n" && printf 'exit\n' | HOME=$nh CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-e2efake "$D" >"$work/last.log" 2>&1)
check "ls shows its policy on, and unenforced once started outside devvm" bash -c "
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +on +- +lima ' &&
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 &&
  '$D' ls | grep -Eq '^$vm +Running +[a-z+]*claude[a-z+]* +off +unenforced +- +lima '"
check "started outside devvm again: without a terminal, devvm stops it and opens nothing" bash -c "
  limactl stop $vm >/dev/null 2>&1 && limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! printf 'echo e2e-opened\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -q e2e-opened '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
check "so does devvm host, which names nothing" bash -c "
  limactl start --tty=false $vm >/dev/null 2>&1 && cd '$n' &&
  ! HOME='$nh' '$D' host </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m net_unenforced_stop vm=$vm | head -1)\" '$work/last.log' &&
  ! grep -qF \"\$(m ssh_host_info dir=$n host=devvm-net | head -1)\" '$work/last.log' && [[ \$(status_of '$vm') == Stopped ]]"
# Stopped by the self-check, it's still unenforced: its last self-check failed.
check "devvm network off: its next start is direct, without a proxy" bash -c "cd '$n' &&
  [[ \$('$D' network | head -1) == unenforced ]] && '$D' network off </dev/null >/dev/null &&
  printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 && [[ \$('$D' network | head -1) == off ]] &&
  [[ \$(status_of '$vm') == Running ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  limactl shell --workdir / $vm curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1 </dev/null"
check "devvm network on while it runs: unenforced until devvm restart starts it confined" bash -c "cd '$n' &&
  '$D' network on </dev/null >/dev/null 2>&1 && [[ \$('$D' network | head -1) == unenforced ]] &&
  printf 'exit\n' | HOME='$nh' '$D' restart >'$work/last.log' 2>&1 && [[ \$('$D' network | head -1) == on ]] &&
  grep -qx net_check=ok '$vms/$vm' && pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "devvm network off while it runs confined: launched, it keeps its proxy until it next starts" bash -c "cd '$n' &&
  '$D' network off </dev/null >/dev/null && printf 'exit\n' | HOME='$nh' '$D' >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m net_off_next_start vm=$vm | head -1)\" '$work/last.log' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null"
check "rm --force deletes it, its proxy and its state" bash -c "cd '$n' && HOME='$nh' '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  ! limactl list -q | grep -qx $vm && [[ ! -e '$vms/$vm' ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
fi

if want desktop; then
section "desktop"
# A base VM with network policy, base having the desktop feature: devvm desktop serves its desktop
# on this machine's loopback alone, a viewer checked against its password here; the VM's server
# listens on a socket of its user's alone, never a port, and gets no way out. Then root in the VM
# replaces the server with one that asks for no password and records what reaches it: a viewer
# still needs the password, and its clipboard still never reaches the VM.
p=$(newproj desk)
launch "$p" exit --network-policy --no-git-protect base
ps_watch
desktop_serve "$p"
ps_unwatch
check "devvm desktop serves its desktop on this machine's 127.0.0.1 alone, its port and password kept" bash -c "
  [[ -n '$vm' && -n '$desk_port' ]] && loopback_only '$desk_port' &&
  grep -qx 'desktop_port=$desk_port' '$vms/$vm' && grep -qx 'desktop_password=$desk_pw' '$vms/$vm'"
check "a viewer with a wrong password is refused, one with the password gets the VM's desktop" desktop_view
check "the VM's server listens on its user's own socket, on no TCP port, the clipboard off" in_vm "$vm" "$desktop_in_vm"
check "the password is in no file of the VM's" bash -c 'printf "%s\n" "$1" | limactl shell --workdir / "$2" sudo sh -c "$3"' \
  _ "$desk_pw" "$vm" "$desktop_secret"
check "... nor any run log, nor any host process's arguments while devvm desktop started" bash -c "
  [[ -s '$work/ps.log' ]] && ! grep -rlF '$desk_pw' '$XDG_DATA_HOME/devvm/logs' && ! grep -F '$desk_pw' '$work/ps.log'"
check "with network policy, the VM still reaches only its proxy, and not the desktop's port here" bash -c "
  grep -qx net_check=ok '$vms/$vm' &&
  ! limactl shell --workdir / $vm curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1 </dev/null &&
  ! limactl shell --workdir / $vm timeout 5 bash -c '</dev/tcp/host.lima.internal/$desk_port' </dev/null"
# Root's own server in its place: no password, its socket anyone's, everything that reaches it
# recorded in /tmp/devvm-e2e-rfb.bin, each connection counted.
cat > "$p/fake-rfb.py" <<'P'
import os, socket
p = "/run/devvm-desktop/vnc.sock"
if os.path.exists(p):
    os.unlink(p)
s = socket.socket(socket.AF_UNIX)
s.bind(p)
os.chmod(p, 0o666)
s.listen(4)
n = 0
while True:
    c, _ = s.accept()
    n += 1
    open("/tmp/devvm-e2e-rfb.count", "w").write(str(n))
    c.sendall(b"RFB 003.008\n"); c.recv(12); c.sendall(b"\x01\x01"); c.recv(1); c.sendall(b"\0\0\0\0")
    c.recv(1)
    c.sendall(b"\0\x01\0\x01" + b"\0" * 16 + b"\0\0\0\0")
    data = b""
    while True:
        b = c.recv(4096)
        if not b:
            break
        data += b
    open("/tmp/devvm-e2e-rfb.bin", "wb").write(data)
    c.close()
P
in_vm "$vm" "sudo pkill -x Xvnc; sleep 1; sudo rm -f /tmp/devvm-e2e-rfb.*; sudo setsid -f python3 '$p/fake-rfb.py' </dev/null >/dev/null 2>&1; sleep 1" >/dev/null 2>&1
check "root's own server, without a password: a viewer with a wrong one is still refused, the password let in" \
  desktop_view DEVVM_E2E_CLIPBOARD=devvm-e2e-clip
check "... the refused viewer never reached the VM, nor the viewer's clipboard: only its key event did" bash -c "sleep 1
  limactl shell --workdir / $vm sh -c '[ \"\$(cat /tmp/devvm-e2e-rfb.count)\" = 1 ] &&
    od -An -tx1 /tmp/devvm-e2e-rfb.bin | tr -d \" \\n\" | grep -q 040100000000006b &&
    ! grep -q devvm-e2e-clip /tmp/devvm-e2e-rfb.bin' </dev/null"
desktop_end
check "Ctrl+C ends devvm desktop, its port closed" bash -c "[[ -n '$desk_port' ]] && ! nc -z 127.0.0.1 '$desk_port'"
in_vm "$vm" "sudo pkill -f '[f]ake-rfb.py'; sudo rm -f /run/devvm-desktop/vnc.sock /tmp/devvm-e2e-rfb.*" >/dev/null 2>&1
desktop_serve "$p" --clipboard
check "devvm desktop --clipboard starts its server again, on the same port, the clipboard on" bash -c "
  [[ -n '$desk_port' && '$desk_port' == \$(sed -n 's/^desktop_port=//p' '$vms/$vm') ]] &&
  limactl shell --workdir / $vm bash -lc 'DISPLAY=:1 XAUTHORITY=\$HOME/.Xauthority vncconfig -get AcceptCutText | grep -qx on' </dev/null"
check "... and a viewer with the password gets it" desktop_view
desktop_end
fi

if want nix; then
section "the nix layer"
# Behind a company proxy, the layer's script (as a layer of the run's own, so it builds each run) gets
# its build proxy's settings, which Nix's installer would write into nix-daemon's: none is anywhere
# on its disk.
corp_start
cp "$root/assets/layers/nix.sh" "$layers/e2enix.sh"
check "built behind a company proxy, the layer leaves no proxy settings for nix-daemon in its image" bash -c "
  env HTTPS_PROXY='$corpurl' HTTP_PROXY='$corpurl' '$D' build e2enix </dev/null >'$work/last.log' 2>&1 &&
  k=\$(keys_of e2enix | head -1) && [[ -n \$k && -d '$LIMA_HOME/devvm-img-'\$k ]] &&
  grep -q 'releases\.nixos\.org' '$work/corp.jsonl' &&
  ! LC_ALL=C grep -rqaF -D skip -e 'Environment=http_proxy=' -e 'Environment=https_proxy=' -e '$corppw' '$LIMA_HOME/devvm-img-'\$k"
owned e2enix
corp_stop
rm -f "$layers/e2enix.sh"
# A node project with a flake: node+nix, with network policy. The flake's dev shell has a node of its
# own, from nixpkgs at a pinned revision (nixos-26.05's), cached by Nix's servers.
nixpkgs_rev=6f06d204d8f2d8761d9da78778745974a3216381
x=$(newproj nixflake); echo '{}' > "$x/package.json"; echo 'use flake' > "$x/.envrc"
cat > "$x/flake.nix" <<EOF
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/$nixpkgs_rev";
  outputs = { self, nixpkgs }: {
    devShells = nixpkgs.lib.genAttrs [ "aarch64-linux" "x86_64-linux" ] (s: let pkgs = nixpkgs.legacyPackages.\${s}; in {
      default = pkgs.mkShellNoCC { packages = [ (pkgs.writeShellScriptBin "node" "echo e2e-flake-node") ]; };
    });
  };
}
EOF
# The shared Nix cache starts empty, so the first VM fetches into it and the second gets hits.
nixcache=$XDG_DATA_HOME/devvm/nix-cache
rm -rf "$nixcache"
launch "$x" exit --network-policy --no-git-protect
export -f in_vm
nv=$(sed -n 's/^nix_version=//p' "$root/assets/layers/nix.sh")
dv=$(sed -n 's/^direnv_version=//p' "$root/assets/layers/nix.sh")
check "a flake.nix and a package.json: a node+nix VM with network policy, Nix and direnv at their pins" bash -c "
  [[ -n '$vm' ]] && grep -qF \"\$(m detected found=node+nix)\" '$work/last.log' && grep -qx image=node+nix '$vms/$vm' &&
  grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  in_vm '$vm' '[[ \$(nix --version) == \"nix (Nix) $nv\" && \$(direnv version) == $dv ]]'"
check "Nix's store is root's, builds are sandboxed, only root is trusted, and the user's own caches are ignored" in_vm "$vm" '
  [[ -z $(sudo find /nix/store -mindepth 1 -maxdepth 1 ! -user root -print -quit) ]] &&
  [[ $(nix config show sandbox) == true && $(nix config show sandbox-fallback) == false ]] &&
  [[ $(nix config show trusted-users) == root ]] &&
  nix store info --option substituters https://example.invalid 2>&1 | grep -q "ignoring untrusted substituter"'
check "nix-daemon gets the VM's proxy alone, and its NO_PROXY, in devvm's drop-in" in_vm "$vm" '
  [[ $(ls /etc/systemd/system/nix-daemon.service.d) == devvm-proxy.conf && $NO_PROXY == localhost,127.0.0.1,::1 ]] &&
  e=$(systemctl show nix-daemon.service -p Environment --value) &&
  [[ $e == "http_proxy=$HTTP_PROXY https_proxy=$HTTP_PROXY no_proxy=$NO_PROXY" ]]'
check "the VM's Nix tries the shared cache first, then cache.nixos.org, trusting cache.nixos.org's key alone" in_vm "$vm" '
  [[ -f /etc/nix/devvm-cache.conf && $(nix config show substituters) == "http://devvm.proxy/nix https://cache.nixos.org"* ]] &&
  [[ $(nix config show trusted-public-keys) == "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=" ]]'
check "nix run nixpkgs#hello works with network policy, through the shared cache" in_vm "$vm" \
  "nix run nixpkgs#hello 2>/tmp/hello.err | grep -qx 'Hello, world!' && grep -q 'from .http://devvm.proxy/nix' /tmp/hello.err &&
   nix build --no-link github:NixOS/nixpkgs/$nixpkgs_rev#hello"
nixev=$XDG_DATA_HOME/devvm/logs/$vm.jsonl
check "... which the host fetched into the shared cache, each file the user's alone" bash -c "
  grep -q '\"event\":\"nix-cache\".*\"reason\":\"fetched\"' '$nixev' &&
  ls '$nixcache'/*.narinfo >/dev/null && ls '$nixcache'/nar/*.nar* >/dev/null &&
  [[ -z \$(find '$nixcache' -type f ! -perm 600 -print -quit) && -z \$(find '$nixcache' -type d ! -perm 700 -print -quit) ]]"
check "a VM's writes to the shared cache, and its requests for anything else through it, are refused" in_vm "$vm" '
  c() { curl -s -o /dev/null -w "%{http_code}" --proxy "$HTTP_PROXY" "$@"; }
  z=00000000000000000000000000000000
  [[ $(c -X PUT --data-binary x http://devvm.proxy/nix/$z.narinfo) == 400 ]] &&
  [[ $(c -X POST --data-binary x http://devvm.proxy/nix/nar/${z}00000000000000000000.nar.xz) == 400 ]] &&
  [[ $(c http://devvm.proxy/nix/https://example.com/x) == 400 ]] &&
  [[ $(c "http://devvm.proxy/nix/$z.narinfo?u=https://example.com") == 400 ]] &&
  [[ $(c --path-as-is http://devvm.proxy/nix/nar/../../etc/passwd) == 400 ]] &&
  [[ $(c http://devvm.proxy/nix/nar/${z}00000000000000000000.nar.xz) == 404 ]] &&
  [[ $(c http://devvm.proxy/nix/$z.narinfo) == 404 ]]'
check "... and the host kept none of them, asking cache.nixos.org alone" bash -c "
  [[ ! -e '$nixcache/00000000000000000000000000000000.narinfo' ]] &&
  grep -q '\"path\":\"/nix/00000000000000000000000000000000.narinfo\".*\"reason\":\"not found: cache.nixos.org answered 404\"' '$nixev' &&
  grep -q '\"path\":\"/nix/nar/0*.nar.xz\".*\"reason\":\"not found: no checked narinfo names it\"' '$nixev' &&
  [[ -z \$(find '$nixcache' -type f ! -name '.*' ! -name '*.narinfo' ! -path '*/nar/*' -print -quit) ]]"
# A second VM of a Nix project gets what the first fetched from the shared cache, as hits.
nixvm1=$vm
y=$(newproj nixflake2); cp "$x/flake.nix" "$x/flake.lock" "$x/package.json" "$y/"
launch "$y" exit --network-policy --no-git-protect
nixvm2=$vm
check "a second VM gets the paths the first fetched as hits from the shared cache, fetching none again" bash -c "
  [[ -n '$nixvm2' && '$nixvm2' != '$nixvm1' ]] &&
  in_vm '$nixvm2' \"nix build --no-link github:NixOS/nixpkgs/$nixpkgs_rev#hello 2>/tmp/hello.err && grep -q 'from .http://devvm.proxy/nix' /tmp/hello.err\" &&
  ev='$XDG_DATA_HOME/devvm/logs/$nixvm2.jsonl' &&
  grep -q '\"event\":\"nix-cache\",.*\"path\":\"/nix/nar/[^\"]*\".*\"reason\":\"hit\"' \"\$ev\" &&
  ! grep -q '\"event\":\"nix-cache\".*\"reason\":\"fetched\"' \"\$ev\""
limactl stop "$nixvm2" >/dev/null 2>&1
vm=$nixvm1
nix_built='derivation { name = "e2e-sandboxed"; system = builtins.currentSystem; builder = "/bin/sh"; args = [ "-c" "echo ok > $out" ]; }'
check "a build runs in Nix's sandbox, which it would fail without" in_vm "$vm" "nix build --no-link --impure --expr '$nix_built'"
check "sudo nix finds Nix, and so does a command over ssh" bash -c "
  in_vm '$vm' 'sudo nix --version' | grep -qx 'nix (Nix) $nv' &&
  ssh -F '$LIMA_HOME/$vm/ssh.config' lima-$vm 'nix --version' </dev/null | grep -qx 'nix (Nix) $nv'"
# An interactive shell, its commands on its input, as a terminal's: direnv's and mise's hooks run
# before each prompt.
printf '%s\n' "cd $x" 'direnv allow' 'cd /' "cd $x" 'echo "e2e-a=$(node -v)"' 'true' 'echo "e2e-b=$(node -v)"' \
  'cd /' 'echo "e2e-c=$(node -v)"' | limactl shell --workdir / "$vm" bash -i >"$work/nix-shell.log" 2>&1
check "the flake's dev shell, through direnv, comes before the shared node, prompt after prompt, and goes" bash -c "
  grep -qx 'e2e-a=e2e-flake-node' '$work/nix-shell.log' && grep -qx 'e2e-b=e2e-flake-node' '$work/nix-shell.log' &&
  grep -qx 'e2e-c=v$(lockv node)' '$work/nix-shell.log'"
check "nix-direnv's cache is in the VM's home, not the project folder" bash -c "
  [[ ! -e '$x/.direnv' && -f '$x/flake.lock' ]] && in_vm '$vm' 'ls -d ~/.cache/direnv/layouts/*/flake-profile-*' >/dev/null"
# A download only the daemon makes (a fixed-output fetch) from a host the policy doesn't allow:
# refused by the proxy, and named by devvm-why.
nix_fetch='derivation { name = "e2e"; system = builtins.currentSystem; builder = "builtin:fetchurl"; url = "https://example.com/devvm-e2e"; outputHashMode = "flat"; outputHashAlgo = "sha256"; outputHash = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="; }'
check "a download from a host that isn't allowed fails, and devvm-why names it" in_vm "$vm" "
  ! nix build --no-link --impure --expr '$nix_fetch' 2>/tmp/fetch.err && grep -q 'response 403' /tmp/fetch.err &&
  devvm-why | grep -q 'devvm network allow example.com'"
check "devvm network off: from the next start, nix-daemon gets no proxy" bash -c "cd '$x' &&
  '$D' network off </dev/null >/dev/null && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  in_vm '$vm' '[[ -z \$(ls -A /etc/systemd/system/nix-daemon.service.d 2>/dev/null) && -z \$(systemctl show nix-daemon.service -p Environment --value) ]]'"
check "... and no shared cache: Nix goes to cache.nixos.org alone" in_vm "$vm" \
  '[[ ! -e /etc/nix/devvm-cache.conf && $(nix config show substituters) != *devvm.proxy* ]]'
limactl stop "$vm" >/dev/null 2>&1
fi

if want from; then
section "--from"
# Registry images need no Docker: these run with a docker that's installed but not running (it
# logs what devvm asked it, which may only be whether it has the image: launches ask only the
# backends a VM's state names).
mkdir -p "$work/nodocker"; : > "$work/docker-calls"
printf '#!/bin/sh\necho "docker $*" >> "%s"\necho "Cannot connect to the Docker daemon" >&2; exit 1\n' \
  "$work/docker-calls" > "$work/nodocker/docker"
chmod 755 "$work/nodocker/docker"
nd() { PATH="$work/nodocker:$PATH" "$@"; }
export -f nd in_vm; export work
[[ -f $lock ]] || quiet "$D" _refresh-tools   # the tools' versions, for lockv
# from_record REF: base's record for that --from.
from_record() { grep -lx "from=$1" "$images"/* 2>/dev/null | xargs grep -lx name=base 2>/dev/null | head -1; }
export -f from_record

f=$(newproj fromdocker); echo '{}' > "$f/package.json"
fh=$work/from-home; mkdir -p "$fh"   # the claude VM's: no agent config of yours
HOME=$fh nd launch "$f" exit --from node:24 --no-network-policy node+python+docker+nix+claude   # node:24 is Debian 12
# A build in Nix's sandbox, which fails without it (the nix layer's sandbox-fallback = false).
nix_built='derivation { name = "e2e-sandboxed"; system = builtins.currentSystem; builder = "/bin/sh"; args = [ "-c" "echo ok > $out" ]; }'
check "a node+python+docker+nix+claude VM from node:24 (Debian 12) boots, its base keyed by the registry's digest" bash -c "
  [[ -n '$vm' ]] && limactl shell --workdir / '$vm' cat /etc/os-release </dev/null | grep -qx VERSION_CODENAME=bookworm &&
  grep -qx from=node:24 '$vms/$vm' && grep -qx libc=gnu '$vms/$vm' &&
  grep -qE '^digest=node@sha256:[0-9a-f]{64}$' \"\$(from_record node:24)\""
check "its node, claude and every feature work" in_vm "$vm" \
  'node --version && claude --version && eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
check "python, python3 and claude are the locked versions" in_vm "$vm" \
  "[[ \$(python --version) == 'Python $(lockv python)' && \$(python3 --version) == 'Python $(lockv python)' && \$(claude --version) == '$(lockv claude) '* ]]"
check "an interactive shell starts without errors" bash -c "out=\$(limactl shell --workdir / $vm bash -ic true </dev/null 2>&1) && ! grep -iE 'error|unknown option|not found' <<<\"\$out\""
check "the docker layer's Docker runs in it (Docker's Debian repository), for the VM's user too" in_vm "$vm" \
  'sudo docker run --rm hello-world | grep -q Hello && id -nG | grep -qw docker'
check "the nix layer's Nix runs in it (systemd): nix run nixpkgs#hello, and a build in Nix's sandbox" in_vm "$vm" "
  systemctl is-active --quiet nix-daemon.socket && nix run nixpkgs#hello | grep -qx 'Hello, world!' &&
  nix build --no-link --impure --expr '$nix_built'"
(cd "$f" && HOME=$fh quiet "$D" rm --force --no-export)

# Alpine: apk, OpenRC and musl, so its own (musl) tool images; the docker layer from Alpine's packages;
# the nix layer's daemon as an OpenRC service of devvm's.
al=$(newproj fromalpine); echo '{}' > "$al/package.json"
nd launch "$al" exit --from alpine:3 --no-network-policy node+docker+nix
check "an Alpine VM runs node from musl tool images" bash -c "[[ -n '$vm' ]] && grep -qx libc=musl '$vms/$vm' && grep -qx libc=musl \"\$(from_record alpine:3)\""
check "and node, pnpm work there" in_vm "$vm" \
  "grep -q '^ID=alpine' /etc/os-release && [[ \$(node -v) == v$(lockv node) ]] && node -e 'require(\"os\")' && pnpm -v"
check "the docker layer's Docker runs in it (OpenRC)" in_vm "$vm" 'sudo docker run --rm hello-world | grep -q Hello'
check "ports below 1024 are open to the VM's user after the setup (Alpine's kernel starts at 1024)" in_vm "$vm" \
  '[[ $(cat /proc/sys/net/ipv4/ip_unprivileged_port_start) == 0 ]]'
check "and every feature" in_vm "$vm" 'eza --version && bat --version && fd --version && fzf --version && tldr --version && [[ -d ~/.oh-my-bash ]]'
# As root: syslog's and acpid's pid files are root-only, so a user's rc-status calls them crashed.
check "no crashed services" in_vm "$vm" '[[ -z "$(sudo rc-status --crashed 2>/dev/null)" ]]'
check "the nix layer's Nix runs in it (OpenRC, musl): nix run nixpkgs#hello, and a build in Nix's sandbox" in_vm "$vm" "
  sudo rc-service nix-daemon status >/dev/null && nix run nixpkgs#hello | grep -qx 'Hello, world!' &&
  nix build --no-link --impure --expr '$nix_built'"
# With network policy, OpenRC's service reads the proxy from its settings: a download only the daemon
# makes, from a host that isn't allowed, gets the proxy's 403.
nix_fetch='derivation { name = "e2e"; system = builtins.currentSystem; builder = "builtin:fetchurl"; url = "https://example.com/devvm-e2e"; outputHashMode = "flat"; outputHashAlgo = "sha256"; outputHash = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="; }'
check "devvm network on: after devvm restart, its nix-daemon goes through the proxy" bash -c "cd '$al' &&
  nd '$D' network on </dev/null >/dev/null 2>&1 && printf 'exit\n' | nd '$D' restart >'$work/last.log' 2>&1 &&
  grep -qx net_check=ok '$vms/$vm' && in_vm '$vm' 'sudo grep -q \"^export http_proxy=http://devvm:\" /etc/conf.d/nix-daemon &&
    ! nix build --no-link --impure --expr '\''$nix_fetch'\'' 2>/tmp/fetch.err && grep -q \"response 403\" /tmp/fetch.err'"
(cd "$al" && quiet "$D" rm --force --no-export)

# Ubuntu's image has empty package lists, where apt 3.2 needs a second update to reach the snapshot.
check "base builds from ubuntu:26.04" nd quiet "$D" build --from ubuntu:26.04 base
check "none of them needed Docker" bash -c "! grep -v '^docker image inspect ' '$work/docker-calls'"
check "and the build VM and its temporary folders are gone" bash -c "
  ! limactl list -q 2>/dev/null | grep -qx devvm-oci-build && ! ls -d \"\${TMPDIR:-/tmp}\"/devvm-oci.* 2>/dev/null"
# An image only Docker has (built, never pushed) isn't checked: making one would change Docker's images.

# Behind a company proxy with a password (corp_start), a --from conversion (an Alpine release no
# other check uses, so it's converted each run) and the builds on it go through a proxy of devvm's
# for each build, which alone goes through the company proxy: the registry, base's and its
# features' downloads and a layer's own all reach it, while nothing a build runs sees its password,
# and no converted disk, image or log keeps it. The layer prints what its commands, and sudo's,
# have of the proxy settings; the password is read from its file, never written into a check.
corp_start
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\n%s\n' \
  'echo "e2e-env: $(env | grep -i "_proxy=" | sort | tr "\n" " ")"' \
  'echo "e2e-sudo: $(sudo env | grep -i "_proxy=" | sort | tr "\n" " ")"' \
  'curl -fsS -m 30 -o /dev/null https://github.com' > "$layers/e2ecorp.sh"
check "behind a company proxy, --from's conversion and the builds on it reach it through devvm's own proxies" bash -c "
  HTTPS_PROXY='$corpurl' HTTP_PROXY='$corpurl' PATH='$work/nodocker':\"\$PATH\" '$D' build --from alpine:3.21 e2ecorp \
    </dev/null >'$work/last.log' 2>&1 &&
  grep -q '\"event\":\"tunnel\",\"host\":\"[a-z0-9.-]*docker\.io\"' '$work/corp.jsonl' &&
  grep -q '\"host\":\"dl-cdn\.alpinelinux\.org\"' '$work/corp.jsonl' &&
  grep -q '\"event\":\"tunnel\",\"host\":\"github\.com\"' '$work/corp.jsonl'"
owned e2ecorp
kac=$(sed -n 's/^chain=base=\([0-9a-f]*\) e2ecorp=.*$/\1/p' "$images"/* 2>/dev/null | head -1)
[[ -z $kac ]] || created+=("devvm-img-$kac")
kcl=$(keys_of e2ecorp | head -1)
check "what the builds' commands and sudo have is devvm's proxy for the build, with a password of its own" bash -c "
  log=\$(grep -l '^e2e-env: ' '$XDG_DATA_HOME'/devvm/logs/runs/*.log | tail -1) && [[ -n \$log ]] &&
  grep -qE '^e2e-env: .*HTTPS_PROXY=http://devvm:[0-9a-f]{64}@192\.168\.5\.2:[0-9]+ ' \"\$log\" &&
  grep -qE '^e2e-sudo: .*HTTPS_PROXY=http://devvm:[0-9a-f]{64}@192\.168\.5\.2:[0-9]+ ' \"\$log\" &&
  ! grep -qF ':18766' \"\$log\""
check "the company proxy's password is in no converted disk, image, log, build file or process's arguments" bash -c "
  [[ -n '$kac' && -n '$kcl' && -d '$LIMA_HOME/devvm-img-$kac' && -d '$LIMA_HOME/devvm-img-$kcl' ]] &&
  ! LC_ALL=C grep -rqaF -D skip -f '$work/corp.auth' '$LIMA_HOME/devvm-img-$kac' '$LIMA_HOME/devvm-img-$kcl' \
    '$XDG_DATA_HOME/devvm/logs' '$XDG_DATA_HOME/devvm/run' &&
  ! ps -axww -o command= | grep -qFf '$work/corp.auth' &&
  [[ -z \$(ls -A '$XDG_DATA_HOME/devvm/run/builds' 2>/dev/null) ]] &&
  ! ls -d \"\${TMPDIR:-/tmp}\"/devvm-oci.* 2>/dev/null"
corp_stop
rm -f "$layers/e2ecorp.sh"
fi

# After from, whose Alpine image a link's client starts from in the --from tier.
if want link; then
section "links between VMs"
link_section lima lk
fi

if want smolvm; then
# The smolvm backend (experimental): its native features, and a one-line refusal for the rest. Its
# machines share your smolvm on macOS: only those with the sandbox's label are looked at, and the
# others must be the same at the end.
S=(--backend smolvm)
me=$(id -un)
others_before=$(smol_others)
# in_smol VM CMD: CMD in a login shell as the VM's user, as devvm's shell has it.
in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -u "$me" -e "HOME=/home/$me.guest" -e "USER=$me" -- bash -lc "$2" </dev/null; }
# smol_home DIR: a home of a check's own that still has your smolvm's machines: on macOS smolvm keeps
# them under HOME, so its folders there are links to yours (rm -rf of DIR removes only the links).
smol_home() { mkdir -p "$1/Library/Application Support" "$1/Library/Caches"
  [[ "$(uname -s)" != Darwin ]] || for d in "Library/Application Support/smolvm" "Library/Caches/smolvm"; do
    [[ -e "$1/$d" ]] || ln -s "$HOME/$d" "$1/$d"; done; }
# smol_pid VM: the host process that runs it (the first line of its agent.pid).
smol_pid() { head -1 "$(SMOLVM_USAGE=0 smolvm machine data-dir --name "$1")/agent.pid" 2>/dev/null; }
export -f smol_pid
# smol_nameserver VM: the resolver the VM's resolv.conf names. smolvm's own network without a port
# forward is TSI, where the VM asks this machine's resolver itself; any forward, or an allow list, is
# virtio-net, where it asks smolvm's (the gateway, 100.96.0.1). design.md's DNS detection rests on it.
smol_nameserver() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf </dev/null; }
# smol_tsi VM: it asks one of this machine's resolvers (not the gateway). The first can be one the VM
# can't use (an IPv6 link-local address, scoped to this machine's interface): smolvm skips it.
smol_tsi() { local ns host; ns=$(smol_nameserver "$1"); host=$(awk '$1 == "nameserver" { print $2 }' /etc/resolv.conf)
  echo "VM's: ${ns:-none}, this machine's: $(paste -sd ' ' - <<<"${host:-none}")"
  [[ -n $ns && $ns != 100.96.0.1 ]] && grep -qxF -- "$ns" <<<"$host"; }
smol_vnet() { local ns; ns=$(smol_nameserver "$1"); echo "VM's: ${ns:-none}"; [[ $ns == 100.96.0.1 ]]; }
# smol_listens_only VM PORT...: the host process that runs it listens on each PORT (the control:
# what lsof sees), and on no other TCP port.
smol_listens_only() { local vm=$1 pid l p re=""; shift
  pid=$(smol_pid "$vm"); [[ $pid =~ ^[0-9]+$ ]] || { echo "no pid"; return 1; }
  l=$(lsof -nP -a -p "$pid" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u)
  echo "listening: ${l:-nothing}; want $*"
  for p in "$@"; do [[ $p =~ ^[0-9]+$ ]] && grep -q ":$p\$" <<<"$l" || return 1; re+="${re:+|}:$p\$"; done
  ! grep -qvE "$re" <<<"$l"; }
section "smolvm: shared tool versions"
check "the shared tools resolve to upstream's newest" quiet "$D" _refresh-tools

section "smolvm: images"
check "devvm doctor finds everything smolvm needs" bash -c "'$D' doctor --backend smolvm </dev/null >'$work/last.log' 2>&1 &&
  '$D' doctor --backend smolvm --json </dev/null | grep -qxF '  \"problems\": [],'"
check "build base" quiet env DEVVM_MESSAGES=names "$D" "${S[@]}" build base
kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
# A cached base: the pack recorded as base, not --from another image (the Alpine checks' bases).
[[ -n $kb ]] || kb=$(for k in "$XDG_DATA_HOME"/devvm/smolvm/images/*; do
  k=$(basename "$k"); grep -qx name=base "$images/$k" 2>/dev/null && grep -qx from= "$images/$k" && echo "$k"; done | head -1)
packs=$XDG_DATA_HOME/devvm/smolvm/images
check "base is a pack, recorded with Ubuntu's container image by its pinned digest; its build machine is gone" bash -c "
  [[ -n '$kb' && -n '$ubuntu_pin' && -f '$packs/$kb/image.smolmachine' ]] && grep -qx libc=gnu '$images/$kb' &&
  grep -qE '^digest=docker\.io/library/ubuntu:[0-9.]+@$ubuntu_pin$' '$images/$kb' && [[ -z \$(smol_status devvm-img-$kb) ]]"
fixture=$layers/e2esmol.sh
token=e2e-smolvm-ran-$$-$(date +%s)
printf '#!/bin/bash\necho %s\necho fixture | sudo tee /etc/e2e-fixture\n' "$token" > "$fixture"
check "a layer builds from base's pack, its script run as the VM's user" bash -c "'$D' ${S[*]} build e2esmol </dev/null >'$work/last.log' 2>&1 &&
  grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
owned e2esmol
kf=$(keys_of e2esmol | head -1)
check "it's a pack too, and devvm images lists both on smolvm" bash -c "[[ -n '$kf' && -f '$packs/$kf/image.smolmachine' ]] &&
  out=\$('$D' images </dev/null 2>/dev/null) && grep -qE '^base +ubuntu [0-9.]+ +[0-9a-f]{12} +$kb +current +[0-9.]+[KMGT] +smolvm$' <<<\"\$out\" &&
  grep -qE '^e2esmol +ubuntu [0-9.]+ +[0-9a-f]{12} +$kf +current +[0-9.]+[KMGT] +smolvm$' <<<\"\$out\""
echo "# edited" >> "$fixture"
check "rebuild makes a new one and deletes the stale one, not base" bash -c "'$D' ${S[*]} rebuild --force e2esmol </dev/null >/dev/null 2>&1 &&
  [[ \$(keys_of e2esmol | wc -l) -eq 1 && \$(keys_of e2esmol) != $kf && ! -e '$packs/$kf' && -f '$packs/$kb/image.smolmachine' ]]"
owned e2esmol
check "images rm deletes it" bash -c "'$D' ${S[*]} images rm --force e2esmol </dev/null >/dev/null 2>&1 && [[ -z \$(keys_of e2esmol) ]]"
rm -f "$fixture"
# The user's CAs: trusted while a build's scripts run, and removed before the machine is packed (the
# run log shows ca.sh ran after the script).
e2e_ca
mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
catoken=e2e-smol-ca-$$-$(date +%s)
printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
  '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt ]]' \
  'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /etc/ssl/devvm.pem' "$catoken" > "$layers/e2esmolca.sh"
check "a build trusts the CAs in devvm's CA folder while its scripts run, then takes them out" bash -c "
  DEVVM_MESSAGES=names '$D' ${S[*]} build e2esmolca </dev/null >'$work/last.log' 2>&1 &&
  f=\$(grep -lx '$catoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log | tail -1) && [[ -n \$f ]] &&
  [[ \$(grep -oE 'running_script script=(ca|e2esmolca)\.sh' \"\$f\" | sed 's/.*=//' | paste -sd' ' -) == 'ca.sh e2esmolca.sh ca.sh' ]]"
owned e2esmolca
rm -rf "$cafolder" "$layers/e2esmolca.sh"
printf '#!/bin/bash\nexit 3\n' > "$layers/e2esmolfail.sh"
DEVVM_MESSAGES=names "$D" "${S[@]}" build e2esmolfail </dev/null >"$work/fail.log" 2>&1; rc=$?
kx=$(sed -n 's/^==> building_image image=e2esmolfail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
[[ -z $kx ]] || created+=("devvm-img-$kx")
check "a failed script keeps its machine, stopped, and says how to look inside" bash -c "
  [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: smolvm_script_failed script=e2esmolfail\.sh vm=devvm-img-$kx$' '$work/fail.log' &&
  [[ \$(smol_status devvm-img-$kx) == stopped ]] && '$D' ${S[*]} images rm --force '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(smol_status devvm-img-$kx) ]]"
rm -f "$layers/e2esmolfail.sh"

section "smolvm: a base project"
p=$(newproj smolweb); git -C "$p" init -q
launch "$p" 'pwd; id -un; echo e2e-shell-ran' "${S[@]}" --no-network-policy --ports 47180,47184 --cpus 2 --memory 3 --disk 120 base
check "devvm creates the VM from base's pack, labelled, and opens a shell in the folder as you" bash -c "
  [[ -n '$vm' && \$(smol_status '$vm') == running ]] && grep -qx '$p' '$work/last.log' && grep -qx '$me' '$work/last.log' &&
  grep -qx e2e-shell-ran '$work/last.log' && grep -qx backend=smolvm '$vms/$vm' && grep -qx net=off '$vms/$vm'"
check "it gets --cpus, --memory and --disk" in_smol "$vm" \
  '[[ $(nproc) == 2 ]] && (( $(awk "/^MemTotal:/ { print int(\$2 / 1048576 + 0.5) }" /proc/meminfo) == 3 )) && (( $(df -BG --output=size / | tail -1 | tr -dc 0-9) >= 115 ))'
check "ls lists it on smolvm" bash -c "'$D' ls 2>/dev/null | grep -Eq '^$vm +Running +base +\S+ +off +- +smolvm +$p\$'"
check "the folder is mounted read-write, and what the VM writes is yours" bash -c "
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- bash -c \"\$1\" </dev/null; }
  in_smol 'echo from-vm > $p/vm.txt' && grep -qx from-vm '$p/vm.txt' && [[ \$(stat -f %Su '$p/vm.txt' 2>/dev/null || stat -c %U '$p/vm.txt') == '$me' ]]"
# smolvm passes on this machine's changes and new files, but no deletions: it watches the folder here
# and fires each change on the file in the VM. A watch on the file sees a change to it, and a watch on
# its folder sees the folder change, with no name: only a change macOS still reports with the file's
# creation, a few seconds after it, comes as a new file there. Each event reaches the VM in about
# 0.1 s, 0.25 s at most in 30 tries at a load of 19, so 5 s is ample.
check "a watcher in the VM sees files changed and made on this machine, but not deleted" watch_sees in_smol "$vm" "$p" host yes no yes
check "the shared tools are read-only, even for root" bash -c "! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- touch /mnt/devvm-tools/x </dev/null 2>/dev/null"
check "hostname is the folder" in_smol "$vm" '[[ $(hostname) == smolweb ]]'
check "Exec's arguments, folder, input and user reach the VM as given" bash -c "cd '$root' &&
  DEVVM_E2E_VM=$vm CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2EExecQuoting\$' ./internal/backend/smolvm |
  grep -q -- '--- PASS: TestE2EExecQuoting'"
# Its desktop (base has the desktop feature), with no init system: devvm desktop starts it.
desktop_serve "$p"
check "devvm desktop serves its desktop on this machine's 127.0.0.1 alone" bash -c "[[ -n '$desk_port' ]] && loopback_only '$desk_port'"
check "a viewer with a wrong password is refused, one with the password gets the VM's desktop" desktop_view
check "the VM's server listens on its user's own socket, on no TCP port, the clipboard off" in_smol "$vm" "$desktop_in_vm"
check "the password is in no file of the VM's nor any run log" bash -c '
  printf "%s\n" "$1" | SMOLVM_USAGE=0 smolvm machine exec --name "$2" -i -- sh -c "$3" && ! grep -rlF "$1" "$4"' \
  _ "$desk_pw" "$vm" "$desktop_secret" "$XDG_DATA_HOME/devvm/logs"
desktop_end
check "Ctrl+C ends devvm desktop, its port closed" bash -c "[[ -n '$desk_port' ]] && ! nc -z 127.0.0.1 '$desk_port'"
check "without the docker layer, it has no Docker" in_smol "$vm" '! command -v dockerd && ! command -v docker && [[ ! -e /run/docker.pid ]]'
check "SSH agent forwarding is off: the agent devvm's commands get, if any, reaches nothing" in_smol "$vm" \
  '[[ -z ${SSH_AUTH_SOCK:-} ]] || ssh-add -l 2>&1 | grep -qi "communication with agent failed\|error connecting to agent"'
# A listener on every address (Ubuntu's container image has perl, not python), left running.
SMOLVM_USAGE=0 smolvm machine exec --name "$vm" -d -- perl -MIO::Socket::INET -e \
  '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:47180", Listen => 5, ReuseAddr => 1) or die; while ($c = $s->accept) { close $c }' \
  </dev/null >/dev/null 2>&1
check "the listed port reaches this machine's loopback" bash -c 'for i in $(seq 20); do nc -z 127.0.0.1 47180 && exit 0; sleep 1; done; exit 1'
# Listeners on the VM's loopback alone: a listed port (47184), and one not listed (47185).
for lp in 47184 47185; do SMOLVM_USAGE=0 smolvm machine exec --name "$vm" -d -- perl -MIO::Socket::INET -e \
  '$p = shift; $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:$p", Listen => 5, ReuseAddr => 1) or die; while ($c = $s->accept) { print $c "vm-$p\n"; close $c }' "$lp" \
  </dev/null >/dev/null 2>&1; done
check "a listed port's server on the VM's loopback alone reaches this machine's, through devvm's forwarder" bash -c '
  for i in $(seq 20); do [[ $(nc -w 2 127.0.0.1 47184 </dev/null 2>/dev/null) == vm-47184 ]] && exit 0; sleep 1; done; exit 1'
check "an unlisted port's doesn't" bash -c 'sleep 3; ! nc -z 127.0.0.1 47185'
check "made ready to turn agent forwarding on later, and given none: smolvm's socket reaches no agent" bash -c "
  grep -qx agent_toggle=on '$vms/$vm' && grep -qx ssh_agent=off '$vms/$vm' &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- sh -c 'SSH_AUTH_SOCK=/tmp/ssh-agent.sock ssh-add -l' </dev/null 2>&1 |
  grep -qi 'communication with agent failed\|error connecting to agent'"
check "smolvm publishes none of its ports: its process listens on its ssh forward alone" smol_listens_only "$vm" "$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json")"
check "... and devvm's forwarder forwards the listed ones alone" bash -c "ps -axww -o command= | grep -- '[_]smolvm-ports -vm $vm ' | grep -q -- ' -only 47180,47184\$'"
# smol_serve VM PORTS...: a server in the VM on each port, every address, left running.
smol_serve() { local p; for p in "${@:2}"; do SMOLVM_USAGE=0 smolvm machine exec --name "$1" -d -- perl -MIO::Socket::INET -e \
  '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:" . shift, Listen => 5, ReuseAddr => 1) or die; while ($c = $s->accept) { close $c }' "$p" \
  </dev/null >/dev/null 2>&1; done; }
check "a port added to a running VM reaches this machine at once, and once removed doesn't, the VM never restarted" ports_live "$p" "$vm" 47186 smol_serve smol_pid
check "a running VM's new size waits for its next start, and applies on restart; its new ports at once" bash -c "cd '$p' &&
  '$D' size --cpus 3 </dev/null >/dev/null 2>&1 && '$D' ports add 47181 </dev/null >/dev/null 2>&1 &&
  ps -axww -o command= | grep -- '[_]smolvm-ports -vm $vm ' | grep -q -- ' -only 47180,47184,47181\$' &&
  printf 'nproc\n' | '$D' restart >'$work/last.log' 2>&1 && grep -qx 3 '$work/last.log' &&
  [[ \$('$D' ports 2>/dev/null) == 47180,47184,47181 ]]"
check "a listed port a program on this machine has, on every address, stays its own until it's freed" host_port_kept "$p" "$vm" 47181 47180 smol_serve
check "a smaller disk is refused" bash -c "cd '$p' && ! DEVVM_MESSAGES=names '$D' size --disk 100 </dev/null >'$work/last.log' 2>&1 &&
  grep -q 'disk_shrink' '$work/last.log'"
check "devvm stop stops it, and devvm starts it again" bash -c "cd '$p' && '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ \$(smol_status $vm) == stopped ]] && printf 'echo again\n' | '$D' 2>&1 | grep -qx again && [[ \$(smol_status $vm) == running ]]"
check "rm deletes only this VM, with its state" bash -c "cd '$p' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status $vm) && ! -e '$vms/$vm' && ! -e '$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json' ]]"

section "smolvm: SSH agent forwarding"
# Turning it on or off later is emulated (agent-toggle), and reviewed. Its guarantee: a VM set to
# devvm's agent or none never reaches yours, nor one set to none any agent: nothing listed, nothing
# added, not even by root; one started outside devvm is stopped. The agents and keys are e2e's own:
# a throwaway one as yours (ua_sock), and devvm's own in the sandbox with a throwaway key.
ua_sock=$work/smol-agent.sock da_sock=$XDG_DATA_HOME/devvm/agent.sock
rm -f "$ua_sock" "$work/smol-key" "$work/smol-key.pub" "$work/smol-dkey" "$work/smol-dkey.pub"
user_agent_pid=$(ssh-agent -a "$ua_sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
ssh-keygen -q -t ed25519 -N '' -C e2e-smolvm-user -f "$work/smol-key"
ssh-keygen -q -t ed25519 -N '' -C e2e-smolvm-devvm -f "$work/smol-dkey"
SSH_AUTH_SOCK=$ua_sock ssh-add -q "$work/smol-key" 2>/dev/null
# as_ua [VAR=VALUE...] CMD...: CMD with your (throwaway) agent, as devvm runs from your shell.
as_ua() { env SSH_AUTH_SOCK="$ua_sock" "$@"; }
# smol_keys VM: what the VM's agent lists, asked as root through smolvm's agent socket in the VM. Each
# exec runs from a shell with your agent: smolvm reads it only at a start, so it reaches no VM.
smol_keys() { SSH_AUTH_SOCK=$ua_sock SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c 'SSH_AUTH_SOCK=/tmp/ssh-agent.sock ssh-add -l' </dev/null 2>&1; }
# smol_add VM: root in the VM adds a key it made to the agent it's given (e2e-from-vm).
smol_add() { SSH_AUTH_SOCK=$ua_sock SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c 'rm -f /tmp/vmkey /tmp/vmkey.pub &&
  ssh-keygen -q -t ed25519 -N "" -C e2e-from-vm -f /tmp/vmkey && SSH_AUTH_SOCK=/tmp/ssh-agent.sock ssh-add /tmp/vmkey' </dev/null 2>&1; }
# agents_clean: your agent holds only its key, and devvm's (if it runs) only its own: nothing of the VM's.
agents_clean() { local u d; u=$(SSH_AUTH_SOCK=$ua_sock ssh-add -l 2>&1); d=$(SSH_AUTH_SOCK=$da_sock ssh-add -l 2>&1)
  echo "yours: $u; devvm's: $d"; [[ $(grep -c . <<<"$u") -eq 1 ]] && grep -q e2e-smolvm-user <<<"$u" && ! grep -q e2e-from-vm <<<"$d"; }
# smol_only VM COMMENT: the VM's agent lists that key alone.
smol_only() { local k; k=$(smol_keys "$1"); echo "VM's agent: $k"; [[ $(grep -c . <<<"$k") -eq 1 ]] && grep -q "$2" <<<"$k"; }
# smol_no_agent VM: the VM has smolvm's agent socket (made forwarding), which reaches no agent at all
# (not an empty one): nothing answers, none added, root asking; your agent and devvm's are untouched.
smol_no_agent() { local k a
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- test -S /tmp/ssh-agent.sock </dev/null || { echo "no agent socket in the VM"; return 1; }
  k=$(smol_keys "$1"); echo "VM's agent: $k"
  grep -qiE 'communication with agent failed|error connecting to agent|could not open a connection' <<<"$k" &&
    ! grep -qiE 'no identities|SHA256:' <<<"$k" || return 1
  a=$(smol_add "$1") && { echo "added: $a"; return 1; }
  echo "adding: $a"; agents_clean; }
# smol_switch DIR MODE CHECK...: devvm ssh-agent MODE, then devvm restart, from your shell; then CHECK.
smol_switch() { (cd "$1" && as_ua "$D" ssh-agent "$2" </dev/null >"$work/last.log" 2>&1 &&
  printf 'exit\n' | as_ua "$D" restart >>"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  shift 2; "$@"; }
# smol_vm_adds VM: root in the VM adds a key of its own to the agent it's given, your agent here (the
# add check's control); the key is taken out again.
smol_vm_adds() { local pub
  smol_add "$1" >/dev/null && SSH_AUTH_SOCK=$ua_sock ssh-add -l | grep -q e2e-from-vm || return 1
  pub=$(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- cat /tmp/vmkey.pub </dev/null) && printf '%s\n' "$pub" > "$work/vmkey.pub" &&
  SSH_AUTH_SOCK=$ua_sock ssh-add -d "$work/vmkey.pub" >/dev/null 2>&1; rm -f "$work/vmkey.pub"; agents_clean; }
# smol_name_keys VM: what ssh by name lists of the agent its session has, ssh run with yours, then
# ssh-add's exit code (rc=2: no agent).
smol_name_keys() { SSH_AUTH_SOCK=$ua_sock ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -o BatchMode=yes "$1" 'ssh-add -l 2>&1; echo "rc=$?"' </dev/null 2>&1; }
export -f smol_keys smol_add smol_only smol_no_agent smol_name_keys agents_clean as_ua; export ua_sock da_sock

g=$(newproj smolagent)
(cd "$g" && printf 'ssh-add -l\n' | as_ua DEVVM_MESSAGES=names "$D" "${S[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "a new VM is made ready to change it, unwarned, and recorded so" bash -c "[[ -n '$vm' ]] &&
  ! grep -q 'backend_unreviewed' '$work/last.log' &&
  grep -qx agent_toggle=on '$vms/$vm' && grep -qx ssh_agent=off '$vms/$vm'"
check "off: started from your shell, with your agent, the VM reaches no agent, and can't add to one" smol_no_agent "$vm"
check "devvm's own shell has no agent's keys either" bash -c "! grep -q 'e2e-smolvm' '$work/last.log'"
check "ssh-agent add loads a throwaway key into devvm's own agent" bash -c "
  as_ua '$D' ssh-agent add '$work/smol-dkey' </dev/null 2>&1 | grep -qF \"\$(m agent_holds keys=e2e-smolvm-devvm)\""
check "ssh-agent devvm while it runs: nothing changes until its next start" bash -c "cd '$g' &&
  as_ua DEVVM_MESSAGES=names '$D' ssh-agent devvm </dev/null >'$work/e' 2>&1 && grep -q 'agent_set_running mode=devvm vm=$vm\$' '$work/e'"
check "  still no agent" smol_no_agent "$vm"
check "  after a launch, ssh by name has none either, until a restart" bash -c "cd '$g' && printf 'exit\n' | as_ua '$D' >'$work/last.log' 2>&1 &&
  out=\$(smol_name_keys '$vm'); echo \"\$out\"; grep -qx 'rc=2' <<<\"\$out\" && ! grep -q SHA256: <<<\"\$out\""
check "devvm, after a restart: the VM uses devvm's key alone, never yours" smol_switch "$g" devvm smol_only "$vm" e2e-smolvm-devvm
check "  so does ssh by name" bash -c "out=\$(smol_name_keys '$vm'); echo \"\$out\"; grep -qx 'rc=0' <<<\"\$out\" &&
  [[ \$(grep -c SHA256: <<<\"\$out\") -eq 1 ]] && grep -q e2e-smolvm-devvm <<<\"\$out\""
check "  with devvm's agent ended while it runs: no agent, never yours" bash -c "pkill -f '[s]sh-agent -a $da_sock';
  for i in \$(seq 20); do [[ ! -S '$da_sock' ]] && break; sleep 0.25; done; smol_no_agent '$vm'"
check "on, after a restart: your agent's key alone, never devvm's" smol_switch "$g" on smol_only "$vm" e2e-smolvm-user
check "  root in the VM can add a key to it then (the add check's control)" smol_vm_adds "$vm"
check "off while it runs, stopped, then started by devvm history export (no launch, its record still forwarding yours): no agent" bash -c "cd '$g' &&
  as_ua '$D' ssh-agent off </dev/null >/dev/null 2>&1 && as_ua '$D' stop </dev/null >/dev/null 2>&1 &&
  ! grep -q agent_off '$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json' &&
  { as_ua '$D' history export '$work/smol-export.tar.gz' </dev/null >'$work/last.log' 2>&1; rm -f '$work/smol-export.tar.gz'; true; } &&
  [[ \$(smol_status '$vm') == running ]] && smol_no_agent '$vm'"
check "off, after a restart: no agent again, your agent and devvm's untouched" smol_switch "$g" off smol_no_agent "$vm"
check "stopped, then smolvm machine start from your shell: the VM, set to off, gets your agent" bash -c "cd '$g' &&
  as_ua '$D' stop </dev/null >/dev/null 2>&1 &&
  SSH_AUTH_SOCK='$ua_sock' SMOLVM_USAGE=0 smolvm machine start --name '$vm' </dev/null >/dev/null 2>&1 && smol_only '$vm' e2e-smolvm-user"
check "  devvm history export stops it, in one line, before anything runs in it" bash -c "cd '$g' &&
  ! as_ua DEVVM_MESSAGES=names '$D' history export '$work/smol-export.tar.gz' </dev/null 2>'$work/e' && ! test -e '$work/smol-export.tar.gz' &&
  grep -qx 'devvm: agent_unchosen_stop vm=$vm' '$work/e' && [[ \$(smol_status '$vm') == stopped ]]"
check "  so does devvm, started that way again; then devvm starts it with none" bash -c "cd '$g' &&
  SSH_AUTH_SOCK='$ua_sock' SMOLVM_USAGE=0 smolvm machine start --name '$vm' </dev/null >/dev/null 2>&1 &&
  ! as_ua DEVVM_MESSAGES=names '$D' </dev/null 2>'$work/e' && grep -qx 'devvm: agent_unchosen_stop vm=$vm' '$work/e' &&
  [[ \$(smol_status '$vm') == stopped ]] && printf 'exit\n' | as_ua '$D' >/dev/null 2>&1 && smol_no_agent '$vm'"
check "  devvm host on its own start keeps it running" bash -c "cd '$g' &&
  as_ua DEVVM_MESSAGES=names '$D' host </dev/null >'$work/e' 2>&1 && ! grep -q agent_unchosen '$work/e' && [[ \$(smol_status '$vm') == running ]]"
(cd "$g" && quiet as_ua "$D" rm --force --no-export)
g=$(newproj smolfwd)
(cd "$g" && printf 'ssh-add -l\n' | as_ua "$D" "${S[@]}" --no-network-policy --ports none --ssh-agent base >"$work/last.log" 2>&1)
vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
check "--ssh-agent: the VM uses the agent it was started with" bash -c "[[ -n '$vm' ]] && grep -q e2e-smolvm-user '$work/last.log'"
check "with its ssh forward, even with no ports of its own, smolvm's own network is virtio-net: the gateway's resolver" smol_vnet "$vm"
check "turned off, after a restart: no agent" smol_switch "$g" off smol_no_agent "$vm"
(cd "$g" && quiet as_ua "$D" rm --force --no-export)
kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
pkill -f "[s]sh-agent -a $da_sock"
rm -f "$work/smol-key" "$work/smol-key.pub" "$work/smol-dkey" "$work/smol-dkey.pub" "$ua_sock" "$XDG_CONFIG_HOME/devvm/agent-keys"

section "smolvm: ssh for editors"
# The emulation's guarantee (reviewed): each VM's own host keys, pinned; the VM's own key the only
# login, as the VM's user; no forwarding that carries the VM's connections out through this machine
# that devvm's entry lets through; agent forwarding as the entry says.
# An ~/.ssh/config of the section's own, set up as the docs say, with a Host * that forwards the agent:
# yours is never read or changed; ssh gets no agent but a throwaway one; a stub code.
UR=(env)   # an unreviewed emulation would add DEVVM_UNREVIEWED=backend:feature here
sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc/home" "$sc/bin"; smol_home "$sc/home"
dsc=$XDG_CONFIG_HOME/devvm/ssh_config kh=$XDG_CONFIG_HOME/devvm/known_hosts
printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$dsc" > "$sc/config"
agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
printf '#!/bin/sh\necho "$*" > %s/code-args\n' "$sc" > "$sc/bin/code"; chmod +x "$sc/bin/code"
# sshs ARGS...: ssh through the section's config, without an agent. smolssh_port VM: the port its
# entry names. smolssh_key VM: its pinned host key.
sshs() { env -u SSH_AUTH_SOCK ssh -F "$sc/config" "$@" </dev/null; }
smolssh_port() { awk -v h="$1" '$1 == "Host" { on = 0; for (i = 2; i <= NF; i++) if ($i == h) on = 1 } on && $1 == "Port" { print $2; exit }' "$dsc"; }
smolssh_key() { awk -v n="$1" '$1 == n { print $2, $3 }' "$kh"; }
# smolssh_loopback PORT: something listens on it, on the host's loopback alone.
smolssh_loopback() { local l; l=$(lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u)
  echo "$l"; [[ -n $l ]] && ! grep -vxE "127\.0\.0\.1:$1|\[::1\]:$1" <<<"$l"; }
# smolssh_pinned VM: the pinned key is the VM's own, its only one, ed25519.
smolssh_pinned() { local vmk
  vmk=$(in_smol "$1" 'cat /etc/ssh/ssh_host_*_key.pub' | awk '{ print $1, $2 }')
  echo "the VM's: $vmk"; echo "pinned: $(smolssh_key "$1")"
  [[ -n $vmk && $vmk == "$(smolssh_key "$1")" && $vmk == "ssh-ed25519 "* ]]; }
# smolssh_sshd VM: sshd's settings as it runs them (sshd -T on its config), as decided.
smolssh_sshd() { local t
  t=$(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- /usr/sbin/sshd -T -f /etc/devvm/ssh/sshd_config </dev/null) || return 1
  echo "$t" | grep -E '^(listenaddress|allowusers|permitrootlogin|authenticationmethods|passwordauthentication|kbdinteractiveauthentication|allowtcpforwarding|allowstreamlocalforwarding|permitlisten|gatewayports|x11forwarding|permittunnel|permituserenvironment|allowagentforwarding) '
  for want in 'listenaddress 0.0.0.0:22' "allowusers $me" 'permitrootlogin no' 'authenticationmethods publickey' \
    'passwordauthentication no' 'kbdinteractiveauthentication no' 'allowtcpforwarding yes' 'allowstreamlocalforwarding local' \
    'permitlisten 127.77.*:*' 'gatewayports clientspecified' 'x11forwarding no' 'permittunnel no' 'permituserenvironment no' 'allowagentforwarding yes'; do
    grep -qxF "$want" <<<"$t" || { echo "not $want"; return 1; }
  done
  [[ $(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- stat -c '%U %a' /etc/devvm/ssh/sshd_config </dev/null) == 'root 600' ]]; }
# smolssh_nopass HOST: password and keyboard-interactive logins are refused, publickey alone offered.
smolssh_nopass() { local out r
  out=$(env -u SSH_AUTH_SOCK ssh -v -F "$sc/config" -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive \
    -o BatchMode=yes "$1" true </dev/null 2>&1); r=$?
  grep -E 'Authentications that can continue|denied' <<<"$out"; echo "rc=$r"
  (( r == 255 )) && grep -qE 'Authentications that can continue: publickey[[:space:]]*$' <<<"$out" &&
  ! grep -qE 'Authentications that can continue: .*(password|keyboard)' <<<"$out"; }
# smolssh_stopped VM DIR: devvm stop drops its entry, and nothing listens on its port after.
smolssh_stopped() { local p
  p=$(smolssh_port "$1"); [[ -n $p ]] || { echo "no port"; return 1; }
  (cd "$2" && "$D" stop </dev/null >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  ! grep -q "$1" "$dsc" || { echo "still in the ssh config"; return 1; }
  for _ in 1 2 3 4 5; do nc -z 127.0.0.1 "$p" 2>/dev/null || return 0; sleep 1; done
  echo "port $p still listening"; return 1; }
# smolssh_onlykey VM HOST: another key, root and another user are refused; devvm's logs in (the control).
smolssh_onlykey() {
  rm -f "$sc/other" "$sc/other.pub"; ssh-keygen -q -t ed25519 -N '' -f "$sc/other"
  ! env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile="$kh" -o HostKeyAlias="$1" -o StrictHostKeyChecking=yes \
    -o BatchMode=yes -o IdentitiesOnly=yes -i "$sc/other" -p "$(smolssh_port "$1")" "$me@127.0.0.1" true </dev/null &&
  ! sshs -o BatchMode=yes -l root "$2" true && ! sshs -o BatchMode=yes -l nobody "$2" true && sshs -o BatchMode=yes "$2" true; }
# smolssh_forwarding HOST: devvm's sshd binds remote forwards only on links' addresses (rfwd_links);
# a local forward, as editors use, works (the control).
smolssh_forwarding() {
  rfwd_links "$1" sshs &&
  [[ $(sshs -W 127.0.0.1:22 "$1" | head -c 4) == SSH- ]]; }
# smolssh_lowport VM HOST: the setup opened ports below 1024 to the VM's user (smolvm's kernel starts
# at 1024), so its sshd binds a link's port 80.
smolssh_lowport() { local p
  p=$(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- cat /proc/sys/net/ipv4/ip_unprivileged_port_start </dev/null)
  echo "ip_unprivileged_port_start: $p"
  [[ $p == 0 ]] && sshs -o ExitOnForwardFailure=yes -R 127.77.0.250:80:127.0.0.1:9 "$2" true; }
# smolssh_hostile VM HOST: root in the VM rewrites its sshd's config to allow remote forwarding, X11
# and tunnels (with xauth for X11), and restarts it. Through a config of the check's own whose Host *
# asks for X11 and to add keys to the agent, the session gets no display and the agent no key; the
# same asked on the command line gets both (the controls), and ssh -R now works (not covered: the
# entry can't refuse it). Then the agent is emptied; the next start restores sshd.
smolssh_hostile() { local hc=$sc/hostile-config d keys
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n' "$dsc" > "$hc"
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c 'apt-get update -q >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends xauth >/dev/null 2>&1 &&
    c=/etc/devvm/ssh/sshd_config && sed -i -e "s/^AllowTcpForwarding .*/AllowTcpForwarding yes/" -e "s/^AllowStreamLocalForwarding .*/AllowStreamLocalForwarding yes/" \
      -e "s/^X11Forwarding .*/X11Forwarding yes/" -e "s/^PermitTunnel .*/PermitTunnel yes/" -e "s/^PermitListen .*/PermitListen any/" "$c" &&
    kill "$(cat /run/devvm-sshd.pid)" && sleep 1 && /usr/sbin/sshd -f "$c" -E /var/log/devvm-sshd.log' </dev/null || { echo "no hostile sshd"; return 1; }
  sshh() { DISPLAY=:9 SSH_AUTH_SOCK=$sc/agent.sock ssh -F "$hc" -o BatchMode=yes "$@" </dev/null 2>/dev/null; }
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  d=$(sshh "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "through Host *: display $d, agent ${keys:-empty}"
  [[ $d == x=none && -z $keys ]] || return 1
  d=$(sshh -Y -o AddKeysToAgent=yes "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "asked on the command line: display $d, agent ${keys:-empty}"
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  [[ $d != x=none && -n $keys ]] || return 1
  sshh -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 "$2" true || { echo "ssh -R refused by the hostile sshd"; return 1; }; }
# smolssh_squatted VM DIR HOST: its recorded port, taken by another program while it's stopped: its
# next start forwards another, and ssh reaches it.
smolssh_squatted() { local p n sq
  p=$(smolssh_port "$1"); [[ -n $p ]] || { echo "no port"; return 1; }
  (cd "$2" && "$D" stop </dev/null >/dev/null 2>&1) || return 1
  perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:" . shift, Listen => 5, ReuseAddr => 1) or die; sleep 120' "$p" &
  sq=$!; sleep 0.5
  (cd "$2" && printf 'exit\n' | "${UR[@]}" "$D" >"$work/last.log" 2>&1); n=$(smolssh_port "$1")
  echo "squatted $p, now ${n:-none}"
  sshs -o BatchMode=yes "$3" true; local r=$?
  kill "$sq" 2>/dev/null; wait "$sq" 2>/dev/null
  [[ -n $n && $n != "$p" ]] && (( r == 0 )); }
# smolssh_nosshd VM DIR: root moves sshd away; devvm's next start warns vm_no_sshd, the VM runs
# with no entry, and devvm host says so; sshd put back, a start has ssh again (the control).
smolssh_nosshd() {
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- mv /usr/sbin/sshd /usr/sbin/sshd.away </dev/null || return 1
  (cd "$2" && printf 'exit\n' | "${UR[@]}" DEVVM_MESSAGES=names "$D" restart >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  if ! grep -qx "devvm: vm_no_sshd vm=$1" "$work/last.log" || [[ $(smol_status "$1") != running ]] || grep -q "$1" "$dsc"; then
    cat "$work/last.log"; return 1
  fi
  if (cd "$2" && "${UR[@]}" DEVVM_MESSAGES=names "$D" host </dev/null 2>"$work/e") || ! grep -qx "devvm: vm_no_sshd vm=$1" "$work/e"; then
    cat "$work/e"; return 1
  fi
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- mv /usr/sbin/sshd.away /usr/sbin/sshd </dev/null || return 1
  (cd "$2" && printf 'exit\n' | "${UR[@]}" "$D" restart >"$work/last.log" 2>&1) && grep -q "$1" "$dsc"; }
# smolssh_otherkey VM OTHER: OTHER's key doesn't log in to VM; VM's own does, the same way (the control).
smolssh_otherkey() {
  smolssh_with_key() { env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile="$kh" -o HostKeyAlias="$1" -o StrictHostKeyChecking=yes \
      -o BatchMode=yes -o IdentitiesOnly=yes -i "$XDG_DATA_HOME/devvm/smolvm/vms/$2.key" -p "$(smolssh_port "$1")" "$me@127.0.0.1" true </dev/null 2>/dev/null; }
  if smolssh_with_key "$1" "$2"; then echo "$2's key logged in to $1"; return 1; fi
  smolssh_with_key "$1" "$1" || { echo "its own key didn't"; return 1; }; }
q=$(newproj smolssh); git -C "$q" init -q
(cd "$q" && printf 'exit\n' | HOME=$sc/home "${UR[@]}" DEVVM_MESSAGES=names "$D" "${S[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm=$(vm_of "$q"); [[ -z $vm ]] || created+=("$vm")
check "the ssh config lists it by the folder's name, its sshd forwarded from the host's loopback" bash -c "
  [[ -n '$vm' ]] && ! grep -q 'backend_unreviewed' '$work/last.log' &&
  grep -q '^Host devvm-smolssh $vm\$' '$dsc'"
check "... on the host's loopback alone" smolssh_loopback "$(smolssh_port "$vm")"
check "with its ssh forward, smolvm's own network is virtio-net: the VM asks the gateway's resolver" smol_vnet "$vm"
check "its host key is pinned, as the VM has it: ed25519, its only one" smolssh_pinned "$vm"
check "ssh devvm-smolssh reaches it, as the VM's user, with the environment devvm's commands get" bash -c "
  out=\$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh 'id -un; hostname; echo \$BASH_ENV' </dev/null) &&
  [[ \$out == '$me'\$'\n'smolssh\$'\n'/etc/profile.d/devvm-tools.sh ]] || { echo \"\$out\"; false; }"
check "agent forwarding stays off, whatever Host * says" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -F '$sc/config' devvm-smolssh 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "... while ssh -A forwards it (the control)" bash -c "
  SSH_AUTH_SOCK='$sc/agent.sock' ssh -A -F '$sc/config' devvm-smolssh 'test -S \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
check "a changed host key is refused" bash -c "
  cp '$kh' '$sc/kh' && ssh-keygen -q -t ed25519 -N '' -f '$sc/changed' &&
  { grep -v '^$vm ' '$sc/kh' || true; echo \"$vm \$(cut -d' ' -f1,2 '$sc/changed.pub')\"; } > '$kh' &&
  { env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh true </dev/null 2>'$sc/err'; r=\$?; cp '$sc/kh' '$kh'; } &&
  (( r == 255 )) && grep -q 'Host key verification failed' '$sc/err'"
check "password and keyboard-interactive logins are refused: publickey alone is offered" smolssh_nopass devvm-smolssh
check "only devvm's key logs in, as the VM's user: another key, root and another user are refused" smolssh_onlykey "$vm" devvm-smolssh
check "devvm's sshd binds remote forwards only on 127.77.0.0/16 and refuses socket forwards; a local forward, as editors use, works" smolssh_forwarding devvm-smolssh
check "ports below 1024 are open to the VM's user after the setup, so a remote forward on 127.77.0.250:80 binds" smolssh_lowport "$vm" devvm-smolssh
check "sshd runs devvm's settings, and its config is root's alone" smolssh_sshd "$vm"
check "an sshd root made hostile gets no display and no key through a Host * asking for them; asked on the command line, both (the controls), and ssh -R" smolssh_hostile "$vm" devvm-smolssh
check "devvm host names it; devvm code opens it over Remote-SSH (a stub code)" bash -c "cd '$q' &&
  HOME='$sc/home' ${UR[*]} '$D' host </dev/null >'$work/last.log' 2>&1 &&
  grep -qF \"\$(m ssh_host_info dir=$q host=devvm-smolssh | head -1)\" '$work/last.log' &&
  HOME='$sc/home' PATH='$sc/bin':\$PATH ${UR[*]} '$D' code </dev/null >'$work/last.log' 2>&1 &&
  [[ \$(cat '$sc/code-args') == '--remote ssh-remote+devvm-smolssh $q' ]]"
k1=$(smolssh_key "$vm")
check "restart: new host keys, pinned again, and ssh reaches it" bash -c "cd '$q' &&
  printf 'exit\n' | ${UR[*]} '$D' restart >'$work/last.log' 2>&1 && [[ \$(smol_status '$vm') == running ]] &&
  k=\$(awk -v n='$vm' '\$1 == n { print \$2, \$3 }' '$kh') && [[ -n \$k && \$k != '$k1' ]] &&
  env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh true </dev/null"
q2=$(newproj smolssh2)
(cd "$q2" && printf 'exit\n' | "${UR[@]}" "$D" "${S[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm2=$(vm_of "$q2"); [[ -z $vm2 ]] || created+=("$vm2")
check "a VM made from the same image has host keys of its own, and a port of its own" bash -c "
  [[ -n '$vm2' ]] && a=\$(awk -v n='$vm' '\$1 == n { print \$3 }' '$kh') && b=\$(awk -v n='$vm2' '\$1 == n { print \$3 }' '$kh') &&
  [[ -n \$a && -n \$b && \$a != \$b ]] && env -u SSH_AUTH_SOCK ssh -F '$sc/config' devvm-smolssh2 true </dev/null"
check "... and a key of its own: its key doesn't log in to the other VM, the other's own does (the control)" smolssh_otherkey "$vm" "$vm2"
check "an image without sshd (root removed it): the next start runs without ssh, warned, and the VM runs" smolssh_nosshd "$vm2" "$q2"
(cd "$q2" && quiet "${UR[@]}" "$D" rm --force --no-export)
# Alpine's container image lacks OpenRC and ssh's client, which base's scripts write into.
qa=$(newproj smolalpine)
(cd "$qa" && printf 'exit\n' | HOME=$sc/home "${UR[@]}" "$D" "${S[@]}" --from alpine:3 --no-network-policy --ports none base >"$work/last.log" 2>&1)
vma=$(vm_of "$qa"); [[ -z $vma ]] || created+=("$vma")
check "a VM --from alpine:3 is made, and ssh devvm-smolalpine reaches it, on Alpine, as the VM's user" bash -c "
  [[ -n '$vma' ]] && grep -qx libc=musl '$vms/$vma' &&
  out=\$(env -u SSH_AUTH_SOCK ssh -F '$sc/config' -o BatchMode=yes devvm-smolalpine '. /etc/os-release; echo \$ID; id -un' </dev/null) &&
  [[ \$out == alpine\$'\n''$me' ]] || { echo \"\${out:-}\"; tail -5 '$work/last.log'; false; }"
(cd "$qa" && quiet "${UR[@]}" "$D" rm --force --no-export)
check "its port taken while it's stopped: its next start forwards another, and ssh reaches it" smolssh_squatted "$vm" "$q" devvm-smolssh
check "devvm stop drops its entry, and nothing listens on its port" smolssh_stopped "$vm" "$q"
check "rm deletes it, its entry and its pinned keys" bash -c "cd '$q' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') ]] && ! grep -q '$vm' '$dsc' && ! grep -q '^$vm ' '$kh'"
kill "$agent_pid" 2>/dev/null; agent_pid=""

section "smolvm: --clone"
# A home of its own: no ~/.ssh/config, so nothing of yours is read or offered the Include line. A
# throwaway repo, with a branch besides main and a file it doesn't track.
ch=$work/clone-home; rm -rf "$ch"; mkdir -p "$ch"; smol_home "$ch"
export GIT_CONFIG_GLOBAL=$ch/.gitconfig GIT_CONFIG_NOSYSTEM=1   # host git: none of your settings
c=$(newproj smolcloned)
git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
  git -C "$c" branch other && echo host-only > "$c/untracked"
(cd "$c" && printf 'pwd; git branch --show-current\n' | HOME=$ch "${UR[@]}" "$D" "${S[@]}" --clone --no-network-policy --ports none base >"$work/last.log" 2>&1)
vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
cgit="GIT_SSH_COMMAND='ssh -F $dsc' git -C '$c'"
# smolclone_seeded VM: its own clone, every branch pushed in over ssh, nothing of the folder mounted.
smolclone_seeded() {
  [[ -n $1 ]] && grep -qx mode=clone "$vms/$1" && grep -qx "$c" "$work/last.log" && grep -qx main "$work/last.log" &&
  [[ $(in_smol "$1" "git -C '$c' branch --format='%(refname:short)'" | tr '\n' ' ') == 'main other ' ]] &&
  in_smol "$1" "git -C '$c' log --oneline" | grep -q one && in_smol "$1" "[ ! -e '$c/untracked' ]"; }
check "the VM has its own clone, every branch pushed in over ssh, nothing of the folder mounted, and the shell opens in it" smolclone_seeded "$vm"
check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
in_smol "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
check "pushing a branch into it works too" bash -c "git -C '$c' branch pushed other && $cgit push -q devvm pushed &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- git -C '$c' rev-parse -q --verify refs/heads/pushed </dev/null >/dev/null"
check "rm --force deletes it and the remote fully; the fetched work stays as a local branch" bash -c "cd '$c' && HOME='$ch' '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status '$vm') && ! -e '$vms/$vm' ]] && ! git -C '$c' remote | grep -q devvm &&
  git -C '$c' log --oneline saved/devvm/main | grep -q from-vm"
unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM

section "smolvm: network policy"
# The emulation's guarantee (reviewed): a VM with network policy reaches nothing but its proxy,
# through devvm's decider; DNS it's refused never leaves this machine; with the proxy gone nothing
# gets out, and smolvm won't start it without a decider; its policy applies as on Lima. A docker VM,
# whose daemon goes through the proxy too: no agent, so no login is read. Your own HOME, as smolvm keeps its machines under it. U is what
# devvm runs with (an unreviewed emulation would add DEVVM_UNREVIEWED=backend:feature here). Its ssh
# is checked to open no way out.
U=(env)
# smol_root VM CMD: CMD as root in the VM. smol_block VM: the VM's object in smolvm's JSON.
smol_root() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- bash -c "$2" </dev/null; }
smol_block() { SMOLVM_USAGE=0 smolvm machine ls --json 2>/dev/null | awk -v n="\"name\": \"$1\"" '
  /^  \{/ { b = "" } { b = b $0 "\n" } /^  \}/ && index(b, n) { printf "%s", b }'; }
# smolnet_lists VM: its allow list is the gateway alone, its DNS a name that doesn't exist.
# smolnet_open VM: it has no allow list.
smolnet_lists() { local b; b=$(smol_block "$1" | tr -d ' \n')
  [[ $b == *'"allowed_cidrs":["100.96.0.1/32"]'* && $b == *'"allowed_hosts":["=devvm.invalid"]'* ]]; }
smolnet_open() { [[ -n $(smol_block "$1") ]] && ! smol_block "$1" | grep -q '"allowed_'; }
smolnet_closed() { smolnet_lists "$1" && smolnet_direct "$1"; }
# smolnet_direct VM: every direct connection fails, even as root: an address, a port of an allowed
# host, IPv6, and the host's loopback through the gateway (the listener, which answers 407).
smolnet_direct() {
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" &&
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null http://1.1.1.1" &&
  ! smol_root "$1" "curl -6 --noproxy '*' -sS -m 5 -o /dev/null https://[2606:4700:4700::1111]" &&
  ! smol_root "$1" "timeout 5 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22 && head -c 4 <&3' | grep -q SSH-" &&
  [[ $(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/") != 407 ]]
}
# The DNS probe, run as root in the VM with a name prefix of this run's ($1): its resolver, and
# queries straight to the gateway's resolver and 1.1.1.1 (UDP, and TCP to 1.1.1.1) for TXT, ANY,
# AAAA, HTTPS, an EDNS0 A, a name under devvm.invalid and DEVVM.INVALID; last, devvm.invalid
# itself, which smolvm sends to this machine's resolver: the detector's positive control.
dns_probe='n=$1
# pkt NAME QTYPE [edns]: a query, as printf escapes.
pkt() { local q="" l t
  for l in ${1//./ }; do q+=$(printf "\\\\x%02x" "${#l}")$l; done
  t=$(printf "\\\\x%02x\\\\x%02x" $(($2 >> 8)) $(($2 & 255)))
  if [[ -n ${3:-} ]]; then
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x01$q\\x00$t\\x00\\x01\\x00\\x00\\x29\\x10\\x00\\x00\\x00\\x00\\x00\\x00\\x00"
  else
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x00$q\\x00$t\\x00\\x01"
  fi; }
send() { local p len; p=$(pkt "$@")
  printf "$p" > /dev/udp/100.96.0.1/53; printf "$p" > /dev/udp/1.1.1.1/53
  len=$(printf "$p" | wc -c)
  { printf "\\x00\\x$(printf %02x "$len")"; printf "$p"; } | timeout 3 bash -c "cat > /dev/tcp/1.1.1.1/53"; }
getent hosts "$n-a.example.com" >/dev/null; echo "resolver rc=$?"
send "$n-b.example.com" 16; send "$n-c.example.com" 255; send "$n-d.example.com" 28; send "$n-e.example.com" 65
send "$n-f.example.com" 1 edns; send "x.$n.devvm.invalid" 1; send DEVVM.INVALID 16
getent hosts "$n-g.example.org" >/dev/null; echo "resolver rc=$?"
sleep 1; getent hosts devvm.invalid >/dev/null; echo "control rc=$?"'
printf '%s\n' "$dns_probe" > "$work/dns-probe.sh"
# The UDP and ICMP probe, as root: datagrams to the host's loopback listener through the gateway, to
# 1.1.1.1 on 443 and 123, an NTP request to time.cloudflare.com (162.159.200.123) and its reply,
# and a ping to 1.1.1.1.
udp_probe='for t in 100.96.0.1/18766 1.1.1.1/443 1.1.1.1/123; do printf devvm-e2e-udp > /dev/udp/$t; done
if exec 3<>/dev/udp/162.159.200.123/123; then
  { printf "\\x1b"; head -c 47 /dev/zero; } >&3
  echo "ntp bytes=$(timeout 3 head -c 48 <&3 | wc -c | tr -d " ")"
else echo "ntp bytes=0"; fi
perl -MNet::Ping -e "\$p = Net::Ping->new(q(icmp), 2); print \$p->ping(q(1.1.1.1)) ? qq(ping reply\n) : qq(ping none\n)"'
printf '%s\n' "$udp_probe" > "$work/udp-probe.sh"
# smolnet_probe VM SCRIPT ARG: SCRIPT as root in the VM, its output in $work/probe.log, while nettop
# (macOS) watches the VM's smolvm process (the first line of its agent.pid) on this machine's
# non-loopback interfaces; prints the flows it had, one per line (closed ones are listed for a while
# too). smolvm's own record of its DNS decisions during the probe goes to $work/decisions.
smolnet_probe() {
  local pid csv=$work/nettop.csv np dec n0
  dec="$(SMOLVM_USAGE=0 smolvm machine data-dir --name "$1")/egress-decisions.jsonl"
  pid=$(head -1 "$(dirname "$dec")/agent.pid" 2>/dev/null)
  [[ $pid =~ ^[0-9]+$ ]] || return 1
  n0=$(cat "$dec" 2>/dev/null | wc -l)
  nettop -t external -L 14 -s 1 -n -x -p "$pid" > "$csv" 2>/dev/null &
  np=$!
  sleep 2
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -i -- bash -s -- "$3" < "$2" > "$work/probe.log" 2>&1
  wait "$np"
  tail -n +$((n0 + 1)) "$dec" 2>/dev/null | grep '"transport":"dns"' > "$work/decisions"
  # Rows: time,what,…: the process (smolvm-bin.PID), or a flow (udp4 LOCAL<->REMOTE).
  awk -F, 'index($2, "<->") { print $2 }' "$csv" | sort -u
}
# smolnet_no_leak VM: refused DNS doesn't leave this machine. No flow beyond loopback, and smolvm
# refused every probe name (each one seen) and let through only devvm.invalid: the control, which
# shows the detector sees a query that does go to this machine's resolver (nettop doesn't: smolvm
# asks it through the system's resolver).
smolnet_no_leak() {
  local f n=devvm-e2e-$$-$RANDOM name allowed
  f=$(smolnet_probe "$1" "$work/dns-probe.sh" "$n") || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"; cat "$work/decisions"
  [[ -z $f && $(grep -c '^resolver rc=2$' "$work/probe.log") == 2 ]] || return 1
  allowed=$(grep '"action":"allow"' "$work/decisions" | sed 's/.*"destination":"\([^"]*\)".*/\1/' | tr '[:upper:]' '[:lower:]' | sort -u)
  [[ $allowed == devvm.invalid ]] || { echo "allowed: $allowed"; return 1; }
  for name in "$n-a.example.com" "$n-b.example.com" "$n-c.example.com" "$n-d.example.com" "$n-e.example.com" \
    "$n-f.example.com" "x.$n.devvm.invalid" "$n-g.example.org"; do
    grep '"action":"deny"' "$work/decisions" | grep -qF "\"destination\":\"$name\"" || { echo "not seen: $name"; return 1; }
  done
}
# smol_raw NAME OPTION...: a machine of the run's own from base's pack, with the sandbox's label and
# those network options, started: no port forward, which every devvm VM has since its ssh. Deleted
# at the end, or by smol_raw_rm.
smol_raw() { local n=$1; shift; created+=("$n")
  SMOLVM_USAGE=0 smolvm machine create --name "$n" --label "devvm.data=$XDG_DATA_HOME/devvm" --from "$packs/$kb/image.smolmachine" \
    --cpus 1 --mem 1024 "$@" >/dev/null 2>&1 && SMOLVM_USAGE=0 smolvm machine start --name "$n" >/dev/null 2>&1; }
smol_raw_rm() { SMOLVM_USAGE=0 smolvm machine delete --name "$1" -f >/dev/null 2>&1; }
# smolnet_tsi_dns_seen: on smolvm's own network without a forward (TSI, smol_raw), the VM asks this
# machine's resolver, and the DNS probe's queries are seen to leave: the nettop control.
smolnet_tsi_dns_seen() { local n=devvm-e2e-tsi r
  smol_raw "$n" --net || { echo "no machine"; return 1; }
  smol_tsi "$n" && smolnet_dns_seen "$n"; r=$?
  smol_raw_rm "$n"; return "$r"; }
# smolnet_dns_seen VM: on smolvm's own network, the same probe's queries do leave (the control).
smolnet_dns_seen() { local f; f=$(smolnet_probe "$1" "$work/dns-probe.sh" "devvm-e2e-$$-$RANDOM") || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"; grep -q ':53$' <<<"$f"; }
# A UDP listener on the host's loopback, 127.0.0.1:18766: each datagram it gets is a line in $work/udp.log.
smolnet_udp_listen() { : > "$work/udp.log"
  perl -MIO::Socket::INET -e '$| = 1; $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:18766", Proto => "udp") or die;
    while ($s->recv($d, 2048)) { print "got $d\n" }' >> "$work/udp.log" 2>&1 &
  udp_listener=$!; sleep 0.5; }
# smolnet_udp_closed VM: no datagram reaches the host's loopback, NTP gets no answer, nor a ping, and
# the VM's process has no flow beyond loopback.
smolnet_udp_closed() { local f
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x) || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log" "$work/udp.log"
  [[ -z $f && ! -s $work/udp.log ]] && grep -qx 'ntp bytes=0' "$work/probe.log" && grep -qx 'ping none' "$work/probe.log"; }
# smolnet_udp_allowed_seen VM: a UDP allow for the NTP server, added live, lets the probe's request
# out and nettop sees it (the UDP probe's control under network policy); then it's taken away.
smolnet_udp_allowed_seen() {
  local f
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --allow-udp 162.159.200.123/32:123 >/dev/null 2>&1 || return 1
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x)
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --remove-allow-udp 162.159.200.123/32:123 >/dev/null 2>&1 || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"
  grep -q '<->162\.159\.200\.123:123$' <<<"$f"
}
# smolnet_udp_seen VM: on smolvm's own network, the probe's datagrams leave and nettop sees them
# (the control). bash's /dev/udp reads no reply on that network, so replies aren't counted.
smolnet_udp_seen() { local f
  f=$(smolnet_probe "$1" "$work/udp-probe.sh" x) || return 1
  echo "flows: ${f:-none}"; cat "$work/probe.log"
  grep -q '<->162\.159\.200\.123:123$' <<<"$f" && grep -q '<->1\.1\.1\.1:443$' <<<"$f"; }
# smolnet_direct_open VM: on smolvm's own network the direct probes do get out (their control):
# 1.1.1.1, github's ssh banner, and IPv6 when this machine has it.
smolnet_direct_open() {
  smol_root "$1" "curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1" &&
  smol_root "$1" "timeout 10 bash -c 'exec 3<>/dev/tcp/140.82.112.3/22 && head -c 4 <&3' | grep -q SSH-" || return 1
  if curl -6 -s -m 5 -o /dev/null 'https://[2606:4700:4700::1111]'; then
    smol_root "$1" "curl -6 --noproxy '*' -sS -m 10 -o /dev/null https://[2606:4700:4700::1111]"
  else echo "this machine has no IPv6: not checked"; fi
}
# smolnet_fake_decider MODE: a decider of nobody's on the VM's decider port, which answers garbage
# (garbage), closes without a verdict (silent), allows every connection direct (allow: verdict 0),
# or takes every one over and answers it itself, 418 (take: verdict 2, the fail-closed probes'
# positive control); its pid in $fake. It fails, naming the port, unless it listens within 10 s.
smolnet_fake_decider() {
  # Each answer's connection is shut for writing, then read to its end (2 s at most) before it's
  # closed: closing it with smolvm's bytes unread would reset it, losing the answer at times.
  perl -MIO::Socket::INET -e '$SIG{ALRM} = sub {}; $| = 1; $m = shift; $p = shift; $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:$p", Listen => 16, ReuseAddr => 1) or die "$@\n";
    while ($c = $s->accept) { sysread($c, $b, 4096); print $c "\x07garbage" if $m eq "garbage"; print $c "\x00" if $m eq "allow";
      print $c "\x02HTTP/1.1 418 squatter\r\nContent-Length: 0\r\nConnection: close\r\n\r\n" if $m eq "take"; $c->flush;
      shutdown($c, 1); alarm 2; 1 while sysread($c, $b, 4096); alarm 0; close $c }' "$1" "$dp" >/dev/null 2>"$work/fake.log" &
  fake=$!
  for _ in $(seq 20); do
    lsof -t -a -p "$fake" -iTCP:"$dp" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    kill -0 "$fake" 2>/dev/null || break
    sleep 0.5
  done
  echo "the fake decider ($1) isn't listening on 127.0.0.1:$dp: $(cat "$work/fake.log")"
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""; return 1; }
# smolnet_port_free PORT: nothing listens on PORT within 10 s, else it fails naming it.
smolnet_port_free() {
  for _ in $(seq 20); do lsof -t -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1 || return 0; sleep 0.5; done
  echo "port $1 is still listened on: $(lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>&1)"; return 1; }
# smolnet_nothing_out VM: nothing gets out: not through the proxy, not directly, not to the host's
# loopback listener through the gateway (which answers 407).
smolnet_nothing_out() {
  ! in_smol "$1" 'curl -fsS -m 10 -o /dev/null https://github.com' &&
  ! smol_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" &&
  [[ $(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/") != 407 ]]
}
# smolnet_fake_allow_seen VM: a decider of nobody's that allows the connection direct (verdict 0)
# still gets nothing to the host's loopback listener: smolvm refuses direct connections to the host
# (direct_target_policy). One that takes the connection over (verdict 2) answers it: the probe sees
# a fail-open when there is one (its positive control).
smolnet_fake_allow_seen() {
  local allow take
  smolnet_fake_decider allow || return 1
  allow=$(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/")
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
  smolnet_fake_decider take || return 1
  take=$(smol_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://100.96.0.1:18765/")
  kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
  [[ $allow != 407 && $take == 418 ]] || { echo "with a decider that allows: $allow (want no 407), that takes over: $take (want 418)"; return 1; }
}
# smolnet_fail_closed VM: with its proxy gone nothing gets out, nor with a decider that answers
# garbage or nothing; and smolvm won't start it without a decider.
smolnet_fail_closed() {
  local pid m
  pid=$(smolnet_proxy "$1") && [[ -n $pid ]] && kill "$pid" && smolnet_port_free "$dp" || return 1
  if smolnet_proxy "$1" >/dev/null || ! smolnet_nothing_out "$1"; then echo "with the proxy gone"; return 1; fi
  for m in garbage silent; do
    smolnet_fake_decider "$m" || return 1
    smolnet_nothing_out "$1"; local rc=$?
    kill "$fake" 2>/dev/null; wait "$fake" 2>/dev/null; fake=""
    (( rc == 0 )) || { echo "with a decider that answers $m"; return 1; }
  done
  # The control: a decider that allows everything lets the same probe through to the listener.
  smolnet_fake_allow_seen "$1" || return 1
  SMOLVM_USAGE=0 smolvm machine stop --name "$1" >/dev/null 2>&1 &&
  ! env -u SMOLVM_INTERCEPTOR_TOKEN SSH_AUTH_SOCK=/dev/null/e2e-no-agent SMOLVM_USAGE=0 smolvm machine start --name "$1" >"$work/e" 2>&1 &&
    [[ $(smol_status "$1") != running ]] && grep -qi interceptor "$work/e"
}
# smolnet_dockerd_stale VM DIR: Docker's pid file names another process (dockerd gone, its pid taken)
# and its run folder is left over, when a launch restarts Docker's daemon (devvm's block in its
# settings, /etc/conf.d/docker, gone, so the launch writes it again: Alpine's, which has that file):
# the other process lives, and dockerd starts again, its run folder new.
smolnet_dockerd_stale() { local out
  out=$(SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c '
    pid=$(cat /run/docker.pid) && kill "$pid" && i=0
    while kill -0 "$pid" 2>/dev/null && [ "$i" -lt 30 ]; do sleep 1; i=$((i + 1)); done
    nohup sleep 900 >/dev/null 2>&1 </dev/null & echo $! > /run/docker.pid
    mkdir -p /run/docker && touch /run/docker/e2e-stale
    sed -i "/^# devvm-proxy begin/,/^# devvm-proxy end/d" /etc/conf.d/docker
    cat /run/docker.pid' </dev/null) || return 1
  (cd "$2" && printf 'exit\n' | "${U[@]}" "$D" >"$work/last.log" 2>&1) || { tail -5 "$work/last.log"; return 1; }
  SMOLVM_USAGE=0 smolvm machine exec --name "$1" -- sh -c '
    echo "the other process: $(cat /proc/'"$out"'/comm 2>/dev/null || echo gone); pid file: $(cat /run/docker.pid)"
    [ "$(cat /proc/'"$out"'/comm 2>/dev/null)" = sleep ] && kill '"$out"' && [ ! -e /run/docker/e2e-stale ] || exit 1
    i=0; while ! docker info >/dev/null 2>&1 && [ "$i" -lt 30 ]; do sleep 1; i=$((i + 1)); done
    [ "$(cat "/proc/$(cat /run/docker.pid)/comm")" = dockerd ] && docker info >/dev/null' </dev/null
}
# smolnet_widened VM DIR: its allow list widened while it runs (smolvm machine update): devvm shows
# it unenforced, and its next launch stops it; the one after starts it confined again.
smolnet_widened() {
  SMOLVM_USAGE=0 smolvm machine update --name "$1" --allow-cidr 1.1.1.1/32 >/dev/null 2>&1 || return 1
  if ! "$D" ls | grep -Eq "^$1 +Running +\S+ +\S+ +unenforced +- +smolvm "; then "$D" ls; return 1; fi
  if (cd "$2" && printf 'echo e2e-opened\n' | "${U[@]}" DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1) ||
    ! grep -qx "devvm: net_unenforced_stop vm=$1" "$work/last.log" || [[ $(smol_status "$1") != stopped ]]; then
    cat "$work/last.log"; return 1
  fi
  (cd "$2" && printf 'exit\n' | "${U[@]}" "$D" >"$work/last.log" 2>&1) && [[ $(smol_status "$1") == running ]] &&
  grep -qx net_check=ok "$vms/$1" && smolnet_lists "$1"
}
# The proxy's lifeline-following process for VM, if it runs.
smolnet_proxy() { pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 " 2>/dev/null; }
# sshn ARGS...: ssh through devvm's ssh config alone, without an agent. smolnet_via VM HOST:PORT: an
# HTTP request through a local forward (ssh -W) from the VM to HOST:PORT, and the start of what comes
# back, in 15 s at most.
sshn() { env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -o BatchMode=yes "$@"; }
smolnet_via() { printf 'GET / HTTP/1.0\r\nHost: %s\r\n\r\n' "${2%:*}" |
  perl -e 'alarm 15; exec @ARGV' env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -W "$2" "$1" 2>/dev/null | head -c 12; }
smolnet_via_open() { local a; a=$(smolnet_via "$1" 1.1.1.1:80); echo "got: ${a:-nothing}"; [[ $a == HTTP/* ]]; }
# smolnet_ssh VM: ssh reaches it on its network policy's port; its sessions, a login's or not, get the
# proxy's settings, and an allowed host works through the proxy from one.
smolnet_ssh() { local key out
  key=$(sed -n 's/^proxy_key=//p' "$vms/$1")
  [[ -n $(smolssh_port "$1") && $(smolssh_port "$1") == "$(sed -n 's/^ssh_port=//p' "$vms/$1")" ]] || { echo "not its ssh_port"; return 1; }
  out=$(sshn "$1" 'printenv HTTPS_PROXY; printenv NO_PROXY; bash -lc "printenv https_proxy"' </dev/null)
  [[ $out == "http://devvm:$key@100.96.0.1:"*$'\n'"localhost,127.0.0.1,::1"$'\n'"http://devvm:$key@100.96.0.1:"* ]] || { echo "sessions: ${out//$key/KEY}"; return 1; }
  sshn "$1" 'curl -fsS -m 20 -o /dev/null https://github.com' </dev/null; }
# smolnet_ssh_closed VM: ssh opens no way out: a local forward to an outside host or to a service on
# the host's loopback (which answers 407) gets nothing back, a remote forward on the VM's loopback to
# that service is refused, and a direct connection from a session fails.
smolnet_ssh_closed() { local a b
  a=$(smolnet_via "$1" 1.1.1.1:80) b=$(smolnet_via "$1" 100.96.0.1:18765)
  echo "1.1.1.1:80: ${a:-nothing}; the host's loopback: ${b:-nothing}"
  [[ $a != HTTP/* && $b != HTTP/* ]] &&
  ! sshn -o ExitOnForwardFailure=yes -R 0:127.0.0.1:18765 "$1" true </dev/null &&
  ! sshn "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null https://1.1.1.1" </dev/null; }
# A service on the host's own loopback (a devvm _proxy, which answers anything with 407).
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
np=$(newproj smolnet)
ps_watch
(cd "$np" && printf 'exit\n' | "${U[@]}" DEVVM_MESSAGES=names "$D" "${S[@]}" --network-policy --ports none --no-git-protect docker >"$work/last.log" 2>&1)
ps_unwatch
vm=$(vm_of "$np"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
dp=$(sed -n 's/^decider_port=//p' "$vms/$vm" 2>/dev/null)
check "--network-policy: the VM runs behind its proxy, which decides for it, and the self-check passed" bash -c "
  [[ -n '$vm' && -n '$dp' && \$(smol_status '$vm') == running ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  ! grep -q 'backend_unreviewed' '$work/last.log' &&
  pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm .* -decider 127.0.0.1:$dp -decider-to 100.96.0.1:$pp ' >/dev/null"
# The control: the guest has the password, from the VM's proxy file, in a login and in Docker's
# daemon, which devvm's services exec started with no login.
check "the proxy's password was in no host process's arguments while devvm launched the VM, nor after; the guest has it" bash -c "
  key=\$(sed -n 's/^proxy_key=//p' '$vms/$vm') && { [[ \${#key} -eq 32 ]] || { echo no key; false; }; } &&
  { [[ -s '$work/ps.log' ]] || { echo nothing sampled; false; }; } &&
  { ! grep -qFf <(echo \"\$key\") '$work/ps.log' || { echo in a process while devvm launched it; false; }; } &&
  { ! ps -axww -o command= | grep -qFf <(echo \"\$key\") || { echo in a process now; false; }; } &&
  { grep -qF \":\$key@\" '$XDG_DATA_HOME/devvm/smolvm/vms/$vm.proxy' || { echo no proxy file; false; }; } &&
  { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc 'printenv HTTPS_PROXY' </dev/null | grep -qF \":\$key@\" || { echo not in a login; false; }; } &&
  { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- sh -c 'tr \"\\\\0\" \"\\\\n\" < /proc/\$(cat /run/docker.pid)/environ' </dev/null |
    grep -qF \"https_proxy=http://devvm:\$key@\" || { echo not in the daemon devvm started; false; }; }"
check "the docker layer's Docker, started by devvm, pulls through the proxy (the layer's allowlist)" in_smol "$vm" \
  'for i in $(seq 30); do sudo docker info >/dev/null 2>&1 && break; sleep 1; done; sudo docker run --rm hello-world | grep -q Hello'
check "its allow list is the host's gateway alone, and its DNS a name that doesn't exist" smolnet_lists "$vm"
check "... on virtio-net: it asks the gateway's resolver" smol_vnet "$vm"
check "an allowed host works, through the proxy" in_smol "$vm" "curl -fsS -m 20 -o /dev/null https://github.com"
check "another host is blocked by the proxy, and so is another port of an allowed one" bash -c "
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc \"\$1\" </dev/null; }
  [[ \$(in_smol 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]] &&
  in_smol 'curl -sS -m 20 -o /dev/null https://example.com' 2>&1 | grep -q 403 &&
  ! in_smol 'curl -fsS -m 20 -o /dev/null https://github.com:8443'"
check "direct connections fail, even as root: an address, other ports, IPv6, the host's loopback" smolnet_direct "$vm"
check "ssh reaches it on its port; its sessions, a login's or not, get the proxy's settings, and an allowed host works" smolnet_ssh "$vm"
check "ssh opens no way out: a local forward to 1.1.1.1 or the host's loopback gets nothing, a remote forward on the VM's loopback is refused" smolnet_ssh_closed "$vm"
check "the host's loopback is refused through the proxy too (internal addresses need a rule)" bash -c "
  [[ \$(SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -- bash -lc 'curl -s -m 10 -o /dev/null -w %{http_code} http://100.96.0.1:18765/' </dev/null) == 403 ]]"
check "DNS fails at once, not after a timeout" in_smol "$vm" 'timeout 3 getent hosts example.com; [ $? -eq 2 ]'
if [[ "$(uname -s)" == Darwin ]]; then
  check "refused DNS doesn't leave this machine (TXT, ANY, AAAA, HTTPS, EDNS0, under devvm.invalid): smolvm refuses each, and only devvm.invalid, the control, goes to the resolver" smolnet_no_leak "$vm"
  smolnet_udp_listen
  check "UDP and ICMP don't get out: nothing reaches the host's loopback, NTP and ping get no answer" smolnet_udp_closed "$vm"
  # The ping has no control: smolvm's own network doesn't forward ICMP either.
  check "... while a UDP allow added live lets the probe's NTP request out (the control), until it's removed" smolnet_udp_allowed_seen "$vm"
else
  skip "refused DNS doesn't leave this machine (TXT, ANY, AAAA, HTTPS, EDNS0, under devvm.invalid): smolvm refuses each, and only devvm.invalid, the control, goes to the resolver" "nettop is macOS's"
  skip "UDP and ICMP don't get out: nothing reaches the host's loopback, NTP and ping get no answer" "nettop is macOS's"
  skip "... while a UDP allow added live lets the probe's NTP request out (the control), until it's removed" "nettop is macOS's"
fi
check "the decider's token is in no host process's arguments, and never reaches the VM" bash -c "
  tok=\$(sed -n 's/^decider_token=//p' '$vms/$vm') && [[ \${#tok} -eq 64 ]] &&
  ! ps -axww -o command= | grep -qFf <(echo \"\$tok\") &&
  ! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- bash -c \"grep -rqsF \$tok /etc /run /root /home /tmp /var/log\" </dev/null &&
  ! SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- bash -c \"cat /proc/[0-9]*/environ 2>/dev/null | tr '\\\\0' '\\\\n' | grep -qF \$tok\" </dev/null"
check "devvm network log shows the proxy's block and the direct connection to the host the decider refused" bash -c "cd '$np' &&
  '$D' network log >'$work/e' 2>&1; grep -q '  BLOCKED  example.com:80  ' '$work/e' && grep -q '  BLOCKED  100.96.0.1:18765  ' '$work/e' ||
  { cat '$work/e'; false; }"
check "devvm network allow applies at once to the running VM's proxy, and rm takes it back" bash -c "cd '$np' &&
  in_smol() { SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -u '$me' -e 'HOME=/home/$me.guest' -- bash -lc \"\$1\" </dev/null; }
  pid=\$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') && [[ -n \$pid ]] &&
  ${U[*]} '$D' network allow example.com </dev/null >/dev/null && in_smol 'curl -fsS -m 20 -o /dev/null https://example.com' &&
  ${U[*]} '$D' network rm example.com </dev/null >/dev/null &&
  [[ \$(in_smol 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]] &&
  [[ \$(pgrep -f -- '[_]proxy -listen 127.0.0.1:$pp -vm $vm ') == \$pid ]]"
check "its allow list widened while it runs: devvm shows it unenforced, stops it, then starts it confined again" smolnet_widened "$vm" "$np"
check "with its proxy gone nothing gets out (the host's loopback listener included), nor with a decider that answers garbage or nothing, while one that allows everything lets the probe through (the control); smolvm won't start it without a decider" \
  smolnet_fail_closed "$vm"
check "started outside devvm, with a decider of someone else's: devvm stops it and opens nothing" bash -c "
  SMOLVM_INTERCEPTOR_TOKEN=\$(openssl rand -hex 32) SSH_AUTH_SOCK=/dev/null/e2e-no-agent SMOLVM_USAGE=0 smolvm machine start --name '$vm' --mediated-egress --egress-interceptor 127.0.0.1:9 >/dev/null 2>&1 &&
  [[ \$(smol_status '$vm') == running ]] && cd '$np' &&
  ! printf 'echo e2e-opened\n' | ${U[*]} DEVVM_MESSAGES=names '$D' >'$work/last.log' 2>&1 &&
  grep -qx 'devvm: net_unenforced_stop vm=$vm' '$work/last.log' && ! grep -q e2e-opened '$work/last.log' && [[ \$(smol_status '$vm') == stopped ]]"
check "devvm starts it again confined, and the self-check passes" bash -c "cd '$np' &&
  printf 'exit\n' | ${U[*]} '$D' >'$work/last.log' 2>&1 && [[ \$(smol_status '$vm') == running ]] && grep -qx net_check=ok '$vms/$vm' &&
  pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null && [[ \$('$D' network 2>/dev/null | head -1) == on ]]"
check "devvm network off: its next start is smolvm's own network, without an allow list, a decider or a proxy" bash -c "cd '$np' &&
  ${U[*]} '$D' network off </dev/null >/dev/null 2>&1 && printf 'exit\n' | ${U[*]} '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' network | head -1) == off && \$(smol_status '$vm') == running ]] && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1 </dev/null"
check "... with no allow list left" smolnet_open "$vm"
check "... where the direct probes get out (their control): 1.1.1.1, github's ssh, IPv6" smolnet_direct_open "$vm"
check "... and a local forward through ssh to 1.1.1.1 gets an answer (its control)" smolnet_via_open "$vm"
check "... on virtio-net, as its ssh forward has it" smol_vnet "$vm"
# A machine with a port forward (its ssh) is on virtio-net, where smolvm resolves through this
# machine's system resolver, which nettop can't tie to it: the DNS control runs on a machine without
# a forward (TSI), as the probe was checked before ssh.
if [[ "$(uname -s)" == Darwin ]]; then
  check "... and on TSI (a machine without a forward) the DNS probe's queries leave (its control)" smolnet_tsi_dns_seen
  check "... and the UDP probe's datagrams leave (its control)" smolnet_udp_seen "$vm"
else
  skip "... and on TSI (a machine without a forward) the DNS probe's queries leave (its control)" "nettop is macOS's"
  skip "... and the UDP probe's datagrams leave (its control)" "nettop is macOS's"
fi
check "devvm network on: devvm restart starts it confined again" bash -c "cd '$np' &&
  ${U[*]} '$D' network on </dev/null >/dev/null 2>&1 && printf 'exit\n' | ${U[*]} '$D' restart >'$work/last.log' 2>&1 &&
  [[ \$('$D' network | head -1) == on ]] && grep -qx net_check=ok '$vms/$vm' && pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
check "... its allow list and DNS closed again, and direct connections fail" smolnet_closed "$vm"
check "rm --force deletes it, its proxy and its state" bash -c "cd '$np' && ${U[*]} '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
  [[ -z \$(smol_status '$vm') && ! -e '$vms/$vm' ]] && sleep 4 && ! pgrep -f -- '[_]proxy -listen .* -vm $vm ' >/dev/null"
# Alpine with the docker layer: launching adds devvm's proxy to Docker's settings, so it restarts
# Docker's daemon, with no init system (not through OpenRC, which isn't running).
na=$(newproj smolnetalp)
(cd "$na" && printf 'exit\n' | "${U[@]}" DEVVM_MESSAGES=names "$D" "${S[@]}" --from alpine:3 --network-policy --ports none --no-git-protect docker >"$work/last.log" 2>&1)
rc=$?
va=$(vm_of "$na"); [[ -z $va ]] || created+=("$va")
check "a VM --from alpine:3 with the docker layer and network policy is made, and the self-check passed" bash -c "
  [[ $rc -eq 0 && -n '$va' && \$(smol_status '$va') == running ]] && grep -qx net_check=ok '$vms/$va' && grep -qx libc=musl '$vms/$va' ||
  { tail -5 '$work/last.log'; false; }"
check "... its Docker, restarted by devvm, pulls through the proxy" in_smol "$va" \
  'for i in $(seq 30); do sudo docker info >/dev/null 2>&1 && break; sleep 1; done; sudo docker run --rm hello-world | grep -q Hello'
check "... a launch restarting it stops no other process its stale pid file names, and clears its run folder" \
  smolnet_dockerd_stale "$va" "$na"
(cd "$na" && quiet "${U[@]}" "$D" rm --force --no-export)
kill "$listener" 2>/dev/null; [[ -z ${udp_listener:-} ]] || kill "$udp_listener" 2>/dev/null; listener="" udp_listener=""
rm -f "$work/listener.auth"

section "smolvm: every port"
# The emulation's guarantee (reviewed): every port a VM listens on, on its loopback or all its
# addresses, IPv4 or IPv6, reaches this machine's loopback, and only its loopback; never a port this
# machine has on any address, one below 1024, or one devvm records for a VM, even while it's free;
# and forwarding opens the VM no way out (with network policy, nothing past its proxy).
# smolpa_serve VM HOST PORT: a server in the VM, as root, answering "vm-PORT" on HOST (an address of
# the VM's; :: takes IPv4 too), left running.
smolpa_serve() { SMOLVM_USAGE=0 smolvm machine exec --name "$1" -d -- perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
  $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1)) or die "$@";
  while ($c = $s->accept) { print $c "vm-$p\n"; close $c }' "$2" "$3" </dev/null >/dev/null 2>&1; }
# smolpa_answers PORT WANT: this machine's 127.0.0.1 and ::1 answer WANT on PORT, within 15 s.
smolpa_answers() { local a b; for _ in $(seq 15); do
    a=$(nc -w 2 127.0.0.1 "$1" </dev/null 2>/dev/null); b=$(nc -6 -w 2 ::1 "$1" </dev/null 2>/dev/null)
    [[ $a == "$2" && $b == "$2" ]] && return 0; sleep 1; done
  echo "127.0.0.1: ${a:-nothing}, ::1: ${b:-nothing}"; return 1; }
# smolpa_closed PORT: nothing answers on this machine's loopback on PORT, within 10 s.
smolpa_closed() { for _ in $(seq 10); do
    nc -z 127.0.0.1 "$1" 2>/dev/null || nc -6 -z ::1 "$1" 2>/dev/null || return 0; sleep 1; done
  echo "port $1 still answers"; return 1; }
# smolpa_forwarder VM: its forwarder's pid. smolpa_log VM: its forwarder's log.
smolpa_forwarder() { pgrep -f -- "[_]smolvm-ports -vm $1 " | head -1; }
smolpa_log() { cat "$XDG_DATA_HOME/devvm/smolvm/vms/$1.ports.log" 2>/dev/null; }
# smolpa_loopback VM PORT...: its forwarder listens on each PORT, and everything it listens on is on
# this machine's loopback alone.
smolpa_loopback() { local vm=$1 pid l p; shift
  pid=$(smolpa_forwarder "$vm"); [[ -n $pid ]] || { echo "no forwarder"; return 1; }
  l=$(lsof -nP -a -p "$pid" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }' | sort -u); echo "$l"
  for p in "$@"; do grep -qx "127\.0\.0\.1:$p" <<<"$l" && grep -qx "\[::1\]:$p" <<<"$l" || return 1; done
  ! grep -vE '^(127\.0\.0\.1|\[::1\]):[0-9]+$' <<<"$l"; }
# smolpa_master VM: its forwarder's ssh reads no configuration of yours, offers the VM's key alone,
# pins its host key and asks for no forwarding.
smolpa_master() { local m; m=$(pgrep -fl -- "ssh -F none -M .*HostKeyAlias=$1 " | head -1); echo "$m"
  [[ -n $m && $m == *"IdentityAgent=none"* && $m == *"StrictHostKeyChecking=yes"* && $m == *"ClearAllForwardings=yes"* &&
    $m == *"PermitRemoteOpen=none"* && $m == *"ForwardAgent=no"* ]] && ! grep -qE ' -[RLDAXY]( |$)' <<<"$m"; }
export -f smolpa_answers smolpa_closed smolpa_forwarder smolpa_log
pa=$(newproj smolall)
(cd "$pa" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${S[@]}" --no-network-policy --no-git-protect base >"$work/last.log" 2>&1)
vm=$(vm_of "$pa"); [[ -z $vm ]] || created+=("$vm")
check "the VM forwards every port (the default), with a forwarder of devvm's, none of smolvm's but its ssh's" bash -c "
  [[ -n '$vm' && \$(smol_status '$vm') == running ]] && ! grep -q backend_unreviewed '$work/last.log' &&
  ! grep -qE 'ports_(no_ssh|forward_failed)' '$work/last.log' && [[ -n \$(smolpa_forwarder '$vm') ]] && smolpa_log '$vm' | grep -q \"forwarding $vm's ports\""
check "... its machine's process listening on its ssh forward alone" smol_listens_only "$vm" "$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json")"
check "... its ssh master reading no config of yours, the VM's key alone, its host key pinned, no forwarding asked for" smolpa_master "$vm"
smolpa_serve "$vm" 127.0.0.1 47391; smolpa_serve "$vm" ::1 47392; smolpa_serve "$vm" 0.0.0.0 47393; smolpa_serve "$vm" :: 47394
check "a server on the VM's loopback alone reaches this machine's, at 127.0.0.1 and ::1" smolpa_answers 47391 vm-47391
check "... one on its IPv6 loopback alone" smolpa_answers 47392 vm-47392
check "... one on all its IPv4 addresses, and one on all its addresses" bash -c "smolpa_answers 47393 vm-47393 && smolpa_answers 47394 vm-47394"
check "the forwarder listens on this machine's loopback alone" smolpa_loopback "$vm" 47391 47392 47393 47394
# This machine's own program, on every address, and the VM's server on the same port.
perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:47395", Listen => 5, ReuseAddr => 1) or die;
  while ($c = $s->accept) { print $c "host-47395\n"; close $c }' </dev/null >/dev/null 2>&1 &
hostprog=$!
sleep 0.5
smolpa_serve "$vm" 127.0.0.1 47395; smolpa_serve "$vm" 0.0.0.0 1023
check "a port this machine has on any address stays its own, and one below 1024 isn't forwarded" bash -c "
  for _ in \$(seq 15); do smolpa_log '$vm' | grep -q 'port 47395: not forwarded' && break; sleep 1; done
  smolpa_log '$vm' | grep -q 'port 47395: not forwarded' && [[ \$(nc -w 2 127.0.0.1 47395 </dev/null) == host-47395 ]] &&
  ! smolpa_log '$vm' | grep -q 'port 1023' && smolpa_closed 1023"
kill "$hostprog" 2>/dev/null; wait "$hostprog" 2>/dev/null; hostprog=""
# smolpa_hostserve ADDR PORT: this machine's own program on ADDR (:: takes IPv4 too, as Node's and
# Go's servers do), answering "host-PORT", left running; its pid in hostprogs.
hostprogs=()
smolpa_hostserve() { perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
    $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1)) or die "$@";
    while ($c = $s->accept) { print $c "host-$p\n"; close $c }' "$1" "$2" </dev/null >/dev/null 2>&1 &
  hostprogs+=($!); }
# smolpa_hosts_kept: each of this machine's programs keeps its port, the VM's server on it isn't
# forwarded, and localhost never reaches two programs: on 127.0.0.1 alone (::1 stays closed), on
# ::1 alone (127.0.0.1 stays closed), and on every address, IPv4 and IPv6 at once.
smolpa_hosts_kept() { local vm=$1 p
  for _ in $(seq 15); do
    smolpa_log "$vm" | grep -q 'port 47403: not forwarded' && smolpa_log "$vm" | grep -q 'port 47402: not forwarded' && break; sleep 1; done
  smolpa_log "$vm" | grep -E 'port 4740[123]'
  for p in 47401 47402 47403; do smolpa_log "$vm" | grep -q "port $p: not forwarded" || { echo "port $p: no refusal logged"; return 1; }; done
  if [[ $(nc -w 2 127.0.0.1 47401 </dev/null) != host-47401 ]] || nc -6 -z ::1 47401 2>/dev/null; then
    echo "47401 not the host's alone"; return 1; fi
  if [[ $(nc -6 -w 2 ::1 47402 </dev/null) != host-47402 ]] || nc -z 127.0.0.1 47402 2>/dev/null; then
    echo "47402 not the host's alone"; return 1; fi
  [[ $(nc -w 2 127.0.0.1 47403 </dev/null) == host-47403 && $(nc -6 -w 2 ::1 47403 </dev/null) == host-47403 ]] ||
    { echo "47403 not the host's"; return 1; }; }
smolpa_hostserve 127.0.0.1 47401; smolpa_hostserve ::1 47402; smolpa_hostserve :: 47403
sleep 0.5
smolpa_serve "$vm" 0.0.0.0 47401; smolpa_serve "$vm" :: 47402; smolpa_serve "$vm" :: 47403
check "... so does one on its loopback alone, IPv4 or IPv6, and one on every address, IPv4 and IPv6 at once" smolpa_hosts_kept "$vm"
kill "${hostprogs[@]}" 2>/dev/null; wait "${hostprogs[@]}" 2>/dev/null; hostprogs=()
# smolpa_not_loopback VM: a server on the VM's own network address alone isn't forwarded, nor a
# port devvm records for another VM (its listed ports), free on this machine; one on the VM's
# loopback, started after them, is (the control).
smolpa_not_loopback() { local vm=$1 ip r=0
  ip=$(SMOLVM_USAGE=0 smolvm machine exec --name "$vm" -- perl -MIO::Socket::INET -e \
    'print IO::Socket::INET->new(PeerAddr => "100.96.0.1:9", Proto => "udp")->sockhost' </dev/null 2>/dev/null)
  [[ $ip =~ ^[0-9.]+$ && $ip != 127.* ]] || { echo "the VM's address: ${ip:-none}"; return 1; }
  printf 'net=off\nports=47406\n' > "$vms/devvm-e2e-reserved"
  smolpa_serve "$vm" "$ip" 47404; smolpa_serve "$vm" 0.0.0.0 47406; sleep 2; smolpa_serve "$vm" 127.0.0.1 47405
  smolpa_answers 47405 vm-47405 || { echo "the control isn't forwarded"; r=1; }
  smolpa_closed 47404 && smolpa_closed 47406 || r=1
  ! smolpa_log "$vm" | grep -E 'port (47404|47406)' || r=1
  rm -f "$vms/devvm-e2e-reserved"; return "$r"; }
check "a server on the VM's own address alone isn't forwarded, nor a port another VM lists, while it's free (a loopback server is, the control)" smolpa_not_loopback "$vm"
check "a server that stops: its forward goes; the others stay" bash -c "
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -- pkill -f '0.0.0.0 47393' </dev/null &&
  smolpa_closed 47393 && smolpa_answers 47391 vm-47391"
check "devvm stop stops the forwarder, and nothing listens on its ports" bash -c "cd '$pa' && '$D' stop </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smolpa_forwarder '$vm') ]] && smolpa_closed 47391 && smolpa_closed 47394"
check "started again, it has a new forwarder, which forwards a server started then" bash -c "cd '$pa' &&
  printf 'exit\n' | '$D' >'$work/last.log' 2>&1 && [[ -n \$(smolpa_forwarder '$vm') ]] &&
  SMOLVM_USAGE=0 smolvm machine exec --name '$vm' -d -- perl -MIO::Socket::IP -e '\$s = IO::Socket::IP->new(LocalHost => q(127.0.0.1), LocalPort => 47396, Listen => 5) or die; while (\$c = \$s->accept) { print \$c qq(vm-47396\n); close \$c }' </dev/null >/dev/null 2>&1 &&
  smolpa_answers 47396 vm-47396"
check "rm deletes it, its forwarder and its files" bash -c "cd '$pa' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') && -z \$(smolpa_forwarder '$vm') ]] && ! ls '$XDG_DATA_HOME/devvm/smolvm/vms/$vm'.ports.* 2>/dev/null && smolpa_closed 47396"

# With network policy: forwarding works, and opens the VM no way out; the forwarder never takes the
# VM's proxy or decider port, even with the proxy gone. A service on this machine's own loopback
# (which answers 407) for the direct probes.
echo listener-key > "$work/listener.auth"
"$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 &
listener=$!
pn=$(newproj smolallnet)
(cd "$pn" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${S[@]}" --network-policy --no-git-protect base >"$work/last.log" 2>&1)
vm=$(vm_of "$pn"); [[ -z $vm ]] || created+=("$vm")
pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
dp=$(sed -n 's/^decider_port=//p' "$vms/$vm" 2>/dev/null)
smolpa_serve "$vm" 127.0.0.1 47397
check "with network policy, every port forwarded too, and the self-check passed" bash -c "
  [[ -n '$vm' && -n '$pp' && -n '$dp' ]] && grep -qx net_check=ok '$vms/$vm' && smolpa_answers 47397 vm-47397"
check "... direct connections still fail while it forwards, even as root: an address, other ports, IPv6, the host's loopback" smolnet_direct "$vm"
# smolpa_reserved VM: with its proxy and decider gone, the VM listens on their ports: neither is
# forwarded (nothing answers there); the next devvm starts the proxy on them again.
smolpa_reserved() {
  local pid
  pid=$(pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 ")
  [[ -n $pid ]] || { echo "no proxy"; return 1; }
  kill "$pid" || return 1
  smolpa_closed "$pp" || return 1
  smolpa_closed "$dp" || return 1
  smolpa_serve "$1" 0.0.0.0 "$pp"; smolpa_serve "$1" 0.0.0.0 "$dp"; smolpa_serve "$1" 127.0.0.1 47398
  smolpa_answers 47398 vm-47398 || { echo "the control: a port of its own isn't forwarded"; return 1; }
  if nc -z 127.0.0.1 "$pp" 2>/dev/null || nc -z 127.0.0.1 "$dp" 2>/dev/null || smolpa_log "$1" | grep -qE "port ($pp|$dp): forwarded"; then
    echo "the proxy's or decider's port was forwarded"; smolpa_log "$1" | tail -5; return 1
  fi
  (cd "$pn" && printf 'exit\n' | "$D" >"$work/last.log" 2>&1) && grep -qx net_check=ok "$vms/$1" &&
  pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 " >/dev/null; }
check "with its proxy and decider gone, the VM listening on their ports: neither is forwarded (a port of its own is, the control); devvm starts the proxy on them again" smolpa_reserved "$vm"
# smolpa_listed_net VM DIR: with network policy and listed ports, 22 among them (the guest port of
# the VM's sshd): the self-check passes; smolvm publishes its ssh alone, from another port, and ssh
# by name still reaches the VM; the forwarder forwards the listed ports alone (22 too, unless a
# program here has it); a listed port's server answers, an unlisted one's doesn't; direct
# connections still fail.
smolpa_listed_net() { local vm=$1 sp
  (cd "$2" && "$D" stop </dev/null >/dev/null 2>&1 && "$D" ports 22,47399 </dev/null >/dev/null 2>&1 &&
    printf 'exit\n' | "$D" >"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }
  grep -qx net_check=ok "$vms/$vm" || { echo "no self-check"; return 1; }
  sp=$(sed -n 's/^ *"ssh_port": \([0-9]*\).*/\1/p' "$XDG_DATA_HOME/devvm/smolvm/vms/$vm.json")
  [[ $sp =~ ^[0-9]+$ && $sp != 22 && $sp != 47399 ]] || { echo "ssh port: ${sp:-none}"; return 1; }
  smol_listens_only "$vm" "$sp" || return 1
  env -u SSH_AUTH_SOCK ssh -F "$XDG_CONFIG_HOME/devvm/ssh_config" -o BatchMode=yes "$vm" true </dev/null || { echo "ssh by name failed"; return 1; }
  pgrep -fl -- "[_]smolvm-ports -vm $vm " | grep -qE -- ' -only (22,)?47399$' || { echo "forwarder's ports"; return 1; }
  smolpa_serve "$vm" 127.0.0.1 47399; smolpa_serve "$vm" 127.0.0.1 47400
  smolpa_answers 47399 vm-47399 && smolpa_closed 47400 && smolnet_direct "$vm"; }
check "with network policy and listed ports, 22 among them: the self-check passes, ssh still reaches the VM from its own port, the listed port alone is forwarded, and direct connections still fail" smolpa_listed_net "$vm" "$pn"
check "rm --force deletes it, its proxy and its forwarder" bash -c "cd '$pn' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
  [[ -z \$(smol_status '$vm') && -z \$(smolpa_forwarder '$vm') ]] && smolpa_closed 47397"
kill "$listener" 2>/dev/null; listener=""
rm -f "$work/listener.auth"

section "smolvm: shared tools"
t=$(newproj smoltools); echo '{}' > "$t/package.json"
launch "$t" 'node -v' "${S[@]}" --no-network-policy --ports none --no-git-protect
check "a node VM: its tools built uncompressed for smolvm, from the share, at the locked versions" bash -c "
  [[ -n '$vm' ]] && grep -qx image=node '$vms/$vm' && grep -qx v$(lockv node) '$work/last.log' &&
  for f in \$(sed -n 's/^tools=//p' '$vms/$vm'); do [[ -f '$share'/\$f && -f '$share'/\$f.sha256 ]] || exit 1; done"
check "node, npm, pnpm and bun work, each a read-only image" in_smol "$vm" \
  "npm -v && pnpm -v && bun -v && [[ \$(findmnt -no FSTYPE /opt/devvm/tools/node/$(lockv node)) == erofs ]] && ! touch /opt/devvm/tools/node/$(lockv node)/x 2>/dev/null"
check "the tools build left no machine behind" bash -c "[[ -z \$(smol_status devvm-tools-build) ]]"
check "restarted, the tools are mounted again" bash -c "cd '$t' && printf 'node -v\n' | '$D' restart 2>&1 | grep -qx v$(lockv node)"
(cd "$t" && quiet "$D" rm --force --no-export)

section "smolvm: the nix layer"
# No init system: devvm starts nix-daemon after each start, with the VM's proxy under network policy
# (in its settings file), and Nix's builds run in its sandbox, which they'd fail without.
export -f in_smol; export me
nx=$(newproj smolnix)
launch "$nx" exit "${S[@]}" --network-policy --ports none --no-git-protect nix
nix_built='derivation { name = "e2e-sandboxed"; system = builtins.currentSystem; builder = "/bin/sh"; args = [ "-c" "echo ok > $out" ]; }'
check "a nix VM with network policy: nix-daemon runs, with the VM's proxy" bash -c "
  [[ -n '$vm' ]] && grep -qx image=nix '$vms/$vm' && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
  in_smol '$vm' '[[ \$(cat /proc/\$(cat /run/nix-daemon.pid)/comm) == nix-daemon ]] &&
    sudo grep -qx \"export http_proxy=\$HTTP_PROXY https_proxy=\$HTTP_PROXY no_proxy=localhost,127.0.0.1,::1\" /etc/conf.d/nix-daemon'"
check "nix run nixpkgs#hello works through the proxy, and a build runs in Nix's sandbox" in_smol "$vm" "
  nix run nixpkgs#hello | grep -qx 'Hello, world!' && [[ \$(nix config show sandbox-fallback) == false ]] &&
  nix build --no-link --impure --expr '$nix_built'"
check "devvm network off: after devvm restart, nix-daemon runs again without the proxy" bash -c "cd '$nx' &&
  '$D' network off </dev/null >/dev/null && printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 &&
  in_smol '$vm' '[[ ! -e /etc/conf.d/nix-daemon && \$(cat /proc/\$(cat /run/nix-daemon.pid)/comm) == nix-daemon ]] &&
    ! sudo grep -qa _proxy=http /proc/\$(cat /run/nix-daemon.pid)/environ'"
(cd "$nx" && quiet "$D" rm --force --no-export)

section "smolvm: links between VMs"
link_section smolvm slk

check "no machine without the sandbox's label was touched" bash -c "[[ \"\$(smol_others)\" == '$others_before' ]]"
fi

if want docker; then
# The Docker backend (experimental): its native features, and a one-line refusal for the rest.
B=(--backend docker)
me=$(id -un)
# in_d VM CMD: CMD in a login shell as the VM's user, as devvm's shell has it; in_root VM CMD: as root.
in_d() { docker exec -u "$me" -e "HOME=/home/$me.guest" -e "USER=$me" "$1" bash -lc "$2" </dev/null; }
in_root() { docker exec -u 0 "$1" sh -c "$2" </dev/null; }
export -f in_d in_root; export me
# mounted VM DEST TYPE RW: the VM's container mounts TYPE (bind, volume) at DEST, read-write or not.
mounted() { docker container inspect -f '{{range .Mounts}}{{.Type}}|{{.Destination}}|{{.RW}} {{end}}' "$1" 2>/dev/null |
  tr ' ' '\n' | grep -qx "$3|$2|$4"; }
export -f mounted d_others
# refused DIR FEATURE ARGS...: devvm ARGS in DIR stops in one line, naming FEATURE, and makes nothing.
refused() { local d=$1 f=$2 before out; shift 2
  before=$(d_ours)
  out=$(cd "$d" && DEVVM_MESSAGES=names "$D" "${B[@]}" "$@" </dev/null 2>&1) && { echo "it ran: $out"; return 1; }
  echo "$out"
  [[ $(grep -c '^devvm: ' <<<"$out") -eq 1 ]] && grep -qE "^devvm: backend_lacks backend=docker feature=$f vm=" <<<"$out" &&
    [[ $(d_ours) == "$before" ]]; }
# owner_is_me FILE: the host file is the user's, and on Linux its group too.
owner_is_me() { local o; o=$(stat -c '%u %g' "$1" 2>/dev/null || stat -f '%u %g' "$1")
  echo "owner: $o"; [[ ${o% *} == "$(id -u)" ]] && { [[ "$(uname -s)" != Linux ]] || [[ ${o#* } == "$(id -g)" ]]; }; }

# docker_project NAME: a base VM's checks, on whichever engine: mounts, its home, the tools, size.
docker_project() {
  local p; p=$(newproj "$1"); git -C "$p" init -q
  launch "$p" 'pwd; id -un; echo e2e-shell-ran' "${B[@]}" --no-network-policy --ports none --cpus 2 --memory 3 base
  check "devvm makes the VM a container from base, labelled, and opens a shell in the folder as you" bash -c "
    [[ -n '$vm' && \$(d_state '$vm') == running ]] && grep -qx '$p' '$work/last.log' && grep -qx '$me' '$work/last.log' &&
    grep -qx e2e-shell-ran '$work/last.log' && grep -qx backend=docker '$vms/$vm' && grep -qx net=off '$vms/$vm' &&
    [[ \$(docker container inspect -f '{{index .Config.Labels \"devvm.kind\"}}' '$vm') == vm ]]"
  check "it gets --cpus and --memory" in_d "$vm" '[[ $(nproc) == 2 && $(cat /sys/fs/cgroup/memory.max) == 3221225472 ]]'
  check "ls lists it on docker" bash -c "'$D' ls 2>/dev/null | grep -Eq '^$vm +Running +base +\S+ +off +- +docker +$p\$'"
  check "the folder is mounted read-write, and what the VM writes is yours" bash -c "
    docker exec -u '$me' '$vm' sh -c 'echo from-vm > \"$p/vm.txt\"' </dev/null && grep -qx from-vm '$p/vm.txt'"
  check "... yours on this machine (and your group on Linux)" owner_is_me "$p/vm.txt"
  # Docker Desktop's file sharing passes on changes and new files, but no deletions.
  if $d_desktop; then
    check "a watcher in the VM sees files changed and made on this machine, but not deleted (Desktop)" watch_sees in_d "$vm" "$p" host yes no
  else
    check "a watcher in the VM sees files changed, made and deleted on this machine" watch_sees in_d "$vm" "$p" host
  fi
  check "its home is a volume of its own, the shared tools one read-only, even for root" bash -c "
    mounted '$vm' /home/$me.guest volume true && mounted '$vm' /mnt/devvm-tools volume false && d_vol '$vm-home' &&
    ! docker exec -u 0 '$vm' touch /mnt/devvm-tools/x </dev/null 2>/dev/null"
  check "hostname is the folder" in_d "$vm" "[[ \$(hostname) == $1 ]]"
  check "it has no Docker" in_d "$vm" '! command -v dockerd && ! command -v docker'
  check "devvm size shows its CPUs, memory and what its disk holds" bash -c "cd '$p' &&
    [[ \$('$D' size 2>/dev/null) =~ ^cpus=2\ memory=3GiB\ disk_used=[0-9.]+GiB\$ ]]"
  in_root "$vm" 'echo kept > /var/tmp/e2e-kept'
  check "devvm stop stops it; devvm starts it again, its disk as it was" bash -c "cd '$p' && '$D' stop </dev/null >/dev/null 2>&1 &&
    [[ \$(d_state $vm) == exited ]] && printf 'cat /var/tmp/e2e-kept\n' | '$D' 2>&1 | grep -qx kept && [[ \$(d_state $vm) == running ]]"
  q=$(newproj "$1-no")
  check "a disk size is refused in one line, nothing made" refused "$q" disk --no-network-policy --ports none --disk 200 base
  # Its containers can't make the namespaces Nix's build sandbox needs.
  check "the nix layer is refused in one line, nothing made" refused "$q" nix-layer --no-network-policy --ports none nix
  check "rm deletes its container, its home volume and its state" bash -c "cd '$p' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
    [[ -z \$(d_state $vm) ]] && ! d_vol '$vm-home' && [[ ! -e '$vms/$vm' && ! -e '$XDG_DATA_HOME/devvm/docker/vms/$vm.json' ]]"
}

# docker_agents NAME: an agent VM's folders, read-only; one added or removed makes its container again
# from a commit of its own, its disk and home kept.
docker_agents() {
  local ah=$work/agent-home a
  rm -rf "$ah"; mkdir -p "$ah/.claude/skills/e2e"; ah=$(cd "$ah" && pwd -P)
  echo '# e2e skill' > "$ah/.claude/skills/e2e/SKILL.md"
  a=$(newproj "$1")
  (cd "$a" && HOME=$ah quiet "$D" "${B[@]}" --no-network-policy --ports none --no-git-protect claude)
  vm=$(vm_of "$a"); [[ -z $vm ]] || created+=("$vm")
  check "an agent's folder is bind-mounted read-only at its host path, even for root, and linked into the VM's home" bash -c "
    [[ -n '$vm' ]] && mounted '$vm' '$ah/.claude/skills' bind false && ! docker exec -u 0 '$vm' touch '$ah/.claude/skills/x' </dev/null 2>/dev/null &&
    docker exec -u '$me' -e HOME=/home/$me.guest '$vm' sh -c 'grep -q e2e ~/.claude/skills/e2e/SKILL.md' </dev/null"
  in_root "$vm" 'echo kept > /var/tmp/e2e-kept'
  in_d "$vm" 'echo home > ~/e2e-home'
  mkdir -p "$ah/.claude/agents"
  check "a new agent folder is noticed on launch, and waits for a restart" bash -c "cd '$a' && printf 'exit\n' | HOME='$ah' '$D' 2>&1 |
    grep -qF \"\$(m mounts_next_start vm=$vm | head -1)\""
  check "restart makes its container again from a commit of its own, mounting it, its disk and home kept" bash -c "cd '$a' &&
    printf 'cat /var/tmp/e2e-kept ~/e2e-home\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 &&
    grep -qF \"\$(m docker_remaking vm=$vm | head -1)\" '$work/last.log' && grep -qx kept '$work/last.log' && grep -qx home '$work/last.log' &&
    mounted '$vm' '$ah/.claude/agents' bind false && d_img 'devvm-$dns:$vm-1'"
  rm -rf "$ah/.claude/skills"
  check "a removed one: made again, the commit before deleted, and it starts" bash -c "cd '$a' &&
    printf 'cat /var/tmp/e2e-kept\n' | HOME='$ah' '$D' restart >'$work/last.log' 2>&1 && grep -qx kept '$work/last.log' &&
    ! mounted '$vm' '$ah/.claude/skills' bind false && [[ \$(d_state $vm) == running ]] && d_img 'devvm-$dns:$vm-2' && ! d_img 'devvm-$dns:$vm-1'"
  check "rm deletes its container, volumes and commit" bash -c "cd '$a' && HOME='$ah' '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_state $vm) ]] && ! d_vol '$vm-home' && ! d_img 'devvm-$dns:$vm-2'"
}

# docker_tools NAME: a node VM's shared tools, unpacked in Docker's tools volume and linked.
docker_tools() {
  local t; t=$(newproj "$1"); echo '{}' > "$t/package.json"
  launch "$t" 'node -v' "${B[@]}" --no-network-policy --ports none --no-git-protect
  check "a node VM: its tools at the locked versions, in the share, recorded" bash -c "
    [[ -n '$vm' ]] && grep -qx image=node '$vms/$vm' && grep -qx v$(lockv node) '$work/last.log' &&
    for f in \$(sed -n 's/^tools=//p' '$vms/$vm'); do [[ -f '$share'/\$f && -f '$share'/\$f.sha256 ]] || exit 1; done"
  check "node, npm, pnpm and bun work, each linked from Docker's tools volume, read-only even for root" bash -c "
    in_d() { docker exec -u '$me' -e HOME=/home/$me.guest -e USER=$me '$vm' bash -lc \"\$1\" </dev/null; }
    in_d 'npm -v && pnpm -v && bun -v' && in_d '[[ \$(readlink /opt/devvm/tools/node/$(lockv node)) == /mnt/devvm-tools/node-$(lockv node)-*.erofs ]]' &&
    ! docker exec -u 0 '$vm' touch /opt/devvm/tools/node/$(lockv node)/x </dev/null 2>/dev/null"
  check "the tools' build and unpacking left no container behind" bash -c "
    [[ -z \$(d_state devvm-tools-build-$dns) && -z \$(d_state devvm-tools-unpack-$dns) ]]"
  check "restarted, the tools are there again" bash -c "cd '$t' && printf 'node -v\n' | '$D' restart 2>&1 | grep -qx v$(lockv node)"
  (cd "$t" && quiet "$D" rm --force --no-export)
}

# d_sysbox: the engine has Sysbox's runtime.
d_sysbox() { docker info --format '{{json .Runtimes}}' 2>/dev/null | grep -q '"sysbox-runc"'; }
# d_inner VM: Docker's engine in the VM runs a container as the VM's user, which reaches the internet.
d_inner() { in_d "$1" 'for _ in $(seq 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
  docker run --rm alpine:3 sh -c "wget -q -O /dev/null https://api.github.com && echo e2e-inner-ran"' 2>&1 | tail -3 | tee /dev/stderr | grep -qx e2e-inner-ran; }
# d_layer_refused DIR NAME ARGS...: devvm ARGS in DIR on docker stops in one line, NAME's message,
# and makes nothing.
d_layer_refused() { local d=$1 n=$2 before out; shift 2
  before=$(d_ours)
  out=$(cd "$d" && DEVVM_MESSAGES=names "$D" --backend docker "$@" </dev/null 2>&1) && { echo "it ran: $out"; return 1; }
  echo "$out"
  [[ $(grep -c '^devvm: ' <<<"$out") -eq 1 ]] && grep -qE "^devvm: $n( |\$)" <<<"$out" && [[ $(d_ours) == "$before" ]]; }
# d_layer_vm VM: a VM made for the docker layer: under Sysbox's runtime, not privileged, its Docker
# engine's volumes mounted read-write, on its own network, alone.
d_layer_vm() { local r m
  r=$(docker container inspect -f '{{.HostConfig.Privileged}} {{.HostConfig.Runtime}}' "$1"); echo "privileged, runtime: $r"
  m=$(docker network inspect -f '{{range $id, $c := .Containers}}{{$id}} {{end}}' "$1-docker-net" 2>/dev/null | xargs)
  echo "networks: $(dn_nets "$1"); members: $m"
  [[ $r == 'false sysbox-runc' ]] && mounted "$1" /var/lib/docker volume true && mounted "$1" /var/lib/containerd volume true &&
    d_vol "$1-docker" && d_vol "$1-containerd" && [[ $(dn_nets "$1") == "$1-docker-net" ]] && d_net "$1-docker-net" &&
    [[ $(docker network inspect -f '{{.Internal}}' "$1-docker-net") == false ]] &&
    [[ $m == "$(docker container inspect -f '{{.Id}}' "$1")" ]]; }
# d_plain_vm VM: a VM as any other: Docker's runtime, without NET_RAW, on Docker's default network.
d_plain_vm() { local r
  r=$(docker container inspect -f '{{.HostConfig.Runtime}} {{.HostConfig.CapDrop}}' "$1"); echo "runtime, dropped: $r; networks: $(dn_nets "$1")"
  [[ $r == 'runc [NET_RAW]' || $r == 'runc [CAP_NET_RAW]' ]] && [[ $(dn_nets "$1") == bridge ]]; }
# d_reach FROM...: FROM (a command prefix) connects to $standin_ip:8080 and reads the stand-in's line.
d_reach() { "$@" bash -c "exec 3<>/dev/tcp/$standin_ip/8080 && head -1 <&3" 2>&1 | grep -qx e2e-standin; }
# d_isolated VM: a stand-in container of the sandbox's from VM's image, on Docker's default network,
# then on another network: VM can't reach it, even as root, while another container on that network
# can (the control); VM reaches the internet.
d_isolated() { local img n r=0
  img=$(docker container inspect -f '{{.Config.Image}}' "$1")
  docker network create --label "devvm.data=$XDG_DATA_HOME/devvm" "$1-othernet" >/dev/null || return 1
  for n in bridge "$1-othernet"; do
    docker run -d --name "$1-standin" --label "devvm.data=$XDG_DATA_HOME/devvm" --network "$n" --entrypoint perl "$img" \
      -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalPort => 8080, Listen => 5, ReuseAddr => 1) or die;
        while ($c = $s->accept) { print $c "e2e-standin\n"; close $c }' >/dev/null || { r=1; break; }
    standin_ip=$(docker container inspect -f "{{(index .NetworkSettings.Networks \"$n\").IPAddress}}" "$1-standin"); export standin_ip
    sleep 1
    echo "stand-in on $n at $standin_ip"
    if d_reach timeout 5 docker exec -u 0 "$1"; then echo "the VM reaches it"; r=1
    elif d_reach docker run --rm --label "devvm.data=$XDG_DATA_HOME/devvm" --network "$n" --entrypoint timeout "$img" 5; then
      echo "the VM doesn't reach it; the control does"
    else echo "the control doesn't reach it"; r=1
    fi
    docker rm -f "$1-standin" >/dev/null 2>&1
  done
  docker network rm "$1-othernet" >/dev/null 2>&1
  (( r == 0 )) && in_root "$1" 'curl -sS -m 20 -o /dev/null https://api.github.com' && echo "the internet reached"; }
# d_netroot VM: root in VM, through Docker's engine in it (a container on the VM's own network
# namespace, privileged in it, as anyone in the VM's docker group can run), runs the script on its
# input.
d_netroot() { docker exec -i -u "$me" -e "HOME=/home/$me.guest" -e "USER=$me" "$1" bash -lc 'docker run --rm -i --privileged --network host alpine:3 sh -s'; }
# d_udp_host ADDR PORT: a server of this machine's on ADDR, UDP port PORT, in the background, which
# writes the first datagram it gets to $work/udp-PORT.
d_udp_host() { rm -f "$work/udp-$2"
  perl -MIO::Socket::IP -e '($h, $p, $f) = @ARGV; $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Proto => "udp") or die "$@";
    $SIG{ALRM} = sub { exit 0 }; alarm 60; $s->recv($m, 100); open(F, ">", $f) or die; print F $m; close F' "$1" "$2" "$work/udp-$2" </dev/null >/dev/null 2>&1 &
  d_hostprogs+=($!); }
# d_layer_loopback VM: root in VM sends UDP through its gateway to this machine's server on every
# address (the control), then routes 127.0.0.1 to its gateway (route_localnet on in the VM, the local
# table moved behind) and sends to this machine's server on its loopback: only the control's arrives,
# as the bridge's route_localnet is off.
d_layer_loopback() { local br out
  br=br-$(docker network inspect -f '{{.Id}}' "$1-docker-net" | cut -c1-12)
  d_udp_host 0.0.0.0 47411; d_udp_host 127.0.0.1 47412; sleep 1
  out=$(printf '%s\n' 'gw=$(ip route | awk "/^default/ { print \$3 }")' 'echo e2e-every | nc -u -w1 "$gw" 47411' \
    'sysctl -qw net.ipv4.conf.all.route_localnet=1 net.ipv4.conf.eth0.route_localnet=1' \
    'ip rule add pref 10 to 127.0.0.1 lookup 100 && ip rule del pref 0 && ip rule add pref 20 lookup local &&' \
    '  ip route add 127.0.0.1/32 via "$gw" dev eth0 table 100 && ip route get 127.0.0.1 | grep -q " via $gw " && echo e2e-routed' \
    'for _ in 1 2 3; do echo e2e-loopback | nc -u -w1 127.0.0.1 47412; done' \
    'ip rule add pref 0 lookup local; ip rule del pref 20; ip rule del pref 10' | d_netroot "$1" 2>&1)
  sleep 2; kill "${d_hostprogs[@]}" 2>/dev/null; wait "${d_hostprogs[@]}" 2>/dev/null; d_hostprogs=()
  echo "$out"; echo "bridge $br route_localnet: $(cat "/proc/sys/net/ipv4/conf/$br/route_localnet")"
  echo "every address got: $(cat "$work/udp-47411" 2>/dev/null); loopback got: $(cat "$work/udp-47412" 2>/dev/null)"
  grep -qx e2e-routed <<<"$out" && grep -qx e2e-every "$work/udp-47411" && [[ ! -s $work/udp-47412 ]]; }
# d_layer_ipv6 VM: root in VM turns IPv6 on for its interface and reaches this machine's server on ::
# through the bridge's link-local address (on its own host's servers on every address, IPv4's gateway
# reaches them too).
d_layer_ipv6() { local br ll out
  br=br-$(docker network inspect -f '{{.Id}}' "$1-docker-net" | cut -c1-12)
  ll=$(ip -6 addr show "$br" | awk '/inet6 fe80/ { sub(/\/.*/, "", $2); print $2; exit }')
  d_hostserve :: 47413; sleep 1
  out=$(printf '%s\n' 'sysctl -qw net.ipv6.conf.eth0.disable_ipv6=0; sleep 3' "nc -w3 $ll%eth0 47413 </dev/null" \
    'sysctl -qw net.ipv6.conf.eth0.disable_ipv6=1' | d_netroot "$1" 2>&1)
  kill "${d_hostprogs[@]}" 2>/dev/null; wait "${d_hostprogs[@]}" 2>/dev/null; d_hostprogs=()
  echo "bridge $br at $ll; the VM got: $out"; grep -qx host-47413 <<<"$out"; }
# d_layer_refused_existing DIR VM: the docker layer named for VM (DIR's, a base VM) that exists:
# refused in one line, naming the layer, and VM as it was (its image base, no docker_layer, a VM as
# any other, still running, no network or engine's volumes of the layer's).
d_layer_refused_existing() { local d=$1 vm=$2
  (cd "$d" && ! DEVVM_MESSAGES=names "$D" docker </dev/null >"$work/last.log" 2>&1); cat "$work/last.log"
  echo "state: $(grep -E '^(image|docker_layer|net)=' "$vms/$vm" | xargs)"
  [[ $(grep -c '^devvm: ' "$work/last.log") -eq 1 ]] && grep -qx "devvm: vm_lacks_layer image=docker layer=docker vm=$vm" "$work/last.log" &&
    grep -qx image=base "$vms/$vm" && ! grep -q '^docker_layer=.' "$vms/$vm" && [[ $(d_state "$vm") == running ]] &&
    d_plain_vm "$vm" && ! d_vol "$vm-docker" && ! d_vol "$vm-containerd" && ! d_net "$vm-docker-net"; }
# d_engine_kept DIR VM: devvm restart in DIR starts VM's Docker engine again, with the image d_inner
# pulled; else what its engine logged.
d_engine_kept() {
  (cd "$1" && printf 'exit\n' | "$D" restart >"$work/last.log" 2>&1) || { tail -5 "$work/last.log"; return 1; }
  in_d "$2" 'for _ in $(seq 30); do docker info >/dev/null 2>&1 && break; sleep 1; done; docker image inspect alpine:3 >/dev/null' && return 0
  in_root "$2" 'echo "pid file: $(cat /run/docker.pid); processes: $(for p in /proc/[0-9]*; do printf "%s=%s " "${p#/proc/}" "$(cat "$p/comm")"; done)"
    tail -3 /var/log/dockerd.log'
  return 1; }
export -f d_inner d_layer_vm d_plain_vm owner_is_me

# docker_layer NAME: the docker layer, under Sysbox where the engine has it (an emulation, reviewed),
# for a new VM; refused elsewhere (on Docker Desktop, saying to use a VM backend; on a Linux engine
# without Sysbox, to install it), with network policy, and for a VM that exists.
docker_layer() {
  local q s p n=docker_layer_no_sysbox
  q=$(newproj "$1-no")
  p=$(newproj "$1-inplace")
  launch "$p" exit "${B[@]}" --no-network-policy --ports none base
  if ! d_sysbox; then
    ! $d_desktop || n=docker_layer_needs_vm
    check "the docker layer is refused without Sysbox ($n), in one line, nothing made" \
      d_layer_refused "$q" "$n" --no-network-policy --ports none docker
    check "... and named for a VM that exists, refused in one line, the VM as it was" d_layer_refused_existing "$p" "$vm"
    (cd "$p" && quiet "$D" rm --force --no-export)
    return 0
  fi
  check "the docker layer with network policy is refused in one line, nothing made" \
    d_layer_refused "$q" net_docker_layer --network-policy --ports none docker

  s=$(newproj "$1-sysbox")
  (cd "$s" && DEVVM_MESSAGES=names "$D" "${B[@]}" --ports none docker </dev/null >"$work/last.log" 2>&1)
  vm=$(vm_of "$s"); [[ -z $vm ]] || created+=("$vm")
  check "under Sysbox: its runtime, not privileged, said, unwarned, its engine's volumes, its own network alone, no network policy (said)" bash -c "
    [[ -n '$vm' ]] && grep -qE '^==> docker_layer_runtime backend=docker runtime=sysbox vm=$vm\$' '$work/last.log' &&
    ! grep -q backend_unreviewed '$work/last.log' && grep -qx 'devvm: net_off_docker_layer backend=docker vm=$vm' '$work/last.log' &&
    d_layer_vm '$vm' && grep -qx docker_layer=sysbox '$vms/$vm' && grep -qx net=off '$vms/$vm' || { tail -5 '$work/last.log'; false; }"
  check "... root in it isn't root here: a user namespace of its own" in_root "$vm" \
    '! grep -qE "^ *0 +0 +4294967295\$" /proc/self/uid_map && cat /proc/self/uid_map'
  check "... root has every capability in its own namespaces, NET_ADMIN and NET_RAW among them" in_root "$vm" \
    'b=$(sed -n "s/^CapBnd:[[:space:]]*//p" /proc/self/status); echo "bounding set: $b"; [ $((0x$b >> 12 & 3)) -eq 3 ]'
  check "... Docker's engine runs in it, started by devvm, for the VM's user" d_inner "$vm"
  check "... it reaches the internet, but no container on Docker's default network or another, even as root" d_isolated "$vm"
  check "... nor this machine's loopback, even as root routing 127.0.0.1 out (its servers on every address, it does)" \
    d_layer_loopback "$vm"
  check "... root turning IPv6 on reaches this machine's servers on :: through the bridge's link-local address" d_layer_ipv6 "$vm"
  check "... what it writes in the folder is yours" bash -c "docker exec -u '$me' '$vm' sh -c 'echo from-vm > \"$s/vm.txt\"' </dev/null &&
    owner_is_me '$s/vm.txt'"
  check "... root's set-user-id file in the folder is root's here too, and its next launch names it as root's" bash -c "
    in_root '$vm' 'cp /bin/true \"$s/e2e-setid\" && chmod 4755 \"$s/e2e-setid\"' && [[ \$(stat -c '%u %a' '$s/e2e-setid') == '0 4755' ]] &&
    cd '$s' && printf 'exit\n' | DEVVM_MESSAGES=names '$D' >'$work/last.log' 2>&1; in_root '$vm' 'rm -f \"$s/e2e-setid\"'
    grep -E '^(==> |devvm: )?setid_root_files ' '$work/last.log' | grep -q e2e-setid"
  check "... ls shows it" bash -c "cd '$s' && '$D' ls 2>/dev/null | grep -Eq '^$vm +Running +.* +docker \(sysbox\) +$s\$'"
  check "... network policy can't be turned on" bash -c "cd '$s' && ! DEVVM_MESSAGES=names '$D' network on </dev/null 2>'$work/last.log' &&
    grep -qx 'devvm: net_docker_layer backend=docker vm=$vm' '$work/last.log' && grep -qx net=off '$vms/$vm'"
  # Its shell, a login, has the docker group the layer adds.
  check "... Docker's engine runs in it, for the VM's user in its shell" bash -c "cd '$s' &&
    printf 'for _ in \$(seq 30); do docker info >/dev/null 2>&1 && break; sleep 1; done\ndocker run --rm alpine:3 echo e2e-inner-ran\n' |
      '$D' 2>&1 | tail -3 | tee /dev/stderr | grep -qx e2e-inner-ran"
  check "... restarted, its engine runs again with its images kept" d_engine_kept "$s" "$vm"
  # 600 MB in the engine's volume, outside the container's own writes: devvm size counts it.
  check "... devvm size's disk_used counts its engine's volumes" bash -c "
    used() { cd '$s' && '$D' size 2>/dev/null | sed -n 's/.*disk_used=\([0-9.]*\)GiB.*/\1/p'; }
    a=\$(used) && in_root '$vm' 'dd if=/dev/urandom of=/var/lib/docker/e2e-fill bs=1M count=600 status=none' &&
    b=\$(used); in_root '$vm' 'rm -f /var/lib/docker/e2e-fill'; echo \"disk_used: \$a GiB, then \$b GiB\"
    [[ -n \$a && -n \$b ]] && awk -v a=\"\$a\" -v b=\"\$b\" 'BEGIN { exit !(b - a >= 0.5) }'"
  check "rm deletes its container, volumes and network" bash -c "cd '$s' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_state $vm) ]] && ! d_vol '$vm-docker' && ! d_vol '$vm-containerd' && ! d_net '$vm-docker-net'"

  vm=$(vm_of "$p")
  check "named for a VM that exists: refused in one line, the VM as it was" d_layer_refused_existing "$p" "$vm"
  (cd "$p" && quiet "$D" rm --force --no-export)
}

# Docker's emulations (ssh for editors, ports, agent forwarding). Their guarantees: nothing published
# or listening, each VM's own host keys pinned and its own key the only login, as the VM's user; no
# forwarding out that devvm's entry lets through, whatever the VM runs as sshd; ports on this
# machine's loopback alone, never one of its programs' or another VM's; the agent in ssh sessions
# alone, as the VM's choice says. A home of the checks' own (Docker's own config kept), with an
# ~/.ssh/config set up as the docs say, whose Host * forwards the agent; ssh gets no agent but
# throwaway ones; a stub code.
# d_emu_begin / d_emu_end: into the checks' home, and out again.
d_emu_begin() {
  d_real_home=$HOME
  export DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
  sc=$work/ssh; rm -rf "$sc"; mkdir -p "$sc/home/.ssh" "$sc/bin"
  dsc=$XDG_CONFIG_HOME/devvm/ssh_config kh=$XDG_CONFIG_HOME/devvm/known_hosts
  printf 'Include %s\nHost *\n  ForwardAgent yes\n' "$dsc" > "$sc/config"
  cp "$sc/config" "$sc/home/.ssh/config"
  printf '#!/bin/sh\necho "$*" > %s/code-args\n' "$sc" > "$sc/bin/code"; chmod +x "$sc/bin/code"
  agent_pid=$(ssh-agent -a "$sc/agent.sock" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
  export HOME=$sc/home sc dsc kh
}
d_emu_end() {
  kill "$agent_pid" 2>/dev/null; agent_pid=""
  export HOME=$d_real_home
}
# dsh ARGS...: ssh through the checks' config, without an agent. dssh_proxy HOST: the ProxyCommand ssh
# runs for it. dssh_key VM: its pinned host key. d_procs VM: the commands running in the VM.
dsh() { env -u SSH_AUTH_SOCK ssh -F "$sc/config" "$@" </dev/null; }
dssh_proxy() { ssh -G -F "$sc/config" "$1" 2>/dev/null | sed -n 's/^proxycommand //p'; }
dssh_key() { awk -v n="$1" '$1 == n { print $2, $3 }' "$kh"; }
d_procs() { docker exec -u 0 "$1" sh -c 'for c in /proc/[0-9]*/cmdline; do tr "\0\n" "  " < "$c"; echo; done 2>/dev/null' </dev/null; }
# d_serve VM HOST PORT: a server in the VM, as root, answering "vm-PORT" on HOST (:: takes IPv4 too),
# left running. d_unserve VM PORT: it stops.
d_serve() { docker exec -d -u 0 "$1" perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
  $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1), GetAddrInfoFlags => 0) or die "$@";
  while ($c = $s->accept) { print $c "vm-$p\n"; close $c }' "$2" "$3" </dev/null >/dev/null 2>&1; }
d_unserve() { docker exec -u 0 "$1" sh -c 'for p in /proc/[0-9]*; do tr "\0\n" "  " < $p/cmdline 2>/dev/null | grep -q "^perl .* $1 *\$" && kill ${p#/proc/}; done; true' sh "$2" </dev/null; }
# d_serve_any VM PORTS...: d_serve on every IPv4 address, for host_port_kept.
d_serve_any() { local p; for p in "${@:2}"; do d_serve "$1" 0.0.0.0 "$p"; done; }
# d_v6 VM: the VM has an IPv6 loopback (Docker turns IPv6 off in containers on some engines).
d_v6() { docker exec "$1" sh -c 'grep -q "^00000000000000000000000000000001 " /proc/net/if_inet6' </dev/null 2>/dev/null; }
# h_v6: this machine has one.
h_v6() { perl -MIO::Socket::IP -e 'IO::Socket::IP->new(LocalHost => "::1", LocalPort => 0, Listen => 1) or exit 1' 2>/dev/null; }
# d_answers PORT WANT [6]: this machine's 127.0.0.1 (and ::1 with 6) answers WANT on PORT, within 20 s.
d_answers() { local a b; for _ in $(seq 20); do
    a=$(nc -w 2 127.0.0.1 "$1" </dev/null 2>/dev/null); b=$2
    [[ -z ${3:-} ]] || b=$(nc -6 -w 2 ::1 "$1" </dev/null 2>/dev/null)
    [[ $a == "$2" && $b == "$2" ]] && return 0; sleep 1; done
  echo "127.0.0.1: ${a:-nothing}${3:+, ::1: ${b:-nothing}}"; return 1; }
# d_closed PORT: nothing answers on this machine's loopback on PORT, within 15 s.
d_closed() { for _ in $(seq 15); do
    if ! nc -z 127.0.0.1 "$1" 2>/dev/null && { ! h_v6 || ! nc -6 -z ::1 "$1" 2>/dev/null; }; then return 0; fi; sleep 1; done
  echo "port $1 still answers"; return 1; }
# d_started VM: when Docker last started its container.
d_started() { docker container inspect -f '{{.State.StartedAt}}' "$1" 2>/dev/null; }
# d_fwd VM: its forwarder's pid. d_fwdlog VM: its log.
d_fwd() { pgrep -f -- "[_]docker-ports -vm $1 " | head -1; }
d_fwdlog() { cat "$XDG_DATA_HOME/devvm/docker/vms/$1.ports.log" 2>/dev/null; }
# d_listens PID: what that process listens on (TCP), ADDRESS:PORT lines.
d_listens() { if command -v lsof >/dev/null; then lsof -nP -a -p "$1" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { print $9 }'
  else ss -ltnpH 2>/dev/null | awk -v p="pid=$1," 'index($0, p) { print $4 }'; fi | sort -u; }
# d_unpublished VM: its container publishes no port.
d_unpublished() { local b; b=$(docker container inspect -f '{{json .HostConfig.PortBindings}} {{json .NetworkSettings.Ports}}' "$1" 2>/dev/null)
  echo "published: ${b:-?}"; [[ -n $b ]] && ! grep -q HostPort <<<"$b"; }
export -f dsh dssh_proxy dssh_key d_procs d_serve d_unserve d_v6 h_v6 d_answers d_closed d_fwd d_fwdlog d_listens d_unpublished
# d_hostile VM HOST: root in the VM rewrites its sshd's config to allow remote forwarding, X11 and
# tunnels (with xauth for X11); each connection's sshd reads it. Through a config of the check's own
# whose Host * asks for X11 and to add keys to the agent, the session gets no display and the agent
# no key; the same asked on the command line gets both (the controls), and ssh -R now works (not
# covered: the entry can't refuse it). Then the agent is emptied; the next start writes devvm's
# config again.
d_hostile() { local hc=$sc/hostile-config d keys
  printf 'Include %s\nHost *\n  ForwardX11 yes\n  ForwardX11Trusted yes\n  AddKeysToAgent yes\n' "$dsc" > "$hc"
  docker exec -u 0 "$1" sh -c 'apt-get update -q >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends xauth >/dev/null 2>&1 &&
    c=/etc/devvm/ssh/sshd_config && sed -i -e "s/^AllowTcpForwarding .*/AllowTcpForwarding yes/" -e "s/^AllowStreamLocalForwarding .*/AllowStreamLocalForwarding yes/" \
      -e "s/^X11Forwarding .*/X11Forwarding yes/" -e "s/^PermitTunnel .*/PermitTunnel yes/" -e "s/^PermitListen .*/PermitListen any/" "$c" &&
    /usr/sbin/sshd -t -f "$c"' </dev/null || { echo "no hostile sshd"; return 1; }
  sshh() { DISPLAY=:9 SSH_AUTH_SOCK=$sc/agent.sock ssh -F "$hc" -o BatchMode=yes "$@" </dev/null 2>/dev/null; }
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  d=$(sshh "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "through Host *: display $d, agent ${keys:-empty}"
  [[ $d == x=none && -z $keys ]] || return 1
  d=$(sshh -Y -o AddKeysToAgent=yes "$2" 'echo "x=${DISPLAY:-none}"'); keys=$(SSH_AUTH_SOCK=$sc/agent.sock ssh-add -L 2>/dev/null | grep '^ssh-')
  echo "asked on the command line: display $d, agent ${keys:-empty}"
  SSH_AUTH_SOCK=$sc/agent.sock ssh-add -D >/dev/null 2>&1
  [[ $d != x=none && -n $keys ]] || return 1
  sshh -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 "$2" true || { echo "ssh -R refused by the hostile sshd"; return 1; }; }
# d_hostserve ADDR PORT: this machine's own program on ADDR (:: takes IPv4 too, as Node's and Go's
# servers do), answering "host-PORT", left running; its pid in d_hostprogs.
d_hostprogs=()
d_hostserve() { perl -MIO::Socket::IP -e '($h, $p) = @ARGV;
    $s = IO::Socket::IP->new(LocalHost => $h, LocalPort => $p, Listen => 5, ReuseAddr => 1, V6Only => ($h eq "::" ? 0 : 1)) or die "$@";
    while ($c = $s->accept) { print $c "host-$p\n"; close $c }' "$1" "$2" </dev/null >/dev/null 2>&1 &
  d_hostprogs+=($!); }
# d_hosts_kept VM: with this machine's programs on 47401 (127.0.0.1 alone), 47402 (::1 alone) and
# 47403 (every address, IPv4 and IPv6 at once) and the VM's servers on all its addresses on each,
# each program keeps its port, the VM's server isn't forwarded, and localhost never reaches two
# programs. Without IPv6 here, 127.0.0.1 alone.
d_hosts_kept() { local vm=$1 p ports=47401 all
  ! h_v6 || ports="47401 47402 47403"
  for _ in $(seq 15); do
    all=true; for p in $ports; do d_fwdlog "$vm" | grep -q "port $p: not forwarded" || all=false; done
    ! $all || break; sleep 1; done
  d_fwdlog "$vm" | grep -E 'port 4740[123]'
  for p in $ports; do d_fwdlog "$vm" | grep -q "port $p: not forwarded" || { echo "port $p: no refusal logged"; return 1; }; done
  if [[ $(nc -w 2 127.0.0.1 47401 </dev/null) != host-47401 ]] || { h_v6 && nc -6 -z ::1 47401 2>/dev/null; }; then
    echo "47401 not the host's alone"; return 1; fi
  h_v6 || return 0
  if [[ $(nc -6 -w 2 ::1 47402 </dev/null) != host-47402 ]] || nc -z 127.0.0.1 47402 2>/dev/null; then
    echo "47402 not the host's alone"; return 1; fi
  [[ $(nc -w 2 127.0.0.1 47403 </dev/null) == host-47403 && $(nc -6 -w 2 ::1 47403 </dev/null) == host-47403 ]] ||
    { echo "47403 not the host's"; return 1; }; }
# d_not_loopback VM: a server on the VM's own network address alone isn't forwarded, nor a port devvm
# records for another VM (its listed ports), free on this machine; one on the VM's loopback, started
# after them, is (the control).
d_not_loopback() { local vm=$1 ip r=0
  ip=$(docker container inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' "$vm" 2>/dev/null | awk '{ print $1 }')
  [[ $ip =~ ^[0-9.]+$ && $ip != 127.* ]] || { echo "the VM's address: ${ip:-none}"; return 1; }
  printf 'net=off\nports=47406\n' > "$vms/devvm-e2e-reserved"
  d_serve "$vm" "$ip" 47404; d_serve "$vm" 0.0.0.0 47406; sleep 2; d_serve "$vm" 127.0.0.1 47405
  d_answers 47405 vm-47405 || { echo "the control isn't forwarded"; r=1; }
  d_closed 47404 && d_closed 47406 || r=1
  ! d_fwdlog "$vm" | grep -E 'port (47404|47406)' || r=1
  rm -f "$vms/devvm-e2e-reserved"; return "$r"; }

# docker_ssh NAME: ssh for editors' emulation.
docker_ssh() {
  local q q2 vm2 k1 qa vma
  section "docker: ssh for editors"
  q=$(newproj "$1"); git -C "$q" init -q
  (cd "$q" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${B[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
  vm=$(vm_of "$q"); [[ -z $vm ]] || created+=("$vm")
  check "the ssh config lists it by the folder's name, through devvm _docker-connect, with no port" bash -c "
    [[ -n '$vm' ]] && ! grep -q backend_unreviewed '$work/last.log' &&
    grep -q '^Host devvm-$1 $vm\$' '$dsc' && [[ \$(dssh_proxy devvm-$1) == *\"'_docker-connect'\"*\"'$vm'\" ]] &&
    ! awk -v h='$vm' '\$1 == \"Host\" { on = (\$3 == h) } on && \$1 == \"Port\"' '$dsc' | grep -q ."
  check "its container publishes nothing, and nothing in it listens for ssh, nor runs sshd, between connections" bash -c "
    d_unpublished '$vm' && ! docker exec '$vm' sh -c 'cat /proc/net/tcp /proc/net/tcp6' </dev/null 2>/dev/null | awk '\$4 == \"0A\" { print \$2 }' | grep -qi ':0016\$' &&
    ! d_procs '$vm' | grep -q sshd"
  check "its host key is pinned, as the VM has it: ed25519, its only one" bash -c "
    vmk=\$(docker exec -u 0 '$vm' sh -c 'cat /etc/ssh/ssh_host_*_key.pub' </dev/null | awk '{ print \$1, \$2 }') &&
    echo \"the VM's: \$vmk; pinned: \$(dssh_key '$vm')\" && [[ -n \$vmk && \$vmk == \"\$(dssh_key '$vm')\" && \$vmk == 'ssh-ed25519 '* ]]"
  check "ssh devvm-$1 reaches it, as the VM's user, with the environment devvm's commands get" bash -c "
    out=\$(dsh devvm-$1 'id -un; hostname; echo \$BASH_ENV') && [[ \$out == '$me'\$'\n''$1'\$'\n'/etc/profile.d/devvm-tools.sh ]] || { echo \"\$out\"; false; }"
  check "... each connection an sshd of its own, gone with it" bash -c "dsh devvm-$1 true && sleep 1 && ! d_procs '$vm' | grep -q sshd"
  # Its desktop (base has the desktop feature), with no init system: devvm desktop starts it.
  desktop_serve "$q"
  check "devvm desktop serves its desktop on this machine's 127.0.0.1 alone, its container publishing nothing" bash -c "
    [[ -n '$desk_port' ]] && loopback_only '$desk_port' && d_unpublished '$vm'"
  if command -v go >/dev/null || command -v mise >/dev/null; then
    check "a viewer with a wrong password is refused, one with the password gets the VM's desktop" desktop_view
  else
    skip "a viewer with a wrong password is refused, one with the password gets the VM's desktop" "no Go here: the Docker Desktop run checks it"
  fi
  check "the VM's server listens on its user's own socket, on no TCP port, the clipboard off" in_d "$vm" "$desktop_in_vm"
  check "the password is in no file of the VM's nor any run log" bash -c '
    printf "%s\n" "$1" | docker exec -i -u 0 "$2" sh -c "$3" && ! grep -rlF "$1" "$4"' \
    _ "$desk_pw" "$vm" "$desktop_secret" "$XDG_DATA_HOME/devvm/logs"
  desktop_end
  check "Ctrl+C ends devvm desktop, its port closed" bash -c "[[ -n '$desk_port' ]] && ! nc -z 127.0.0.1 '$desk_port'"
  check "agent forwarding stays off, whatever Host * says; ssh -A forwards it (the control)" bash -c "
    SSH_AUTH_SOCK='$sc/agent.sock' ssh -F '$sc/config' devvm-$1 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null &&
    SSH_AUTH_SOCK='$sc/agent.sock' ssh -A -F '$sc/config' devvm-$1 'test -S \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
  check "a changed host key is refused" bash -c "
    cp '$kh' '$sc/kh' && rm -f '$sc/changed' '$sc/changed.pub' && ssh-keygen -q -t ed25519 -N '' -f '$sc/changed' &&
    { grep -v '^$vm ' '$sc/kh' || true; echo \"$vm \$(cut -d' ' -f1,2 '$sc/changed.pub')\"; } > '$kh' &&
    { dsh devvm-$1 true 2>'$sc/err'; r=\$?; cp '$sc/kh' '$kh'; } && (( r == 255 )) && grep -q 'Host key verification failed' '$sc/err'"
  check "password and keyboard-interactive logins are refused: publickey alone is offered" bash -c "
    out=\$(env -u SSH_AUTH_SOCK ssh -v -F '$sc/config' -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive \
      -o BatchMode=yes devvm-$1 true </dev/null 2>&1); r=\$?
    (( r == 255 )) && grep -qE 'Authentications that can continue: publickey[[:space:]]*\$' <<<\"\$out\" &&
    ! grep -qE 'Authentications that can continue: .*(password|keyboard)' <<<\"\$out\""
  check "only devvm's key logs in, as the VM's user: another key, root and another user are refused" bash -c "
    rm -f '$sc/other' '$sc/other.pub'; ssh-keygen -q -t ed25519 -N '' -f '$sc/other' &&
    ! env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile='$kh' -o HostKeyAlias='$vm' -o StrictHostKeyChecking=yes \
      -o BatchMode=yes -o IdentitiesOnly=yes -i '$sc/other' -o ProxyCommand=\"\$(dssh_proxy devvm-$1)\" -l '$me' '$vm' true </dev/null &&
    ! dsh -o BatchMode=yes -l root devvm-$1 true && ! dsh -o BatchMode=yes -l nobody devvm-$1 true && dsh -o BatchMode=yes devvm-$1 true"
  d_serve "$vm" 127.0.0.1 47501
  check "devvm's sshd binds remote forwards only on 127.77.0.0/16 and refuses socket forwards; a local forward to the VM's loopback works" bash -c "
    rfwd_links devvm-$1 dsh &&
    for _ in \$(seq 10); do [[ \$(dsh -W 127.0.0.1:47501 devvm-$1 2>/dev/null | head -1) == vm-47501 ]] && exit 0; sleep 1; done; false"
  check "sshd runs devvm's settings, and its config is root's alone" bash -c "
    t=\$(docker exec -u 0 '$vm' /usr/sbin/sshd -T -f /etc/devvm/ssh/sshd_config </dev/null) || exit 1
    for want in 'allowusers $me' 'permitrootlogin no' 'authenticationmethods publickey' 'passwordauthentication no' \
      'kbdinteractiveauthentication no' 'allowtcpforwarding yes' 'allowstreamlocalforwarding local' 'permitlisten 127.77.*:*' \
      'gatewayports clientspecified' 'x11forwarding no' 'permittunnel no' 'permituserenvironment no' 'allowagentforwarding yes'; do
      grep -qxF \"\$want\" <<<\"\$t\" || { echo \"not \$want\"; exit 1; }; done
    [[ \$(docker exec -u 0 '$vm' stat -c '%U %a' /etc/devvm/ssh/sshd_config </dev/null) == 'root 600' ]]"
  check "devvm _docker-connect refuses another devvm's container, in one line, running nothing" bash -c "
    out=\$(DEVVM_MESSAGES=names '$D' _docker-connect -docker \"\$(command -v docker)\" -data /tmp/devvm-e2e-elsewhere '$vm' </dev/null 2>&1)
    r=\$?; echo \"\$out\"; (( r != 0 )) && grep -qx 'devvm: docker_container_foreign name=$vm' <<<\"\$out\" &&
    [[ \$(grep -c '^devvm: ' <<<\"\$out\") -eq 1 ]] && ! d_procs '$vm' | grep -q sshd"
  check "devvm host names it; devvm code opens it over Remote-SSH (a stub code)" bash -c "cd '$q' &&
    '$D' host </dev/null >'$work/last.log' 2>&1 && grep -qF \"\$(m ssh_host_info dir=$q host=devvm-$1 | head -1)\" '$work/last.log' &&
    PATH='$sc/bin':\$PATH '$D' code </dev/null >'$work/last.log' 2>&1 && [[ \$(cat '$sc/code-args') == '--remote ssh-remote+devvm-$1 $q' ]]"
  check "devvm's shell is an ssh session: SSH_CONNECTION set, in the folder" bash -c "cd '$q' &&
    out=\$(printf 'pwd; echo \"conn=\${SSH_CONNECTION:+yes}\"\n' | '$D' 2>&1) && grep -qx '$q' <<<\"\$out\" && grep -qx conn=yes <<<\"\$out\" || { echo \"\$out\"; false; }"
  check "an sshd root made hostile gets no display and no key through a Host * asking for them; asked on the command line, both (the controls), and ssh -R" d_hostile "$vm" "devvm-$1"
  k1=$(dssh_key "$vm")
  check "restart: new host keys, pinned again, devvm's sshd config again (a remote forward on 127.0.0.1 refused), and ssh reaches it" bash -c "cd '$q' &&
    printf 'exit\n' | '$D' restart >'$work/last.log' 2>&1 && [[ \$(d_state '$vm') == running ]] &&
    k=\$(dssh_key '$vm') && [[ -n \$k && \$k != '$k1' ]] && ! dsh -o ExitOnForwardFailure=yes -R 0:127.0.0.1:9 devvm-$1 true && dsh devvm-$1 true"
  check "started outside devvm (docker restart): devvm host says so, its entry goes, and devvm's shell is a docker exec; devvm restart has ssh again" bash -c "cd '$q' &&
    docker restart -t 1 '$vm' >/dev/null && ! DEVVM_MESSAGES=names '$D' host </dev/null >'$work/e' 2>&1 &&
    grep -qx 'devvm: vm_no_ssh vm=$vm' '$work/e' && ! grep -q '$vm' '$dsc' &&
    out=\$(printf 'echo \"conn=\${SSH_CONNECTION:-none}\"\n' | '$D' 2>&1) && grep -qx conn=none <<<\"\$out\" &&
    printf 'exit\n' | '$D' restart >/dev/null 2>&1 && grep -q '^Host devvm-$1 $vm\$' '$dsc' && dsh devvm-$1 true"
  q2=$(newproj "${1}2")
  (cd "$q2" && printf 'exit\n' | "$D" "${B[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
  vm2=$(vm_of "$q2"); [[ -z $vm2 ]] || created+=("$vm2")
  check "a VM made from the same image has host keys of its own" bash -c "
    [[ -n '$vm2' ]] && a=\$(dssh_key '$vm') && b=\$(dssh_key '$vm2') && [[ -n \$a && -n \$b && \$a != \$b ]] && dsh devvm-${1}2 true"
  check "... and a key of its own: its key doesn't log in to the other VM, the other's own does (the control)" bash -c "
    with_key() { env -u SSH_AUTH_SOCK ssh -F /dev/null -o UserKnownHostsFile='$kh' -o HostKeyAlias='$vm' -o StrictHostKeyChecking=yes \
      -o BatchMode=yes -o IdentitiesOnly=yes -i '$XDG_DATA_HOME/devvm/docker/vms/'\$1.key -o ProxyCommand=\"\$(dssh_proxy devvm-$1)\" -l '$me' '$vm' true </dev/null 2>/dev/null; }
    ! with_key '$vm2' && with_key '$vm'"
  check "an image without sshd (root moved it away): the next start runs without ssh, warned, and the VM runs" bash -c "cd '$q2' &&
    docker exec -u 0 '$vm2' mv /usr/sbin/sshd /usr/sbin/sshd.away </dev/null &&
    printf 'exit\n' | DEVVM_MESSAGES=names '$D' restart >'$work/last.log' 2>&1 && grep -qx 'devvm: vm_no_sshd vm=$vm2' '$work/last.log' &&
    [[ \$(d_state '$vm2') == running ]] && ! grep -q '$vm2' '$dsc' &&
    ! DEVVM_MESSAGES=names '$D' host </dev/null 2>'$work/e' && grep -qx 'devvm: vm_no_sshd vm=$vm2' '$work/e'"
  (cd "$q2" && quiet "$D" rm --force --no-export)
  qa=$(newproj "${1}alpine")
  (cd "$qa" && printf 'exit\n' | "$D" "${B[@]}" --from alpine:3 --no-network-policy --ports none base >"$work/last.log" 2>&1)
  vma=$(vm_of "$qa"); [[ -z $vma ]] || created+=("$vma")
  check "a VM --from alpine:3 is made, and ssh devvm-${1}alpine reaches it, on Alpine, as the VM's user" bash -c "
    [[ -n '$vma' ]] && grep -qx libc=musl '$vms/$vma' &&
    out=\$(dsh -o BatchMode=yes devvm-${1}alpine '. /etc/os-release; echo \$ID; id -un') &&
    [[ \$out == alpine\$'\n''$me' ]] || { echo \"\${out:-}\"; tail -5 '$work/last.log'; false; }"
  (cd "$qa" && quiet "$D" rm --force --no-export)
  check "devvm stop drops its entry; devvm _docker-connect refuses the stopped VM, running nothing" bash -c "cd '$q' &&
    '$D' stop </dev/null >/dev/null 2>&1 && ! grep -q '$vm' '$dsc' &&
    ! DEVVM_MESSAGES=names '$D' _docker-connect -docker \"\$(command -v docker)\" -data '$XDG_DATA_HOME/devvm' '$vm' </dev/null 2>'$work/e' &&
    grep -qx 'devvm: vm_not_running vm=$vm' '$work/e'"
  check "rm deletes it, its entry, its key and its pinned keys" bash -c "cd '$q' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_state '$vm') ]] && ! grep -q '$vm' '$dsc' && ! grep -q '^$vm ' '$kh' && ! ls '$XDG_DATA_HOME/devvm/docker/vms/$vm'.* 2>/dev/null"
}

# docker_ports NAME: ports' and every port's emulation.
docker_ports() {
  local pa pl pid hp v6=""
  section "docker: ports"
  pa=$(newproj "$1")
  (cd "$pa" && printf 'exit\n' | DEVVM_MESSAGES=names "$D" "${B[@]}" --no-network-policy --no-git-protect base >"$work/last.log" 2>&1)
  vm=$(vm_of "$pa"); [[ -z $vm ]] || created+=("$vm")
  check "a VM forwards every port (the default) with a forwarder of devvm's, and publishes nothing" bash -c "
    [[ -n '$vm' && \$(d_state '$vm') == running ]] && ! grep -q backend_unreviewed '$work/last.log' &&
    ! grep -q 'ports_' '$work/last.log' && [[ -n \$(d_fwd '$vm') ]] && d_fwdlog '$vm' | grep -q \"forwarding $vm's ports\" && d_unpublished '$vm'"
  check "... its ssh master reading no config of yours, the VM's key alone, its host key pinned, no forwarding asked for, through devvm _docker-connect" bash -c "
    m=\$(ps -axww -o command= | grep -- '[s]sh -F none -M .*HostKeyAlias=$vm ' | head -1); echo \"\$m\"
    [[ \$m == *IdentityAgent=none* && \$m == *StrictHostKeyChecking=yes* && \$m == *ClearAllForwardings=yes* && \$m == *PermitRemoteOpen=none* &&
      \$m == *ForwardAgent=no* && \$m == *\"ProxyCommand='\"*\"' '_docker-connect' \"* ]] && ! grep -qE ' -[RLDAXY]( |\$)' <<<\"\$m\""
  ! d_v6 "$vm" || ! h_v6 || v6=6
  d_serve "$vm" 127.0.0.1 47511; d_serve "$vm" 0.0.0.0 47513
  [[ -z $v6 ]] || { d_serve "$vm" ::1 47512; d_serve "$vm" :: 47514; }
  check "a server on the VM's loopback alone reaches this machine's$([[ -z $v6 ]] || echo ", at 127.0.0.1 and ::1")" d_answers 47511 vm-47511 $v6
  check "... one on all its IPv4 addresses too" d_answers 47513 vm-47513 $v6
  if [[ -n $v6 ]]; then
    check "... one on its IPv6 loopback alone, and one on all its addresses" bash -c "d_answers 47512 vm-47512 6 && d_answers 47514 vm-47514 6"
  else
    skip "... one on its IPv6 loopback alone, and one on all its addresses" "no IPv6 loopback in the VM or here"
  fi
  check "the forwarder listens on this machine's loopback alone" bash -c "
    l=\$(d_listens \"\$(d_fwd '$vm')\"); echo \"\$l\"; grep -qE '^(127\.0\.0\.1|\[::1\]):47511\$' <<<\"\$l\" && ! grep -vE '^(127\.0\.0\.1|\[::1\]):[0-9]+\$' <<<\"\$l\""
  perl -MIO::Socket::INET -e '$s = IO::Socket::INET->new(LocalAddr => "0.0.0.0:47515", Listen => 5, ReuseAddr => 1) or die;
    while ($c = $s->accept) { print $c "host-47515\n"; close $c }' </dev/null >/dev/null 2>&1 &
  hp=$!; sleep 0.5
  d_serve "$vm" 127.0.0.1 47515; d_serve "$vm" 0.0.0.0 1023
  check "a port this machine has on any address stays its own, and one below 1024 isn't forwarded" bash -c "
    for _ in \$(seq 15); do d_fwdlog '$vm' | grep -q 'port 47515: not forwarded' && break; sleep 1; done
    d_fwdlog '$vm' | grep -q 'port 47515: not forwarded' && [[ \$(nc -w 2 127.0.0.1 47515 </dev/null) == host-47515 ]] &&
    ! d_fwdlog '$vm' | grep -q 'port 1023' && d_closed 1023"
  kill "$hp" 2>/dev/null; wait "$hp" 2>/dev/null
  d_hostserve 127.0.0.1 47401
  ! h_v6 || { d_hostserve ::1 47402; d_hostserve :: 47403; }
  sleep 0.5
  d_serve "$vm" 0.0.0.0 47401; d_serve "$vm" 0.0.0.0 47402; d_serve "$vm" 0.0.0.0 47403
  check "... so does one on its loopback alone$(h_v6 && echo ", IPv4 or IPv6, and one on every address, IPv4 and IPv6 at once")" d_hosts_kept "$vm"
  kill "${d_hostprogs[@]}" 2>/dev/null; wait "${d_hostprogs[@]}" 2>/dev/null; d_hostprogs=()
  check "a server on the VM's own address alone isn't forwarded, nor a port another VM lists, while it's free (a loopback server is, the control)" d_not_loopback "$vm"
  check "a server that stops: its forward goes; the others stay" bash -c "d_unserve '$vm' 47513 && d_closed 47513 && d_answers 47511 vm-47511"
  check "devvm stop stops the forwarder, and nothing listens on its ports" bash -c "cd '$pa' && '$D' stop </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_fwd '$vm') ]] && d_closed 47511"
  check "started again, it has a new forwarder, which forwards a server started then" bash -c "cd '$pa' &&
    printf 'exit\n' | '$D' >'$work/last.log' 2>&1 && [[ -n \$(d_fwd '$vm') ]] && d_serve '$vm' 127.0.0.1 47516 && d_answers 47516 vm-47516"
  check "docker stop outside devvm ends its forwarder" bash -c "docker stop -t 1 '$vm' >/dev/null &&
    for _ in \$(seq 40); do [[ -z \$(d_fwd '$vm') ]] && break; sleep 1; done; [[ -z \$(d_fwd '$vm') ]] && d_closed 47516"
  check "rm deletes it, its forwarder and its files" bash -c "cd '$pa' && '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_state '$vm') && -z \$(d_fwd '$vm') ]] && ! ls '$XDG_DATA_HOME/devvm/docker/vms/$vm'.* 2>/dev/null"
  # Listed ports: the same forwarder, those alone.
  pl=$(newproj "${1}list")
  (cd "$pl" && printf 'exit\n' | "$D" "${B[@]}" --no-network-policy --no-git-protect --ports 47521,47180-47181 base >"$work/last.log" 2>&1)
  vm=$(vm_of "$pl"); [[ -z $vm ]] || created+=("$vm")
  d_serve "$vm" 127.0.0.1 47521; d_serve "$vm" 127.0.0.1 47522
  check "listed ports: through the forwarder too, only those (an unlisted port isn't forwarded), nothing published" bash -c "
    [[ -n '$vm' ]] && d_answers 47521 vm-47521 && d_closed 47522 && d_unpublished '$vm' &&
    ps -axww -o command= | grep -- '[_]docker-ports -vm $vm ' | grep -q -- ' -only 47521,47180-47181\$'"
  check "a port added to a running VM reaches this machine at once, and once removed doesn't, the container never restarted" \
    ports_live "$pl" "$vm" 47523 d_serve_any d_started
  check "a listed port a program on this machine has, on every address, stays its own until it's freed" host_port_kept "$pl" "$vm" 47181 47180 d_serve_any
  (cd "$pl" && quiet "$D" rm --force --no-export)
}

# docker_agent NAME: agent forwarding's and turning it on or off's emulations. Your agent is a
# throwaway one (ua_sock); devvm's own has a throwaway key.
docker_agent() {
  local g ua da
  section "docker: SSH agent forwarding"
  ua=$work/d-agent.sock da=$XDG_DATA_HOME/devvm/agent.sock
  rm -f "$ua" "$work/d-key" "$work/d-key.pub" "$work/d-dkey" "$work/d-dkey.pub"
  user_agent_pid=$(ssh-agent -a "$ua" | sed -n 's/^SSH_AGENT_PID=\([0-9]*\);.*/\1/p')
  ssh-keygen -q -t ed25519 -N '' -C e2e-docker-user -f "$work/d-key"
  ssh-keygen -q -t ed25519 -N '' -C e2e-docker-devvm -f "$work/d-dkey"
  SSH_AUTH_SOCK=$ua ssh-add -q "$work/d-key" 2>/dev/null
  # d_shell_keys DIR: what devvm's shell in DIR lists of its agent, devvm run with yours.
  d_shell_keys() { (cd "$1" && printf 'ssh-add -l 2>&1 || true\n' | SSH_AUTH_SOCK=$ua "$D" 2>&1); }
  # d_ssh_keys HOST: what ssh HOST lists of its agent, ssh run with yours.
  d_ssh_keys() { SSH_AUTH_SOCK=$ua ssh -F "$sc/config" "$1" 'ssh-add -l 2>&1 || true' </dev/null 2>&1; }
  # d_switch DIR MODE: devvm ssh-agent MODE, then devvm restart, run with yours.
  d_switch() { (cd "$1" && SSH_AUTH_SOCK=$ua "$D" ssh-agent "$2" </dev/null >"$work/last.log" 2>&1 &&
    printf 'exit\n' | SSH_AUTH_SOCK=$ua "$D" restart >>"$work/last.log" 2>&1) || { cat "$work/last.log"; return 1; }; }
  # d_only OUT COMMENT: OUT lists that key alone. d_none OUT: no agent at all.
  d_only() { echo "agent: $1"; [[ $(grep -c 'SHA256:' <<<"$1") -eq 1 ]] && grep -q "$2" <<<"$1"; }
  d_none() { echo "agent: $1"; grep -qiE 'could not open a connection|error connecting to agent' <<<"$1" && ! grep -q 'SHA256:' <<<"$1"; }
  export ua work; export -f d_shell_keys d_ssh_keys d_switch d_only d_none
  g=$(newproj "$1")
  (cd "$g" && printf 'exit\n' | SSH_AUTH_SOCK=$ua DEVVM_MESSAGES=names "$D" "${B[@]}" --no-network-policy --ports none --ssh-agent base >"$work/last.log" 2>&1)
  vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
  check "--ssh-agent: no socket is mounted, and nothing outside an ssh session has an agent, even root" bash -c "
    [[ -n '$vm' ]] && ! grep -q backend_unreviewed '$work/last.log' &&
    ! docker container inspect -f '{{range .Mounts}}{{.Source}} {{end}}' '$vm' | grep -qiE '\.sock|ssh-agent|ssh-auth' &&
    docker exec -u 0 '$vm' sh -c 'test -z \"\${SSH_AUTH_SOCK:-}\" && ! ls /tmp/ssh-* >/dev/null 2>&1' </dev/null"
  check "... devvm's shell, an ssh session, lists your key" bash -c "d_only \"\$(d_shell_keys '$g')\" e2e-docker-user"
  check "... so does ssh by name" bash -c "d_only \"\$(d_ssh_keys devvm-$1)\" e2e-docker-user"
  check "ssh-agent add loads a throwaway key into devvm's own agent" bash -c "
    SSH_AUTH_SOCK='$ua' '$D' ssh-agent add '$work/d-dkey' </dev/null 2>&1 | grep -qF \"\$(m agent_holds keys=e2e-docker-devvm)\""
  check "ssh-agent devvm while it runs: devvm's next shell and ssh by name still list yours alone, until a restart" bash -c "cd '$g' &&
    SSH_AUTH_SOCK='$ua' DEVVM_MESSAGES=names '$D' ssh-agent devvm </dev/null >'$work/e' 2>&1 && grep -q 'agent_set_running mode=devvm vm=$vm\$' '$work/e' &&
    d_only \"\$(d_shell_keys '$g')\" e2e-docker-user && d_only \"\$(d_ssh_keys devvm-$1)\" e2e-docker-user"
  check "devvm, after a restart: the shell and ssh by name list devvm's key alone, never yours" bash -c "
    d_switch '$g' devvm && d_only \"\$(d_shell_keys '$g')\" e2e-docker-devvm && d_only \"\$(d_ssh_keys devvm-$1)\" e2e-docker-devvm"
  check "on, after a restart: yours alone" bash -c "
    d_switch '$g' on && d_only \"\$(d_shell_keys '$g')\" e2e-docker-user && d_only \"\$(d_ssh_keys devvm-$1)\" e2e-docker-user"
  check "off, after a restart: no agent, in the shell or by name" bash -c "
    d_switch '$g' off && d_none \"\$(d_shell_keys '$g')\" &&
    SSH_AUTH_SOCK='$ua' ssh -F '$sc/config' devvm-$1 'test -z \"\${SSH_AUTH_SOCK:-}\"' </dev/null"
  check "... your agent holds only its key, devvm's only its own" bash -c "
    u=\$(SSH_AUTH_SOCK='$ua' ssh-add -l 2>&1); d=\$(SSH_AUTH_SOCK='$da' ssh-add -l 2>&1); echo \"yours: \$u; devvm's: \$d\"
    [[ \$(grep -c . <<<\"\$u\") -eq 1 ]] && grep -q e2e-docker-user <<<\"\$u\" && ! grep -q e2e-docker-user <<<\"\$d\""
  (cd "$g" && quiet env SSH_AUTH_SOCK="$ua" "$D" rm --force --no-export)
  g=$(newproj "${1}on")
  (cd "$g" && printf 'exit\n' | SSH_AUTH_SOCK=$ua "$D" "${B[@]}" --no-network-policy --ports none base >"$work/last.log" 2>&1)
  vm=$(vm_of "$g"); [[ -z $vm ]] || created+=("$vm")
  check "a VM made without it: none; turned on while it runs, neither devvm's shell nor ssh by name has one until a restart; then both yours" bash -c "
    [[ -n '$vm' ]] && d_none \"\$(d_shell_keys '$g')\" &&
    (cd '$g' && SSH_AUTH_SOCK='$ua' '$D' ssh-agent on </dev/null >'$work/last.log' 2>&1) &&
    d_none \"\$(d_shell_keys '$g')\" && d_none \"\$(d_ssh_keys devvm-${1}on)\" &&
    d_switch '$g' on && d_only \"\$(d_shell_keys '$g')\" e2e-docker-user && d_only \"\$(d_ssh_keys devvm-${1}on)\" e2e-docker-user"
  (cd "$g" && quiet env SSH_AUTH_SOCK="$ua" "$D" rm --force --no-export)
  kill "$user_agent_pid" 2>/dev/null; user_agent_pid=""
  pkill -f "[s]sh-agent -a $da"
  rm -f "$work/d-key" "$work/d-key.pub" "$work/d-dkey" "$work/d-dkey.pub" "$ua" "$XDG_CONFIG_HOME/devvm/agent-keys"
}

# docker_clone NAME: a --clone VM, over its ssh. A throwaway repo, with a branch besides main and a
# file it doesn't track; host git without your settings.
docker_clone() {
  local c cgit
  section "docker: --clone"
  export GIT_CONFIG_GLOBAL=$sc/home/.gitconfig GIT_CONFIG_NOSYSTEM=1
  c=$(newproj "$1")
  git -C "$c" init -q -b main && echo a > "$c/a" && git -C "$c" add a && git -C "$c" -c user.email=e@e -c user.name=e commit -qm one &&
    git -C "$c" branch other && echo host-only > "$c/untracked"
  (cd "$c" && printf 'pwd; git branch --show-current\n' | "$D" "${B[@]}" --clone --no-network-policy --ports none base >"$work/last.log" 2>&1)
  vm=$(vm_of "$c"); [[ -z $vm ]] || created+=("$vm")
  cgit="GIT_SSH_COMMAND='ssh -F $dsc' git -C '$c'"
  check "the VM has its own clone in a volume of its own, every branch pushed in over ssh, nothing of the folder mounted, and the shell opens in it" bash -c "
    [[ -n '$vm' ]] && grep -qx mode=clone '$vms/$vm' && grep -qx '$c' '$work/last.log' && grep -qx main '$work/last.log' &&
    mounted '$vm' '$c' volume true && ! mounted '$vm' '$c' bind true &&
    [[ \$(in_d '$vm' \"git -C '$c' branch --format='%(refname:short)'\" | tr '\n' ' ') == 'main other ' ]] &&
    in_d '$vm' \"git -C '$c' log --oneline\" | grep -q one && in_d '$vm' \"[ ! -e '$c/untracked' ]\""
  check "the repo has a devvm remote, by the VM's name" bash -c "[[ \$(git -C '$c' remote get-url devvm) == '$vm:$c' ]]"
  in_d "$vm" "cd '$c' && echo b > b && git add b && git -c user.email=e@e -c user.name=e commit -qm from-vm" >/dev/null 2>&1
  check "fetching through devvm's ssh config brings the VM's commit" bash -c "$cgit fetch -q devvm && git -C '$c' log --oneline devvm/main | grep -q from-vm"
  check "pushing a branch into it works too" bash -c "git -C '$c' branch pushed other && $cgit push -q devvm pushed &&
    in_d '$vm' \"git -C '$c' rev-parse -q --verify refs/heads/pushed\" >/dev/null"
  check "rm --force deletes it, its clone's volume and the remote; the fetched work stays as a local branch" bash -c "cd '$c' && '$D' rm --force --no-export </dev/null >'$work/last.log' 2>&1 &&
    [[ -z \$(d_state '$vm') && ! -e '$vms/$vm' ]] && ! d_vol '$vm-clone' && ! git -C '$c' remote | grep -q devvm &&
    git -C '$c' log --oneline saved/devvm/main | grep -q from-vm"
  unset GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
}

# Network policy's emulation (devvm run as NU). Its guarantee: a VM with network policy reaches
# nothing but its proxy, on this machine, even as root: it's on an internal network of its own with
# nothing else but its relay, which passes its connections to the proxy's socket alone; no outside
# name resolves, and no DNS query leaves its network; the proxy follows its container's start and is
# gone when it stops; anything else devvm didn't set up shows it unenforced, and devvm stops it;
# tokens stay on this machine, out of the VM and its relay. A home of its own (a base VM reads no
# login; Docker's own config kept; on macOS the Keychain linked, as the network section does).
# dn_relay VM: the relay's settings, as Docker has them: the image, read-only, capabilities, user,
# networks, ports published, mounts. dn_nets NAME: a container's networks.
dn_nets() { docker container inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$1" 2>/dev/null | xargs; }
# dn_closed VM: every direct connection fails, even as root: an address, IPv6, a name (DNS), this
# machine's programs on its loopback and on every address (through Docker's gateway on Linux, its
# host address on Docker Desktop); the relay answers as the proxy does (407 without the password).
dn_closed() { local gw u
  for u in https://1.1.1.1 'https://[2606:4700:4700::1111]'; do
    if in_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null $u"; then echo "$u got through"; return 1; fi
  done
  if in_root "$1" "getent hosts example.com"; then echo "a name resolved"; return 1; fi
  if [[ $d_desktop == true ]]; then set -- "$1" http://192.168.65.254:18765/ http://host.docker.internal:18765/
  else gw=$(docker network inspect bridge -f '{{(index .IPAM.Config 0).Gateway}}'); set -- "$1" "http://$gw:18767/" "http://$gw:18765/"; fi
  for u in "${@:2}"; do
    if in_root "$1" "curl --noproxy '*' -sS -m 5 -o /dev/null $u"; then echo "$u got through"; return 1; fi
  done
  [[ $(in_root "$1" "curl --noproxy '*' -s -m 5 -o /dev/null -w %{http_code} http://devvm-proxy:$pp/") == 407 ]] || { echo "the relay"; return 1; }
}
# dn_open VM: on Docker's default network the same probes get out (their control).
dn_open() { in_root "$1" "curl --noproxy '*' -sS -m 10 -o /dev/null https://1.1.1.1" && in_root "$1" "getent hosts example.com" >/dev/null; }
# dn_proxy VM: the pid of its proxy on this machine, if it runs.
dn_proxy() { pgrep -f -- "[_]proxy -listen 127.0.0.1:$pp -vm $1 .* -listen-unix " 2>/dev/null | head -1; }
# dn_gone VM: its proxy is gone within 15 s.
dn_gone() { for _ in $(seq 15); do [[ -n $(dn_proxy "$1") ]] || return 0; sleep 1; done; echo "its proxy still runs"; return 1; }
# dn_confined VM: it runs on its own network alone, which is internal, gives this machine no address
# and holds nothing but it and its relay; its relay runs; its proxy runs.
dn_confined() { local n=$1-net
  [[ $(d_state "$1") == running && $(dn_nets "$1") == "$n" && $(d_state "$1-proxy") == running && -n $(dn_proxy "$1") ]] &&
  [[ $(docker network inspect -f '{{.Internal}} {{index .Options "com.docker.network.bridge.inhibit_ipv4"}} {{index .Labels "devvm.data"}}' "$n") == "true true $XDG_DATA_HOME/devvm" ]] &&
  [[ $(docker network inspect -f '{{range .Containers}}{{.Name}} {{end}}' "$n" | xargs -n1 | sort | xargs) == "$1 $1-proxy" ]] ||
  { echo "networks: $(dn_nets "$1"); relay $(d_state "$1-proxy"); proxy $(dn_proxy "$1")"; return 1; }
}
# dn_relay VM: the relay is devvm's Linux build, read-only, in an empty image of devvm's, its root
# read-only, with no capabilities and no new privileges, on the VM's network alone, publishing
# nothing, the proxy's socket its only other mount; the VM has no capability added and isn't
# privileged.
dn_relay() { local r=$1-proxy
  [[ $(docker container inspect -f '{{.HostConfig.ReadonlyRootfs}} {{.HostConfig.CapDrop}} {{.HostConfig.CapAdd}} {{.HostConfig.SecurityOpt}} {{.HostConfig.Privileged}} {{json .HostConfig.PortBindings}}' "$r") == \
     'true [ALL] [] [no-new-privileges] false {}' ]] || { docker container inspect -f '{{.HostConfig.ReadonlyRootfs}} {{.HostConfig.CapDrop}} {{.HostConfig.CapAdd}} {{.HostConfig.SecurityOpt}} {{.HostConfig.Privileged}} {{json .HostConfig.PortBindings}}' "$r"; return 1; }
  [[ $(dn_nets "$r") == "$1-net" && $(docker container inspect -f '{{.Config.Image}} {{.Path}} {{index .Config.Labels "devvm.kind"}}' "$r") == "devvm-$dns:proxy /devvm proxy" ]] &&
  (( $(docker image inspect -f '{{.Size}}' "devvm-$dns:proxy") < 4096 )) &&
  docker container inspect -f '{{range .Mounts}}{{.Destination}} {{.RW}}{{"\n"}}{{end}}' "$r" | grep -qx '/devvm false' &&
  [[ $(docker container inspect -f '{{len .Mounts}}' "$r") == 2 ]] &&
  [[ $(docker container inspect -f '{{.HostConfig.CapAdd}} {{.HostConfig.Privileged}}' "$1") == '[] false' ]] ||
  { docker container inspect -f '{{.Config.Image}} {{.Path}} {{.Mounts}} / {{.HostConfig.CapAdd}} {{.HostConfig.Privileged}}' "$r" "$1"; return 1; }
}
# dn_launch DIR [INPUT]: devvm in DIR as network policy's checks run it (in their home, nu_home), with
# INPUT for its shell.
dn_launch() { (cd "$1" && printf '%s\n' "${2:-exit}" | env HOME="$nu_home" DEVVM_MESSAGES=names "$D" >"$work/last.log" 2>&1); }
export -f dn_nets dn_closed dn_open dn_proxy dn_gone dn_confined dn_launch
# dn_secret VM DIR: a fake GitHub token, bound to it: the VM gets its own placeholder and never the
# token, nor does its relay; the proxy puts it in for its host; unbound and deleted at once.
dn_secret() { local salt events=$XDG_DATA_HOME/devvm/logs/$1.jsonl
  salt=$(sed -n 's/^proxy_salt=//p' "$vms/$1")
  if ! { echo ghp_E2EFAKE | "${NU[@]}" "$D" secret set github --stdin >/dev/null && (cd "$2" && "${NU[@]}" "$D" secret use github >/dev/null) &&
    dn_launch "$2"; }; then cat "$work/last.log"; return 1; fi
  # shellcheck disable=SC2016  # $GH_TOKEN is the VM's
  [[ $(in_d "$1" 'echo $GH_TOKEN') == devvm-proxy-managed-github-$salt ]] || { echo "no placeholder"; return 1; }
  if in_root "$1" "grep -rqsF ghp_E2EFAKE /etc /run /root /home /tmp /var/log || cat /proc/[0-9]*/environ 2>/dev/null | tr '\\0' '\\n' | grep -qF ghp_E2EFAKE" ||
    docker container inspect "$1" "$1-proxy" | grep -qF ghp_E2EFAKE; then echo "the token is in the VM or its relay"; return 1; fi
  # shellcheck disable=SC2016  # $GH_TOKEN is the VM's
  if ! in_d "$1" 'curl -sS -m 20 -o /dev/null https://api.github.com/user -H "Authorization: token $GH_TOKEN"' ||
    ! grep '"path":"/user"' "$events" | grep -q '"secrets":\["github"\]'; then echo "not put in"; return 1; fi
  (cd "$2" && "${NU[@]}" "$D" secret unuse github >/dev/null && "${NU[@]}" "$D" secret rm --force github >/dev/null) &&
  ! grep -q '"name":"github"' "$XDG_DATA_HOME/devvm/run/$1.secrets.json"
}
# dn_caps VM: its container is made without NET_RAW and given nothing: NET_RAW isn't in its bounding
# set, so even root opens no raw socket, IPv4 or packet (AF_PACKET).
dn_caps() { local c b
  c=$(docker container inspect -f '{{.HostConfig.CapDrop}} {{.HostConfig.CapAdd}} {{.HostConfig.Privileged}}' "$1")
  b=$(in_root "$1" "sed -n 's/^CapBnd:[[:space:]]*//p' /proc/1/status")
  echo "dropped, added, privileged: $c; bounding set: $b"
  [[ $c =~ ^\[(CAP_)?NET_RAW\]\ \[\]\ false$ && $b =~ ^[0-9a-f]+$ ]] && (( (0x$b >> 13 & 1) == 0 )) || return 1
  if in_root "$1" "perl -MSocket -e 'socket(S, PF_INET, SOCK_RAW, 1) or die qq(raw: \$!\\n)'"; then echo "a raw socket opened"; return 1; fi
  if in_root "$1" "perl -e 'socket(S, 17, 3, 768) or die qq(packet: \$!\\n)'"; then echo "a packet socket opened"; return 1; fi
}
# dn_net VM: its network is the sandbox's, internal, with inhibit_ipv4 and without IPv6.
dn_net() { local o
  o=$(docker network inspect -f '{{.Internal}} {{index .Options "com.docker.network.bridge.inhibit_ipv4"}} {{.EnableIPv6}} {{index .Labels "devvm.data"}}' "$1-net")
  echo "internal, inhibit_ipv4, IPv6, label: $o"; [[ $o == "true true false $XDG_DATA_HOME/devvm" ]]; }
# dn_members VM: its network's members, by ID, are the VM's container and its relay's, the one its
# record names.
dn_members() { local rec want got
  rec=$(sed -n 's/^ *"relay_id": "\([0-9a-f]*\)",*$/\1/p' "$XDG_DATA_HOME/devvm/docker/vms/$1.json")
  want=$(docker container inspect -f '{{.Id}}' "$1" "$1-proxy" | sort | xargs)
  got=$(docker network inspect -f '{{range $id, $c := .Containers}}{{$id}} {{end}}' "$1-net" | xargs -n1 | sort | xargs)
  echo "members: $got; the VM and its relay: $want; the record's relay: $rec"
  [[ -n $rec && -n $got && $got == "$want" && " $want " == *" $rec "* ]]; }
# dn_no_names VM: names don't resolve in it, even as root; its relay's does (the resolver's control).
dn_no_names() { local n
  for n in example.com github.com "devvm-e2e-$$-$RANDOM.example.com"; do
    if in_root "$1" "getent hosts $n"; then echo "$n resolved"; return 1; fi
  done
  in_root "$1" "getent hosts devvm-proxy" || { echo "the relay's name didn't resolve"; return 1; }; }
# dn_ipv6 VM: IPv6 in it: off on its network interface (disable_ipv6), and any other but the loopback
# (a kernel's fallback tunnels) down; ::1 its only address; root can't change either (/proc/sys is
# read-only, and it has no NET_ADMIN). What its network's bridge has on the engine's side (its IPv6
# addresses and disable_ipv6, from the capture's container, on the engine's own network) is printed
# and kept in $work/ipv6.log: without NET_RAW the VM sends nothing that reaches it over IPv6.
dn_ipv6() { local v br e w i off flags
  v=$(in_root "$1" 'for f in /proc/sys/net/ipv6/conf/*/disable_ipv6; do i=${f%/disable_ipv6}; i=${i##*/}
    case $i in lo|all|default) ;; *) echo "if $i $(cat "$f") $(cat "/sys/class/net/$i/flags")" ;; esac; done
    awk "{ print \"addr\", \$1, \$6 }" /proc/net/if_inet6
    echo 0 > /proc/sys/net/ipv6/conf/all/disable_ipv6 2>/dev/null && echo writable || echo read-only')
  br=br-$(docker network inspect -f '{{.Id}}' "$1-net" | cut -c1-12)
  e=$(docker exec "$d_cap" sh -c "awk -v b=$br '\$6 == b { print \$1 }' /proc/net/if_inet6; cat /proc/sys/net/ipv6/conf/$br/disable_ipv6" 2>&1 | xargs)
  echo "the VM's: $(xargs <<<"$v")"; echo "$br on the engine's side (addresses, disable_ipv6): $e" | tee -a "$work/ipv6.log"
  # The bridge's engine side: read (its disable_ipv6 last), and no IPv6 address but link-local ones.
  [[ $e =~ (^|\ )[01]$ ]] || { echo "the bridge's side unread"; return 1; }
  for w in ${e% *}; do [[ $w == "${e##* }" || $w == fe80* ]] || { echo "$br has $w"; return 1; }; done
  # Each other interface with IPv6 on is down (IFF_UP, 0x1, clear).
  while read -r w i off flags; do
    [[ $w == if && $off == 0 ]] || continue
    (( (flags & 1) == 0 )) || { echo "$i is up with IPv6 on"; return 1; }
  done <<<"$v"
  grep -q '^if eth[0-9]* 1 ' <<<"$v" && ! grep -q '^if eth[0-9]* 0 ' <<<"$v" &&
    [[ $(grep '^addr ' <<<"$v") == "addr 00000000000000000000000000000001 lo" ]] && grep -qx read-only <<<"$v"; }
# The capture: DNS (port 53, UDP and TCP) on every interface of the engine's own network namespace:
# this machine's (the docker host VM's) on a Linux engine, Docker Desktop's VM's there, where Docker's
# resolver would forward from and every bridge is. A container of the sandbox's (its label), on the
# host's network, from Alpine's image the run already has, tcpdump added from Alpine's packages;
# d_cap_stop deletes it, as does the end of the run.
d_cap=devvm-e2e-cap-$dns
# d_alpine: Alpine's image by the digest the Alpine base's record names (docker_ssh built it), so no
# tag is added to Docker Desktop's images; alpine:3 without one.
d_alpine() { local r f
  f=$(grep -lx 'from=alpine:3' "$images"/* 2>/dev/null | head -1)
  r=$([[ -z $f ]] || sed -n 's/^digest=//p' "$f")
  if [[ -n $r ]]; then f=${r%@*}; echo "${f%:*}@${r##*@}"; else echo alpine:3; fi; }
d_cap_start() { local _
  [[ -z $(d_state "$d_cap") ]] || docker container rm -f "$d_cap" >/dev/null 2>&1
  docker run -d --name "$d_cap" --label "devvm.data=$XDG_DATA_HOME/devvm" --network host "$d_alpine_ref" \
    sh -c 'apk add --no-cache -q tcpdump >/dev/null && exec tcpdump -p -n -l -i any -Z root port 53' >/dev/null || return 1
  for _ in $(seq 60); do docker logs "$d_cap" 2>&1 | grep -q 'listening on any' && return 0; sleep 1; done
  docker logs "$d_cap" 2>&1 | tail -5; return 1; }
d_cap_stop() { [[ -z $(d_state "$d_cap") ]] || docker container rm -f "$d_cap" >/dev/null 2>&1; }
# d_squat: a container of the sandbox's put on a VM's network by hand.
d_squat=devvm-e2e-squat-$dns
# The DNS probe, as root in the VM with a name prefix of the check's own ($1): its resolver, and
# queries straight to Docker's resolver (127.0.0.11), 1.1.1.1 and 8.8.8.8 (UDP, and TCP to 1.1.1.1)
# for TXT, ANY, AAAA, HTTPS and an EDNS0 A.
d_dns_probe='n=$1
# pkt NAME QTYPE [edns]: a query, as printf escapes.
pkt() { local q="" l t
  for l in ${1//./ }; do q+=$(printf "\\\\x%02x" "${#l}")$l; done
  t=$(printf "\\\\x%02x\\\\x%02x" $(($2 >> 8)) $(($2 & 255)))
  if [[ -n ${3:-} ]]; then
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x01$q\\x00$t\\x00\\x01\\x00\\x00\\x29\\x10\\x00\\x00\\x00\\x00\\x00\\x00\\x00"
  else
    printf "%s" "\\x12\\x34\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x00$q\\x00$t\\x00\\x01"
  fi; }
send() { local p len a; p=$(pkt "$@")
  for a in 127.0.0.11 1.1.1.1 8.8.8.8; do printf "$p" 2>/dev/null > /dev/udp/$a/53; done 2>/dev/null
  len=$(printf "$p" | wc -c)
  { printf "\\x00\\x$(printf %02x "$len")"; printf "$p"; } | timeout 3 bash -c "cat > /dev/tcp/1.1.1.1/53" 2>/dev/null; }
getent hosts "$n-a.example.com" >/dev/null; echo "resolver rc=$?"
send "$n-b.example.com" 16; send "$n-c.example.com" 255; send "$n-d.example.com" 28; send "$n-e.example.com" 65
send "$n-f.example.com" 1 edns
getent ahosts "$n-g.example.org" >/dev/null; echo "resolver rc=$?"
true'
printf '%s\n' "$d_dns_probe" > "$work/d-dns-probe.sh"
# dn_dns VM: the DNS probe in it, then what the capture saw of its names: lines after "seen:".
dn_dns() { local n=devvm-e2e-$$-$RANDOM
  docker exec -i -u 0 "$1" bash -s -- "$n" < "$work/d-dns-probe.sh" 2>&1 | sed 's/^/probe: /'
  sleep 3; echo "capture: $(d_state "$d_cap")"; docker logs "$d_cap" 2>&1 | grep -iF "$n" | sed 's/^/seen: /'; }
# dn_dns_kept VM: none of the probe's queries left the VM's network, while the capture ran, and its
# resolver answered none.
dn_dns_kept() { local s; s=$(dn_dns "$1"); echo "$s"
  [[ $(grep -c '^probe: resolver rc=2$' <<<"$s") == 2 ]] && grep -qx 'capture: running' <<<"$s" && ! grep -q '^seen: ' <<<"$s"; }
# dn_dns_left VM: the probe's lookup through its resolver is seen leaving (the capture's control).
dn_dns_left() { local s; s=$(dn_dns "$1"); echo "$s"
  grep -qx 'capture: running' <<<"$s" && grep '^seen: ' <<<"$s" | grep -qiF -- '-a.example.com'; }
# dn_ping VM: ping works as the VM's user, through ICMP datagram sockets (no NET_RAW): iputils' ping
# installed as root from the distribution's packages, then a ping to Docker's default network's
# gateway.
dn_ping() {
  in_root "$1" 'export DEBIAN_FRONTEND=noninteractive; apt-get update -q >/dev/null 2>&1; apt-get install -y -q iputils-ping >/dev/null 2>&1' ||
    { echo "iputils-ping didn't install"; return 1; }
  in_d "$1" 'echo "ping_group_range: $(cat /proc/sys/net/ipv4/ping_group_range)"; ls -l "$(command -v ping)"
    gw=$(awk '\''$2 == "00000000" { print $3; exit }'\'' /proc/net/route)
    gw=$(printf "%d.%d.%d.%d" 0x${gw:6:2} 0x${gw:4:2} 0x${gw:2:2} 0x${gw:0:2}); ping -c 1 -W 5 "$gw"'; }
export -f dn_caps dn_net dn_members dn_no_names dn_ipv6 dn_dns dn_dns_kept dn_dns_left dn_ping d_cap_stop
export d_cap d_squat
docker_netpolicy() {
  local np nh
  section "docker: network policy"
  export DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
  nh=$work/net-home; rm -f "$nh/Library/Keychains"; rm -rf "$nh"; mkdir -p "$nh"
  if [[ "$(uname -s)" == Darwin ]]; then mkdir -p "$nh/Library" && ln -s "$HOME/Library/Keychains" "$nh/Library/Keychains"; fi
  NU=(env HOME="$nh")
  export nu_home=$nh
  # This machine's own services: on its loopback (a devvm _proxy, which answers 407) and on every address.
  echo listener-key > "$work/listener.auth"
  "$D" _proxy -listen 127.0.0.1:18765 -auth-file "$work/listener.auth" >/dev/null 2>&1 & listener=$!
  d_hostserve 0.0.0.0 18767
  d_alpine_ref=$(d_alpine); export d_alpine_ref
  check "a capture of DNS runs on the engine's own network: a container of the sandbox's, on the host's network" d_cap_start
  np=$(newproj "$1")
  ps_watch
  (cd "$np" && printf 'exit\n' | "${NU[@]}" DEVVM_MESSAGES=names "$D" "${B[@]}" --network-policy --ports none --no-git-protect base >"$work/last.log" 2>&1)
  ps_unwatch
  vm=$(vm_of "$np"); [[ -z $vm ]] || created+=("$vm")
  pp=$(sed -n 's/^proxy_port=//p' "$vms/$vm" 2>/dev/null)
  export pp d_desktop
  check "--network-policy: the VM runs on an internal network of its own, beside its relay alone, its proxy runs, and the self-check passed" bash -c "
    [[ -n '$vm' && -n '$pp' ]] && grep -qx net=on '$vms/$vm' && grep -qx net_check=ok '$vms/$vm' &&
    ! grep -q backend_unreviewed '$work/last.log' || { tail -5 '$work/last.log'; false; }"
  check "... so" dn_confined "$vm"
  check "the relay: devvm's Linux build, read-only, in an empty image, no capabilities, on the VM's network alone, publishing nothing; the VM gets no capability" dn_relay "$vm"
  check "the VM's container is made without NET_RAW: not in its bounding set, and root opens no raw or packet socket" dn_caps "$vm"
  check "its network is internal, with inhibit_ipv4, without IPv6, and the sandbox's" dn_net "$vm"
  check "its network's members, by ID, are the VM and the relay its record names, alone" dn_members "$vm"
  # The key is read inside the check, never in its own arguments, which ps would see.
  check "the proxy's password was in no host process's arguments while devvm launched it, nor in the relay's; the VM's proxy settings have it" bash -c "
    key=\$(sed -n 's/^proxy_key=//p' '$vms/$vm') && { [[ -n \$key && -s '$work/ps.log' ]] || { echo no key or nothing sampled; false; }; } &&
    { ! grep -qFf <(echo \"\$key\") '$work/ps.log' || { echo in a process while devvm launched it; false; }; } &&
    { ! ps -axww -o command= | grep -qFf <(echo \"\$key\") || { echo in a process now; false; }; } &&
    { ! docker container inspect -f '{{.Args}} {{.Config.Env}}' '$vm-proxy' | grep -qFf <(echo \"\$key\") || { echo in the relay; false; }; } &&
    { [[ \$(in_d '$vm' 'printenv HTTPS_PROXY') == \"http://devvm:\$key@devvm-proxy:$pp\" ]] || { echo not in the VM; false; }; }"
  check "an allowed host works, through the proxy" in_d "$vm" "curl -fsS -m 20 -o /dev/null https://github.com"
  check "another host is blocked by the proxy, and this machine's loopback too (internal addresses need a rule)" bash -c "
    [[ \$(in_d '$vm' 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]] &&
    [[ \$(in_d '$vm' 'curl -s -m 20 -o /dev/null -w %{http_code} --noproxy \"\" --proxy \"\$HTTP_PROXY\" http://127.0.0.1:18765/') == 403 ]]"
  check "direct connections fail, even as root: an address, IPv6, a name, this machine's programs; the relay is the proxy (407)" dn_closed "$vm"
  check "no name resolves in it, even as root; its relay's does" dn_no_names "$vm"
  check "no DNS query leaves its network: none of the probe's names in the capture (resolver, 127.0.0.11, 1.1.1.1, 8.8.8.8; UDP and TCP)" dn_dns_kept "$vm"
  check "IPv6: off on its every interface but the loopback, which root can't change; ::1 its only address; its bridge's side link-local alone" dn_ipv6 "$vm"
  echo "  recorded: $(tail -1 "$work/ipv6.log" 2>/dev/null)"
  check "devvm network log shows the proxy's block" bash -c "cd '$np' && '$D' network log >'$work/e' 2>&1; grep -q '  BLOCKED  example.com:80  ' '$work/e' || { cat '$work/e'; false; }"
  check "devvm network allow applies at once to its running proxy, and rm takes it back" bash -c "cd '$np' &&
    '$D' network allow example.com </dev/null >/dev/null && in_d '$vm' 'curl -fsS -m 20 -o /dev/null https://example.com' &&
    '$D' network rm example.com </dev/null >/dev/null && [[ \$(in_d '$vm' 'curl -s -m 20 -o /dev/null -w %{http_code} http://example.com/') == 403 ]]"
  check "a token bound to it stays on this machine: the VM gets its own placeholder, the proxy puts the token in" dn_secret "$vm" "$np"
  check "with its proxy gone nothing gets out; launched, it gets its proxy back, through the same relay" bash -c "
    p=\$(dn_proxy '$vm') && [[ -n \$p ]] && kill \$p && dn_gone '$vm' &&
    ! in_d '$vm' 'curl -fsS -m 10 -o /dev/null https://github.com' && cd '$np' &&
    printf 'exit\n' | ${NU[*]} '$D' >'$work/last.log' 2>&1 && [[ -n \$(dn_proxy '$vm') ]] && in_d '$vm' 'curl -fsS -m 20 -o /dev/null https://github.com'"
  check "on another network too (docker network connect): unenforced, and its next launch stops it and opens nothing" bash -c "
    docker network connect bridge '$vm' && '$D' ls | grep -Eq '^$vm +Running +base +\S+ +unenforced +- +docker ' && cd '$np' &&
    ! printf 'echo e2e-opened\n' | ${NU[*]} DEVVM_MESSAGES=names '$D' >'$work/last.log' 2>&1 &&
    grep -qx 'devvm: net_unenforced_stop vm=$vm' '$work/last.log' && ! grep -q e2e-opened '$work/last.log' && [[ \$(d_state '$vm') == exited ]]"
  check "devvm starts it confined again: its own network alone, and the self-check passes" bash -c "
    dn_launch '$np' && grep -qx net_check=ok '$vms/$vm'"
  check "... so" dn_confined "$vm"
  check "its relay replaced by another container by its name, on its network: unenforced, and its next launch stops it; the one after starts it confined" bash -c "
    docker container rm -f '$vm-proxy' >/dev/null && docker run -d --name '$vm-proxy' --label 'devvm.data=$XDG_DATA_HOME/devvm' --label devvm.kind=proxy \
      --network '$vm-net' '$d_alpine_ref' sleep 600 >/dev/null && '$D' ls | grep -Eq '^$vm +Running +base +\S+ +unenforced +- +docker ' &&
    ! dn_launch '$np' 'echo e2e-opened' && grep -qx 'devvm: net_unenforced_stop vm=$vm' '$work/last.log' && [[ \$(d_state '$vm') == exited ]] &&
    dn_launch '$np' && dn_confined '$vm' && dn_members '$vm'"
  check "docker stop outside devvm: its proxy follows it, and is gone" bash -c "docker stop -t 1 '$vm' >/dev/null && dn_gone '$vm'"
  check "docker start outside devvm: unenforced, and its next launch stops it" bash -c "
    docker start '$vm' >/dev/null && '$D' ls | grep -Eq '^$vm +Running +base +\S+ +unenforced +- +docker ' &&
    ! dn_launch '$np' 'echo e2e-opened' && grep -qx 'devvm: net_unenforced_stop vm=$vm' '$work/last.log' && [[ \$(d_state '$vm') == exited ]]"
  check "another container on its network: its next launch is refused in one line naming the network, before the VM starts; once it's gone, it starts" bash -c "
    docker run -d --name '$d_squat' --label 'devvm.data=$XDG_DATA_HOME/devvm' --network '$vm-net' '$d_alpine_ref' sleep 600 >/dev/null &&
    ! dn_launch '$np' 'echo e2e-opened' && grep -qx 'devvm: docker_network_shared name=$vm-net' '$work/last.log' &&
    [[ \$(grep -c '^devvm: ' '$work/last.log') -eq 1 && \$(d_state '$vm') == exited ]] && ! grep -q e2e-opened '$work/last.log' &&
    docker container rm -f '$d_squat' >/dev/null && dn_launch '$np' && dn_confined '$vm' || { cat '$work/last.log'; false; }"
  check "devvm stop stops it, its relay and its proxy" bash -c "dn_launch '$np' && cd '$np' && '$D' stop </dev/null >/dev/null 2>&1 &&
    [[ \$(d_state '$vm') == exited && \$(d_state '$vm-proxy') == exited ]] && dn_gone '$vm'"
  check "devvm network off: its next start is on Docker's default network alone, without a relay, network or proxy, nor proxy settings" bash -c "cd '$np' &&
    ${NU[*]} '$D' network off </dev/null >/dev/null 2>&1 && dn_launch '$np' && [[ \$('$D' network | head -1) == off && \$(dn_nets '$vm') == bridge ]] &&
    [[ -z \$(d_state '$vm-proxy') ]] && ! d_net '$vm-net' && [[ -z \$(dn_proxy '$vm') ]] && ! in_root '$vm' 'grep -qiE \"^(https?|no|all)_proxy=\" /etc/environment'"
  check "... where the direct probes get out (their control)" dn_open "$vm"
  check "... and the capture sees its lookup leave (the DNS check's control)" dn_dns_left "$vm"
  check "... and ping works as the VM's user, without NET_RAW (ICMP datagram sockets)" dn_ping "$vm"
  check "devvm network on: devvm restart starts it confined again, and direct connections fail" bash -c "cd '$np' &&
    ${NU[*]} '$D' network on </dev/null >/dev/null 2>&1 && printf 'exit\n' | ${NU[*]} '$D' restart >'$work/last.log' 2>&1 &&
    [[ \$('$D' network | head -1) == on ]] && grep -qx net_check=ok '$vms/$vm' && dn_confined '$vm' && dn_closed '$vm'"
  check "rm --force deletes it, its relay, its network, its proxy and its state" bash -c "cd '$np' && ${NU[*]} '$D' rm --force --no-export </dev/null >/dev/null 2>&1 &&
    [[ -z \$(d_state '$vm') && -z \$(d_state '$vm-proxy') && ! -e '$vms/$vm' ]] && ! d_net '$vm-net' && dn_gone '$vm'"
  kill "$listener" "${d_hostprogs[@]}" 2>/dev/null; wait "$listener" "${d_hostprogs[@]}" 2>/dev/null; listener="" d_hostprogs=()
  d_cap_stop
  rm -f "$work/listener.auth" "$nh/Library/Keychains"; rm -rf "$nh"
}

# docker_emulated PREFIX: every emulation's checks, its folders named from PREFIX.
docker_emulated() {
  d_emu_begin
  docker_ssh "${1}ssh"
  docker_ports "${1}ports"
  docker_agent "${1}agent"
  docker_clone "${1}cloned"
  d_emu_end
}

# docker_linux: the checks on this machine's engine, a Linux one.
docker_linux() {
  local others_before kb fixture token kf kx rc
  # alpine:3, which docker_ssh builds a base from, and Ubuntu's pinned image, which base starts from,
  # pulled first: Docker keeps the images devvm pulls.
  docker image pull -q alpine:3 >/dev/null 2>&1
  docker image pull -q "docker.io/library/ubuntu@$ubuntu_pin" >/dev/null 2>&1
  others_before=$(d_others)
  section "docker: shared tool versions"
  check "the shared tools resolve to upstream's newest" quiet "$D" _refresh-tools

  section "docker: images"
  check "devvm doctor finds everything, an engine of this machine's, builds confined, network policy emulated" bash -c "
    DEVVM_MESSAGES=names '$D' doctor --backend docker </dev/null >'$work/last.log' 2>&1 &&
    grep -qE '^docker_engine_linux version=' '$work/last.log' && grep -qE '^build-confinement +native ' '$work/last.log' &&
    grep -qE '^egress-proxy +emulated ' '$work/last.log' && '$D' doctor --backend docker --json </dev/null | grep -qxF '  \"problems\": [],'"
  check "build base" quiet env DEVVM_MESSAGES=names "$D" "${B[@]}" build base
  kb=$(sed -n 's/^==> building_image image=base key=\([0-9a-f]*\)$/\1/p' "$work/last.log")
  [[ -n $kb ]] || kb=$(for k in $(keys_of base); do grep -qx from= "$images/$k" && d_img "devvm-$dns:$k" && echo "$k"; done | head -1)
  check "base is an image of devvm's, recorded with Ubuntu's container image by its pinned digest; its build container is gone" bash -c "
    [[ -n '$kb' && -n '$ubuntu_pin' ]] && d_img 'devvm-$dns:$kb' && grep -qx libc=gnu '$images/$kb' &&
    grep -qE '^digest=docker\.io/library/ubuntu:[0-9.]+@$ubuntu_pin$' '$images/$kb' && [[ -z \$(d_state devvm-img-$kb-$dns) ]]"
  # Build confinement: servers on this machine, on its loopback and on every address, and a layer that
  # tries both through the build container's gateway.
  python3 -m http.server -b 127.0.0.1 47190 >/dev/null 2>&1 & d_servers+=($!)
  python3 -m http.server -b 0.0.0.0 47191 >/dev/null 2>&1 & d_servers+=($!)
  sleep 1
  fixture=$layers/e2edock.sh token=e2e-docker-ran-$$-$(date +%s)
  cat > "$fixture" <<EOF
#!/bin/bash
gw=\$(awk '\$2 == "00000000" { print \$3; exit }' /proc/net/route)
gw=\$(printf '%d.%d.%d.%d' 0x\${gw:6:2} 0x\${gw:4:2} 0x\${gw:2:2} 0x\${gw:0:2})
curl -sS -m 5 -o /dev/null "http://\$gw:47190/" && echo e2e-loopback-reached || echo e2e-loopback-refused
curl -sS -m 5 -o /dev/null "http://\$gw:47191/" && echo e2e-every-reached || echo e2e-every-refused
curl -sS -m 20 -o /dev/null https://api.github.com && echo e2e-internet-reached || echo e2e-internet-refused
echo $token
EOF
  check "a layer builds from base's image, its script run as the VM's user" bash -c "'$D' ${B[*]} build e2edock </dev/null >'$work/last.log' 2>&1 &&
    grep -qx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
  owned e2edock
  check "... confined: it reached the internet, and this machine only on every address, never its loopback" bash -c "
    f=\$(grep -lx '$token' '$XDG_DATA_HOME'/devvm/logs/runs/*.log | tail -1) && grep '^e2e-' \"\$f\" && grep -qx e2e-internet-reached \"\$f\" &&
    grep -qx e2e-every-reached \"\$f\" && grep -qx e2e-loopback-refused \"\$f\""
  kill "${d_servers[@]}" 2>/dev/null; d_servers=()
  kf=$(keys_of e2edock | head -1)
  check "devvm images lists both on docker" bash -c "out=\$('$D' images </dev/null 2>/dev/null) &&
    grep -qE '^base +ubuntu [0-9.]+ +[0-9a-f]{12} +$kb +current +[0-9.]+[KMGT] +docker$' <<<\"\$out\" &&
    grep -qE '^e2edock +ubuntu [0-9.]+ +[0-9a-f]{12} +$kf +current +[0-9.]+[KMGT] +docker$' <<<\"\$out\""
  echo "# edited" >> "$fixture"
  check "rebuild makes a new one and deletes the stale one, not base" bash -c "'$D' ${B[*]} rebuild --force e2edock </dev/null >/dev/null 2>&1 &&
    [[ \$(keys_of e2edock | wc -l) -eq 1 && \$(keys_of e2edock) != $kf ]] && ! d_img 'devvm-$dns:$kf' && d_img 'devvm-$dns:$kb'"
  owned e2edock
  check "images rm deletes it" bash -c "'$D' ${B[*]} images rm --force e2edock </dev/null >/dev/null 2>&1 && [[ -z \$(keys_of e2edock) ]]"
  rm -f "$fixture"
  # The user's CAs: trusted while a build's scripts run, kept by no image (a container of the image,
  # the sandbox's, with no network, shows what it holds).
  e2e_ca
  mkdir -p "$cafolder"; cp "$cad/ca.pem" "$cafolder/e2e.pem"
  catoken=e2e-dock-ca-$$-$(date +%s)
  printf '#!/bin/bash\nset -euo pipefail\n%s\n%s\necho %s\n' \
    '[[ $NODE_EXTRA_CA_CERTS == /etc/ssl/devvm.pem && $SSL_CERT_FILE == /etc/ssl/certs/ca-certificates.crt ]]' \
    'openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt /etc/ssl/devvm.pem' "$catoken" > "$layers/e2edockca.sh"
  check "a build trusts the CAs in devvm's CA folder while its scripts run" bash -c "'$D' ${B[*]} build e2edockca </dev/null >'$work/last.log' 2>&1 &&
    grep -qx '$catoken' '$XDG_DATA_HOME'/devvm/logs/runs/*.log"
  owned e2edockca
  kc=$(keys_of e2edockca | head -1)
  rm -rf "$cafolder"
  check "its image keeps none of them" bash -c "[[ -n '$kc' ]] && d_img 'devvm-$dns:$kc' &&
    docker run --rm --network none --label 'devvm.data=$XDG_DATA_HOME/devvm' --entrypoint sh 'devvm-$dns:$kc' -c \
      '! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 && [ ! -e /etc/ssl/devvm.pem ] && [ ! -e /var/lib/devvm-ca ] &&
       ! grep -qF \"\$1\" /etc/ssl/certs/ca-certificates.crt' _ '$(sed -n 4p "$cad/ca.pem")'"
  rm -f "$layers/e2edockca.sh"
  printf '#!/bin/bash\nexit 3\n' > "$layers/e2edockfail.sh"
  DEVVM_MESSAGES=names "$D" "${B[@]}" build e2edockfail </dev/null >"$work/fail.log" 2>&1; rc=$?
  kx=$(sed -n 's/^==> building_image image=e2edockfail key=\([0-9a-f]*\)$/\1/p' "$work/fail.log")
  [[ -z $kx ]] || created+=("devvm-img-$kx")
  check "a failed script keeps its container, stopped, and says how to look inside" bash -c "
    [[ -n '$kx' && $rc -eq 1 ]] && grep -qE '^devvm: docker_script_failed script=e2edockfail\.sh vm=devvm-img-$kx-$dns$' '$work/fail.log' &&
    [[ \$(d_state devvm-img-$kx-$dns) == exited ]] && '$D' ${B[*]} images rm --force '$kx' </dev/null >/dev/null 2>&1 && [[ -z \$(d_state devvm-img-$kx-$dns) ]]"
  rm -f "$layers/e2edockfail.sh"

  section "docker: a base project"
  docker_project dweb
  section "docker: agent folders"
  docker_agents dagents
  section "docker: shared tools"
  docker_tools dtools
  section "docker: the docker layer"
  docker_layer dlayer
  docker_emulated d
  docker_netpolicy dnet
  section "docker: links between VMs"
  link_section docker dlk
  check "nothing without the sandbox's label was touched" bash -c "[[ \"\$(d_others)\" == '$others_before' ]]"
}

# dh ARGS: limactl on the docker host VM's own Lima home, from outside the repo.
dh_home=${DEVVM_E2E_DOCKER_LIMA_HOME:-$HOME/.lt-e2e-go-docker}
dh() { (cd /tmp && LIMA_HOME=$dh_home limactl "$@"); }
# Sysbox, which the docker host VM's engine runs the docker layer's VMs under: its release's package,
# pinned.
sysbox_version=0.7.1
sysbox_sha256_arm64=04ca894ae0b53f0fa54eaacc173ce40363c9a95ea5450f773716a84ef650a69b
sysbox_sha256_amd64=9d6d5484f980d0a17f86c492c1262015c2afb66280bdb97215b79fde6a0261c5
# dockerhost: the docker host VM, made the first time (Ubuntu with Docker's engine from its archive,
# this checkout mounted read-only, you in the docker group, Sysbox) and kept, running.
dockerhost() {
  mkdir -p "$dh_home"
  if ! dh list -q 2>/dev/null | grep -qx dockerhost; then
    dh create --name=dockerhost --tty=false --vm-type=vz --cpus=6 --memory=8 --disk=60 --containerd=none \
      --mount-only "$root" template:ubuntu-26.04 >"$work/dockerhost.log" 2>&1 || return 1
  fi
  [[ $(dh list --format '{{.Status}}' dockerhost) == Running ]] || dh start --tty=false dockerhost >>"$work/dockerhost.log" 2>&1 || return 1
  if ! dh shell --workdir / dockerhost sh -c 'command -v docker >/dev/null && id -nG | grep -qw docker' </dev/null; then
    dh shell --workdir / dockerhost sudo sh -c 'apt-get update -q && DEBIAN_FRONTEND=noninteractive apt-get install -y -q \
      docker.io netcat-openbsd jq perl curl git python3 && usermod -aG docker "$SUDO_USER"' </dev/null >>"$work/dockerhost.log" 2>&1 &&
      dh stop dockerhost >>"$work/dockerhost.log" 2>&1 && dh start --tty=false dockerhost >>"$work/dockerhost.log" 2>&1 || return 1
  fi
  dockerhost_sysbox; }
# dockerhost_sysbox: Sysbox in the docker host VM, installed once: its package, checked against its
# pinned SHA-256, and (Ubuntu 25.04 on) fusermount3's AppArmor profile allowing Sysbox's FUSE mounts.
dockerhost_sysbox() {
  dh shell --workdir / dockerhost sh -c "docker info --format '{{json .Runtimes}}' | grep -q sysbox-runc" </dev/null && return 0
  dh shell --workdir / dockerhost sudo env v="$sysbox_version" arm64="$sysbox_sha256_arm64" amd64="$sysbox_sha256_amd64" bash -c '
    set -eu; a=$(dpkg --print-architecture); case $a in arm64) sum=$arm64 ;; amd64) sum=$amd64 ;; *) exit 1 ;; esac
    d=$(mktemp -d); trap "rm -rf \"$d\"" EXIT
    curl -fsSLo "$d/sysbox.deb" "https://github.com/nestybox/sysbox/releases/download/v$v/sysbox-ce_$v.linux_$a.deb"
    echo "$sum  $d/sysbox.deb" | sha256sum -c
    DEBIAN_FRONTEND=noninteractive apt-get install -y -q "$d/sysbox.deb"
    if [ -f /etc/apparmor.d/fusermount3 ]; then
      printf "%s\n" "mount fstype=fuse options=(nosuid,nodev) options in (ro,rw) -> /var/lib/sysboxfs/**/," \
        "umount /var/lib/sysboxfs/**/," > /etc/apparmor.d/local/fusermount3
      apparmor_parser -r /etc/apparmor.d/fusermount3
    fi' </dev/null >>"$work/dockerhost.log" 2>&1; }
# in_dh CMD: CMD in the docker host VM.
in_dh() { dh shell --workdir / dockerhost bash -c "$1"; }

# docker_linux_vm: this script's Linux checks, run in the docker host VM with devvm built here for it;
# its results count here.
docker_linux_vm() {
  local arch out np nf
  section "docker: a Linux engine, in the docker host VM ($dh_home)"
  if ! dockerhost; then echo "  the docker host VM didn't start: see $work/dockerhost.log"; fail=$((fail + 1)); failed+=("docker host VM"); return; fi
  arch=$(cd "$root" && "${gocmd[@]}" env GOARCH)
  mkdir -p "$work/linux"
  if ! (cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=$arch "${gocmd[@]}" "${testbuild[@]}" -o "$work/linux/devvm" ./cmd/devvm) ||
    ! dh copy "$work/linux/devvm" dockerhost:/tmp/devvm-e2e-bin >/dev/null; then
    echo "  couldn't build devvm for it"; fail=$((fail + 1)); failed+=("devvm for the docker host VM"); return
  fi
  in_dh "DEVVM_E2E_BIN=/tmp/devvm-e2e-bin bash '$root/tests/e2e.sh' --backend docker $($clean && echo --clean)" </dev/null 2>&1 |
    tee "$work/linux.log" | while IFS= read -r l; do echo "  linux | $l"; done
  out=$(grep -E '^e2e: [0-9]+ passed, [0-9]+ failed' "$work/linux.log" | tail -1)
  np=$(sed -E 's/^e2e: ([0-9]+) passed.*/\1/' <<<"$out") nf=$(sed -E 's/^e2e: [0-9]+ passed, ([0-9]+) failed.*/\1/' <<<"$out")
  if [[ -z $out ]] || grep -q '^e2e: FAILED' "$work/linux.log"; then nf=$((${nf:-0} + 1)); fi
  pass=$((pass + ${np:-0})) fail=$((fail + ${nf:-0}))
  (( ${nf:-0} == 0 )) || failed+=("the Linux engine's (see $work/linux.log)")
}

# dd_confined_build: a layer built on Docker Desktop reached the internet, through its proxy, and
# not this machine's loopback: not directly (Desktop's host address, its name), nor through the
# proxy (127.0.0.1, which its floor refuses); its container had no NET_RAW.
dd_confined_build() {
  local fixture=$layers/e2eddock.sh token f
  token=e2e-ddocker-ran-$$-$(date +%s)
  cat > "$fixture" <<EOF
#!/bin/bash
curl -sS -m 5 -o /dev/null --noproxy '*' http://192.168.65.254:18768/ && echo e2e-host-reached || echo e2e-host-refused
curl -sS -m 5 -o /dev/null --noproxy '*' http://host.docker.internal:18768/ && echo e2e-name-reached || echo e2e-name-refused
curl -fsS -m 5 -o /dev/null http://127.0.0.1:18768/ && echo e2e-proxied-reached || echo e2e-proxied-refused
curl -fsS -m 20 -o /dev/null https://api.github.com && echo e2e-internet-reached || echo e2e-internet-refused
getent hosts example.com >/dev/null && echo e2e-dns-reached || echo e2e-dns-refused
b=\$(sed -n 's/^CapBnd:[[:space:]]*//p' /proc/self/status)
(( (0x\$b >> 13 & 1) == 0 )) && echo e2e-no-net-raw || echo e2e-net-raw
echo $token
EOF
  "$D" "${B[@]}" build e2eddock </dev/null >"$work/last.log" 2>&1 || { tail -5 "$work/last.log"; return 1; }
  owned e2eddock
  f=$(grep -lx "$token" "$XDG_DATA_HOME"/devvm/logs/runs/*.log | tail -1) && cat "$f" | grep '^e2e-'
  grep -qx e2e-internet-reached "$f" && grep -qx e2e-host-refused "$f" && grep -qx e2e-name-refused "$f" &&
    grep -qx e2e-proxied-refused "$f" && grep -qx e2e-dns-refused "$f" && grep -qx e2e-no-net-raw "$f" &&
    [[ -z $(d_state "devvm-img-$(keys_of e2eddock)-$dns-proxy") ]] && ! d_net "devvm-img-$(keys_of e2eddock)-$dns-net"
}

# dd_pulled: the images bases started from, by the digests their records name, which devvm pulled:
# Docker keeps them (devvm never deletes an image without its label).
dd_pulled() { local r repo
  sed -n 's/^digest=//p' "$images"/* 2>/dev/null | sort -u | while read -r r; do
    repo=${r%@*}; repo=${repo%:*}
    docker image inspect -f '{{.Id}}' "$repo@${r##*@}" 2>/dev/null | sed 's/^/image /'
  done | sort -u; }
# dd_untouched BEFORE: what Docker has without the sandbox's label is BEFORE, but for the images
# bases started from, pulled (dd_pulled).
dd_untouched() { local now gone added
  now=$(d_others); gone=$(comm -23 <(echo "$1") <(echo "$now")); added=$(comm -13 <(echo "$1") <(echo "$now"))
  echo "pulled: $(dd_pulled | xargs)"; echo "gone: ${gone:-none}"; echo "added: ${added:-none}"
  [[ -z $gone && -z $(comm -23 <(echo "$added" | grep .) <(dd_pulled)) ]]; }
export -f dd_pulled dd_untouched

# docker_desktop: the checks on Docker Desktop, yours: only what has the sandbox's label is touched,
# and it's all deleted at the end; the images bases start from, pulled by digest, are kept, as devvm
# keeps them. Its builds are confined through a proxy of devvm's each.
docker_desktop() {
  local others_before
  section "docker: Docker Desktop"
  d_desktop=true
  others_before=$(d_others)
  d_purge
  rm -rf "$images" "$share" "$lock" "$XDG_DATA_HOME/devvm/docker"
  check "devvm doctor finds Docker Desktop, the folders it shares, devvm's Linux build; builds and network policy emulated" bash -c "
    DEVVM_MESSAGES=names '$D' doctor --backend docker </dev/null >'$work/last.log' 2>&1 &&
    grep -qE '^docker_engine_desktop version=' '$work/last.log' && grep -qE '^docker_file_sharing dirs=' '$work/last.log' &&
    grep -qE '^build-confinement +emulated ' '$work/last.log' && grep -qE '^egress-proxy +emulated ' '$work/last.log' &&
    ! grep -q docker_linux_build '$work/last.log'"
  check "the shared tools resolve to upstream's newest" quiet "$D" _refresh-tools
  check "build base, through a proxy of its own, which is gone at the end" bash -c "DEVVM_MESSAGES=names '$D' ${B[*]} build base </dev/null >'$work/last.log' 2>&1 &&
    ! grep -q backend_unreviewed '$work/last.log' &&
    k=\$(keys_of base) && d_img devvm-$dns:\$k && [[ -z \$(d_state devvm-img-\$k-$dns) && -z \$(d_state devvm-img-\$k-$dns-proxy) ]] &&
    ! d_net devvm-img-\$k-$dns-net && ! pgrep -f -- '[_]proxy .* -vm devvm-img-\$k-$dns ' >/dev/null || { tail -5 '$work/last.log'; false; }"
  d_hostserve 127.0.0.1 18768
  check "... confined: a layer reaches the internet through it, and not this machine's loopback, directly or through it, nor a name; no NET_RAW" dd_confined_build
  kill "${d_hostprogs[@]}" 2>/dev/null; wait "${d_hostprogs[@]}" 2>/dev/null; d_hostprogs=()
  rm -f "$layers/e2eddock.sh"
  docker_project ddesk
  local x; x=$(newproj ddeskx)
  launch "$x" exit "${B[@]}" --no-network-policy --ports none base
  check "Exec's arguments, folder, input and user reach the VM as given" bash -c "[[ -n '$vm' ]] && cd '$root' &&
    DEVVM_E2E_VM=$vm CGO_ENABLED=0 ${gocmd[*]} test -count=1 -v -run '^TestE2EExecQuoting\$' ./internal/backend/docker |
    grep -q -- '--- PASS: TestE2EExecQuoting'"
  (cd "$x" && quiet "$D" rm --force --no-export)
  docker_agents ddagents
  docker_tools ddtools
  docker_layer ddlayer
  docker_emulated dd
  docker_netpolicy ddnet
  section "docker: links between VMs, on Docker Desktop"
  link_section docker ddlk
  d_purge
  check "all with the sandbox's label is deleted, and nothing else was touched (but the images bases started from, pulled)" bash -c "
    [[ \$(d_ours) == 0 ]] && dd_untouched '$others_before'"
  d_desktop=false
}

if [[ "$(uname -s)" == Linux && " $engines " == *" linux "* ]]; then docker_linux
elif [[ " $engines " == *" linux "* ]]; then docker_linux_vm; fi
[[ " $engines " != *" desktop "* ]] || docker_desktop
fi

# A script error can end a section early without failing a check: compare with the last complete run
# (same options). Fewer checks than last time fails the run, and the timings aren't kept.
short=""
if [[ $total != "?" ]] && (( ncheck < total )); then
  short="ran $ncheck checks, but the last complete run ran $total: a section ended early (look for errors above)"
else
  mv "$timings.new" "$timings"   # a complete run: the next one estimates from it
fi
echo
echo "e2e: $pass passed, $fail failed$( (( skipped == 0 )) || echo ", $skipped skipped") in $(mins $((SECONDS - started)))${failed[*]+: ${failed[*]}}"
[[ -z $short ]] || echo "e2e: FAILED: $short"
(( fail == 0 )) && [[ -z $short ]]
