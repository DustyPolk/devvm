# Security

devvm's job is to keep what runs in a VM away from your machine. The [security model](https://devvm.dev/docs/internals/security/) explains what devvm protects, how it does it, and what it doesn't protect.

## Report a vulnerability

Report a vulnerability privately through [GitHub's vulnerability reporting](https://github.com/w3cj/devvm/security/advisories/new), not in a public issue. Include the devvm version from `devvm --version`, the host OS, the Lima version and the steps to reproduce it.

## Verifying a release

Each release's `SHA256SUMS` is signed with minisign, and the public key is `packaging/minisign.pub`. The install script checks the signature when minisign is installed. The script and the key in it come from the same release, though, so that check can't catch a release that someone replaced whole. Homebrew's checksum can, because it's kept in a separate repository. [The signature and what it trusts](https://devvm.dev/docs/internals/install/#the-signature-and-what-it-trusts) has the details.

To check a release by hand, run these in the folder with its files:

```sh
minisign -Vm SHA256SUMS -p packaging/minisign.pub
shasum -a 256 -c --ignore-missing SHA256SUMS
```

## What counts

Anything that breaks the security model counts, for example:

- Code in a VM reaching the host, by writing outside its project folder, changing `.git` hooks or config while they're locked, or running code on the host.
- A VM reaching another VM's files, or devvm's state in `~/.local/share/devvm` and `~/.config/devvm`.
- Secrets, logins or SSH keys reaching a VM that should have only a placeholder.
- A VM with network policy reaching a host that isn't allowed, the host's own services, or private addresses.
- Credentials or host files ending up in an image or a shared tool image.

## Out of scope

- What the security model lists under [What it doesn't protect](https://devvm.dev/docs/internals/security/#what-it-doesnt-protect), such as what allowed hosts and secrets permit. That includes VMs without network policy, which a VM gets when you answer no or pass `--no-network-policy`.
- Escapes through Lima, QEMU or Apple's Virtualization framework themselves, which you should report upstream. Do tell us if devvm uses them in an unsafe way.
