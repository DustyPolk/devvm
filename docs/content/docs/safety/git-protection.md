---
title: .git protection
explainer: Keep code in the VM out of your .git
weight: 2
---

The project folder is read-write in the VM, so without protection anything in the VM could add a git hook, or set `core.fsmonitor`, `core.hooksPath` or `core.sshCommand` in `.git/config`, and your next git command on this machine would run it. On macOS, devvm locks the git hooks and config with a flag that even root in the VM can't clear, so nothing the VM does runs through them on this machine. On Linux, devvm makes them read-only in the VM's own view of the folder, which root in the VM can't change either ([Linux hosts](#linux-hosts)).

Every new VM gets `.git` protection. `--no-git-protect` creates a VM without it, and [`devvm git-protect`](#turning-it-on-or-off) changes it later. [How it works › .git protection](../internals/git-protection.md) lists what devvm locks and when it checks the locks again.

## What still works

git in the VM can commit, branch, check out, stash, fetch and pull, and `git worktree add` works on this machine. Commands that write `.git/config` fail in the VM, and on macOS they fail on this machine too, with `Operation not permitted`. Those are `git config`, `git remote add`, `push -u` to a new branch, and hook installers such as husky or pre-commit. On macOS, `git worktree remove` and `prune` also fail for the worktrees that were there when devvm locked `.git`. On Linux, these commands work on this machine as usual, and the VM sees what they change ([Linux hosts](#linux-hosts)).

To run them on macOS, stop the VM, unlock `.git`, run the commands on this machine, and lock it again:

```sh
devvm stop
devvm git-protect off
git config …
devvm git-protect on
```

## Turning it on or off

`devvm git-protect` shows the VM's protection, which is `on`, `strict`, `off`, or `clone` for a [`--clone`](../using/clone.md) VM, which mounts no `.git`. `devvm git-protect on`, `strict` or `off` changes it at once when the VM is stopped. A running VM could race the locks, so for a running VM devvm saves your choice and asks in a terminal whether to restart the VM now, which stops anything running in it. If you answer no, or there's no terminal, or you pass `--no-input`, devvm says it saved the choice. It applies the choice the next time it finds the VM stopped: at the restart you agreed to, at `devvm stop`, at `devvm restart`, or, when the VM was stopped some other way, at the next launch, before the VM starts. Until then, `devvm git-protect` shows the protection the VM has, and a later choice replaces the saved one. Launches lock only a stopped VM for the same reason.

- `on` locks `.git` as it is now and takes it as yours, so a `.git` that was replaced since the last lock isn't reported, and the next launches compare with this one. It reports what [a lock can't stop](../internals/git-protection.md#what-a-lock-cant-stop) and what's [not covered](#not-covered), as devvm did when it made the VM. If the folder has no `.git` yet, devvm locks it on the first launch after one appears.
- `strict` is `on` plus [strict](#strict).
- `off` takes the locks off. The VM can then change `.git` hooks and config, which run on your machine.

On an existing VM, devvm ignores `--no-git-protect` and warns you. A `--clone` VM's protection can't be changed. `devvm clone on` takes the locks off once it has made the VM again with its own clone, and `devvm clone off` makes it mount the folder again with protection as a new VM gets it ([switching a VM that exists](../using/clone.md#switching-a-vm-that-exists)).

### devvm git-protect --json

`devvm git-protect --json` prints the VM's protection as one JSON object with `mode`, which is `on`, `strict`, `off` or `clone`, as [output for scripts](../using/command-line.md#output-for-scripts) describes. With `--json` the command only shows, so devvm refuses `on`, `strict` or `off` with it.

```json
{
  "version": 1,
  "mode": "on"
}
```

## Strict

The VM can add a submodule at any time, and `git status` and `git diff` on this machine read the submodule's config. So the submodule's `core.fsmonitor` would run on this machine before the VM's next launch locks it. `devvm git-protect strict` closes that gap by setting `diff.ignoreSubmodules = all` in the locked `.git/config`. `git status` and `git diff` on this machine then never look inside submodules, and they don't show the submodules' changes. `git submodule …` and `--recurse-submodules` still enter them, so for a repo with submodules you don't trust, use [`--clone`](../using/clone.md).

`devvm git-protect on` and `off` take `diff.ignoreSubmodules` back out.

## When devvm reports .git

Every launch of an existing VM, and `devvm stop`, check `.git` again, and devvm reports what it finds there that it didn't lock. When it names one of these, check it before you run git in the folder, because git on this machine would run what the VM put there:

- **A repo that appeared** in the folder after its VM was made. devvm locks it from then on, but if you didn't make it, check its hooks and config.
- **Hooks in a folder devvm hadn't locked,** because the folder wasn't there to lock or belongs to a new submodule.
- **A submodule whose git folder isn't one.** The VM can name any folder in the project as a submodule's git folder, such as your `src`, so devvm locks nothing in a folder that doesn't look like a git folder, and names the submodule instead.
- **A path that's a symlink or under one.** devvm never locks anything through a symlink, because the VM could point one at a file elsewhere on this machine, so it leaves that path unlocked.
- **A path that moved while devvm locked it.** devvm leaves it unlocked and takes the lock off wherever it landed, so check `.git`.

[How it works › .git protection](../internals/git-protection.md#on-each-launch) lists every check.

## Not covered

The lock covers `.git`, and the VM can still put things elsewhere that git on this machine runs. devvm tells you about each of these, so read what it reports before you run git in the folder, and use [`--clone`](../using/clone.md) for a repo you don't trust.

- **Hooks in the working tree**, such as husky's `.husky/` with `core.hooksPath=.husky`, or an absolute path into the project folder. The VM can edit them, and they run on this machine. devvm warns about them when it creates the VM.
- **What the locked files hand over to the project folder**, which is an `include.path` or `includeIf.*.path` that leads into it, or a hook that's a symlink into it. The VM can edit those files, and git on this machine reads or runs them. devvm warns about them when it creates the VM, and on a launch after they change.
- **Submodules whose git folder is outside the project.** devvm can't lock that folder, because another project's VM may write it, and `git status` on this machine reads its config. devvm names these submodules when it creates the VM.
- **A replaced `.git`.** The VM can rename `.git` and put a new one in its place, or point git at another config. devvm reports this on each launch but doesn't undo it ([what a lock can't stop](../internals/git-protection.md#what-a-lock-cant-stop)).

## Linux hosts

On Linux, devvm runs each VM's QEMU in a user namespace of its own, without root, and there it makes the paths that macOS's flag would lock read-only: the hooks and config of `.git` and of its submodules, and the rest of [what's locked](../internals/git-protection.md#whats-locked). The VM reaches the project folder only through QEMU, so root in the VM gets `Read-only file system` when it writes one of them, and `Device or resource busy` when it renames or removes one, or renames `.git` itself. Nothing changes on this machine: your own git commands write `.git/config` as usual, and the read-only view ends when the VM stops. `devvm git-protect` works as it does on macOS, and a change to a running VM's protection applies at its next start.

Start VMs with devvm. A VM that `limactl start` starts runs without the read-only view, and so does one whose view went for another reason, so each launch of a running VM, and `devvm stop`, check that the VM still can't rename `.git` and still sees its hooks and config read-only, and name what it doesn't: run `devvm restart` then. If devvm's own process that runs the VM ends, the VM ends with it at once, as if you pulled its power, so it never runs without the view.

Because a lock in the VM's view covers one name of a file, devvm also [watches](#when-devvm-names-a-change) `.git` as a Linux host without locks does, and names what changed at each launch that finds the VM stopped and at `devvm stop`. That includes your own changes, such as a `git remote add` or `git push -u`.

### When git on this machine replaces a locked file

The VM sees your changes, because its view is the same folder. A git command on this machine that writes `.git/config`, such as `git config`, `git remote add` or `git push -u`, writes a new file and renames it over the old one, and the new file isn't read-only in the VM until devvm locks it again. devvm watches the folders of what it locks, so it locks the new file within milliseconds, but until then the VM can change it, replace it with a file of its own, which devvm then locks as yours, or open it to write it later, which the lock doesn't stop. A VM running code that watches for that moment can use it, so this is a real gap whenever you change a locked file on this machine while the VM runs. devvm doesn't stop it, but it names it: at `devvm stop`, or at the next launch when the VM stopped another way, devvm names each file it locked again, once however often it happened, and the watch names each one whose contents changed. If devvm couldn't record whether its locks held while the VM ran, it says so then, and you should check `.git`'s hooks and config yourself. Check what it names before you run git in the folder, or stop the VM before you change its `.git` on this machine.

### Hard links

A lock in the VM's view covers one name of a file, and macOS's flag covers the file itself, whatever its names. So devvm doesn't start a VM whose `.git/config`, or a hook in a locked hooks folder, has another name (a hard link), because the VM could change the file through that other name. Make it a file of its own, by copying it and moving the copy over it, and start the VM again. A file that gets another name while the VM runs isn't locked again, and devvm names it when the VM stops.

### What else differs in the VM

Two more things work differently in the VM. The VM sees `.git` as a file system of its own, so renaming a file between the working tree and `.git` fails with `Invalid cross-device link`, which `mv` handles by copying the file, and git never does. And the VM can't remove or rename a submodule's git folder, so `git submodule absorbgitdirs`, or `rm -rf` on a submodule, fails while the VM runs.

A VM that a devvm without these locks made on this machine stays off, and each launch says that devvm can lock it now: run `devvm git-protect on` to lock it, or `devvm git-protect off` to keep it off and stop the warning.

### When Linux can't lock

The locks need a kernel that lets a program make a user namespace and mount in it, which Debian, Fedora, RHEL, openSUSE and Arch do by default. Where the kernel doesn't, devvm doesn't lock `.git`, and it [watches](#when-devvm-names-a-change) it instead. `devvm doctor` says why, and what lets devvm lock:

- **AppArmor**, on Ubuntu 23.10 and newer, lets only programs that a profile names use a user namespace ([Ubuntu](#ubuntu)).
- **`kernel.unprivileged_userns_clone` is 0**, as on Arch's `linux-hardened` kernel. `sudo sysctl kernel.unprivileged_userns_clone=1` lets devvm lock, until the next boot unless you set it in `/etc/sysctl.d/`.
- **`user.max_user_namespaces` is 0**, which `sudo sysctl user.max_user_namespaces=15000` changes.

Without the locks, VMs get `off` as their mode, devvm refuses `devvm git-protect on` and `strict`, and [`--clone`](../using/clone.md) keeps the VM out of `.git` instead, so use it for any repo where you don't trust what runs in the VM. A VM that's on, made when the kernel let devvm lock, doesn't start until it can again, or until you run `devvm git-protect off`.

### Ubuntu

Ubuntu 23.10 and newer set `kernel.apparmor_restrict_unprivileged_userns` to 1, so a program gets a user namespace with the rights to mount in it only when an AppArmor profile names the program with the `userns` rule. Ubuntu does that to keep programs that don't need one out of the kernel code behind it. devvm's binary lives in a folder you can write, such as `~/.local/lib/devvm/`, and a profile for that path would let any program running as you put itself there and get the same rights, which undoes what Ubuntu set the restriction for. So devvm doesn't ask for a profile, and on Ubuntu with the restriction on it doesn't lock `.git`: it [watches](#when-devvm-names-a-change) it, and `devvm doctor` and each launch say so. Use [`--clone`](../using/clone.md) there for any repo where you don't trust what runs in the VM.

### The warning at each launch

On a Linux host that can't lock, each launch of a VM that mounts a folder with a `.git` warns that the VM can change its hooks and config, which run on this machine. That includes the launch that creates the VM, and launches of a VM that's already running. Once you've read the warning and decided to go on without protection, run `devvm git-protect off` in the folder. devvm records that you chose it and stops warning for that VM, but it still names the changes that the next section describes. A VM that you create with `--no-git-protect` doesn't warn either, and a `--clone` VM never warns, because it mounts no `.git`.

### When devvm names a change

On a Linux host that can't lock, devvm can't stop the VM from changing `.git`, and on one that locks, the VM can change a file that git on this machine [replaced](#when-git-on-this-machine-replaces-a-locked-file), but devvm notices the change afterwards on both. When devvm creates a VM, it records a hash of each path that a lock would cover on macOS, which are the hooks in `.git/hooks`, `.git/config` and the rest of [what's locked](../internals/git-protection.md#whats-locked), submodules' included. It also hashes `.git/commondir`, whether it's there or not, and what the config hands over to the project folder: the files that an `include.path` leads to, and the hooks folder that `core.hooksPath` names, such as husky's `.husky`. Each launch that finds the VM stopped, and `devvm stop`, hash them again and name every path that changed since the last check, such as `.git/config` and `.git/hooks/pre-commit`. A warning names ten paths at most, and then how many more changed. The same checks report what a lock can't stop either, as they do on macOS: a `.git` that was replaced, a `commondir`, a repo or submodule that appeared, and includes, hook links and a `core.hooksPath` into the project folder ([not covered](#not-covered)). A launch of a VM that's already running checks nothing, so run `devvm stop` when you want the check. A VM that devvm has no hashes for yet, such as one that an older devvm made, records them at its first check and names nothing.

devvm names a change only after it happened, so git on this machine may already have run a changed hook or read a changed config. devvm can't tell who made a change, so it also names your own, such as a `git remote add` or `git push -u` that you ran on this machine. When it names a path that you didn't change, check it before you run git in the folder:

- `git config --list --show-origin` shows each setting of the repo and the file it comes from. Look for settings that run a program, such as `core.fsmonitor`, `core.hooksPath`, `core.sshCommand`, `core.pager`, `filter.*` and `include.path`.
- `ls -la .git/hooks` lists the hooks, and git runs a hook there by its name, such as `pre-commit`, so read every file whose name doesn't end in `.sample`.

If you find something that you didn't put there, remove it, and use [`--clone`](../using/clone.md) for that repo.

### What the watch can't see

The watch isn't a security boundary, and it doesn't stop anything. It compares what's in `.git` when the VM is stopped with what was there at the last check, so it misses a change that the VM made and then undid before it stopped, even though git on this machine may have run the change in between. It also can't fully watch some paths, and it names them at every check until they're gone:

- **A symlink, or a path under one,** such as a hook that's a symlink, or a `.git` that's a symlink. devvm never follows a symlink out of the project folder, so it can't see changes to what such a link points to. For a link that stays inside the project folder, it hashes what the link leads to as well.
- **A file larger than 16 MiB,** which devvm records by its size, inode and change time, so it notices a change that keeps the size but can't hash the file.
- **A hooks folder with more than 1,000 entries,** which devvm records by its count and change time. It always hashes the hooks that git runs, at the folder's top, before the rest.
- **What's past 2,000 paths in all,** across every git folder and hooks folder of the repo.

A repo rarely has any of these, so when devvm names one that you didn't make, check what it holds, and use [`--clone`](../using/clone.md) for that repo.
