---
title: A VM's desktop
weight: 8
---

A VM has no screen of its own, but with the `desktop` feature it gets an Xfce desktop, which `devvm desktop` opens in a window on this machine. This page covers adding the desktop, opening it, its password and the clipboard, and what it costs. [The desktop](../internals/desktop.md) explains how devvm keeps it to this machine.

## Adding the desktop

The desktop is one of base's [features](customizing.md#base-image-features), so `base` and every VM made from it get it once you choose it. devvm never chooses it for you, because it makes `base` about 370 MB bigger. To add it, run `devvm setup` and turn on `desktop` in the picker, or name every feature you want:

```sh
devvm setup --features omb,fzf,cli-tools,desktop
```

The new choice rebuilds `base`, and the images built on it, the next time they're used. A VM keeps the image it was made from, so a VM made before you chose the desktop has none, and `devvm desktop` says so. To give it one, save any work the VM holds outside the project folder, then run `devvm rm` and `devvm` to make a new VM.

The desktop works on every backend, Lima and smolvm, and on a base made `--from` a Debian, Ubuntu or Alpine image.

## Opening it

```sh
devvm desktop              # serves this folder's VM's desktop and opens it in Screen Sharing
devvm desktop --no-open    # only shows its address and password
devvm desktop --clipboard  # shares the clipboard both ways, for this run
```

`devvm desktop` gets the VM ready as `devvm host` does, and starts it if it's stopped. Then it starts the VM's desktop, unless it's running already, and shows its address and password:

```text
devvm-website-0123456789abcd's desktop is at vnc://127.0.0.1:41234, with the password k3mxq7tw.
It stays there until you press Ctrl+C.
```

On macOS, devvm opens the address in Screen Sharing, which comes with macOS and asks for the password. Screen Sharing can keep the password in your keychain, and the address stays the same each time. On Linux, open the address in a VNC viewer of your own, such as TigerVNC's `vncviewer` or Remmina.

The desktop stays at that address until you press Ctrl+C, or until the VM stops, and then `devvm desktop` ends. The desktop goes on running in the VM, so the windows you left open are there the next time you run `devvm desktop`. It's 1600 by 1000 pixels.

The desktop is only ever on this machine, and only while `devvm desktop` runs:

- devvm serves it on this machine's `127.0.0.1` alone, so nothing else on your network reaches it. Each VM keeps its port, which no other VM and no program on this machine has. When a program has taken it, devvm chooses another one and keeps that.
- In the VM, the desktop listens on no port at all, so a VM that [forwards every port](ports-size.md#ports) never forwards it.
- With [network policy](../safety/network.md), the VM still reaches only its proxy while you use its desktop.

## The password

Each VM has its own password of eight lowercase letters and digits. devvm makes it the first time you run `devvm desktop` and keeps it with the VM's state on this machine, so `devvm rm` deletes it. The password never goes into the VM, and it isn't in devvm's run log. devvm checks it on this machine before a viewer reaches the VM, whatever the VM's own desktop asks for.

Other users and programs on this machine can also connect to `127.0.0.1`, so the password is what keeps them out. VNC passwords are eight characters at most, and devvm lets a viewer try one password a second, so guessing it would take thousands of years.

## The clipboard

The clipboard isn't shared unless you add `--clipboard`. With sharing on, what you copy on this machine goes into the VM, where a coding agent can read it, and what the VM copies lands on this machine's clipboard. Without `--clipboard`, devvm keeps this machine's clipboard out of the VM, even when something in the VM has changed its desktop's settings, and the VM's desktop doesn't send its clipboard.

`--clipboard` turns sharing on in both directions for that run, and the next run without it turns sharing off again. devvm can't stop a VM whose desktop was changed from sending its clipboard to Screen Sharing, so when you don't want the VM's clipboard on this machine, also turn off Use Shared Clipboard in Screen Sharing's Edit menu.

## What it costs

- About 370 MB more in `base`, which every VM made from it shares.
- About 500 MB of the VM's memory while its desktop runs, which it does from your first `devvm desktop` until the VM stops. A VM whose desktop you never open uses none.

## When the desktop doesn't open

- **The VM has no desktop:** its image was built without the `desktop` feature. [Add it](#adding-the-desktop), then make the VM again.
- **The desktop didn't start:** devvm shows the end of its run log. The desktop's own log is `~/.cache/devvm-desktop.log` in the VM.
- **Screen Sharing didn't open:** open the address devvm shows in Screen Sharing yourself, by typing it into Finder's Go > Connect to Server, or in another VNC viewer.
- **Screen Sharing says the password is wrong:** use the password `devvm desktop` shows for this VM. Each VM has its own.
