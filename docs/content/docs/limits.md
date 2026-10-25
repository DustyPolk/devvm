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
- **smolvm is experimental:** network policy is emulated, with one narrow DNS channel ([details](smolvm.md#network-policy)); no ssh for editors, `--clone`, every port forwarded, or SSH agent forwarding changed once a VM is made; project paths can't contain colons ([details](smolvm.md)).
