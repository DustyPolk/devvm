---
title: Known limits
weight: 33
---

- **Shared tools are read-only:** they can't update themselves or add components (`rustup component add`) ([details](images.md#shared-tools)).
- **Layers can be added to a VM in place but not taken out:** `devvm rm` and make the VM again. Shared tools can (`devvm remove`) ([details](commands.md#adding-to-a-vm)).
- **`--from`** takes Debian-, Ubuntu- and Alpine-based images only. Alpine, Debian 12 (`node:24`) and Ubuntu before 26.04 on arm64 build with unpinned packages ([details](from.md#supported-images)).
- **Linux hosts** have no `.git` protection: use `--clone` ([details](git-protection.md#linux-hosts)).
- **Agent folders** made or deleted on this machine reach a running VM only after a restart; devvm offers one ([details](coding-agents.md#mounts)).
- **With Lima:** project paths can't contain commas, folder names in VM names are cut short, disks grow but never shrink ([details](lima.md#limits)); forwarding every port can take one a program on this machine has, on macOS ([details](lima.md#ports)).
- **Docker is experimental, and runs containers, not VMs:** a container shares the host's kernel on Linux, and Docker Desktop's one VM on macOS ([details](docker.md#isolation-a-container-is-not-a-vm)); a disk size isn't there; the docker layer runs only under Sysbox on a Linux engine, with no network policy ([details](docker.md#the-docker-layer)); raw packet tools, firewalls and VPNs, mounting, nested virtualization and some profiling don't work in a Docker VM ([details](docker.md#what-docker-vms-cant-do)); network policy, and build confinement on Docker Desktop, are emulated, and VMs (but the docker layer's) and Desktop's builds have no `NET_RAW` (no `tcpdump`) ([details](docker.md#network-policy)); ssh for editors, forwarding ports and SSH agent forwarding are emulated ([details](docker.md#ssh-for-editors)); a build on Docker Desktop reaches any host on ports 80 and 443 through its proxy, not an allowlist, and no other port ([details](docker.md#building-images)).
- **smolvm is experimental:** network policy is emulated, with one narrow DNS channel ([details](smolvm.md#network-policy)); ssh for editors is emulated ([details](smolvm.md#ssh-for-editors)); every port forwarded is emulated, from port 1024 up ([details](smolvm.md#every-port)); SSH agent forwarding changed once a VM is made is emulated, and a VM has its agent while it runs ([details](smolvm.md#ssh-agent-forwarding)); at most 63 listed ports; project paths can't contain colons ([details](smolvm.md)).
