---
title: Staying safe
weight: 30
---

These pages say what devvm protects you from and what's still yours to watch.

## The habits

- **Run installs, project code and agents in the VM.** Whatever the VM writes into the project folder runs as you when a program on this machine runs it, so `npm install`, `make`, tests and agents belong in the VM ([files the VM writes](attack-vectors.md#files-the-vm-writes)).
- **Keep [network policy](network.md) on, and keep allowlists short.** New VMs get it by default. Without it, code in the VM reaches everything this machine can, and with it, stolen data can go only to the hosts you allowed.
- **Use [`--clone`](../using/clone.md) for a repo or an agent you don't trust.** The VM then works on a clone of its own, and nothing it writes reaches your folder until you fetch it.
- **Review the VM's changes before you run them on this machine or merge them.** Treat its commits as an untrusted contributor's.
- **Use fine-grained tokens, and bind only the ones a VM needs.** A VM can do whatever its tokens allow, even when it never sees them ([tokens and keys](attack-vectors.md#tokens-and-keys-the-vm-may-use)).
- **Keep secrets out of the project folder.** The VM sees every file in it, a `.env` file included.
- **Leave SSH agent forwarding off,** or use [devvm's own agent](../using/ssh.md#devvms-own-agent) with only the keys the VM needs.

## What a VM can reach

| | |
| --- | --- |
| The project folder | read and write; with `--clone`, not mounted: the VM has its own clone ([details](../using/clone.md)) |
| `.git` hooks and config | locked (macOS); what a lock can't stop is reported, not prevented ([details](git-protection.md)); with `--clone`, out of reach |
| Your agents' config (skills, settings, logins) | its folders read-only, its files copied; never its history ([details](../using/coding-agents.md)) |
| The rest of your home folder, other projects | not mounted |
| Your SSH keys | only with `devvm ssh-agent on`, while you're connected (on smolvm, while the VM runs; on Docker, in ssh sessions alone); with `devvm ssh-agent devvm`, only the keys you added to devvm's own agent ([details](../using/ssh.md#agent-forwarding), [smolvm](../internals/smolvm.md#ssh-agent-forwarding)) |
| The network, with network policy | allowlisted hosts only, through a proxy on this machine ([details](network.md)) |
| Claude and GitHub tokens, Codex and opencode logins, Claude Code's MCP secrets, with network policy | kept on this machine; the proxy puts them into requests ([details](network.md#secrets)) |
| The network, without network policy | everything this machine reaches, its own services on `localhost` included |
