---
title: Known limits
explainer: What devvm can't do, and where
weight: 18
---

These are the things devvm can't do, everywhere, on Linux hosts and on Intel Macs.

## Everywhere

- **devvm runs on macOS and Linux only,** not on Windows ([requirements](../getting-started/install.md#requirements)).
- **Shared tools are read-only,** so a tool can't update itself or add components, for example with `rustup component add` ([details](images.md#shared-tools)).
- **A layer goes in and comes out by making the VM again.** A layer's script changes the VM's disk, so `devvm layer add` gives a VM more layers by making it again, and everything on its disk but its home folder is lost ([details](images.md#adding-layers-to-a-vm)). `devvm layer rm` takes layers out in the same way ([details](images.md#taking-layers-out-of-a-vm)). Shared tools go in with `devvm tools add` and come out with `devvm tools rm`, and the VM's disk stays as it is ([details](toolchains.md#adding-and-removing-tools)).
- **`--from` takes only Debian-, Ubuntu- and Alpine-based images,** and Alpine, Debian 12 (`node:24`) and Ubuntu before 26.04 on arm64 build with unpinned packages ([details](from.md#supported-images)).
- **Agent folders** that you make or delete on this machine reach a running VM only after a restart, which devvm offers ([details](coding-agents.md#mounts)).
- **Ports** are forwarded over TCP only, and forwarding every port means every port from 1024 up. You can tunnel UDP over a forwarded TCP port with socat ([details](ports-size.md#ports)).
- **Links between VMs** carry TCP over IPv4 alone, and a container in a VM reaches them only with `--network host` ([details](links.md#ports)).
- **Tools that ignore the proxy settings** reach a confined VM's proxy only over HTTPS and HTTP to a host by name, on ports 443 and 80 and up to 32 more that your allow rules name. Other ports, protocols that name no host in their first bytes, such as a database's, addresses in URLs, HTTP/3 alone and IPv6 alone need the tool's own proxy settings, and a client that doesn't trust the VM's CA sees a certificate error for a blocked HTTPS host ([details](../safety/network.md#tools-that-ignore-the-proxy)).
- **Changing a running VM's ports** closes the connections that are open through its forwards ([details](ports-size.md#ports)).
- **With Lima,** project paths can't contain commas, folder names in VM names are cut short, and disks grow but never shrink ([details](../backends/lima.md#limits)).
- **File watchers in a Lima VM** don't see the edits you make on this machine, only those made in the VM, so set a dev server that should reload on your edits to poll the project folder ([details](../backends/lima.md#file-watchers)).

## Linux hosts

- `.git` protection is read-only in the VM's view, not a lock on the host's files, so after a git command on this machine replaces `.git/config` while the VM runs, the VM can change or replace the new file until devvm locks it again. devvm names each such file when the VM stops ([details](../safety/git-protection.md#when-git-on-this-machine-replaces-a-locked-file)). A VM whose `.git/config` or hook has another name doesn't start, and the VM can't rename a file between the working tree and `.git` ([details](../safety/git-protection.md#linux-hosts)).
- Where the kernel doesn't let devvm use a user namespace, such as on Ubuntu 23.10 and newer, there's no `.git` protection. devvm warns about it at each launch, and it names the `.git` hooks and config that changed, but only after the fact, so use `--clone` there ([details](../safety/git-protection.md#when-linux-cant-lock)).
- VMs mount the project folder with 9p instead of virtiofs, so file watchers in a VM also miss the VM's own changes in the folder's subfolders, and need polling there ([details](../backends/lima.md#file-watchers)).
- New VMs cache the project folder with the `none` mode, because Linux's 9p client loses data with Lima's `mmap`. A VM made before devvm chose `none` keeps `mmap`, with a warning at each launch, until you run `devvm mount-cache none` ([details](../backends/lima.md#the-project-folders-cache)).
- A program can't map a file in the project folder for shared writing, and gets `Invalid argument`, such as LMDB, which Parcel's `.parcel-cache` and Gatsby's cache use, numpy's `memmap` in `r+` or `w+` mode, Java's read-write mappings and the mold linker. Keep their files on the VM's own disk, or use a `--clone` VM ([details](../backends/lima.md#programs-that-map-files-for-writing)).
- SQLite's WAL mode fails in the project folder unless the connection sets `PRAGMA locking_mode=EXCLUSIVE` first, while SQLite's default journal works. Use a `--clone` VM for a project that needs WAL mode, as its repo is on the VM's own disk, or keep the database outside the folder ([details](../backends/lima.md#sqlite-and-the-cache)).
- Set-id bits that root in a VM sets on files in the project folder reach the host, and devvm names such files when it makes a VM and once a VM is stopped ([what to do](troubleshooting.md#devvm-names-set-id-files)).
- devvm keeps your secrets in files under `~/.local/share/devvm/secrets/`, which only you can read, instead of the Keychain ([details](settings.md#devvms-state)).
- devvm has no Time Machine to keep VMs' CAs out of, so keep `~/.local/share/devvm/vms` out of your backups ([details](../internals/security.md#secrets-stay-on-the-host)).

## Intel Macs

devvm has a release for Intel Macs, which the install script and Homebrew install, but nobody has tested it on one yet. The tested systems are Apple silicon Macs and Linux on arm64 and x86_64, so on an Intel Mac, VMs, `--from` and the network tiers' confinement may fail in ways that devvm's tests haven't caught.
