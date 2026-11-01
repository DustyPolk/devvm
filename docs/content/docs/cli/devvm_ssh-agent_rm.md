---
title: devvm ssh-agent rm
weight: 41
---

Remove a key from devvm's own SSH agent

## Synopsis

Remove a key added with devvm ssh-agent add from devvm's own SSH agent, and from ~/.config/devvm/agent-keys, so devvm doesn't load it again. The key file stays as it is, and needn't still exist. A key that wasn't added is refused: devvm ssh-agent lists those that were.

```text
devvm ssh-agent rm KEY_FILE [flags]
```

## Options

```text
  -h, --help   help for rm
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
