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
- **Docker is experimental, and runs containers, not VMs:** a container shares the host's kernel on Linux, and Docker Desktop's one VM on macOS ([details](docker.md#isolation-a-container-is-not-a-vm)); network policy, ssh for editors, forwarding ports, SSH agent forwarding, a disk size and the docker layer aren't there yet, so `--ports none --no-network-policy` for now; Docker Desktop can't build images yet ([details](docker.md)).
- **smolvm is experimental:** network policy is emulated, with one narrow DNS channel ([details](smolvm.md#network-policy)); ssh for editors is emulated ([details](smolvm.md#ssh-for-editors)); every port forwarded is emulated, from port 1024 up ([details](smolvm.md#every-port)); SSH agent forwarding changed once a VM is made is emulated, and a VM has its agent while it runs ([details](smolvm.md#ssh-agent-forwarding)); at most 63 listed ports; project paths can't contain colons ([details](smolvm.md)).
