---
title: devvm ssh-agent add
weight: 40
---

Load a key into devvm's own SSH agent

## Synopsis

Load a private key into devvm's own SSH agent, starting it first, and record it in ~/.config/devvm/agent-keys with its fingerprint and options, replacing what was recorded for it. devvm loads it again when a VM needs it (after a reboot, or once its lifetime ran out), only while the file holds the same key. A key in a folder a VM mounts is refused.

```text
devvm ssh-agent add KEY_FILE [flags]
```

## Options

```text
      --confirm             ask before each use of the key, through an ssh-askpass program
  -h, --help                help for add
      --lifetime DURATION   how long the agent keeps the key once loaded: a DURATION such as 30m, 8h or 1d, or forever (default 12h)
```

## See also

- [devvm ssh-agent](devvm_ssh-agent.md): Show or change SSH agent forwarding for this folder's VM
