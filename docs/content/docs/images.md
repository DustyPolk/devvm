---
title: Images and VMs
weight: 20
---

## Where state lives

Nothing is written into the project folder. devvm keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`) and ports (`ports`) of new VMs, your own layers and features, and the keys in devvm's SSH agent (`agent-keys`) |

A VM's state file records the image it came from, its project folder, its backend and its settings, one `key=value` per line. It lasts as long as the VM: `devvm rm` deletes it too, so the next devvm in that folder starts fresh.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, to a sandbox or a shorter path.

Each write replaces a file whole, so a file is never left half written. Files are readable only by you. When a file can't be read, devvm names it and the line: fix the line or delete the file.
