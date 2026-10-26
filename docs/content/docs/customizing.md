---
title: Customizing images
weight: 23
---

Your own layers and features live in `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`), not next to devvm's binary, so upgrading devvm keeps them.

## Your own layers

```sh
devvm layer new postgres    # writes ~/.config/devvm/layers/postgres.sh and postgres.policy
$EDITOR ~/.config/devvm/layers/postgres.sh
devvm node+postgres         # a VM with node and your layer; or devvm build postgres
```

A [layer](images.md#layers) is a script, `~/.config/devvm/layers/<name>.sh`, that runs once, with bash as the VM's user, who has sudo, in an image on top of its parent. `devvm layer new NAME` writes a starter script, and `NAME.policy` next to it unless that exists; it refuses a script that exists.

- **Names:** lowercase letters, digits and dashes, starting with a letter or digit, and neither `base` nor a shared tool's (`node`, `claude`, …).
- Put PATH changes in `~/.profile`.
- Download with `devvm-fetch URL SHA256 OUT`, which `base` installs: it fails when the checksum doesn't match.
- `NAME.policy` lists the hosts VMs with the layer can reach with [network policy](network.md#allowlists), one rule per line.
- Editing the script rebuilds its image, and the ones built on it, on next use ([caching](images.md#caching)).
- A layer can be [added to an existing VM](commands.md#adding-to-a-vm) in place, but not taken out: `devvm rm` and make the VM again.

To edit layers with an agent in a VM, keep them in a repo of their own and copy them in: devvm refuses a VM for a folder inside `~/.config/devvm` ([details](security.md#only-the-project-folder)).

### Example: postgres

A PostgreSQL server that starts with the VM, and a superuser role and database for the VM's user, so `psql` works without options: in `~/.config/devvm/layers/postgres.sh`, below the starter's `set -euo pipefail`,

```bash
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql postgresql-client
sudo systemctl enable --now postgresql
sudo -u postgres createuser --superuser "$USER"
sudo -u postgres createdb --owner "$USER" "$USER"
```

The packages come from the distribution's archive, which `base`'s allowlist has, so `postgres.policy` can stay empty. Postgres listens on port 5432 in the VM, which is [forwarded](images.md#ports) to the host like any port. On a base `--from` an Alpine image, use `apk add` and OpenRC (`rc-update`, `rc-service`) instead.

## Base image features

Features are optional extras built into [`base`](images.md#base), after `assets/layers/base.sh`, so every VM has them.

| Feature | What it adds |
| --- | --- |
| `omb` | [oh-my-bash](https://ohmybash.github.io/): a themed prompt, completions and aliases (auto-update off) |
| `fzf` | fuzzy Ctrl-R history and Ctrl-T file search |
| `cli-tools` | bat, fd, eza, zoxide (`z`), btop, tree, tldr |

Like `assets/layers/base.sh`, each feature pins its downloads to a version and SHA-256 at the top of its script, `assets/features/<name>.sh` in the repo, built into devvm.

### Choosing features

The first time devvm builds anything, it asks which features `base` gets, with every one chosen to start with: up and down (or `k` and `j`) move, space toggles, Enter confirms. Without a terminal, every feature is chosen. devvm prints the choice and saves it in `~/.config/devvm/base-features`, one name per line, in the order they run.

To change the choice:

```sh
devvm setup                     # asks again, starting from the current choice
devvm setup --features fzf,omb  # without asking; --features '' chooses none
```

In a terminal, `devvm setup` first asks for the [default backend](backends.md#choosing-a-backend), then the features; `--features` doesn't ask for the backend. Without a terminal, `devvm setup` keeps the choice. A name that isn't a feature is refused, and nothing changes. You can also edit `base-features` yourself: an empty file chooses none, and a name without a script is skipped, with a warning.

The chosen features are part of `base`'s [key](images.md#caching), so a new choice makes `base` and the images built on it rebuild on next use; `devvm setup` says so. The old images are then stale: [`devvm gc`](images.md#cleaning-up) deletes them.

### Your own features

A script of yours in `~/.config/devvm/features/<name>.sh` is a feature too, listed after devvm's own. Its name is lowercase letters, digits and dashes, starting with a letter or digit.

- It runs with bash in `base`'s VM as its user, who has sudo, like a [layer](#your-own-layers).
- Line 2 is a comment saying what it adds: the picker shows it.
- Put interactive shell setup in `~/.bashrc.d/<name>.sh`, and PATH changes in `~/.profile`.
- Download with `devvm-fetch URL SHA256 OUT`, which `base` installs: it fails when the checksum doesn't match.
- Editing a chosen feature's script rebuilds `base` on next use.

A feature of yours named like one of devvm's is ignored, with a warning: rename it.
