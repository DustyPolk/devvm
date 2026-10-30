# Security

devvm's job is to keep what runs in a VM away from your machine. The [security model](https://devvm.dev/docs/internals/security/) says what it protects and what it doesn't.

## Report a vulnerability

Report privately through [GitHub's vulnerability reporting](https://github.com/w3cj/devvm/security/advisories/new), not in a public issue. Include the devvm version (`devvm --version`), the host OS and Lima version, and the steps to reproduce.

## Verifying a release

Releases' `SHA256SUMS` are signed with minisign; the public key is `packaging/minisign.pub`. The install script checks it when minisign is installed, but the script and the key in it come from the same release, so that check can't catch a release replaced whole; Homebrew's checksum, kept in a separate repository, can. To check by hand: `minisign -Vm SHA256SUMS -p packaging/minisign.pub`, then `shasum -a 256 -c --ignore-missing SHA256SUMS`.

## What counts

Anything that breaks the security model, for example:

- code in a VM reaching the host: writing outside its project folder, changing `.git` hooks or config while they're locked, or running code on the host;
- a VM reaching another VM's files, or devvm's state in `~/.local/share/devvm` and `~/.config/devvm`;
- a real token, login or SSH key reaching a VM that should only have a placeholder;
- a VM with network policy reaching a host that isn't allowed, the host's own services, or private addresses;
- credentials or host files ending up in an image or a shared tool image.

## Out of scope

- What the [security model](https://devvm.dev/docs/internals/security/#what-it-doesnt-protect) lists under "What it doesn't protect", such as what allowed hosts and tokens permit, or VMs without network policy.
- Escapes through Lima, QEMU or Apple's Virtualization framework themselves: report those upstream. Do tell us if devvm uses them in an unsafe way.
