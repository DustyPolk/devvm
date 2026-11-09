---
title: devvm network deny
explainer: "Deny hosts for this folder's VM"
---

Deny hosts for this folder's VM

## Synopsis

Add RULEs as denies to this folder's VM's rules, or with -g every VM's, from the next connection on, and a deny beats any allow. RULE is a host, *.host for its subdomains, ** for any host, or an address or CIDR, and with a port, such as example.com:8443, it matches only that port instead of 443 and 80. --method and --path narrow a host's rule to some of its requests, which its proxy decrypts to check.

```text
devvm network deny RULE... [flags]
```

## Examples

```text
  devvm network deny example.com                                    # keep the VM from reaching example.com
  devvm network deny api.github.com --method POST,PUT,PATCH,DELETE  # keep it from changing anything through GitHub's API
  devvm network deny github.com --path '/*/*/git-receive-pack'      # keep it from pushing to GitHub over HTTPS
  devvm network deny -g '*.example.com'                             # keep every VM from its subdomains
```

## Options

```text
  -g, --global           change your rules for every VM instead
  -h, --help             help for deny
      --here             use this folder's own VM, never that of a folder it's inside
      --method METHODS   narrow each rule to these METHODS of a host's requests, such as GET,HEAD
      --parent           use the VM of the nearest folder this one is inside, even across a git repo of its own
      --path PATH        narrow each rule to the requests whose path matches PATH, such as /repos/ for everything below /repos/
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm network deny](../safety/network.md#changing-rules) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network tier of this folder's VM
