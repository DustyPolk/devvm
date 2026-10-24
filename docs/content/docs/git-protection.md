---
title: .git protection
weight: 26
---

The project folder is read-write in the VM, so without protection anything in the VM could add a git hook, or set `core.fsmonitor`, `core.hooksPath` or `core.sshCommand` in `.git/config`, and your next git command on the host would run it. A read-only mount wouldn't hold: root in the VM can unmount it. So on macOS devvm locks those files with the user-immutable flag (`chflags uchg`), which even root in the VM can't clear.

It's on for every new VM. `--no-git-protect` creates one without it.

## What's locked

devvm locks when it creates the VM, while the VM is stopped, so nothing in it can race the locks:

- `.git/hooks` and `.git/config`. A repo without a `hooks` folder gets an empty one, locked, so the VM can't make one.
- The same for each submodule devvm finds: git folders under `.git/modules`, and those in the working tree (`sub/.git`, or the folder a `sub/.git` file points to), submodules of submodules included. devvm finds those from the index, read with `core.fsmonitor` off.
- With `extensions.worktreeConfig` on, `.git/config.worktree`, created empty if it's missing.
- What linked worktrees elsewhere on your machine read from here: `.git/worktrees/<name>/commondir` (and `config.worktree`).
- If the project folder is itself a linked worktree, its `.git` file. The repo it points to is outside the VM.

A new VM interrupted before its first shell is deleted, and its locks are taken off.

devvm never locks or creates anything through a symlink: the VM could plant one under `.git` to make devvm lock a file elsewhere. A path that is a symlink, or is under one, is reported instead. Each lock is checked again once it's set: a path that moved meanwhile is unlocked where it landed, and reported.

## What still works

git in the VM can commit, branch, check out, stash, fetch and pull, and `git worktree add` works on the host. Commands that write `.git/config` fail with `Operation not permitted`, in the VM and on the host: `git config`, `git remote add`, `push -u` to a new branch, and hook installers like husky or pre-commit. So do `git worktree remove` and `prune` for the worktrees that were there when devvm locked.

To change them, unlock on the host, with the VM stopped, and lock again after:

```sh
chflags -R nouchg .git/hooks .git/config
git config …
chflags -R uchg .git/hooks .git/config
```

## What a lock can't stop

The VM can rename `.git` and put a new one in its place, add `.git/commondir` to point git at another config, or do the same with a linked worktree's folder in `.git/worktrees`. When it creates the VM, devvm reports a `commondir` that's already there, linked worktrees whose `commondir` leads somewhere else, and paths it couldn't lock because they're symlinks. It doesn't repair anything: check what it names before you run git in the folder.

The VM writes `.git/index` (it has to, to commit), so it can also add a submodule with its own config, which host `git status` reads: a submodule's `core.fsmonitor` runs on the host too.

## Not covered

- **Hooks in the working tree**, such as husky's `.husky/` with `core.hooksPath=.husky` (or an absolute path into the project folder). The VM can edit them, and they run on the host. devvm warns about them when it creates the VM.
- **What the locked files hand over to the project folder:** an `include.path` or `includeIf.*.path` that leads into it, and hooks that are symlinks into it. The VM can edit those files, and host git reads or runs them. devvm warns about them when it creates the VM.
- **Submodules whose git folder is outside the project:** devvm can't lock it (another project's VM may write it), and host `git status` reads its config. devvm names them when it creates the VM.

## Linux hosts

Linux has no immutable flag a user can set, so VMs on Linux hosts have no `.git` protection: the VM can change `.git` hooks and config, which run on the host. devvm says so when it creates a VM for a folder with a `.git`.
