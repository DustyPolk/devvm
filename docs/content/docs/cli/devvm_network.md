---
title: devvm network
weight: 27
---

Show or change the network policy of this folder's VM

## Synopsis

Show the network policy of this folder's VM, which is on, off or unenforced, and then its allowlist, policy by policy: devvm's built-in ones by name, then your files by path. devvm network on or off changes it from the VM's next start, so in a terminal devvm offers to restart a running VM now. With -g, show or change whether new VMs get network policy instead: ask, on or off.

```text
devvm network [on|off] [flags]
```

## Examples

```text
  devvm network                    # show the VM's network policy and allowlist
  devvm network allow example.com  # let the VM reach example.com
  devvm network log --blocked      # list what the VM's network policy blocked
  devvm network off                # turn network policy off from the VM's next start
  devvm network -g on              # give every new VM network policy without asking
```

## Options

```text
  -g, --global   show or change whether new VMs get network policy instead
  -h, --help     help for network
      --json     print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network](../safety/network.md#turning-it-on-or-off) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm network allow](devvm_network_allow.md): Allow hosts for this folder's VM
- [devvm network deny](devvm_network_deny.md): Deny hosts for this folder's VM
- [devvm network log](devvm_network_log.md): Show what this folder's VM connected to and what its network policy blocked
- [devvm network rm](devvm_network_rm.md): Remove rules from this folder's VM
