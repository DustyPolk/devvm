---
title: Design notes
weight: 103
---

For contributors: why devvm is built the way it is, what was tried and dropped, and what's still open. How each part works is on its own page; the layout and rules are in [AGENTS.md](https://github.com/w3cj/devvm/blob/main/AGENTS.md).

## Emulated features

A backend feature devvm builds from other parts of the backend ([emulated](backends.md#features)) gets a note here before its review: what it's built from, the guarantee it keeps, what could break it, and the end-to-end checks that prove it. Then the threat check, then it's marked reviewed. Every feature Lima has is native.

### Network policy on smolvm

**The guarantee:** as on Lima, a VM with network policy reaches nothing but its own proxy, even as root, and a refused DNS query never leaves the host, but for one narrow channel, a query for `devvm.invalid` (the DNS channel, under Not covered below). Reviewed: the threat check's findings are fixed or listed below, and the end-to-end checks prove the rest. Before that, devvm refused it, and a test build's `DEVVM_UNREVIEWED` let the end-to-end checks through ([testing](testing.md#unreviewed-emulations)).

**What it's built from**, all set by `Start` while the machine is stopped:

- **An allow list of the gateway alone** (`--reset-allow-list --allow-cidr 100.96.0.1/32`): smolvm refuses TCP to any other address, IPv6 included, and UDP but DNS. The gateway is every port of the host's loopback, so the allow list alone isn't enough.
- **DNS closed** (`--allow-host-pattern devvm.invalid`): smolvm's resolver, which takes every DNS query on port 53 whatever its address (UDP or TCP), answers NXDOMAIN itself for any other name, without asking anyone. `--dns` would be simpler, but `machine update` can't change it.
- **devvm's proxy as the decider** (`machine start --mediated-egress --egress-interceptor 127.0.0.1:PORT`, its token in `SMOLVM_INTERCEPTOR_TOKEN`): before smolvm connects anywhere, it sends the proxy's decider (`devvm _proxy -decider`, on a host loopback port of its own) the connection's destination and first bytes. The decider takes over only connections to the proxy's own address in the VM (`100.96.0.1:<proxy port>`): the same connection then carries it, and the proxy serves it as any other, password and policy included. It refuses every other connection the allow list lets through (another port of the host's), logged as `blocked` (`TCP`, refused). smolvm connects nowhere itself for a VM's TCP.
- **The token**: 32 random bytes, new at each start, kept in the VM's state and a file of the proxy's (mode 600), never in a process's arguments, never in the VM. It goes one way only: smolvm sends it to the decider at the start of each connection, and the decider proves nothing back. So it protects the decider from others: a program on the host that connects to the decider's port without it gets no answer, and can't have the proxy take over a connection of its own as the VM's (the decider reads nothing past the token without it, and at most 8 KiB of first bytes with it). It doesn't protect smolvm from a fake decider (below). A proxy restarted while the VM runs gets the same port and token.
- **Confined** compares the machine's `agent.pid` (its process and start time) with the one recorded after devvm's start through the decider, checks that process runs, and that the allow lists smolvm reports are still the ones above: smolvm changes them on a running machine (`smolvm machine update --allow-cidr`), which makes the VM `unenforced`, and its next launch stops it. A start without network policy removes the allow list and the decider requirement (`--net --reset-allow-list --no-egress-interceptor`).

**What it relies on:** smolvm's virtio-net gateway enforcing the allow list, the mediated-egress decision (protocol version 2) before any upstream connection and failing closed when the decider is gone or answers badly, its resolver's hostname filter, and the requirement it records: once a machine started through a decider, smolvm refuses a start without one (`smolvm machine start` by itself fails). None of it is in the VM, so root there can't change it.

**What could break it:** a smolvm release that changes the protocol, stops recording the requirement, or lets a connection through before the decision (each fails the end-to-end checks); a program on the host that takes the decider's port while the proxy is down: smolvm sends it the token and lets it decide. Protocol version 2 has a third verdict, `0`, allow direct, which a squatter could give, but smolvm refuses a direct connection to the host (`direct_target_policy`), and the allow list admits nothing else; what a squatter can do is take connections over (`2`) and carry them anywhere itself (checked: such a decider answers the probe), as on Lima a program that takes the proxy's port gets the VM's traffic. A decider that answers garbage, or nothing, refuses (checked). It starts before the VM, so it's down only while it's restarted (new rules, a new devvm) or once it fails; a start that finds a port taken, or fails, stops the VM (`net_proxy_down_stopped`), on Lima too, but whatever took the port meanwhile decided for it until then. Someone on the host who starts the machine with a decider of their own isn't confined: devvm reports it `unenforced` and stops it.

**Not covered:**

- smolvm's JSON (`machine ls`, `machine status`) shows only the allowed CIDRs and host patterns, not its UDP allows (`--allow-udp`), deny lists or egress rules (API only), and `--reset-allow-list` leaves UDP allows in place (`--remove-allow-udp` needs each one by name). Someone on the host who adds a UDP allow, live or not, lets the VM send UDP there, past the decider, and devvm can't see it (checked: one added live lets an NTP request out). Only the host's user can. A UDP allow of the gateway still doesn't reach the host's loopback.
- ICMP to the gateway (ping) passes; to anywhere else it gets no answer (checked; smolvm's own network doesn't forward ICMP either, so that check has no control).
- **A DNS channel:** a query for `devvm.invalid` itself is forwarded to smolvm's upstream resolver, by default the host's first nameserver, usually on the network. The name is fixed, but the VM chooses when to ask, the query type, the name's case and EDNS options: a narrow way out to whoever sees that resolver's traffic. It can't be closed from here: smolvm takes only valid DNS names as patterns and forwards every allowed one, `localhost` included (with a black-hole upstream, `devvm.invalid`, `localhost`, `localhost.localdomain` and `local` all waited on it), and the upstream is set only by `machine create --dns`, for every query, with policy or without. Closing it would take `--dns` at create pointing at a resolver devvm runs (port 53 on the host's loopback, which only root can take; `0.0.0.0:53` is open to users but to the network too), or a VM's network policy fixed when it's made.
- The proxy's own limits ([Not covered](network.md#not-covered)). Linux hosts are untested.

**End-to-end checks** (`tests/e2e.sh --backend smolvm`):

- the VM runs behind its proxy and decider and passes the self-check; its allow list and DNS are as above; an allowed host works through the proxy, others get `403`;
- direct connections fail even as root (an address, a port of an allowed host, IPv6, a service on the host's loopback through the gateway); on smolvm's own network the same probes get out, their control;
- DNS fails at once. While the VM resolves names of its own (through its resolver, and straight to the gateway's resolver and `1.1.1.1` over UDP and TCP: A, TXT, ANY, AAAA, HTTPS, EDNS0, a name under `devvm.invalid`), smolvm's process has no flow beyond the host's loopback (nettop, macOS), and smolvm's own decision log (`egress-decisions.jsonl`) refuses every name; `DEVVM.INVALID`, sent too, is the channel's own name and is let through, as nothing but `devvm.invalid` is. nettop is blind to queries smolvm forwards under a hostname filter (it asks through the system's resolver), so the log is the detector, and a query for `devvm.invalid`, which it does forward, is the control that the log sees one. On smolvm's own network, nettop sees the probe's queries leave;
- UDP and ICMP: datagrams to a host loopback listener through the gateway, to `1.1.1.1` on 443 and 123, and an NTP request reach nothing, and a ping to `1.1.1.1` gets no answer; a UDP allow added live lets the NTP request out, and on smolvm's own network nettop sees the datagrams leave (their controls);
- the token is in no host process's arguments and nowhere in the VM, and the proxy's password in no host process's arguments; `devvm log` shows the proxy's blocks and the refused connections to the host; `devvm policy allow` and `rm` apply at once;
- an allow list widened while the VM runs makes it `unenforced`, and devvm stops it, then starts it confined;
- with the proxy gone, or a decider of nobody's that answers garbage, nothing or allow direct, nothing gets out (the host's loopback listener included), while one that takes connections over answers the probe (its control), and `smolvm machine start` refuses the machine; started with someone else's decider, devvm stops it; `devvm network off` and `on` switch it to smolvm's own network and back.

## smolvm

Tried with smolvm 1.32.2 on an arm64 Mac before devvm had the backend ([its column](backends.md#features), [its page](smolvm.md)). The backend has its native features and network policy, emulated ([above](#network-policy-on-smolvm)); the other emulations are to come, each refused until reviewed.

**Network policy.** smolvm's allow lists work per address, not per port, and DNS has its own path:

- With `--allow-cidr` of the host's gateway (`100.96.0.1`), the VM reaches every port on the host's loopback (ssh, local databases), not just the proxy's, and nothing else.
- Under any allow list, the gateway answers DNS for any name, even queries sent to `8.8.8.8`: a way out for data. `--allow-host-pattern` of a name that doesn't exist makes it answer NXDOMAIN, and then smolvm's process sends no query anywhere (checked with nettop: on its own network it queries the host's resolver itself). `--dns 127.0.0.1` makes it time out, but only `machine create` sets it.
- `--mediated-egress` with `--egress-interceptor` hands each connection the allow list admits to a host process, with its address, port and first bytes: it allows, denies, or takes the connection over. A test decider that took over only the proxy's port reached the proxy, and nothing else got through. It fails closed: with the decider gone, or a wrong token, connections are reset, and once started so, the VM won't start without one.
- A connection the decider will refuse still opens in the VM (smolvm collects its first bytes first), then resets: only data, not a connect, shows it's refused.

**How the backend fits the interface:**

- **Tool images:** uncompressed EROFS, since smolvm's kernel can't mount compressed ones. The tools build machine runs `tools/build.sh` as on Lima, then makes each image again without compression (`plain.sh`), so `build.sh` and Lima's tool keys stay as they are. A backend names its format (`ToolFormat`, `plain-<plain.sh's hash>`), which a tool image's key hashes when it isn't build.sh's own; one share holds both kinds.
- **No init system:** the image runs as a container in a microVM. While a build's scripts run, `systemctl` and `rc-update` are stand-ins that do nothing; after each start devvm makes the loop devices, mounts the tools and starts Docker (`services.sh`). Docker's storage is `-v /var/lib/docker`, on the machine's own disk. A project VM's setup tells a running systemd by `/run/systemd/system`, not by `systemctl` being there (Docker's packages install it).
- **Ubuntu's container image, not its cloud image:** before the scripts, base gets `ca-certificates`, `sudo`, `bash` and the VM's user (`prepare.sh`). The user has the host's name and uid, as on Lima: virtiofs shows the project folder as the host's uid, so `ubuntu` (uid 1000) couldn't write it; a user with that uid is removed first. `Built.Origin` is the image by its digest. The backend's own scripts that go into base are in its key (`KeyFiles`).
- **Network at create:** packs carry the build machine's network, CPUs and memory, and `--block-private` can't be removed from them, so builds use smolvm's default network and `CreateVM` sets everything. The network changes on a stopped VM (`machine update`), so `Start` sets it where it differs. For network policy, the decider's address and token come with `Confinement` (a backend with a decider implements `backend.Decider`), and the proxy is at the gateway.
- **ssh-agent forwarding is chosen at create:** `machine update` can't change it, so turning it on or off later is a feature of its own (`agent-toggle`). The agent socket is read at each start.
- **Every port** isn't what smolvm forwards (at most 64, servers on all addresses), so `ports-all` is a feature of its own, which new VMs ask for by default.
- **Lifeline:** smolvm's `agent.pid` has a second line, the start time, so the proxy reads only the first.
- **Ports** are set on a stopped VM, so `Confinement.SSHPort` will be a forward set before the start (`ssh`, to come).
- **State:** `--state-dir` is Linux only, so on macOS devvm's machines share the user's smolvm, told apart by a label naming devvm's data folder; devvm acts only on machines with it. smolvm keeps a local usage journal; devvm turns it off (`SMOLVM_USAGE=0`).

## Shared tools

Each tool version is one compressed EROFS image, loop-mounted read-only in each VM from a read-only share ([details](images.md#shared-tools)).

- **Measured (macOS):** ten tools take 0.85 GB as images (1.5 GB installed), once for every VM and mix. Start times from an image match a local disk warm (`node -e 0` 13 ms vs 14 ms, `claude --version` 3 ms), and are 10–30 ms slower cold. Mounting five images takes 0.15 s; VM start is unchanged.
- **Why images, not a shared folder:** tools run from a virtiofs folder were about 3× slower warm (`node -e 0` 38 ms, `claude --version` 30 ms), the Mac's case-insensitive disk collided 33 file names in Python's terminfo, and mise's version links failed in a read-only folder.
- **Why not per-tool layers:** an image chain for every mix of languages and agents repeats each agent's files in every mix.
- **Instead of Nix:** Nix and NixOS were weighed and rejected: NixOS breaks prebuilt binaries, and a hidden Nix shows through read-only-store errors.

## Linux hosts: overlays instead of copies

Lima on QEMU keeps one qcow2 disk per VM, and a clone copies it (2.0 GB for `base`). A project VM's disk is a qcow2 overlay of its image's instead ([details](lima.md#clones-and-overlays)): 196 KB new, 12 MB after booting, the same speed except fsync writes (1.8–2.5 s vs 1.2–2.1 s per GiB). Flattening one (`qemu-img rebase -b ""`) took 8 s, if it's ever needed.

## devvm's own Lima home

Lima merges `_config/default.yaml` and `override.yaml` of its home into every VM. In `~/.lima`, a `networks: [{vzNAT: true}]` there gave a VM with network policy a second network card with a default route around its proxy (the self-check caught it), and an extra mount went unnoticed. Hence a Lima home of devvm's own, which refuses those files and any network devvm didn't give a VM ([details](lima.md#a-lima-home-of-its-own)). The costs: plain `limactl list` doesn't show devvm's VMs, and VM names are shorter, as Lima's ssh socket path is at most 104 characters.

## Failed attempts

- **Claude's host login in VMs:** on macOS it's in the Keychain, which can't be shared. VMs use a `claude setup-token` token instead. The first token saved was the code the browser shows, so `devvm auth claude` refuses one without the `sk-ant-oat01-` prefix.
- **krunkit (Lima on libkrun) to return freed memory:** it didn't visibly return any.
- **Debian's arm64 cloud kernel for `--from`:** it has no `gpio_keys`, so it ignored Lima's stop request; Lima hard-killed the VM after 30 s, and node, bun and dockerd in the image came out corrupted. `--from` uses the generic kernel.
- **Every layer as a Docker build stage, flattened:** it worked, but each image became its own full disk (3.7 GB for `node`).
- **Shared read-only layers:** each Docker layer as an EROFS file, stacked as `/` with overlayfs over each VM's own disk by an initramfs of devvm's own. It worked (`node` took 1.0 GB of layers instead of a 1.8 GB disk), but devvm owned the boot path (the initramfs, an exit initramfs for clean shutdowns, kernel extraction), and cold program starts were 10–50 ms slower. Shared tools, and overlays on Linux, give the same savings without it.
- **A read-only mount for `.git/hooks`:** root in the VM can unmount it and write hooks through the project mount underneath. Locking `.git` itself (`uappnd`) breaks git, and a locked placeholder `.git/commondir` makes git fail. Hence `chflags uchg` on hooks and config, plus detection ([details](git-protection.md)).
- **Kernel extraction trying gzip first:** Ubuntu's arm64 kernel is an EFI executable with a zstd payload.
- **`mke2fs -d` from a tarball:** it needed libarchive and a UTF-8 locale. A directory works.

## Things learned

- **Lima `vz` never returns freed guest memory to the Mac.** sbx returns it with a VM monitor of its own; libkrun has `MADV_FREE`, with a stronger fix still an open PR; smolvm's libkrun fork includes it.
- **APFS clones** cost nothing until written, and `du` overcounts them: real use was 5.5 GB where `du` said 12.7 GB.
- **Lima locks each additional disk to one running VM,** so images shared across VMs go through a shared folder (virtiofs on macOS, 9p on Linux) and are loop-mounted from it.
- **Read-only mounts are enforced by the host:** even root in the VM can't write to them, but root can unmount a nested one and reach what's underneath.
- **Codex and opencode refreshing their OAuth logins in a VM can rotate the token and sign the host out.** With network policy the VM holds only placeholders and the proxy refreshes on the host (`internal/proxy/logins.go`). Codex decodes `id_token`'s claims and the access token's `exp` without checking signatures, so placeholders are unsigned JWTs with the real account claims and a far-off `exp`.
- **No existing tool** gives Lima Dockerfile-style layers with sharing at run time; smolvm, nerdbox and Kata do it on VM runtimes of their own.
- **Where devvm fits,** next to sbx, smolvm and Colima: sbx puts agents first; devvm gives a full Linux VM per project, with agents optional.

## Open work and risks

User-facing limits are in [Known limits](limits.md) and [what the security model doesn't protect](security.md#what-it-doesnt-protect).

- **`--from`:** a moved tag is noticed only at the next build, not when listing images; devvm doesn't log in to registries, so private images work only through Docker; registries over plain HTTP (`localhost:5000`) aren't supported.
- **Alpine:** `rc-status` as a normal user shows `acpid` and `syslog` as crashed, as their pid files are root's (`sudo rc-status` is clean).
- **x86_64 hosts** are untested on real hardware; on Intel Macs, `--from`'s kernel is untested with `vz`.
- **Linux hosts:** the QEMU wrapper depends on the shape of Lima's `-netdev user,id=net0,…` argument. If Lima changes it, the wrapper refuses to start QEMU rather than run it unrestricted.
- **Network policy, not built:**
  - tokens for services devvm doesn't know: a host, a header, a format and an environment variable;
  - method and path rules on decrypted hosts, such as read-only GitHub, or no `git-receive-pack` unless approved;
  - approving blocked hosts as they're hit;
  - Claude's `/login` OAuth, refreshed in the proxy (`setup-token` works, so maybe never).
- **Network policy, risks of the mechanism:**
  - `sandbox-exec` is deprecated. If Apple removes it, macOS needs another way to confine the hostagent: an egress filter upstream (gvisor-tap-vsock#609 and #631; Lima's `hostLoopback: false`, lima#5581), a user-v2 daemon of devvm's own (lima#5464), or QEMU with the Linux mechanism on macOS too (it works there, slower than `vz`). The self-check catches it breaking silently.
  - VMs started outside devvm (`limactl start`, `limactl shell` starting a stopped VM, Lima's start at login) aren't confined: devvm reports them `unenforced`. An `override.yaml` setting a dead `HTTPS_PROXY` might harden this (untested).
  - A Lima upgrade that adds a hostagent connection fails closed: the start hangs until Lima gives up (10 minutes in the experiment). devvm should time-box the start and say it's the sandbox. That includes unix sockets: the profile allows only those in the VM's Lima folder and the forwarded agent's. Allowing every unix socket would let a compromised hostagent reach `docker.sock` or another ssh agent.
  - Tunnels the proxy doesn't read are decided by the `CONNECT` host and checked against the TLS server name, so domain fronting through a shared CDN may still reach other hosts.
