---
title: Images and VMs
weight: 20
---

## Caching

Images are built in layers and cached, like Docker's. `base` comes first, and each custom layer is built on its parent: `foo+bar` is `bar` on `foo` on `base`. A layer starts from its parent, runs its script inside, and is stored under a key, a hash of everything that goes into it. When an image with the key already exists, devvm uses it; otherwise it builds one.

| Image | Its key hashes |
| --- | --- |
| `base` | the backend, `base.yaml`, `layers/base.sh`, the chosen features in order (each name and its script), its rebuild counter, and with `--from`, the image, its digest and the scripts that convert it |
| a custom layer | its parent's key, its name, its script (`~/.config/devvm/layers/<name>.sh`) and its rebuild counter |

So:

- Edit a script and its layer and every layer above it rebuild on next use. The layers below are reused.
- Each backend builds its own images, and never uses another's.
- With `--from`, base's key has the image's digest, recorded when devvm looks it up. When the tag moves to a new image, base and everything on it rebuild.
- A layer whose script is gone hashes as missing, so its images no longer match.
- Nothing else counts: features you haven't chosen, and network policy files (`*.policy`, which the proxy reads at run time and never go into an image), change no key.

An image whose key no longer matches its inputs is stale. It stays until it's deleted, and the next use builds a new one.

Each shared tool image is named `<tool>-<version>-<key>.erofs`, and its key hashes the tool's entry in the tools lock (its version, downloads and their checksums), `tools/build.sh` and the C library it's built for. A new version, a new download or an edited build script builds it again, and a base with a different C library (Alpine's musl) gets its own.

A key is the first 12 hex digits of a SHA-256. Each input goes in with its length, so different inputs never hash the same bytes. An image's record keeps its chain: the key of each image it's built on, from `base` up.

## Where state lives

Nothing is written into the project folder. devvm keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`), ports (`ports`) and [backend](backends.md#choosing-a-backend) (`backend`) of new VMs, your own layers and features, and the keys in devvm's SSH agent (`agent-keys`) |

A VM's state file records the image it came from, its project folder, its backend and its settings, one `key=value` per line. It lasts as long as the VM: `devvm rm` deletes it too, so the next devvm in that folder starts fresh.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, to a sandbox or a shorter path.

Each write replaces a file whole, so a file is never left half written. Files are readable only by you. When a file can't be read, devvm names it and the line: fix the line or delete the file.

## Build logs and interrupts

The output of the commands devvm runs goes to a log, not the terminal: devvm prints its own steps, and the end of the log when a step fails. `DEVVM_VERBOSE=1` prints the output instead.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, readable only by you. devvm keeps the newest 20. Each command is a `+` line with its arguments, then its output. A numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

Ctrl+C, or closing the terminal, stops the command running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.
