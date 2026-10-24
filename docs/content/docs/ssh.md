---
title: SSH
weight: 27
---

## SSH access by name

Each running project VM can be reached as `devvm-<folder>`: `ssh devvm-website`, or VS Code's *Remote-SSH: Connect to Host*. The hash suffix is kept only when two folders share a name; the VM's full name (`devvm-website-<hash>`) always works too.

devvm writes these entries to `~/.config/devvm/ssh_config` each time it launches a VM, because Lima picks a new SSH port each time a VM starts. To use them, add this line to the top of `~/.ssh/config`, before any `Host` block:

```text
Include ~/.config/devvm/ssh_config
```

Or point ssh at the file: `ssh -F ~/.config/devvm/ssh_config devvm-website`. A new [`--clone`](clone.md) VM offers to add the line, as its repo's remote needs it.

- **Only running VMs are listed.** A stopped VM's port is free for any program on the host to take, so an entry for it could hand your connection to that program. Start the VM with `devvm` before connecting from an editor.
- **Each entry checks the VM's own host keys,** kept in `~/.config/devvm/known_hosts`. devvm reads them from the VM through Lima once it runs, and again after each start: a VM whose Lima config changed gets new host keys when it boots. A VM whose keys can't be read gets no entry. A host key that doesn't match is refused.
- **Each connection is its own:** entries set `ControlMaster no`, so a session never rides on a connection another program opened. Of Lima's own ssh settings for the VM, only the address, port, user and login key are copied; Lima's turn host key checks off.

## Agent forwarding

VMs don't get your SSH agent: forwarding is off, in devvm's shells and in these entries, which say `ForwardAgent no` whatever your own `Host *` settings say.

Without it, git over SSH in the VM fails. devvm says so when it creates a VM for a repo whose `origin` is an SSH URL. Use an HTTPS remote with a token instead.

GitHub's host keys are preinstalled in every VM, so a first connection to GitHub doesn't prompt.
