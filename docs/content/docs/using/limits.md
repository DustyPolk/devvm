---
title: Known limits
weight: 13
---

These are the things devvm can't do, everywhere and then by host and backend.

## Everywhere

- **Shared tools are read-only,** so a tool can't update itself or add components, for example with `rustup component add` ([details](images.md#shared-tools)).
- **Layers can be added to a VM in place but not taken out.** To take one out, run `devvm rm` and make the VM again, but shared tools come out with `devvm remove` ([details](images.md#adding-and-removing-tools)).
- **`--from` takes only Debian-, Ubuntu- and Alpine-based images,** and Alpine, Debian 12 (`node:24`) and Ubuntu before 26.04 on arm64 build with unpinned packages ([details](from.md#supported-images)).
- **Agent folders** that you make or delete on this machine reach a running VM only after a restart, which devvm offers ([details](coding-agents.md#mounts)).
- **Ports** are forwarded over TCP only, on every backend, and forwarding every port means every port from 1024 up. You can tunnel UDP over a forwarded TCP port with socat ([details](ports-size.md#ports)).
- **Changing a running VM's ports** closes the connections that are open through its forwards ([details](ports-size.md#ports)).
- **With Lima,** project paths can't contain commas, folder names in VM names are cut short, and disks grow but never shrink ([details](../backends/lima.md#limits)).
- **File watchers in a Lima VM** don't see the edits you make on this machine, only those made in the VM, so set a dev server that should reload on your edits to poll the project folder ([details](../backends/lima.md#file-watchers)).

## Linux hosts

- They have no `.git` protection, so use `--clone` there ([details](../safety/git-protection.md#linux-hosts)).
- VMs mount the project folder with 9p instead of virtiofs, so file watchers in a VM also miss the VM's own changes in the folder's subfolders, and need polling there ([details](../backends/lima.md#file-watchers)).
- Set-id bits that root in a VM sets on files in the project folder reach the host, and devvm names such files at each launch and stop ([what to do](troubleshooting.md#devvm-names-set-id-files)).
- devvm keeps your secrets in files under `~/.config/devvm/secrets/`, which only you can read, instead of the Keychain ([details](settings.md#your-settings)).
- devvm has no Time Machine to keep its CA out of, so keep `~/.local/share/devvm/ca` out of your backups ([details](../internals/security.md#secrets-stay-on-the-host)).

## smolvm

smolvm is experimental.

- Network policy has one narrow way out, through DNS ([details](../internals/smolvm.md#network-policy)).
- A VM that forwards your SSH agent has it the whole time it runs, not only while you're connected ([details](../backends/smolvm.md#whats-different)).
- Project paths can't contain colons ([details](../backends/smolvm.md#whats-different)).
- File watchers in a VM don't see the files you delete on this machine, though they see the ones you change or make ([details](../backends/smolvm.md#whats-different)).

## Docker

Docker is experimental, and it runs containers, not VMs.

- On Linux a container shares the host's kernel, and on macOS it shares Docker Desktop's one VM ([details](../backends/docker.md#isolation-a-container-is-not-a-vm)).
- A Docker VM has no disk size, and raw packet tools such as `tcpdump`, firewalls and VPNs, mounting, nested virtualization and some profiling don't work in one ([details](../backends/docker.md#what-docker-vms-cant-do)).
- On Docker Desktop, file watchers in a VM don't see the files you delete on this machine, though they see the ones you change or make ([details](../backends/docker.md#docker-desktops-file-sharing)).
- The docker layer runs only under Sysbox on a Linux engine, without network policy, and a build on Docker Desktop reaches any host on ports 80 and 443 ([docker layer](../backends/docker.md#the-docker-layer), [builds](../internals/docker.md#building-images)).
