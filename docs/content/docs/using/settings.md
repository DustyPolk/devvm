---
title: Settings and files
weight: 11
---

devvm writes nothing into your project folders. It keeps the settings you edit in `~/.config/devvm` and everything it writes itself in `~/.local/share/devvm`, and it reads a few environment variables. [Your settings](#your-settings) lists the files you edit, [devvm's data folder](space.md#devvms-data-folder) lists the ones devvm writes, and [Overview](../internals/overview.md#state-files) explains how devvm keeps them.

## Your settings

`~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`) holds what you edit, and it's the only folder of devvm's that you should change by hand. devvm never rewrites a file of yours there. It reads `config.toml` and your own layers, features and CAs, it creates a new layer's files when you run `devvm layer new`, it changes single lines of `policy` when you add or remove a rule for every VM, and it writes `version` when the folder has none and when it changes its folders to a newer layout.

| File | What it holds | Written by |
| --- | --- | --- |
| `config.toml` | the defaults that new VMs get and the [retention settings](#retention), which win over what devvm's commands save ([config.toml](#configtoml)) | you |
| `policy` | your [network policy](../safety/network.md#changing-rules) rules for every VM | `devvm network allow -g`, `deny -g`, `rm -g`, or you |
| `layers/` | your own [layers](customizing.md#your-own-layers) and their allowlists | `devvm layer new`, then you |
| `features/` | your own [features](customizing.md#your-own-features) | you |
| `ca/` | your [company's CA certificates](#a-companys-ca), which VMs and builds trust | you |
| `version` | the folder's [layout version](troubleshooting.md#a-newer-devvm-changed-devvms-folders), which says how devvm keeps its files here | devvm, when it's missing or older |

Edit the files in `~/.config/devvm`, and never edit the ones in `~/.local/share/devvm`. devvm writes those itself and rewrites them whole, so a change you make there can be lost, or can stop devvm from reading the file. To set a default by hand, put it in `config.toml` instead.

Upgrading devvm keeps this folder as it is. Back it up, such as in a dotfiles repo, to keep your settings, layers and features.

## config.toml

`~/.config/devvm/config.toml` sets the defaults that new VMs get, and the retention settings, in a file you write by hand. devvm reads it and never writes it. Every key is optional, and devvm has no `config.toml` until you write one. This example sets every key:

```toml
# Whether new VMs get network policy: ask, on or off.
network = "on"

# The ports new VMs forward: all, none, or ports and ranges.
ports = "5173,8000-8010"

# The features base gets, in the order they run, or [] for none.
features = ["cli-tools", "fzf", "omb"]

# The coding agents new VMs get, or [] for none.
agents = ["claude", "codex"]

# The layers new VMs get, bottom first, or [] for none.
layers = ["docker"]

# New VMs' CPUs, and their memory and disk in GiB.
[size]
cpus = 4
memory = 8
disk = 100

# What devvm deletes on its own, with the keys devvm retention takes.
[retention]
logs.runs = 20
history.age = "90d"
history.count = 10
tools.unused-age = "30d"
orphans.age = "30d"
nix-cache.size = "20G"
```

Each key takes what its command takes. `network` is `ask`, `on` or `off`, as `devvm network -g` sets it ([turning it on or off](../safety/network.md#turning-it-on-or-off)). `ports` is `all`, `none` or a list, as `devvm ports -g` takes it ([size and ports](ports-size.md#size-and-ports)). `features` lists the [features](customizing.md#choosing-features) that base gets, `agents` the [coding agents](#coding-agents-for-new-vms) that new VMs get, and `layers` the [layers](#layers-for-new-vms) that new VMs get, bottom first. `cpus` is a whole number, and `memory` and `disk` are a number of GiB, such as `8`, `8.5` or `"8G"`, as `devvm size -g` takes them. The `[retention]` keys are the [retention settings](#retention), with a duration such as `"90d"` or a size such as `"20G"` in quotes, and a count as a number.

### Where a default comes from

For each default, devvm uses the first of these that sets it:

1. A flag for one VM, such as `--ports` or `--cpus`.
2. `config.toml`.
3. What devvm's commands saved: `devvm setup`, `devvm size -g`, `devvm ports -g`, `devvm network -g` and `devvm retention` save in `~/.local/share/devvm/settings.json`.
4. devvm's own default.

A command that would save a default that `config.toml` sets refuses, names the file and the key, and changes nothing, because the value in `config.toml` would still win. To change that default, edit it in `config.toml`, or delete it there to let devvm's commands set it again. In a terminal, `devvm setup` doesn't ask about what `config.toml` sets, and it says so.

`devvm size -g`, `devvm ports -g`, `devvm network -g` and `devvm retention` show where each value came from, after a `#`. They show `config.toml`'s path for a value from `config.toml`, `devvm: settings.json` for one that a command saved, and `devvm: default` for devvm's own:

```sh
$ devvm ports -g
5173,8000-8010 # /Users/you/.config/devvm/config.toml
$ devvm network -g
ask # devvm: default
```

With `--json`, the same commands add `source`, or `sources` for each value of `devvm size -g` and `devvm retention`, which is `config.toml`, `settings.json` or `built-in`.

### When devvm can't read it

devvm reads all of `config.toml` each time it needs a default, and it checks every key and value. A key it doesn't know, such as a typo, a value that a key can't take, and a file that isn't TOML each stop the command, and devvm names the file and the line. Fix the line, and run the command again. `config.toml` has no key for the backend, because every new VM runs on [Lima](../backends/lima.md), so a `backend` key stops the command as an unknown key.

## devvm setup

`devvm setup` chooses what new VMs get. In a terminal, it asks five questions, each starting from the current choice, and saves each answer in `~/.local/share/devvm/settings.json`, as the command for that setting saves it. It skips a question that [`config.toml`](#configtoml) answers, saying so, and it refuses a flag that would change what `config.toml` sets.

1. The [features](customizing.md#choosing-features) that base gets.
2. The [CPUs and memory](ports-size.md#choosing-new-vms-size) that new VMs get, as `devvm size -g` saves them.
3. The [network policy](../safety/network.md) that new VMs get, as `devvm network -g` saves it. With `ask`, the default, devvm asks for each new VM in a terminal, and a new VM gets network policy when there's no terminal to ask in. With `on` or `off`, devvm makes every new VM that way without asking. `--network-policy` and `--no-network-policy` choose for one VM and win over the saved choice.
4. The [coding agents](#coding-agents-for-new-vms) that new VMs get.
5. The [layers](#layers-for-new-vms) that new VMs get.

```sh
devvm setup                          # asks all five questions
devvm setup --features fzf,omb       # base's features, without asking
devvm setup --features none          # base gets no features
devvm setup --agents claude,codex    # new VMs' coding agents, without asking
devvm setup --layers docker          # new VMs' layers, without asking
```

`devvm setup` has three flags of its own, and each one chooses without asking anything. `--features LIST` chooses base's features, as a comma-separated list or `none` for none, and devvm refuses an empty `--features ''`. `--agents LIST` chooses new VMs' coding agents in the same way, and `--layers LIST` chooses new VMs' layers. With any of the flags, what the flags leave out stays as it is. Without a terminal and without flags, `devvm setup` asks nothing and keeps base's features, saving every feature when none was chosen yet. To change new VMs' size without asking, use `devvm size -g` ([choosing new VMs' size](ports-size.md#choosing-new-vms-size)), and to change their network policy, use `devvm network -g` ([turning it on or off](../safety/network.md#turning-it-on-or-off)).

### Coding agents for new VMs

The coding agents you choose in `devvm setup` are the ones checked for a new VM in the list devvm shows in a terminal, each row saying it comes from devvm setup, and the ones a new VM gets without a terminal ([what a new VM gets](images.md#what-a-new-vm-gets)). `--agents` chooses others for one VM. The choice is for new VMs only, so a VM that exists keeps its agents, and `devvm tools` changes them.

devvm asks for them on its first run in a terminal, together with the size and base's features, until you've chosen them. Until then, and when you choose `none`, new VMs get no agents by default. `devvm setup --agents claude,codex` and `devvm setup --agents none` choose them without asking, and devvm refuses a name that isn't an agent's, suggesting the closest.

### Layers for new VMs

The layers you choose in `devvm setup` are the ones every new VM gets unless you choose others for it. They can be devvm's [built-in layers](images.md#built-in-layers), `desktop`, `docker` and `nix`, and [your own](customizing.md#your-own-layers). In the list devvm shows for a new VM in a terminal, they're checked under Layers, each row saying it comes from devvm setup, and you can uncheck any of them for that VM. Without a terminal, a new VM gets them, and devvm says so in one line ([what a new VM gets](images.md#what-a-new-vm-gets)).

```sh
devvm setup --layers docker,desktop   # every new VM gets docker, with desktop on top
devvm setup --layers none             # new VMs get no layers
devvm --layers nix                    # this VM gets nix alone, not setup's layers
devvm --layers none                   # this VM gets no layers
```

`devvm setup --layers` takes the layers bottom first, in the order the image's name joins them, so `--layers docker,desktop` gives new VMs the image `docker+desktop`. It refuses a name that isn't a layer's, suggesting the closest, and changes nothing. Until you choose some, new VMs get no layers by default, and devvm doesn't ask for them on its first run. In a terminal, `devvm setup` asks for them last, after the coding agents. To set them by hand, use the `layers` key in [`config.toml`](#configtoml), which `devvm setup --layers` then refuses to change.

`--layers` on `devvm` is the exact list of one VM's layers, so it replaces setup's layers, and `--layers none` makes a VM without any. A `devcontainer.json`'s layers go on top of setup's ([`devcontainer.json`](devcontainer.md)).

The choice is for new VMs only. A VM that exists keeps the layers it was made with, and `devvm setup` says so when it saves the choice. To give that VM more layers, run `devvm layer add` in its folder ([adding layers to a VM](images.md#adding-layers-to-a-vm)), and to take some out, `devvm layer rm` ([taking layers out of a VM](images.md#taking-layers-out-of-a-vm)).

When a layer of yours that you chose in `devvm setup` has no script in `~/.config/devvm/layers`, such as after you delete it, a new VM is made without it, and devvm warns, naming the layer. Run `devvm setup --layers` to choose the layers again. When the VM's backend can't run a layer you chose, such as `docker` on a backend without Docker's engine, devvm makes no VM and names the layer. Then choose other layers for that VM with `--layers`, or change new VMs' layers with `devvm setup --layers`.

## Retention

devvm keeps what it makes until you delete it, and the retention settings tell it to delete some of it on its own. Every setting is off until you set it, so first run `devvm space` to see what each one would free: its `RETAIN` column shows each setting that's on, beside the row it applies to, with what it would delete now ([devvm space](space.md#devvm-space)).

`devvm retention` shows every setting and its value, and `off` for each one that isn't set. `devvm retention KEY=VALUE...` sets them, and `devvm retention KEY=off` turns one off, for example `devvm retention history.age=90d tools.unused-age=30d`. devvm refuses a key or value it doesn't know, and a key with no value such as `history.age=`, and then it writes nothing. devvm saves the settings in `~/.local/share/devvm/settings.json`. The `[retention]` table of [`config.toml`](#configtoml) sets them too, and a setting there wins over the saved one, so `devvm retention` refuses to change it. Each line that `devvm retention` shows ends with where its value came from, after a `#`. When devvm can't read a file that sets them, it names the file, and the line for `config.toml`, and applies none of them.

| Key | What devvm deletes |
| --- | --- |
| `logs.runs` | the [run logs](troubleshooting.md#a-command-fails) beyond the newest this many, which is 20 when it's off |
| `history.age` | the [exports of agents' history](coding-agents.md#history) written this long ago or more, such as `90d` |
| `history.count` | the exports of agents' history beyond the newest this many |
| `tools.unused-age` | the current shared tools that no VM has had for this long, counted from when devvm first found that no VM has them. While a VM is being made or is getting new shared tools, retention deletes none of them in that run, because that VM may not list the tools it picked yet, and it still applies the other settings |
| `orphans.age` | what devvm keeps for VMs that their backend no longer has, once their state hasn't changed for this long, as `devvm gc --orphans` does |
| `nix-cache.size` | the [shared Nix cache](toolchains.md#nix-projects)'s least recently used files, while it takes more than this, such as `20G`; it's 10 GiB when it's off |

Ages are [durations](../getting-started/commands.md#flags-and-values), a whole number of minutes, hours, days or weeks, such as `30m`, `12h`, `30d` or `2w`. Sizes are a whole number of `K`, `M`, `G` or `T`, each 1024 of the one before, such as `500M` or `20G`, as `devvm space` shows them. The shared Nix cache also keeps to its size while VMs fill it, because a VM's proxy deletes the cache's least recently used files before a new file would take it past that size. A size you set applies to each VM from its next start. devvm applies the settings at the start of a command, at most once a day, except for `orphans.age`, which needs to ask each backend about its VMs, so only `devvm gc --retention` applies it. `devvm gc --retention` applies all of them at once. When it deletes something, devvm says what in one line. Retention never deletes a VM, an image, a shared tool that any VM's state lists, or anything in the backends' own caches, which only `devvm gc --caches` frees. In `cache/`, the folder in [devvm's data folder](space.md#devvms-data-folder) that holds what devvm makes again when it needs it, retention deletes only the shared Nix cache's files. When two devvms start at once, only one of them applies the settings.

### devvm retention --json

`devvm retention --json` prints the settings as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes. `settings` has every key with its value as you wrote it, and `off` for each one that isn't set. `sources` has every key with where its value came from: `config.toml`, `settings.json` for one that `devvm retention` saved, or `built-in` for one that's off. With `--json` the command only shows, so devvm refuses a `KEY=VALUE` with it.

```json
{
  "version": 1,
  "settings": {
    "history.age": "90d",
    "history.count": "off",
    "logs.runs": "off",
    "nix-cache.size": "off",
    "orphans.age": "off",
    "tools.unused-age": "30d"
  },
  "sources": {
    "history.age": "config.toml",
    "history.count": "built-in",
    "logs.runs": "built-in",
    "nix-cache.size": "built-in",
    "orphans.age": "built-in",
    "tools.unused-age": "settings.json"
  }
}
```

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

`~/.local/share/devvm` (`$XDG_DATA_HOME/devvm`) holds what devvm makes and keeps for you, and devvm writes everything in it, so don't edit it by hand. It holds the defaults that `devvm setup`, `devvm size -g`, `devvm ports -g`, `devvm network -g` and `devvm retention` save in `settings.json`, below [`config.toml`](#configtoml)'s, and a folder for each VM in `vms/`, with the VM's state, its own allowlist and its network logs. It also holds the images and the shared tools, the [run logs](troubleshooting.md#a-command-fails), the agents' history that `devvm history export` and `devvm rm` save, the [ssh config](ssh.md#ssh-access-by-name) for your VMs, and devvm's own Lima home (`lima/`), where the Lima backend's VMs and images live. [devvm's data folder](space.md#devvms-data-folder) lists each file and folder in it, with how each one is cleaned up.

On Linux hosts this folder also holds your [secrets](../safety/network.md#saving-and-binding-secrets), in files under `secrets/` that only you can read, so keep that in mind when you back it up. On macOS they're in the Keychain.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Only you can read devvm's files, in either folder, because each time devvm runs, it makes both folders yours alone ([devvm's data folder](space.md#devvms-data-folder)). When devvm can't read one of its files, it names the file and the line ([a file devvm can't read](troubleshooting.md#a-file-devvm-cant-read)).

## Project folders devvm refuses

A VM mounts its project folder read-write, so devvm refuses to make a VM for a folder that would give the VM more than the project. It refuses a folder that is or holds your home folder, `~/.config/devvm`, `~/.local/share/devvm` or devvm's Lima home. It also refuses a folder inside one of devvm's own folders, because a VM there could rewrite the scripts that every image build runs, or the files that devvm runs on this machine.

When devvm refuses a folder, make the VM in a project folder inside it, or keep the project somewhere else. A [`--clone`](clone.md) VM mounts nothing from the folder, so devvm doesn't refuse its folder.

> [!WARNING]
> If you mean to give the VM the whole folder, pass `--unsafe-folder` when you make the VM, and devvm warns you that the VM gets read-write access to it ([only the project folder](../internals/security.md#only-the-project-folder)).

## Colors

In a terminal, devvm colors the labels and links around its messages, while the messages' own text stays in your terminal's usual color. The values a message fills in, such as a VM's name, a path, a port or a count, are bold, so they stand out from the words around them. An error's `devvm:` label is red and a warning's is yellow, a doc link is blue and underlined, and the `==>` before what devvm did is bold. When a step fails, the end of the run log that devvm prints is dim, and so is the log's latest line on the status line and each line that `-v` shows. The mark a slow step leaves when it's done is green.

The tables that devvm ls, devvm images, devvm space and devvm doctor print are colored too. Each table's header row and the totals row are bold, and a state is green when it's good, such as current, Running or native, yellow when it needs your attention, such as stale, unused, Stopped or emulated, and red when something is wrong, such as an unenforced network policy or an unreviewed emulation. Digests, image keys and a feature a backend doesn't have, which devvm doctor lists as none, are dim, and names, paths, sizes and counts stay in your terminal's usual color. The columns line up the same with colors or without.

devvm uses only your terminal's 16 base colors, so your theme picks shades that read well on its background, light or dark. It never sets a background color.

devvm decides for standard output and standard error apart, and colors one only when it's a terminal, so output you pipe or redirect to a file stays plain, tables included. What a command prints with `--json` is plain even in a terminal, and the run log is never colored. These environment variables change when devvm colors, and the first one that applies decides:

- `NO_COLOR`, set to anything but an empty value, turns colors off.
- `FORCE_COLOR` or `CLICOLOR_FORCE`, set to anything but `0`, turns them on even when the output isn't a terminal.
- `TERM=dumb` turns them off.

## Environment

| | |
| --- | --- |
| `NO_COLOR`, `FORCE_COLOR`, `CLICOLOR_FORCE` | turn devvm's colors off, or on when the output isn't a terminal ([details](#colors)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](../internals/lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image that the base of new VMs and builds starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#shared-tools)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads secrets in, instead of `devvm` ([details](../safety/network.md#saving-and-binding-secrets)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude Code token that VMs get, over a saved one ([details](coding-agents.md#claude-code-login)) |
