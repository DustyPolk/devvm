---
title: Working on a clone
weight: 28
---

`devvm --clone` gives the VM its own clone of the repo instead of mounting the folder, so nothing in the VM can touch your working tree or `.git`. Use it when you don't trust what runs in the VM, such as an agent working unattended, or on Linux hosts, which have no [`.git` protection](git-protection.md#linux-hosts).

```sh
devvm --clone              # a new VM: your branches are copied into it, and the shell opens in its clone
git fetch devvm            # later, on this machine: the VM's work, as devvm/<branch>
git log -p main..devvm/main
```

## What's copied

The VM mounts nothing from this machine but the shared tools and its [agents' config folders](coding-agents.md#mounts). Once it runs, devvm makes a repo at the folder's path in the VM and pushes every local branch into it, over [the VM's SSH entry](ssh.md#ssh-access-by-name), which checks its pinned host keys. Your current branch is checked out there, and the shell opens in it.

Nothing else is copied: not tags, not uncommitted changes, not untracked or ignored files (`.env`, `node_modules`). devvm warns about uncommitted changes. Commit them and `git push devvm BRANCH` to send them later.

A copy that fails (the VM can't be reached, say) is tried again the next time you run devvm in the folder.

## The remote

The repo gets a git remote for the VM, named `devvm`, or `devvm-<folder>` when `devvm` is taken, then the VM's full name. Its URL is the VM's full name and the clone's path, so it goes through devvm's ssh config:

- **It needs the `Include` line** in `~/.ssh/config` ([SSH access by name](ssh.md#ssh-access-by-name)). devvm offers to add it when it makes the clone; without a terminal, or until you add it, fetch with `GIT_SSH_COMMAND='ssh -F ~/.config/devvm/ssh_config' git fetch devvm`.
- **It needs the VM running.** A stopped VM has no SSH entry, so `git fetch` says it can't resolve the name. Run `devvm` in the folder first.

## Fetching the VM's work

Fetching runs nothing from the VM on this machine: git only receives objects and updates `devvm/*`. The commits themselves are untrusted, so review them before you merge them, check them out or run anything they change, as you would a contributor's branch.

## Deleting the VM

`devvm rm` checks the clone first, starting the VM if it's stopped:

1. When it has branches with commits this repo hasn't fetched, devvm names them and offers to fetch them into `devvm/*`.
2. When it has uncommitted changes, devvm lists them (the first 20) and asks before deleting them with the VM.

Without a terminal to ask in, either one refuses, and the VM stays. Then devvm deletes the VM and removes the remote if it still points at the VM. Branches fetched from it stay, as `devvm/*`, until you delete them (`git branch -rd devvm/main`).

`devvm rm -f` skips the checks.

## Requirements

- The folder is the top folder of a git repo, with at least one commit and a branch checked out (HEAD not detached).
- git on this machine. The VM's images have git.
- The choice is made when the VM is created. devvm refuses `--clone` for a VM that mounts its folder: save any work in it, then `devvm rm` and `devvm --clone`. A `--clone` VM stays one; `--clone` again is optional.
