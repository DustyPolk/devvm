---
title: SSH
weight: 27
---

## SSH access by name

Each running project VM can be reached as `devvm-<folder>`: `ssh devvm-website`, or VS Code's *Remote-SSH: Connect to Host*. The hash suffix is kept only when two folders share a name; the VM's full name (`devvm-website-<hash>`) always works too.

devvm writes these entries to `~/.config/devvm/ssh_config` each time it launches or stops a VM, because a VM's SSH port can change each time it starts. To use them, add this line to the top of `~/.ssh/config`, before any `Host` block:

```text
Include ~/.config/devvm/ssh_config
```

Or point ssh at the file: `ssh -F ~/.config/devvm/ssh_config devvm-website`. `devvm ssh`, `devvm code` and a new [`--clone`](clone.md) VM offer to add the line in a terminal; without one, they show it.

- **Only running VMs are listed.** A stopped VM's port is free for any program on the host to take, so an entry for it could hand your connection, and your agent, to that program. Start the VM with `devvm ssh` (or `devvm`) before connecting from an editor.
- **Each entry checks the VM's own host keys,** kept in `~/.config/devvm/known_hosts`. devvm reads them from the VM through its backend once it runs, and again after each start: a VM whose Lima config changed gets new host keys when it boots, and a [smolvm](smolvm.md#ssh-for-editors) VM new ones at every start. A VM whose keys can't be read gets no entry. A host key that doesn't match is refused.
- **Each connection is its own:** entries set `ControlMaster no`, so a session never rides on a connection another program opened. Of Lima's own ssh settings for the VM, only the address, port, user and login key are copied; Lima's turn host key checks off. On smolvm the login key is devvm's own, one per VM.
- **Nothing else of this machine's for the VM:** entries set `ForwardX11 no`, `ForwardX11Trusted no`, `Tunnel no`, `PermitRemoteOpen none`, `AddKeysToAgent no`, `ProxyJump none`, `ProxyCommand none`, `UpdateHostKeys no` and `HashKnownHosts no`, whatever your `Host *` says. `PermitRemoteOpen` needs OpenSSH 8.7 or newer (`ssh -V`); an older ssh refuses the entries. A `RemoteForward` of yours still applies, as entries can't turn it off without refusing `ssh -R` too: keep remote forwards out of `Host *`. The VM's sshd allows only local forwarding (`-L`, `-D`, what VS Code uses), no remote forwarding, tunnels or X11, but root in the VM can change that: it's your ssh that decides.

## devvm ssh and devvm code

`devvm ssh` gets this folder's VM ready for ssh and editors, then shows its name (`devvm-website`) and its folder in the VM. It does what [a launch of an existing VM](commands.md#an-existing-vm) does, without opening a shell: it checks `.git`, applies the size, ports and agent forwarding a stopped VM's state asks for, starts the VM if it's stopped, runs the [self-check](network.md#the-self-check), sets the VM up and writes the ssh config. A VM whose network policy doesn't hold is stopped, and gets no name.

`devvm code` does the same, then opens the folder in VS Code over Remote-SSH with VS Code's `code` command: `code --remote ssh-remote+devvm-website ~/website`. VS Code gets the SSH agent devvm was started with, not the one devvm's own connections to the VM carry. Without `code` on `PATH`, devvm names the host to connect to with *Remote-SSH: Connect to Host*.

### VS Code

Over Remote-SSH, VS Code reaches further than ssh:

- **Ports.** It forwards ports that programs it starts in the VM (its terminals, tasks, an agent) listen on to this machine's `localhost`, whatever `devvm ports` says.
- **Git sign-in.** It answers git's credential prompts in the VM with your GitHub sign-in in VS Code.

devvm turns both off in VS Code's settings for the VM, `~/.vscode-server/data/Machine/settings.json` in the VM: `remote.autoForwardPorts`, `git.terminalAuthentication` and `github.gitAuthentication` are set to `false` each time devvm sets the VM up, and its other settings are kept. A file with comments isn't plain JSON, so devvm leaves it as it is and says so in the run log.

The VM can change that file back. For a VM you don't trust, set the three in your own VS Code settings on this machine too. Programs in the VM can also open URLs in this machine's browser (`$BROWSER`).

## Agent forwarding

SSH agent forwarding is off by default. While it's on and you're connected, anything in the VM, coding agents included, can *use* the agent's keys, to push or to log in to servers, say. It can't copy them. Turn it on for one VM when you need git over SSH there.

- `devvm --ssh-agent` creates the VM forwarding your agent. `devvm ssh-agent on`, `devvm` or `off` changes it for an existing VM from its next start (`devvm restart` for a running one); `devvm ssh-agent` shows it. On smolvm, changing it is emulated, and reviewed; there the VM has the agent while it runs, not only while you're connected ([smolvm](smolvm.md#ssh-agent-forwarding)).
- It applies to devvm's shells and to these entries alike, both as the VM's current start was made with: a change reaches neither until the VM next starts, whichever folder devvm runs from. For a VM started outside devvm (`limactl start`), they follow the current choice. Off, an entry says `ForwardAgent no`, whatever your own `Host *` settings say, and none of devvm's own connections to the VM carry your agent.
- With it off, git over SSH in the VM fails. devvm says so when it creates a VM for a repo whose `origin` is an SSH URL. Use an HTTPS remote with a token instead, or turn forwarding on.
- **No keys loaded:** when a VM forwards your agent, it has no keys and `~/.ssh` has one, devvm offers to load it (`ssh-add --apple-use-keychain` on macOS). To keep it loaded across reboots, add `AddKeysToAgent yes` (and `UseKeychain yes` on macOS) under `Host *` in `~/.ssh/config`.
- Or use an agent that asks you to approve each use, such as 1Password's or Secretive.

GitHub's host keys are preinstalled in every VM, so a first connection to GitHub doesn't prompt.

### devvm's own agent

devvm's own agent keeps your other keys out of reach. It holds only the keys you add to it: a deploy key, or one with access to fewer repos.

- `devvm ssh-agent add ~/.ssh/deploy_key` starts the agent (its socket is `~/.local/share/devvm/agent.sock`) and loads the key. `devvm ssh-agent devvm` makes this folder's VM forward that agent instead of yours, in devvm's shells and these entries alike: they name it as the agent too, so an editor forwards it however it was started. `devvm ssh-agent keys` lists its keys.
- devvm loads a key again when a VM that uses the agent launches without it: after a reboot, asking for its passphrase if it has one, and once its lifetime ran out. It does so only while the file holds the key you added: `add` records the key's fingerprint, so a file replaced since, or a link swapped to another key, isn't loaded. A key you replaced in place needs `devvm ssh-agent add` again.
- A key inside a folder a VM mounts is refused, and not loaded: that VM could read it. Keep keys for devvm's agent outside project folders.
- Encrypted keys in the old PEM format need converting first, as their fingerprint can't be read without the passphrase: `ssh-keygen -p -f KEY`.

**Limits on its keys:** the agent drops each key 12 hours after loading it, so a VM can't keep using it indefinitely. `--lifetime 30m` (any duration `ssh-add -t` takes, or `forever`) changes that per key. `--confirm` makes the agent ask you before each use (each push, each login) through an `ssh-askpass` program, which macOS doesn't include: `brew install theseal/ssh-askpass/ssh-askpass`. The keys and their options are kept in `~/.config/devvm/agent-keys`; `devvm ssh-agent add` on the same key again replaces its options.
