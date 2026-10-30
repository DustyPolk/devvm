---
title: Settings and files
weight: 9
---

This page lists the folders devvm keeps its state and settings in, and the environment variables it reads.

## Where state lives

devvm writes nothing into the project folder. It keeps everything in two folders:

| Folder | Holds |
| --- | --- |
| `~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) | each VM's state (`vms/<vm-name>`), image records (`images/<key>`), rebuild counters (`images/rebuilds/`) and `--from` digests (`images/from-digests`), the shared tools, logs, and devvm's own Lima home (`lima/`) |
| `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) | your settings: the base image's features (`base-features`), the size (`size`), ports (`ports`) and [backend](../backends/_index.md#choosing-a-backend) (`backend`) of new VMs, your own layers and features, the keys in devvm's SSH agent (`agent-keys`), and the [ssh config](ssh.md) for VMs and their host keys (`ssh_config`, `known_hosts`) |

A VM's state file records the image the VM came from, its project folder, its backend and its settings, one `key=value` per line. The file lasts as long as the VM, because `devvm rm` also deletes it, so the next devvm in that folder starts fresh.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Each write replaces a file whole, so a file is never left half written, and only you can read the files. When a file can't be read, devvm names the file and the line, and you fix the line or delete the file.

## Environment

| | |
| --- | --- |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](troubleshooting.md#build-logs-and-interrupts)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](../internals/lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image new VMs' and builds' base starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](../internals/images.md#versions)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads tokens in, instead of `devvm` ([details](../safety/network.md#saving-and-binding-tokens)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude token VMs' proxies put in, over a saved one ([details](../safety/network.md#secrets)) |
