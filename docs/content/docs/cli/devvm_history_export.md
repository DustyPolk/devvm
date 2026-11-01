---
title: devvm history export
weight: 11
---

Save this folder's VM's agent history to a .tar.gz

## Synopsis

Save this folder's VM's agent history (transcripts and sessions, without the agents' logins) to FILE, a .tar.gz, or to a new file in ~/.local/share/devvm/exports. A stopped VM is started for it. FILE is replaced only once the export is complete.

```text
devvm history export [FILE] [flags]
```

## Options

```text
  -h, --help   help for export
```

## See also

- [devvm history](devvm_history.md): List the saved exports of agents' history
