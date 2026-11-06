# Security

devvm's job is to keep what runs in a VM away from your machine. The [security model](https://devvm.dev/docs/internals/security/) explains what devvm protects, how it does it, and what it doesn't protect.

## Report a vulnerability

Report a vulnerability privately through [GitHub's vulnerability reporting](https://github.com/devvmhq/devvm/security/advisories/new), not in a public issue. Include the devvm version from `devvm --version`, the host OS, the Lima version and the steps to reproduce it.

## Verifying a release

Each release's `SHA256SUMS` lists the checksums of its archives and of its `install.sh`, and it's signed with minisign. The release key's public half, which is also in `packaging/minisign.pub`, is:

```text
RWSjzFsn7ETaijKJ2y7F4DtZobXmyv+LmdclJwZP/KmAkLUwOCzWF/2b
```

The install script checks the signature when minisign is installed. The script and the key in it come from the same release, though, so that check can't catch a release that someone replaced whole. Homebrew's checksum can, because it's kept in a separate repository, and so can your own check with the key above, because it doesn't come from the release. [The signature and what it trusts](https://devvm.dev/docs/internals/install/#the-signature-and-what-it-trusts) has the details.

To check a release by hand, run these in the folder with its files:

```sh
minisign -Vm SHA256SUMS -P RWSjzFsn7ETaijKJ2y7F4DtZobXmyv+LmdclJwZP/KmAkLUwOCzWF/2b
shasum -a 256 -c --ignore-missing SHA256SUMS
```

[Checking the install script](https://devvm.dev/docs/internals/install/#checking-the-install-script) shows how to download the script, `SHA256SUMS` and its signature, check them, then run the script.

## What counts

Anything that breaks the security model counts, for example:

- Code in a VM reaching the host, by writing outside its project folder, changing `.git` hooks or config while they're locked, or running code on the host.
- A VM reaching another VM's files, or devvm's state in `~/.local/share/devvm` and `~/.config/devvm`.
- Secrets, logins or SSH keys reaching a VM that should have only a placeholder.
- A VM with network policy reaching a host that isn't allowed, the host's own services, or private addresses.
- Credentials or host files ending up in an image or a shared tool image.

## Out of scope

- What [Attack vectors](https://devvm.dev/docs/safety/attack-vectors/) lists under [Narrowed, not closed](https://devvm.dev/docs/safety/attack-vectors/#narrowed-not-closed) and [Left to you](https://devvm.dev/docs/safety/attack-vectors/#left-to-you), such as what allowed hosts and secrets permit. That includes open VMs, which a VM is when you choose the open tier or pass `--network open`.
- Escapes through Lima, QEMU or Apple's Virtualization framework themselves, which you should report upstream. Do tell us if devvm uses them in an unsafe way.
