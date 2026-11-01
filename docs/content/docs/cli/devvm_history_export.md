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

## Options inherited from parent commands

```text
      --no-input   never ask anything: a question devvm would ask fails instead, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm history](devvm_history.md): List the saved exports of agents' history
