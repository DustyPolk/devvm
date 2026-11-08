---
title: Pasting images
explainer: Paste your clipboard's image into an agent in a VM
weight: 13
description: "devvm clipboard images lets Claude Code and pi in a VM paste the image on your clipboard, such as a screenshot. This page covers turning it on, which agents it works with, what a VM can read once it's on, what this machine needs and what to do when pasting gets nothing."
---

`devvm clipboard images` lets the coding agents in this folder's VM paste the image on your clipboard, such as a screenshot, as they do on this machine, for example with Ctrl+V in Claude Code. It's off in every VM until you turn it on, because once it's on, anything in the VM can read that image while the VM runs ([what a VM can read](#what-a-vm-can-read)). Only an image crosses, from this machine into the VM, and only when something in the VM asks for it. Text never crosses, and nothing the VM copies reaches this machine.

```sh
devvm clipboard          # show whether this folder's VM reads your clipboard's images: images or off
devvm clipboard images   # let agents in the VM paste the image on your clipboard
devvm clipboard off      # stop it
```

## Turning it on

`devvm clipboard` shows `images` or `off`. `devvm clipboard images` and `devvm clipboard off` change it at once, on a running VM too, without a restart. devvm sends a running VM's setup again, and within a second the VM reads your clipboard's images, or stops. A stopped VM gets the change when it next starts. When the VM has the value you ask for already, devvm says so and changes nothing.

