---
title: Settings and files
weight: 11
---

devvm writes nothing into your project folders. It keeps your settings in `~/.config/devvm` and its own state in `~/.local/share/devvm`, and it reads a few environment variables. [Overview](../internals/overview.md#state-files) describes the state files in full.

## Your settings

`~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) holds your settings. Each file has a command that writes it, and you can also edit the files yourself.

| File | What it holds | Written by |
| --- | --- | --- |
| `base-features` | the [features](customizing.md#choosing-features) that base gets, one name per line | `devvm setup` |
| `backend` | the [default backend](../backends/_index.md#choosing-a-backend) for new VMs | `devvm setup` |
| `size` | the [size](ports-size.md#size-and-ports) of new VMs | `devvm size -g`, `devvm setup` |
| `network` | the [network policy](../safety/network.md) that new VMs get: `ask`, `on` or `off` ([devvm setup](#devvm-setup)) | `devvm network -g`, `devvm setup` |
| `ports` | the [ports](ports-size.md#size-and-ports) that new VMs forward | `devvm ports -g` |
| `retention` | what devvm deletes on its own, and after how long ([retention](#retention)) | `devvm retention` |
| `policy` | your [network policy](../safety/network.md#changing-rules) rules for every VM | `devvm network allow -g`, `deny -g`, `rm -g` |
| `layers/` | your own [layers](customizing.md#your-own-layers) and their allowlists | `devvm layer new` |
| `features/` | your own [features](customizing.md#your-own-features) | you |
| `ca/` | your [company's CA certificates](#a-companys-ca), which VMs and builds trust | you |
| `agent-keys` | the keys in [devvm's own SSH agent](ssh.md#devvms-own-agent) and their options | `devvm ssh-agent add` |
| `ssh_config`, `known_hosts` | the [ssh config](ssh.md#ssh-access-by-name) for running VMs and their host keys | devvm, on each launch and stop |

On Linux hosts, devvm also keeps your [secrets](../safety/network.md#saving-and-binding-secrets) in files under `~/.config/devvm/secrets/`, which only you can read. On macOS they're in the Keychain.

Upgrading devvm keeps this folder as it is. Back it up to keep your settings, layers and features, and keep in mind that on Linux hosts it holds your secrets.

## devvm setup

`devvm setup` chooses what new VMs get. In a terminal, it asks four questions, each starting from the current choice, and saves each answer where the command for that setting saves it:

1. The [default backend](../backends/_index.md#choosing-a-backend) for new VMs, saved in `backend`.
2. The [features](customizing.md#choosing-features) that base gets, saved in `base-features`.
3. The [CPUs and memory](ports-size.md#choosing-new-vms-size) that new VMs get, saved in `size`, as `devvm size -g` saves them.
4. The [network policy](../safety/network.md) that new VMs get, saved in `network`, as `devvm network -g` saves it. With `ask`, the default, devvm asks for each new VM in a terminal, and a new VM gets network policy when there's no terminal to ask in. With `on` or `off`, devvm makes every new VM that way without asking. `--network-policy` and `--no-network-policy` choose for one VM and win over the saved choice.

```sh
devvm setup                          # asks all four questions
devvm setup --backend smolvm         # the default backend, without asking
devvm setup --features fzf,omb       # base's features, without asking
devvm setup --features none          # base gets no features
```

`devvm setup` has two flags of its own, and each one chooses without asking anything. `--backend NAME` chooses the default backend. `--features LIST` chooses base's features, as a comma-separated list or `none` for none, and devvm refuses an empty `--features ''`. With either flag, what the flags leave out stays as it is. Without a terminal and without flags, `devvm setup` asks nothing and keeps base's features, saving every feature when none was chosen yet. To change new VMs' size without asking, use `devvm size -g` ([choosing new VMs' size](ports-size.md#choosing-new-vms-size)), and to change their network policy, use `devvm network -g` ([turning it on or off](../safety/network.md#turning-it-on-or-off)).

## Retention

devvm keeps what it makes until you delete it, and the retention settings tell it to delete some of it on its own. Every setting is off until you set it, so first run `devvm space` to see what each one would free: its `RETAIN` column shows each setting that's on, beside the row it applies to, with what it would delete now ([devvm space](space.md#devvm-space)).

`devvm retention` shows every setting and its value, and `off` for each one that isn't set. `devvm retention KEY=VALUE...` sets them, and `devvm retention KEY=off` turns one off, for example `devvm retention history.age=90d tools.unused-age=30d`. devvm refuses a key or value it doesn't know, and a key with no value such as `history.age=`, and then it writes nothing. The settings are `key=value` lines in `~/.config/devvm/retention`, and when a line can't be read, devvm names the file and the line and applies none of them.

| Key | What devvm deletes |
| --- | --- |
| `logs.runs` | the [run logs](troubleshooting.md#a-command-fails) beyond the newest this many, which is 20 when it's off |
| `history.age` | the [exports of agents' history](coding-agents.md#history) written this long ago or more, such as `90d` |
| `history.count` | the exports of agents' history beyond the newest this many |
| `tools.unused-age` | the current shared tools that no VM has had for this long, counted from when devvm first found that no VM has them. While a VM is being made or is getting new shared tools, retention deletes none of them in that run, because that VM may not list the tools it picked yet, and it still applies the other settings |
| `orphans.age` | what devvm keeps for VMs that their backend no longer has, once their state hasn't changed for this long, as `devvm gc --orphans` does |
| `nix-cache.size` | the [shared Nix cache](toolchains.md#nix-projects)'s least recently used files, while it takes more than this, such as `20G`; it's 10 GiB when it's off |

Ages are a whole number of days, weeks or hours, such as `30d`, `2w` or `12h`. Sizes are a whole number of `K`, `M`, `G` or `T`, each 1024 of the one before, such as `500M` or `20G`, as `devvm space` shows them. The shared Nix cache also keeps to its size while VMs fill it, because a VM's proxy deletes the cache's least recently used files before a new file would take it past that size. A size you set applies to each VM from its next start. devvm applies the settings at the start of a command, at most once a day, except for `orphans.age`, which needs to ask each backend about its VMs, so only `devvm gc --retention` applies it. `devvm gc --retention` applies all of them at once. When it deletes something, devvm says what in one line. Retention never deletes a VM, an image, a shared tool that any VM's state lists, or anything in the backends' own caches, which only `devvm gc --caches` frees. When two devvms start at once, only one of them applies the settings.

## A company's CA

Some company networks inspect TLS through a proxy, such as Zscaler or Netskope, that shows its own certificates, signed by the company's certificate authority (CA). This machine trusts that CA, but VMs and the VMs that devvm builds images in don't, so their TLS connections fail. To make them trust it, put the CA's certificate in `~/.config/devvm/ca`:

```sh
mkdir -p ~/.config/devvm/ca
cp company-root.pem ~/.config/devvm/ca/
```

devvm reads every `*.pem` and `*.crt` file in that folder each time you open a VM and each time it builds an image, and it leaves other files alone. A file can hold one certificate or several, in PEM, the text form that starts with `-----BEGIN CERTIFICATE-----`. devvm accepts only CA certificates. When a file holds anything else, such as a private key, a certificate devvm can't read or a certificate that isn't a CA's, devvm refuses the whole folder and names the file. It sends only the certificates it read out of a file, never the rest of the file. devvm doesn't follow links there, so copy each certificate into the folder rather than linking to it. When a `*.pem` or `*.crt` file is a link, or the folder itself is one, devvm refuses the whole folder and names the link, because a link into a project folder would let that project's VM replace the CA that every VM, build and proxy trusts. devvm also refuses the folder, naming the file, when it can't read a file there or the folder itself.

Every project VM trusts these CAs in its system's store from its next launch, and so does every VM that devvm builds an image or shared tools in. devvm takes them out of a build VM before the image is finished, so images never keep them, and adding or removing a CA rebuilds nothing. A layer script of yours that copies the system's CA bundle somewhere else does keep that copy. With [network policy](../safety/network.md), the VM's proxy also trusts them for the connections it makes itself. When you take a certificate out of the folder, VMs stop trusting it at their next launch, and stores that copy the system's CAs, such as Java's `cacerts`, drop it too.

Most tools read the system's store, among them apt, apk, curl, git and Python's `ssl` module. For those that don't, every VM's environment points them at it:

| Variable | Value | For |
| --- | --- | --- |
| `NODE_EXTRA_CA_CERTS` | `/etc/ssl/devvm.pem`, which holds only devvm's CAs and is empty when there are none | Node, npm and Bun |
| `SSL_CERT_FILE` | `/etc/ssl/certs/ca-certificates.crt`, the system's bundle | Go programs, and Rust programs that read the system's roots |
| `REQUESTS_CA_BUNDLE`, `PIP_CERT` | the system's bundle | Python's `requests` and pip, which use their own CA list otherwise |
| `UV_SYSTEM_CERTS` | `1` | uv |

A CA in the folder can read every TLS connection a VM makes, as it can this machine's, so put only CAs there that you trust as much as this machine does. [Network policy](../internals/network.md#a-companys-ca) explains how the CAs reach VMs, and [Images and caching](../internals/images.md#a-companys-ca-in-builds) how builds use them.

## devvm's state

`~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) holds what devvm makes and keeps for you:

