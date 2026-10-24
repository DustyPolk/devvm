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

## Build logs and interrupts

The output of the commands devvm runs goes to a log, not the terminal: devvm prints its own steps, and the end of the log when a step fails. `DEVVM_VERBOSE=1` prints the output instead.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, readable only by you. devvm keeps the newest 20. Each command is a `+` line with its arguments, then its output. A numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

Ctrl+C, or closing the terminal, stops the command running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.
