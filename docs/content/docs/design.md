---
title: Design notes
weight: 103
---

For contributors: why devvm is built the way it is, what was tried and dropped, and what's still open. How each part works is on its own page; the layout and rules are in [AGENTS.md](https://github.com/w3cj/devvm/blob/main/AGENTS.md).

## Emulated features

A backend feature devvm builds from other parts of the backend ([emulated](backends.md#features)) gets a note here before its review: what it's built from, the guarantee it keeps, what could break it, and the end-to-end checks that prove it. Then the threat check, then it's marked reviewed. None so far: every feature Lima has is native.

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
