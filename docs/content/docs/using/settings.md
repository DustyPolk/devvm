---
title: Settings and files
weight: 10
---

devvm writes nothing into your project folders. It keeps your settings in `~/.config/devvm` and its own state in `~/.local/share/devvm`, and it reads a few environment variables. [Overview](../internals/overview.md#state-files) describes the state files in full.

## Your settings

`~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) holds your settings. Each file has a command that writes it, and you can also edit the files yourself.

| File | What it holds | Written by |
| --- | --- | --- |
| `base-features` | the [features](customizing.md#choosing-features) that base gets, one name per line | `devvm setup` |
| `backend` | the [default backend](../backends/_index.md#choosing-a-backend) for new VMs | `devvm setup` |
| `size` | the [size](ports-size.md#size-and-ports) of new VMs | `devvm size -g` |
| `ports` | the [ports](ports-size.md#size-and-ports) that new VMs forward | `devvm ports -g` |
| `policy` | your [network policy](../safety/network.md#changing-rules) rules for every VM | `devvm policy -g` |
| `layers/` | your own [layers](customizing.md#your-own-layers) and their allowlists | `devvm layer new` |
| `features/` | your own [features](customizing.md#your-own-features) | you |
| `agent-keys` | the keys in [devvm's own SSH agent](ssh.md#devvms-own-agent) and their options | `devvm ssh-agent add` |
| `ssh_config`, `known_hosts` | the [ssh config](ssh.md#ssh-access-by-name) for running VMs and their host keys | devvm, on each launch and stop |

On Linux hosts, devvm also keeps your [secrets](../safety/network.md#saving-and-binding-secrets) in files under `~/.config/devvm/secrets/`, which only you can read. On macOS they're in the Keychain.

Upgrading devvm keeps this folder as it is. Back it up to keep your settings, layers and features, and keep in mind that on Linux hosts it holds your secrets.

## devvm's state

`~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) holds what devvm makes and keeps for you:

- each VM's state, its own allowlist and its network logs;
- the images and their records, and the shared tools;
- the [run logs](troubleshooting.md#a-command-fails) in `logs/runs/`;
- the agents' history that `devvm export` and `devvm rm` save, in `exports/`;
- devvm's own Lima home (`lima/`), where the Lima backend's VMs and images live.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Only you can read devvm's files. When devvm can't read one, it names the file and the line ([a file devvm can't read](troubleshooting.md#a-file-devvm-cant-read)).

## Project folders devvm refuses

A VM mounts its project folder read-write, so devvm refuses to make a VM for a folder that would give the VM more than the project. It refuses a folder that is or holds your home folder, `~/.config/devvm`, `~/.local/share/devvm` or devvm's Lima home. It also refuses a folder inside one of devvm's own folders, because a VM there could rewrite the scripts that every image build runs, or the files that devvm runs on this machine.

When devvm refuses a folder, make the VM in a project folder inside it, or keep the project somewhere else. A [`--clone`](clone.md) VM mounts nothing from the folder, so devvm doesn't refuse its folder. If you mean to give the VM the whole folder, pass `--unsafe-folder` when you make the VM, and devvm warns you that the VM gets read-write access to it ([only the project folder](../internals/security.md#only-the-project-folder)).

## Environment

| | |
| --- | --- |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](troubleshooting.md#a-command-fails)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](../internals/lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image that the base of new VMs and builds starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#shared-tools)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads secrets in, instead of `devvm` ([details](../safety/network.md#saving-and-binding-secrets)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude Code token that VMs get, over a saved one ([details](coding-agents.md#claude-code-login)) |
