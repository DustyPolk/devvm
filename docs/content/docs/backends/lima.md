---
title: Lima
weight: 1
---

[Lima](https://lima-vm.io) is devvm's default backend. Each project gets a full VM, which runs on Apple's Virtualization framework (`vz`) on macOS and on QEMU on Linux.

## Requirements

- You need Lima 2.0 or newer, with `limactl` on your `PATH`. See [Lima's install guide](https://lima-vm.io/docs/installation/), or install it with Homebrew: `brew install lima`.
- The host runs macOS 13 or newer, or Linux, on arm64 or x86_64.
- On Linux, you need QEMU from your package manager: `qemu-system-aarch64` (or `qemu-system-x86_64`) and `qemu-img`.
- On Linux, you also need to be able to read and write `/dev/kvm`. That means virtualization is turned on in the firmware (or nested virtualization, in a VM), and your user is in the `kvm` group.

Every command that needs Lima checks these first and stops at the first one that's missing. `devvm doctor` lists them all, each with the command that installs it where there's one to run. That's Homebrew's command for Lima when you have Homebrew, your package manager's for QEMU (apt, dnf, pacman or zypper), and `sudo usermod -aG kvm $USER` for `/dev/kvm`, after which you log in again. devvm never runs them itself ([details](../getting-started/install.md#checking-what-devvm-needs)).

## Running limactl

`devvm limactl ARGS...` runs `limactl` with ARGS on devvm's Lima home, in devvm's place. Its input, output, terminal and exit code are `limactl`'s, and devvm passes every option on to it, `--help` included. `devvm help limactl` shows devvm's own help for the command.

```sh
devvm limactl list
devvm limactl shell --start devvm-img-<key>
```

- It runs from devvm's data folder, as devvm's own `limactl` commands do, so give files by their absolute path.
- A VM you start this way, with `devvm limactl start` or `shell --start`, runs unconfined, without its network policy, and `devvm ls` shows it as `unenforced` ([confinement](../internals/lima.md#confinement)). Start project VMs with devvm.

## Limits

- **Folder names in VM names are cut short.** Lima keeps each VM's ssh socket in the VM's folder in its home, and a socket path has at most 104 characters. A long Lima home leaves less room for the name, so set `DEVVM_LIMA_HOME` to a shorter folder if devvm says the home is too long.
- **Paths can't contain commas.** Lima and QEMU split mount options on commas, so devvm refuses to mount a project folder whose path has one, and it leaves out an agent's folder whose path has one, with a warning.
- **Disks can grow but not shrink.**
- **Mounts on Linux are 9p.** A 9p share mounts once per guest.
- **A start Lima reports as failed** carries on with a warning when the VM is running but degraded, because one of Lima's boot scripts failed. A VM that doesn't answer over ssh within a minute is stopped instead, and its console log is in its Lima folder (`serial*.log`).
