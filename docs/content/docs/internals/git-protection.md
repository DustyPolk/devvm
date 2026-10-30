---
title: .git protection
weight: 6
---

This page explains what devvm locks in a project's git folder, when it checks the locks again, and what they can't stop.

## What's locked

devvm locks when it creates the VM, and again on each launch that finds it stopped and on `devvm stop`, so nothing in the VM can race the locks:

- `.git/hooks` and `.git/config`. A repo without a `hooks` folder gets an empty one, locked, so the VM can't make one.
- The same for each submodule devvm finds: git folders under `.git/modules`, and those in the working tree (`sub/.git`, or the folder a `sub/.git` file points to), submodules of submodules included. devvm finds those from the index, read with `core.fsmonitor` off.
- With `extensions.worktreeConfig` on, `.git/config.worktree`, created empty if it's missing.
- What linked worktrees elsewhere on your machine read from here: `.git/worktrees/<name>/commondir` (and `config.worktree`).
- If the project folder is itself a linked worktree, its `.git` file. The repo it points to is outside the VM.

A new VM interrupted before its first shell is deleted, and its locks are taken off.

devvm never locks or creates anything through a symlink: the VM could plant one under `.git` to make devvm lock a file elsewhere. A path that is a symlink, or is under one, is reported instead. Each lock is checked again once it's set: a path that moved meanwhile is unlocked where it landed, and reported.

## On each launch

Every launch of an existing VM, and `devvm stop`, checks `.git` again, and reports what a lock can't stop, each once:

- a `.git` replaced since devvm locked it (it then does nothing else: check the new one, and look for the old one, renamed);
- a repo that appeared in the folder after the VM was made, which it then locks;
- a submodule git folder that appeared, in the working tree or under `.git/modules`, which it then locks;
- hooks in a hooks folder that wasn't locked (one that wasn't there to lock, or a new submodule's);
- a `commondir`, and linked worktrees whose `commondir` leads elsewhere, on every launch;
- includes and hook links into the project folder ([not covered](../safety/git-protection.md#not-covered)), again when they change.

A running VM keeps the locks it had: new ones are set by `devvm stop`, or on the next launch that finds it stopped, such as `devvm restart`. When `devvm rm` deletes the VM, its locks are taken off.

## What a lock can't stop

The VM can rename `.git` and put a new one in its place, add `.git/commondir` to point git at another config, or do the same with a linked worktree's folder in `.git/worktrees`. When it creates the VM, and on each launch ([above](#on-each-launch)), devvm reports a `commondir` that's there, linked worktrees whose `commondir` leads somewhere else, and paths it couldn't lock because they're symlinks. It doesn't repair anything: check what it names before you run git in the folder. When you don't trust what runs in the VM, use [`--clone`](../using/clone.md), which mounts no `.git` at all.

The VM writes `.git/index` (it has to, to commit), so it can also add a submodule with its own config, which host `git status` reads: a submodule's `core.fsmonitor` runs on the host too. devvm names a new one on the next launch, but host git run before that reads it, unless the VM is [strict](../safety/git-protection.md#strict).
