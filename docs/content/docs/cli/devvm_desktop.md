---
title: devvm desktop
weight: 6
---

Open this folder's VM's desktop in a VNC viewer

## Synopsis

Get this folder's VM ready as devvm host does, start the desktop of its desktop feature, and serve it on this machine's 127.0.0.1 alone, with a password of the VM's own, until you press Ctrl+C or the VM stops. On macOS, Screen Sharing opens it and asks for the password, which devvm shows. The clipboard isn't shared unless you add --clipboard.

```text
devvm desktop [flags]
```

## Options

```text
      --clipboard   share the clipboard both ways: this machine's reaches the VM, and the VM's this machine
  -h, --help        help for desktop
      --no-open     show the address and password without opening Screen Sharing
```

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
