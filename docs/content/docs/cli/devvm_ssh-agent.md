---
title: devvm ssh-agent
weight: 44
---

Show or change SSH agent forwarding for this folder's VM

## Synopsis

Show whether this folder's VM forwards an SSH agent, which is off, on for yours, or devvm for devvm's own with only the keys you added to it, and then those keys. devvm ssh-agent on, devvm or off changes it from the VM's next start, such as with devvm restart. While forwarding is on and you're connected, anything in the VM can use the agent's keys.

```text
devvm ssh-agent [on|devvm|off] [flags]
```

## Examples

```text
  devvm ssh-agent                        # show the VM's agent forwarding
  devvm ssh-agent add ~/.ssh/deploy_key  # load a key into devvm's own agent
  devvm ssh-agent devvm                  # forward devvm's own agent from the VM's next start
  devvm ssh-agent off                    # stop forwarding an agent
```

## Options

```text
  -h, --help   help for ssh-agent
      --json   print it as one JSON object, for scripts
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm ssh-agent](../using/ssh.md#agent-forwarding) explains it in full.

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
- [devvm ssh-agent add](devvm_ssh-agent_add.md): Load a key into devvm's own SSH agent
- [devvm ssh-agent rm](devvm_ssh-agent_rm.md): Remove a key from devvm's own SSH agent
