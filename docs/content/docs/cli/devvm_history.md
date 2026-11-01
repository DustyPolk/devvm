---
title: devvm history
weight: 10
---

List the saved exports of agents' history

## Synopsis

List the exports of agents' history in ~/.local/share/devvm/exports, which devvm history export and devvm rm write there, oldest first: each one's file, the VM it's of, that VM's folder, when it was written and its size. Exports written to a file of your own aren't listed. It changes nothing and starts no VM.

```text
devvm history [flags]
```

## Options

```text
  -h, --help   help for history
      --json   print it as one JSON object, for scripts
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm history export](devvm_history_export.md): Save this folder's VM's agent history to a .tar.gz
