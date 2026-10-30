---
title: .git protection
weight: 4
---

The project folder is read-write in the VM, so without protection anything in the VM could add a git hook, or set `core.fsmonitor`, `core.hooksPath` or `core.sshCommand` in `.git/config`, and your next git command on the host would run it. A read-only mount wouldn't hold: root in the VM can unmount it. So on macOS devvm locks those files with the user-immutable flag (`chflags uchg`), which even root in the VM can't clear.

It's on for every new VM. `--no-git-protect` creates one without it, and [`devvm git-protect`](#turning-it-on-or-off) changes it later.

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
- includes and hook links into the project folder ([not covered](#not-covered)), again when they change.

A running VM keeps the locks it had: new ones are set by `devvm stop`, or on the next launch that finds it stopped, such as `devvm restart`. When `devvm rm` deletes the VM, its locks are taken off.

## What still works

git in the VM can commit, branch, check out, stash, fetch and pull, and `git worktree add` works on the host. Commands that write `.git/config` fail with `Operation not permitted`, in the VM and on the host: `git config`, `git remote add`, `push -u` to a new branch, and hook installers like husky or pre-commit. So do `git worktree remove` and `prune` for the worktrees that were there when devvm locked.

To run them, unlock with the VM stopped, run them on the host, and lock again:

```sh
devvm git-protect off
git config …
devvm git-protect on
```

## Turning it on or off

`devvm git-protect` shows the VM's protection: `on`, `strict`, `off`, or `clone` for a [`--clone`](clone.md) VM, which mounts no `.git`. `devvm git-protect on`, `strict` or `off` changes it, only while the VM is stopped: a running VM could race the locks, the same reason launches lock only a stopped VM.

- `on` locks `.git` as it is now, taking it as yours: a `.git` replaced since the last lock isn't reported, and the next launches compare with this one. It reports what [a lock can't stop](#what-a-lock-cant-stop) and what's [not covered](#not-covered), as when the VM was made. Without a `.git` yet, it's locked on the first launch after one appears.
- `strict` is `on`, plus [strict](#strict).
- `off` takes the locks off. The VM can then change `.git` hooks and config, which run on your machine.

`--no-git-protect` on an existing VM is ignored, with a warning. A `--clone` VM's protection can't be changed.

## Strict

The VM can add a submodule at any time ([below](#what-a-lock-cant-stop)), and host `git status` and `git diff` read its config, so its `core.fsmonitor` would run on the host before the next launch locks it. `devvm git-protect strict` closes that: it sets `diff.ignoreSubmodules = all` in the locked `.git/config`, so host `git status` and `git diff` never look inside submodules, and no longer show their changes. `git submodule …` and `--recurse-submodules` still enter them: for a repo with submodules you don't trust, use [`--clone`](clone.md).

`devvm git-protect on` and `off` take `diff.ignoreSubmodules` back out.

## What a lock can't stop

The VM can rename `.git` and put a new one in its place, add `.git/commondir` to point git at another config, or do the same with a linked worktree's folder in `.git/worktrees`. When it creates the VM, and on each launch ([above](#on-each-launch)), devvm reports a `commondir` that's there, linked worktrees whose `commondir` leads somewhere else, and paths it couldn't lock because they're symlinks. It doesn't repair anything: check what it names before you run git in the folder. When you don't trust what runs in the VM, use [`--clone`](clone.md), which mounts no `.git` at all.

The VM writes `.git/index` (it has to, to commit), so it can also add a submodule with its own config, which host `git status` reads: a submodule's `core.fsmonitor` runs on the host too. devvm names a new one on the next launch, but host git run before that reads it, unless the VM is [strict](#strict).

## Not covered

- **Hooks in the working tree**, such as husky's `.husky/` with `core.hooksPath=.husky` (or an absolute path into the project folder). The VM can edit them, and they run on the host. devvm warns about them when it creates the VM.
- **What the locked files hand over to the project folder:** an `include.path` or `includeIf.*.path` that leads into it, and hooks that are symlinks into it. The VM can edit those files, and host git reads or runs them. devvm warns about them when it creates the VM, and on a launch after they change.
- **Submodules whose git folder is outside the project:** devvm can't lock it (another project's VM may write it), and host `git status` reads its config. devvm names them when it creates the VM.

## Linux hosts

Linux has no immutable flag a user can set, so VMs on Linux hosts have no `.git` protection: the VM can change `.git` hooks and config, which run on the host. devvm says so when it creates a VM for a folder with a `.git`, and refuses `devvm git-protect on` and `strict`. [`--clone`](clone.md) keeps the VM out of `.git` instead.
