---
title: The desktop
weight: 4
---

This page explains how `devvm desktop` serves a VM's desktop on this machine, how it checks a viewer's password and keeps the clipboard out of the VM, and why devvm uses VNC and never forwards X11. [A VM's desktop](../using/desktop.md) covers adding the desktop and opening it.

## In the VM

The built-in `desktop` layer installs Xfce, TigerVNC's `Xvnc` and a command of devvm's, `devvm-desktop`, into the `desktop` image, which only the VMs with the layer are made from ([the desktop layer](images.md#the-desktop-layer)). Its packages come from base's package snapshot, as base's own do, and it downloads nothing else. When you run `devvm desktop`, devvm first checks that the VM has `devvm-desktop`, and refuses a VM without it (`desktop_no_layer`). Then it runs `devvm-desktop start off`, or `start on` with `--clipboard`, as the VM's user.

`devvm-desktop` starts `Xvnc` on display `:1` unless it's running. `Xvnc` listens on the socket `/run/devvm-desktop/vnc.sock` alone, in a folder of the VM's user's own (mode 700), the socket itself mode 600. It listens on no TCP port (`-rfbport -1`), and takes no X11 connections over TCP (`-nolisten tcp`), so devvm's port forwarder never sees it, even on a VM that forwards every port. It asks for no password of its own (`-SecurityTypes None`), because devvm checks the viewer's password on this machine first. The clipboard settings (`AcceptCutText`, `SendCutText`, `SetPrimary`, `SendPrimary`) are on or off as asked, and on a desktop that runs already, `devvm-desktop` changes them in place with `vncconfig`. Then it starts an Xfce session on the display, in a D-Bus session of its own. Nothing starts the desktop at boot, so a VM whose desktop you never open runs none. Its log is `~/.cache/devvm-desktop.log` in the VM.

## On this machine

`devvm desktop` listens on `127.0.0.1` alone, on the VM's port. That's the port its state records as `desktop_port` while no program on this machine has it, and otherwise a new one that no program and no VM has, which devvm records in its place. New ports come from 20000 up to just below the range this machine gives outgoing connections, which starts at 32768 on Linux by default and 49152 on macOS, because Linux refuses a listener on a port that a live outgoing connection holds. devvm reads that range from the system, and when it can't, or the range leaves fewer than 1000 ports above 20000, it picks from 20000 to 32767. The port is among those devvm records for VMs, with their ssh and proxy ports, so no VM's [port forwarder](ports.md#what-it-never-takes) takes it and no new VM gets it for its ssh or its proxy. devvm listens without address reuse, so a program that has the port on any address keeps it. The listener stays open until you press Ctrl+C, or until the VM no longer runs, which devvm checks every 5 seconds (`desktop_vm_stopped`). Nothing else listens for the desktop on this machine, at any other time.

Each connection to the port goes through devvm's relay, which runs in `devvm desktop`:

1. The relay speaks VNC's protocol (RFB 3.8, and 3.7 and 3.3 for older viewers) as a server, and offers VNC's own authentication alone. It sends a random challenge and checks the viewer's answer against the VM's password, with one check at a time, so a wrong answer holds every other viewer's check for a second. A viewer whose answer is wrong is refused and never reaches the VM.
2. Only then does the relay connect to the VM: an ssh channel to the VM's socket (`ssh -W /run/devvm-desktop/vnc.sock`), through devvm's ssh config's entry for the VM, with its pinned host key. The channel opens no session, and it carries no agent and no forwarding either way (`IdentityAgent=none`, `ForwardAgent=no`, `ForwardX11=no`, `ClearAllForwardings=yes`). The relay does the viewer's side of the VM's handshake, asking for no authentication, and tells the viewer it's in.
3. From then on it carries the viewer's messages to the VM and the VM's to the viewer. Without `--clipboard`, it reads each of the viewer's messages and drops its clipboard ones (`ClientCutText`, extended clipboard ones included). It ends a connection that sends a message it doesn't know, so nothing it can't read reaches the VM.

At most 8 viewers connect at once. The VM's password is 8 characters of lowercase letters and digits without `0`, `o`, `1`, `l` and `i`, about 40 bits, because VNC's authentication uses 8 characters at most. devvm makes it the first time and keeps it in the VM's state as `desktop_password`, in a file that only you can read. It never goes into the VM, and devvm prints it only with the address, never into the run log.

## What the VM can and can't change

Root in the VM can change its desktop, or run another server on the socket in its place. The relay doesn't depend on the VM's server for its guarantees:

- **The password:** the relay checks it on this machine before it opens the channel, so a VM's server that asks for none still gets no viewer without it.
- **The address:** the relay listens on this machine's loopback, so no server in the VM reaches other machines through it. A server root starts on a TCP port of the VM's is an ordinary server of the VM's, which the VM's [ports](../using/ports-size.md#ports) forward to this machine's loopback, or don't, as for any other.
- **This machine's clipboard:** without `--clipboard`, the relay drops it, whatever the VM's server accepts.
- **No way out:** every connection starts on this machine, and the VM opens none to it through the relay. The channel is a local forward that devvm's ssh asks for, which the VM's sshd connects to the VM's own socket. With network policy, the VM's own connections still reach only its proxy.

The relay can't stop a VM's server from sending its clipboard to the viewer, because that would mean reading every screen update the server sends. So the VM's clipboard stays off this machine only while its server is as devvm set it up, or while the viewer doesn't take it, which Screen Sharing's Use Shared Clipboard setting decides.

## Why not X11

ssh's X11 forwarding gives programs in the VM a connection to this machine's display, and an X11 program on a display can read the keys typed into other windows on it, read their contents and send them input. So devvm turns X11 forwarding off on both sides of the VM's ssh. Its ssh config's entries set `ForwardX11 no`, and the VM's sshd has `X11Forwarding no` ([ssh into VMs](security.md#ssh-into-vms)). With VNC, the VM's programs draw on a display of the VM's own, and this machine only shows a picture of it, so nothing in the VM reaches this machine's windows.
