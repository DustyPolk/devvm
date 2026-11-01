---
title: devvm ports
weight: 24
---

Show or change which ports this folder's VM forwards

## Synopsis

Show which ports this folder's VM forwards to the same port on this machine's localhost: all, none, or a LIST of ports and ranges like 5173,8000-8010. devvm ports all, none or LIST changes it at once: a stopped VM forwards the new ports from its next start, and a running one's forwarder starts again with them, closing the connections open through it, while the VM runs on. With -g, show or change what new VMs forward instead.

```text
devvm ports [all|none|LIST] [flags]
```

## Options

```text
  -g, --global   show or change what new VMs forward instead
  -h, --help     help for ports
      --json     print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm ports add](devvm_ports_add.md): Forward more ports from this folder's VM
- [devvm ports rm](devvm_ports_rm.md): Stop forwarding some ports from this folder's VM