Every VM starts with it off, and new VMs have no default for it, so `devvm clipboard` takes no `-g`, `config.toml` has no setting for it and `devvm` has no flag for it. Turn it on in the folder of each VM that you want to paste images into. When you turn it on, devvm warns you that anything in the VM can now read the image on your clipboard while the VM runs, and it also warns you when this machine has nothing to read its clipboard with ([what this machine needs](#what-this-machine-needs)).

In a subfolder without a VM of its own, `devvm clipboard` acts on the parent folder's VM, as the other commands do, and `--here` and `--parent` choose the VM in the same way ([a subfolder](vms.md#a-subfolder)). `devvm clipboard images` gives that VM more access, so in a terminal it names the VM and its folder and asks first, with no as the default, and without a terminal it refuses.

devvm reads the clipboard for a VM in the same way that it carries [links between VMs](links.md), so it needs a backend that can carry links. Lima can, and devvm refuses `devvm clipboard images` for a VM whose backend can't.

## Which agents

Claude Code and pi paste images in a VM with images on. When you paste, each of them runs `xclip` or `wl-paste` to ask for the image on the clipboard, and devvm answers them, which devvm checked with Claude Code 2.1.319 and pi 1.5.0. Codex reads the clipboard through an X11 library rather than a command, and opencode doesn't read images from the clipboard, so neither of them gets an image.

While images are on, the VM's setup puts `xclip` and `wl-paste` in `/usr/local/bin`, which comes before `/usr/bin` on the VM's `PATH`, as links to a small script of devvm's, `devvm-clip`. It adds each link only when `/usr/local/bin` has nothing of that name already, and `devvm clipboard off` removes only the links that devvm made, together with `devvm-clip`. `devvm-clip` answers exactly what the agents ask when you paste:

| Command | What it prints |
| --- | --- |
| `xclip -selection clipboard -t TARGETS -o` | the types on your clipboard, which are `image/png` when it holds an image |
| `xclip -selection clipboard -t image/png -o` | the image on your clipboard, as a PNG |
| `wl-paste --list-types`, or `wl-paste -l` | the types on your clipboard, which are `image/png` when it holds an image |
| `wl-paste --type image/png` | the image on your clipboard, as a PNG, and `-n` or `--no-newline` changes nothing |

Anything else, such as reading text, another type or the primary selection, or copying, goes to the VM's own `xclip` or `wl-paste` in `/usr/bin` when it has one, and fails when it doesn't. So an `xclip` that you install in the VM with apt still reads and copies text on the VM's own [desktop](desktop.md), and in a VM without one, an agent's attempt to copy fails rather than reach this machine.

## What a VM can read

While the VM runs with images on, anything in it can read the image on your clipboard, at any time, and not only when you paste. That includes a command that an agent runs on its own and a script that a package installs. devvm can't tell your paste from any other request, so keep images off in a VM that runs code you don't trust, and turn them off with `devvm clipboard off` once you're done pasting. An image that you copy for another app, such as a screenshot of a document, is readable by the VM until you copy something else.

What the VM gets is only the image's pixels. devvm reads the image itself, decodes it and encodes it again as a PNG, which drops its metadata, such as a photo's EXIF location, and anything after the image, so no text crosses. When the clipboard holds text alone, the VM gets nothing. Nothing goes the other way, so what the VM copies never reaches this machine's clipboard.

`devvm network log` in the VM's folder shows every image devvm sent and every request it refused, whatever the VM's [network tier](../safety/network.md#network-tiers):

```text
2026-11-07T10:00:00Z  clipboard  clipboard:7737  image sent, 48213 bytes
2026-11-07T10:00:09Z  REFUSED    clipboard:7737  clipboard image: no image
```

A refused line ends with why devvm sent nothing:

| Reason | |
| --- | --- |
| `bad request` | the VM asked for something other than the types or the image |
| `no image` | the clipboard held no image when the VM asked for the image itself |
| `too large` | the image was larger than devvm sends ([what this machine needs](#what-this-machine-needs)) |
| `not an image` | the clipboard's data wasn't a PNG or JPEG image that devvm could decode |
| `no reader` | this machine had nothing to read its clipboard with ([what this machine needs](#what-this-machine-needs)) |
| `reader failed` | the command that reads this machine's clipboard failed in another way |

An agent asks for the clipboard's types before its image, and anything in the VM can ask for the types, so devvm logs an empty clipboard only when the VM asks for the image itself. A read that fails is logged either way. [devvm network log --json](../safety/network.md#devvm-network-log---json) gives these as `clipboard` and `clipboard_refused` events.

## What this machine needs

devvm reads this machine's clipboard only when the VM asks for it, with a command that it runs on this machine:

- **On macOS,** devvm reads the clipboard's image as a PNG with `osascript`, which every Mac has.
- **On Linux,** devvm reads it with `wl-paste`, from wl-clipboard, under Wayland, when `WAYLAND_DISPLAY` is set, and otherwise with `xclip` under X11, when `DISPLAY` is set. It reads the clipboard's PNG, or its JPEG when it has no PNG. Install wl-clipboard or xclip, whichever your desktop needs.

devvm reads the clipboard with the environment of the devvm command that started the VM, or that turned images on while the VM ran. So on Linux, a VM started from a terminal without `WAYLAND_DISPLAY` or `DISPLAY`, such as an ssh login, can't read your clipboard. devvm warns you when you turn images on, and at each start of a VM with images on, when this machine has nothing to read its clipboard with. Then install wl-clipboard or xclip, and run `devvm restart` from a terminal on your desktop.

devvm reads at most 32 MiB of the clipboard's image, and it refuses an image larger than 16384 pixels on a side or 50 megapixels in all before decoding it. An agent asks for the types and then for the image within one paste, so devvm reads the clipboard once for any requests within 3 seconds of each other, and they all get the same image.

## When pasting gets nothing

- **The VM has images off:** `devvm clipboard` shows `off`. Run `devvm clipboard images`.
- **The agent isn't Claude Code or pi:** Codex and opencode don't paste images in a VM ([which agents](#which-agents)).
- **The clipboard holds no image:** `devvm network log` shows `no image`. Copy the image again, as an image rather than a file.
- **This machine can't read its clipboard:** devvm warned you when the VM started, and `devvm network log` shows `no reader`. Install wl-clipboard or xclip, and run `devvm restart` from a terminal on your desktop ([what this machine needs](#what-this-machine-needs)).
- **The image is too large:** `devvm network log` shows `too large`. Copy a smaller image, such as a screenshot of part of the screen.
- **A program in the VM listens on the clipboard's address:** devvm warns you that pasting images doesn't work in the VM, because a program there already listens on the address where devvm answers. Stop that program, then run `devvm restart`.
- **The VM has an `xclip` or `wl-paste` of its own in `/usr/local/bin`:** devvm doesn't replace it, so the agent reads the VM's own clipboard instead. Remove it, then run `devvm` in the folder, whose setup links devvm's in its place.

## The desktop's clipboard

`devvm clipboard` and [`devvm desktop --clipboard`](desktop.md#the-clipboard) share different things. `devvm clipboard images` sends only the image on your clipboard, only into the VM, to whatever in the VM asks for it, in any VM and from any shell. `devvm desktop --clipboard` shares text in both directions between this machine and the VM's desktop, for that run of `devvm desktop` alone, and only in a VM with the desktop layer.

[How it works](../internals/ports.md#clipboard-images) explains how devvm answers the VM.

### devvm clipboard --json

`devvm clipboard --json` prints one JSON object with `clipboard`, which is `images` or `off`, as [output for scripts](command-line.md#output-for-scripts) describes. With `--json` the command only shows, so devvm refuses `images` or `off` with it.

```json
{
  "version": 1,
  "clipboard": "off"
}
```
