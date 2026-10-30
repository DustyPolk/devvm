---
title: Known limits
weight: 33
---

- **Shared tools are read-only.** A tool can't update itself or add components, for example with `rustup component add` ([details](images.md#shared-tools)).
- **Layers can be added to a VM in place but not taken out.** To take one out, run `devvm rm` and make the VM again. Shared tools can be taken out with `devvm remove` ([details](commands.md#adding-to-a-vm)).
- **`--from`** takes only Debian-, Ubuntu- and Alpine-based images. Alpine, Debian 12 (`node:24`) and Ubuntu before 26.04 on arm64 build with unpinned packages ([details](from.md#supported-images)).
- **Linux hosts** have no `.git` protection, so use `--clone` there ([details](git-protection.md#linux-hosts)).
- **Agent folders** that you make or delete on this machine reach a running VM only after a restart, which devvm offers ([details](coding-agents.md#mounts)).
- **With Lima,** project paths can't contain commas, folder names in VM names are cut short, and disks grow but never shrink ([details](lima.md#limits)).
- **Ports** are forwarded over TCP only, on every backend, and forwarding every port means every port from 1024 up. You can tunnel UDP over a forwarded TCP port with socat ([details](images.md#ports)).
- **Docker is experimental, and it runs containers, not VMs.**
  - On Linux a container shares the host's kernel, and on macOS it shares Docker Desktop's one VM ([details](docker.md#isolation-a-container-is-not-a-vm)).
  - A Docker VM has no disk size.
  - The docker layer runs only under Sysbox on a Linux engine, and with no network policy ([details](docker.md#the-docker-layer)).
  - Raw packet tools, firewalls and VPNs, mounting, nested virtualization and some profiling don't work in a Docker VM ([details](docker.md#what-docker-vms-cant-do)).
  - Network policy, and build confinement on Docker Desktop, are emulated. VMs, except those with the docker layer, and Desktop's builds have no `NET_RAW`, so `tcpdump` doesn't work ([details](docker.md#network-policy)).
  - ssh for editors, forwarding ports and SSH agent forwarding are emulated ([details](docker.md#ssh-for-editors)).
  - A build on Docker Desktop reaches any host on ports 80 and 443 through its proxy, without an allowlist, and it reaches no other port ([details](docker.md#building-images)).
- **smolvm is experimental.**
  - Network policy is emulated, with one narrow DNS channel ([details](smolvm.md#network-policy)).
  - ssh for editors is emulated ([details](smolvm.md#ssh-for-editors)).
  - Forwarding every port is emulated, from port 1024 up ([details](smolvm.md#every-port)).
  - Changing SSH agent forwarding once a VM is made is emulated, and a VM has its agent the whole time it runs ([details](smolvm.md#ssh-agent-forwarding)).
  - A VM can list at most 63 ports, and project paths can't contain colons ([details](smolvm.md)).
