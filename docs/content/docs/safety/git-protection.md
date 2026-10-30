---
title: .git protection
weight: 2
---

The project folder is read-write in the VM, so without protection anything in the VM could add a git hook, or set `core.fsmonitor`, `core.hooksPath` or `core.sshCommand` in `.git/config`, and your next git command on the host would run it. A read-only mount wouldn't hold: root in the VM can unmount it. So on macOS devvm locks those files with the user-immutable flag (`chflags uchg`), which even root in the VM can't clear.

It's on for every new VM. `--no-git-protect` creates one without it, and [`devvm git-protect`](#turning-it-on-or-off) changes it later.

## What still works

git in the VM can commit, branch, check out, stash, fetch and pull, and `git worktree add` works on the host. Commands that write `.git/config` fail with `Operation not permitted`, in the VM and on the host: `git config`, `git remote add`, `push -u` to a new branch, and hook installers like husky or pre-commit. So do `git worktree remove` and `prune` for the worktrees that were there when devvm locked.

To run them, unlock with the VM stopped, run them on the host, and lock again:

```sh
devvm git-protect off
git config …
devvm git-protect on
```

## Turning it on or off

`devvm git-protect` shows the VM's protection: `on`, `strict`, `off`, or `clone` for a [`--clone`](../using/clone.md) VM, which mounts no `.git`. `devvm git-protect on`, `strict` or `off` changes it, only while the VM is stopped: a running VM could race the locks, the same reason launches lock only a stopped VM.

- `on` locks `.git` as it is now, taking it as yours: a `.git` replaced since the last lock isn't reported, and the next launches compare with this one. It reports what [a lock can't stop](../internals/git-protection.md#what-a-lock-cant-stop) and what's [not covered](#not-covered), as when the VM was made. Without a `.git` yet, it's locked on the first launch after one appears.
- `strict` is `on`, plus [strict](#strict).
- `off` takes the locks off. The VM can then change `.git` hooks and config, which run on your machine.

`--no-git-protect` on an existing VM is ignored, with a warning. A `--clone` VM's protection can't be changed.

## Strict

The VM can add a submodule at any time ([below](../internals/git-protection.md#what-a-lock-cant-stop)), and host `git status` and `git diff` read its config, so its `core.fsmonitor` would run on the host before the next launch locks it. `devvm git-protect strict` closes that: it sets `diff.ignoreSubmodules = all` in the locked `.git/config`, so host `git status` and `git diff` never look inside submodules, and no longer show their changes. `git submodule …` and `--recurse-submodules` still enter them: for a repo with submodules you don't trust, use [`--clone`](../using/clone.md).

`devvm git-protect on` and `off` take `diff.ignoreSubmodules` back out.

## Not covered

- **Hooks in the working tree**, such as husky's `.husky/` with `core.hooksPath=.husky` (or an absolute path into the project folder). The VM can edit them, and they run on the host. devvm warns about them when it creates the VM.
- **What the locked files hand over to the project folder:** an `include.path` or `includeIf.*.path` that leads into it, and hooks that are symlinks into it. The VM can edit those files, and host git reads or runs them. devvm warns about them when it creates the VM, and on a launch after they change.
- **Submodules whose git folder is outside the project:** devvm can't lock it (another project's VM may write it), and host `git status` reads its config. devvm names them when it creates the VM.

## Linux hosts

Linux has no immutable flag a user can set, so VMs on Linux hosts have no `.git` protection: the VM can change `.git` hooks and config, which run on the host. devvm says so when it creates a VM for a folder with a `.git`, and refuses `devvm git-protect on` and `strict`. [`--clone`](../using/clone.md) keeps the VM out of `.git` instead.