- each VM's state, its own allowlist and its network logs;
- the images and their records, and the shared tools;
- the [run logs](troubleshooting.md#a-command-fails) in `logs/runs/`;
- the agents' history that `devvm history export` and `devvm rm` save, in `exports/`;
- devvm's own Lima home (`lima/`), where the Lima backend's VMs and images live.

[What devvm keeps on this machine](space.md) lists everything in this folder, with how each is cleaned up.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Only you can read devvm's files. Each time devvm runs, it makes `~/.config/devvm` and `~/.local/share/devvm` yours alone (mode 700), whatever mode they had, so nothing in them is open to others even when a file or folder inside has a wider mode. When devvm can't read one, it names the file and the line ([a file devvm can't read](troubleshooting.md#a-file-devvm-cant-read)).

## Project folders devvm refuses

A VM mounts its project folder read-write, so devvm refuses to make a VM for a folder that would give the VM more than the project. It refuses a folder that is or holds your home folder, `~/.config/devvm`, `~/.local/share/devvm` or devvm's Lima home. It also refuses a folder inside one of devvm's own folders, because a VM there could rewrite the scripts that every image build runs, or the files that devvm runs on this machine.

When devvm refuses a folder, make the VM in a project folder inside it, or keep the project somewhere else. A [`--clone`](clone.md) VM mounts nothing from the folder, so devvm doesn't refuse its folder.

> [!WARNING]
> If you mean to give the VM the whole folder, pass `--unsafe-folder` when you make the VM, and devvm warns you that the VM gets read-write access to it ([only the project folder](../internals/security.md#only-the-project-folder)).

## Colors

In a terminal, devvm colors the labels and links around its messages, while the messages' own text stays in your terminal's usual color. The values a message fills in, such as a VM's name, a path, a port or a count, are bold, so they stand out from the words around them. An error's `devvm:` label is red and a warning's is yellow, a doc link is blue and underlined, and the `==>` before what devvm did is bold. When a step fails, the end of the run log that devvm prints is dim, and so is the log's latest line on the status line. The mark a slow step leaves when it's done is green.

The tables that devvm ls, devvm images, devvm space and devvm doctor print are colored too. Each table's header row and the totals row are bold, and a state is green when it's good, such as current, Running or native, yellow when it needs your attention, such as stale, unused, Stopped or emulated, and red when something is wrong, such as an unenforced network policy or an unreviewed emulation. Digests, image keys and a feature a backend doesn't have, which devvm doctor lists as none, are dim, and names, paths, sizes and counts stay in your terminal's usual color. The columns line up the same with colors or without.

devvm uses only your terminal's 16 base colors, so your theme picks shades that read well on its background, light or dark. It never sets a background color.

devvm decides for standard output and standard error apart, and colors one only when it's a terminal, so output you pipe or redirect to a file stays plain, tables included. devvm space `--json` and devvm doctor `--fixes` are plain even in a terminal, and the run log is never colored. These environment variables change when devvm colors, and the first one that applies decides:

- `NO_COLOR`, set to anything but an empty value, turns colors off.
- `FORCE_COLOR` or `CLICOLOR_FORCE`, set to anything but `0`, turns them on even when the output isn't a terminal.
- `TERM=dumb` turns them off.

## Environment

| | |
| --- | --- |
| `NO_COLOR`, `FORCE_COLOR`, `CLICOLOR_FORCE` | turn devvm's colors off, or on when the output isn't a terminal ([details](#colors)) |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](troubleshooting.md#a-command-fails)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](../internals/lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image that the base of new VMs and builds starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#shared-tools)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads secrets in, instead of `devvm` ([details](../safety/network.md#saving-and-binding-secrets)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude Code token that VMs get, over a saved one ([details](coding-agents.md#claude-code-login)) |
