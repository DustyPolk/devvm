---
title: Working on a clone
weight: 2
---

`devvm --clone` gives the VM its own clone of the repo instead of mounting your folder, so nothing in the VM can touch your working tree or `.git`. Use it when you don't trust what runs in the VM, such as an agent working unattended, and on Linux hosts, which have no [`.git` protection](../safety/git-protection.md#linux-hosts). [Files the VM writes](../safety/attack-vectors.md#files-the-vm-writes) lists what `--clone` keeps away from this machine.

```sh
devvm --clone              # create the VM, copy your branches into it and open a shell in its clone
git fetch devvm            # later, on this machine, fetch the VM's work as devvm/<branch>
git log -p main..devvm/main
```

## Requirements

- The folder is the top folder of a git repo that has at least one commit and a branch checked out, not a detached HEAD.
- git is installed on this machine. The VM's images already have it.
- You choose `--clone` when the VM is created. devvm refuses `--clone` for an existing VM that mounts its folder, so to switch, save any work in that VM, then run `devvm rm` and `devvm --clone`. Once a VM is a `--clone` VM it stays one, and you don't need to pass `--clone` again.

## What's copied

From this machine, the VM mounts only the shared tools and its [agents' config folders](coding-agents.md#mounts). Once the VM is running, devvm creates a repo in it at the same path as your folder and pushes every local branch into it. The push goes over [the VM's SSH entry](ssh.md#ssh-access-by-name), which checks the VM's pinned host keys. devvm checks out your current branch in the clone, and your shell opens there.

devvm copies only your local branches. Tags, uncommitted changes, and untracked or ignored files such as `.env` and `node_modules` stay on this machine. devvm warns you about uncommitted changes, so you can commit them and send them later with `git push devvm BRANCH`.

If the copy fails, for example because the VM can't be reached, devvm tries again the next time you run it in the folder.

## The remote

devvm adds a git remote for the VM to your repo. It's named `devvm`, or `devvm-<folder>` if `devvm` is already taken, or the VM's full name if both are. The remote's URL is the VM's full name followed by the clone's path, so git connects through devvm's ssh config, and fetching needs two things:

- Your `~/.ssh/config` needs devvm's `Include` line ([SSH access by name](ssh.md#ssh-access-by-name)). devvm offers to add it when it creates the clone, and again when you run `devvm ssh` or `devvm code`. Until the line is there, or when devvm had no terminal to ask in, fetch with `GIT_SSH_COMMAND='ssh -F ~/.config/devvm/ssh_config' git fetch devvm`.
- The VM must be running. A stopped VM has no SSH entry, so `git fetch` fails because it can't resolve the name. Run `devvm` or `devvm ssh` in the folder first.

## Fetching the VM's work

Fetching runs nothing from the VM on this machine. git only receives objects and updates the `devvm/*` branches, or creates new `saved/devvm/*` branches when `devvm rm` does the fetch. The commits themselves are untrusted, though, so treat them as you would a contributor's branch and review them before you merge them, check them out or run anything they change.

## Deleting the VM

Before `devvm rm` deletes a `--clone` VM, it checks the clone for work you'd lose, starting the VM if it's stopped:

1. If the clone has branches with commits this repo hasn't fetched, devvm names them and offers to fetch them into new local branches named `saved/devvm/<branch>`. It skips a branch whose name git on this machine refuses and names it, and that branch goes with the VM. To keep such a branch, rename it in the VM with `git branch -m` before you run `devvm rm`.
2. If the clone has uncommitted changes, devvm lists the first 20 and asks before deleting them along with the VM.

If either check finds something and there's no terminal to ask in, devvm refuses and keeps the VM.

Once the checks pass, devvm deletes the VM. If the remote still points at the VM, devvm then removes it with `git remote remove`, which also deletes its `devvm/*` branches and their upstream settings. Before it does, devvm keeps every `devvm/*` branch that has commits no other branch has as a new local branch under `saved/devvm/`, whoever fetched it. devvm names each branch it creates, and they're yours to review, merge or delete, as with `git branch -D saved/devvm/main`.

The saved branches are named after the remote, as `saved/<remote>/<branch>`, and devvm never overwrites a branch. When a branch is in the way, devvm uses `saved/<remote>-2`, then `-3` and so on, and if it finds no free name, `devvm rm` refuses and keeps the VM ([saved branches](../internals/overview.md#saved-branches)).

`devvm rm -f` skips both checks, but it still keeps the fetched branches as described above.
