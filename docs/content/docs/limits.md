---
title: Known limits
weight: 33
---

- **Shared tools are read-only:** they can't update themselves or add components (`rustup component add`) ([details](images.md#shared-tools)).
- **Layers can be added to a VM in place but not taken out:** `devvm rm` and make the VM again. Shared tools can (`devvm remove`) ([details](commands.md#adding-to-a-vm)).
- **`--from`** takes Debian-, Ubuntu- and Alpine-based images only. Alpine, Debian 12 (`node:24`) and Ubuntu before 26.04 on arm64 build with unpinned packages ([details](from.md#supported-images)).
- **Linux hosts** have no `.git` protection: use `--clone` ([details](git-protection.md#linux-hosts)).
- **Agent folders** made or deleted on this machine reach a running VM only after a restart; devvm offers one ([details](coding-agents.md#mounts)).
- **With Lima:** project paths can't contain commas, folder names in VM names are cut short, disks grow but never shrink ([details](lima.md#limits)).
- **smolvm is experimental:** network policy is emulated, with one narrow DNS channel ([details](smolvm.md#network-policy)); ssh for editors is emulated ([details](smolvm.md#ssh-for-editors)); every port forwarded is emulated, from port 1024 up ([details](smolvm.md#every-port)); SSH agent forwarding changed once a VM is made is emulated, and a VM has its agent while it runs ([details](smolvm.md#ssh-agent-forwarding)); at most 63 listed ports; project paths can't contain colons ([details](smolvm.md)).
