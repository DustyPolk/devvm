---
title: devvm network deny
weight: 21
---

Deny hosts for this folder's VM

## Synopsis

Add RULEs as denies to this folder's VM's rules, or with -g every VM's: a deny beats any allow. They apply at once, to the next connection. RULE is a host (example.com), any subdomain of one (*.example.com), any host (**), an address or CIDR (10.0.0.5, fd00::/8), each with an optional port (example.com:8443, [fd00::1]:22); without a port, 443 and 80.

```text
devvm network deny RULE... [flags]
```

## Options

```text
  -g, --global   change your rules for every VM instead
  -h, --help     help for deny
```

## See also

- [devvm network](devvm_network.md): Show or change the network policy of this folder's VM
