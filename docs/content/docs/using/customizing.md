---
title: Customizing images
weight: 3
---

You customize images with your own layers, which add what a project needs, and with base's features, which are extras that every VM gets. Both live in `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`), not next to devvm's binary, so upgrading devvm keeps them.

## Your own layers

```sh
devvm layer new postgres    # writes ~/.config/devvm/layers/postgres.sh and postgres.policy
$EDITOR ~/.config/devvm/layers/postgres.sh
devvm node+postgres         # a VM with node and your layer; or devvm build postgres
```

A [layer](images.md#layers) is a script, `~/.config/devvm/layers/<name>.sh`, that bash runs once as the VM's user, who has sudo, in an image on top of its parent. `devvm layer new NAME` writes a starter script, and also `NAME.policy` next to it unless that file exists. It refuses to overwrite a script that exists.

- **Names** are made of lowercase letters, digits and dashes, and start with a letter or digit. A name can't be `base`, a [built-in layer's](images.md#built-in-layers) name (`docker`) or a shared tool's name (`node`, `claude` and so on).
- Built-in layers go in the same image names as yours, as in `devvm node+docker+postgres`.
- Put PATH changes in `~/.profile`.
- Download files with `devvm-fetch URL SHA256 OUT`, which `base` installs. It fails when the file's checksum doesn't match.
- `NAME.policy` lists the hosts that VMs with the layer can reach with [network policy](../safety/network.md#allowlists), one rule per line.
- Editing the script rebuilds its image, and the ones built on it, the next time they're used ([rebuilding](images.md#rebuilding)).
- You can [add a layer to an existing VM](../getting-started/commands.md#adding-to-a-vm) in place, but you can't take it out again. To take it out, run `devvm rm` and make the VM again.

To edit layers with an agent in a VM, keep them in a repo of their own and copy them in, because devvm refuses a VM for a folder inside `~/.config/devvm` ([details](../internals/security.md#only-the-project-folder)).

### Example: postgres

This layer installs a PostgreSQL server that starts with the VM, and it makes a superuser role and a database for the VM's user, so `psql` works without options. Put these lines in `~/.config/devvm/layers/postgres.sh`, below the starter's `set -euo pipefail`:

```bash
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql postgresql-client
sudo systemctl enable --now postgresql
sudo -u postgres createuser --superuser "$USER"
sudo -u postgres createdb --owner "$USER" "$USER"
```

The packages come from the distribution's archive, which `base`'s allowlist has, so `postgres.policy` can stay empty. Postgres listens on port 5432 in the VM, which devvm [forwards](ports-size.md#ports) to the host like any port. On a base made `--from` an Alpine image, use `apk add` and OpenRC (`rc-update`, `rc-service`) instead.

## Base image features

Features are optional extras that devvm builds into [`base`](images.md#base) after base's own setup, so every VM has them.

| Feature | What it adds |
| --- | --- |
| `omb` | [oh-my-bash](https://ohmybash.github.io/): a themed prompt, completions and aliases, with auto-update off |
| `fzf` | fuzzy Ctrl-R history and Ctrl-T file search |
| `cli-tools` | bat, fd, eza, zoxide (`z`), btop, tree, tldr |

### Choosing features

The first time devvm builds anything, it asks which features `base` gets, with every one chosen to start with. Up and down (or `k` and `j`) move, space toggles, and Enter confirms. Without a terminal, devvm chooses every feature. devvm prints the choice and saves it in `~/.config/devvm/base-features`, one name per line, in the order they run.

To change the choice:

```sh
devvm setup                     # asks again, starting from the current choice
devvm setup --features fzf,omb  # without asking; --features '' chooses none
```

In a terminal, `devvm setup` first asks for the [default backend](../backends/_index.md#choosing-a-backend) and then for the features, but `--features` doesn't ask for the backend. Without a terminal, `devvm setup` keeps the choice. devvm refuses a name that isn't a feature, and then nothing changes. You can also edit `base-features` yourself. An empty file chooses none, and devvm skips a name without a script, with a warning.

A new choice makes `base` and the images built on it rebuild the next time they're used, and `devvm setup` says so. The old images are then stale, and [`devvm gc`](images.md#cleaning-up) deletes them.

### Your own features

A script of yours in `~/.config/devvm/features/<name>.sh` is also a feature, listed after devvm's own. Its name is made of lowercase letters, digits and dashes, and starts with a letter or digit.

- It runs with bash in `base`'s VM as the VM's user, who has sudo, like a [layer](#your-own-layers).
- Line 2 is a comment that says what it adds, which the picker shows.
- Put interactive shell setup in `~/.bashrc.d/<name>.sh`, and PATH changes in `~/.profile`.
- Download files with `devvm-fetch URL SHA256 OUT`, which `base` installs. It fails when the file's checksum doesn't match.
- Editing a chosen feature's script rebuilds `base` the next time it's used.

devvm ignores a feature of yours that has the name of one of devvm's, and warns you to rename it.
