---
title: devvm network allow
explainer: "Allow hosts for this folder's VM"
---

Allow hosts for this folder's VM

## Synopsis

Add RULEs to the allowlist of this folder's VM, or with -g of every VM, from the next connection on. RULE is a host, *.host for its subdomains, ** for any host, or an address or CIDR, and with a port, such as example.com:8443, it matches only that port instead of 443 and 80. --method and --path narrow a host's rule to some of its requests, which its proxy decrypts to check.

```text
devvm network allow RULE... [flags]
```

## Examples

```text
  devvm network allow example.com                        # let the VM reach example.com
  devvm network allow '*.example.com'                    # and any of its subdomains
  devvm network allow 10.0.0.5:5432                      # one port of one address
  devvm network allow api.example.com --method GET,HEAD  # let it only read from api.example.com
  devvm network allow -g pypi.org                        # let every VM reach pypi.org
```

## Options

```text
  -g, --global           change your rules for every VM instead
  -h, --help             help for allow
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

[The docs page for devvm network allow](../safety/network.md#changing-rules) explains it in full.

## See also

- [devvm network](devvm_network.md): Show or change the network tier of this folder's VM
