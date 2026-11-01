---
title: devvm network
weight: 19
---

Show or change the network policy of this folder's VM

## Synopsis

Show the network policy of this folder's VM, on, off or unenforced (on, but not confined: started outside devvm, or before it was turned on), then its allowlist: each policy file that has rules, in order (its image's, yours for every VM, the VM's own, its secrets' hosts), and the rules in it. devvm network on or off changes the VM's network policy from its next start: devvm restart. Secrets the VM got while it was off still work once it's on. With -g, show or change whether new VMs get network policy instead: ask (in a terminal, and on without one), on or off. devvm's --network-policy and --no-network-policy still choose for one new VM.

```text
devvm network [on|off] [flags]
```

## Options

```text
  -g, --global   show or change whether new VMs get network policy instead
  -h, --help     help for network
      --json     print it as one JSON object, for scripts
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm network allow](devvm_network_allow.md): Allow hosts for this folder's VM
- [devvm network deny](devvm_network_deny.md): Deny hosts for this folder's VM
- [devvm network log](devvm_network_log.md): Show what this folder's VM connected to and what its network policy blocked
- [devvm network rm](devvm_network_rm.md): Remove rules from this folder's VM
