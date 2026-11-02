---
title: .git protection
weight: 6
---

This page explains what devvm locks in a project's git folder, when it locks and checks the locks again, and what the locks can't stop. [.git protection](../safety/git-protection.md) covers what still works, turning protection on or off, strict, and what isn't covered.

On macOS devvm locks with the user-immutable flag (`chflags uchg`), which even root in the VM can't clear. A read-only mount wouldn't hold, because root in the VM can unmount it.

## What's locked

devvm locks these paths:

- `.git/hooks` and `.git/config`. A repo without a `hooks` folder gets an empty one, locked, so the VM can't make one.
- The same paths for each submodule devvm finds, submodules of submodules included. Those are the git folders under `.git/modules`, and those in the working tree, which are `sub/.git` or the folder that a `sub/.git` file points to. devvm finds the submodules from the index, which it reads with `core.fsmonitor` off.
- `.git/config.worktree`, when `extensions.worktreeConfig` is on. devvm creates it empty if it's missing.
- What linked worktrees elsewhere on your machine read from here, which is `.git/worktrees/<name>/commondir` and `config.worktree`.
- The project folder's `.git` file, if the project folder is itself a linked worktree. The repo it points to is outside the VM.

### When devvm locks

devvm locks when it creates the VM, and again on each launch that finds the VM stopped and on `devvm stop`, so nothing in the VM can race the locks. A running VM keeps the locks it had, and `devvm stop` or the next launch that finds the VM stopped, such as `devvm restart`, sets new ones. `devvm git-protect` on a running VM saves the mode in the VM's state (`git_protect_next`), and the same places set it first, then check `.git` under the new mode.

A new VM that's interrupted before its first shell is deleted, and devvm takes its locks off. When `devvm rm` deletes a VM, devvm also takes its locks off.

### Symlinks

devvm never locks or creates anything through a symlink, because the VM could plant one under `.git` to make devvm lock a file elsewhere. devvm reports a path that is a symlink, or is under one, instead of locking it (`git_not_locked`). devvm also checks each lock again once it's set. If the path moved in the meantime, devvm unlocks it where it landed and reports it (`git_lock_raced`).

## On each launch

Every launch of an existing VM, and `devvm stop`, checks `.git` again. devvm reports each of these once:

- A `.git` that was replaced since devvm locked it (`git_replaced`). devvm then does nothing else, so check the new `.git`, and look for the old one, renamed.
- A repo that appeared in the folder after the VM was made (`git_appeared`), which devvm then locks.
- A submodule git folder that appeared, in the working tree or under `.git/modules` (`git_embedded_new`), which devvm then locks.
- Hooks in a hooks folder that wasn't locked, because it wasn't there to lock or belongs to a new submodule (`git_hooks_unlocked`).

devvm also reports these, the first two on every launch and the last when they change:

- A `commondir` (`git_commondir`).
- Linked worktrees whose `commondir` leads elsewhere (`git_worktree_stray`).
- Includes and hook links into the project folder (`git_include_inside`, `git_hook_link_inside`), which [not covered](../safety/git-protection.md#not-covered) describes.

## What a lock can't stop

The VM can rename `.git` and put a new one in its place, add `.git/commondir` to point git at another config, or do the same with a linked worktree's folder in `.git/worktrees`. When devvm creates the VM, and [on each launch](#on-each-launch), it reports a `commondir` that's there, linked worktrees whose `commondir` leads somewhere else, and paths it couldn't lock because they're symlinks. devvm doesn't repair anything, so check what it names before you run git in the folder. When you don't trust what runs in the VM, use [`--clone`](../using/clone.md), which mounts no `.git` at all.

The VM writes `.git/index`, because it has to in order to commit, so it can also add a submodule with its own config, which `git status` on the host reads. A submodule's `core.fsmonitor` therefore also runs on the host. devvm names a new submodule on the next launch, but git on the host reads it before that, unless the VM is [strict](../safety/git-protection.md#strict).
