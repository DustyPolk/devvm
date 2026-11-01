---
title: devvm ssh-agent
weight: 39
---

Show or change SSH agent forwarding for this folder's VM

## Synopsis

Show whether this folder's VM forwards an SSH agent: off (the default), on (yours) or devvm (devvm's own, which holds only the keys added to it with devvm ssh-agent add). Then, once a key is added, it lists the keys devvm's own agent holds, as ssh-add -l does, loading those it lacks first, and each key added to it: its file, its fingerprint and how it's loaded. devvm ssh-agent on, devvm or off changes the VM's forwarding from its next start: devvm restart. While it's on and you're connected, anything in the VM can use the agent's keys.

```text
devvm ssh-agent [on|devvm|off] [flags]
```

## Options

```text
  -h, --help   help for ssh-agent
      --json   print it as one JSON object, for scripts
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm ssh-agent add](devvm_ssh-agent_add.md): Load a key into devvm's own SSH agent
- [devvm ssh-agent rm](devvm_ssh-agent_rm.md): Remove a key from devvm's own SSH agent
