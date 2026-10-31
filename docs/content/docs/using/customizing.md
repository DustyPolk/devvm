---
title: Customizing images
weight: 4
---

You customize images with your own layers, which add what a project needs, and with base's features, which are extras that every VM gets. Both live in `~/.config/devvm` (`$XDG_CONFIG_HOME/devvm`), not next to devvm's binary, so upgrading devvm keeps them.

## Your own layers

```sh
devvm layer new mailpit    # writes ~/.config/devvm/layers/mailpit.sh and mailpit.policy
$EDITOR ~/.config/devvm/layers/mailpit.sh
devvm node+mailpit         # a VM with node and your layer; or devvm build mailpit
```

A [layer](images.md#layers) is a script, `~/.config/devvm/layers/<name>.sh`, that bash runs once as the VM's user, who has sudo, in an image on top of its parent. `devvm layer new NAME` writes a starter script, and also `NAME.policy` next to it unless that file exists. It refuses to overwrite a script that exists.

- **Names** are made of lowercase letters, digits and dashes, and start with a letter or digit. A name can't be `base`, a [built-in layer's](images.md#built-in-layers) name (`docker` or `nix`) or a shared tool's name (`node`, `claude` and so on).
- Built-in layers go in the same image names as yours, as in `devvm node+docker+mailpit`.
- Put PATH changes in `~/.profile`.
- Download files with `devvm-fetch URL SHA256 OUT`, which `base` installs. It fails when the file's checksum doesn't match.
- `NAME.policy` lists the hosts that VMs with the layer can reach with [network policy](../safety/network.md#allowlists), one rule per line.
- Editing the script rebuilds its image, and the ones built on it, the next time they're used ([rebuilding](images.md#rebuilding)).
- You can [add a layer to an existing VM](../getting-started/commands.md#adding-to-a-vm) in place, but you can't take it out again. To take it out, run `devvm rm` and make the VM again.

To edit layers with an agent in a VM, keep them in a repo of their own and copy them in, because devvm refuses a VM for a folder inside `~/.config/devvm` ([details](../internals/security.md#only-the-project-folder)).

### Example: mailpit

This layer installs [Mailpit](https://mailpit.axllent.org/), which catches the emails your app sends while you develop it, and starts it with the VM. Point your app's SMTP settings at `localhost:1025`, and read the emails at `localhost:8025` in your browser, because devvm [forwards](ports-size.md#ports) both ports to your machine. Put these lines in `~/.config/devvm/layers/mailpit.sh`, below the starter's `set -euo pipefail`:

```bash
mailpit_version=1.32.0
mailpit_sha256_amd64=4b5993c978e8604c70d43927e2ad103b311bba3b1822fa0cf1d4ee4ec890a93c
mailpit_sha256_arm64=e1e45d1dc72bd497fb262a1e4100178eee057c42c5fc36d143eb8621769e1137

case "$(uname -m)" in
  x86_64) arch=amd64 sha256=$mailpit_sha256_amd64 ;;
  aarch64) arch=arm64 sha256=$mailpit_sha256_arm64 ;;
  *) echo "mailpit: no build for $(uname -m)" >&2; exit 1 ;;
esac
devvm-fetch "https://github.com/axllent/mailpit/releases/download/v$mailpit_version/mailpit-linux-$arch.tar.gz" \
  "$sha256" /tmp/mailpit.tar.gz
sudo tar -xzf /tmp/mailpit.tar.gz -C /usr/local/bin mailpit
rm /tmp/mailpit.tar.gz

printf '%s\n' '[Unit]' 'Description=Mailpit' '[Service]' 'ExecStart=/usr/local/bin/mailpit' \
  'DynamicUser=yes' '[Install]' 'WantedBy=multi-user.target' |
  sudo tee /etc/systemd/system/mailpit.service >/dev/null
sudo systemctl enable --now mailpit
```

The version and each download's SHA-256 sit at the top, so `devvm-fetch` refuses a download that doesn't match. To update Mailpit, change all three and rebuild the image. The downloads come from GitHub's releases, which `base`'s allowlist has, so `mailpit.policy` can stay empty. On a base made `--from` an Alpine image, start it with OpenRC (`rc-update`, `rc-service`) instead of systemd.

## Base image features

Features are optional extras that devvm builds into [`base`](images.md#base) after base's own setup, so every VM has them.

| Feature | What it adds |
| --- | --- |
| `omb` | [oh-my-bash](https://ohmybash.github.io/): a themed prompt, completions and aliases, with auto-update off |
| `fzf` | fuzzy Ctrl-R history and Ctrl-T file search |
| `cli-tools` | bat, fd, eza, zoxide (`z`), btop, tree, tldr |
| `desktop` | an Xfce desktop, which [`devvm desktop`](desktop.md) opens in Screen Sharing, about 370 MB |

### Choosing features

The first time devvm builds anything, it asks which features `base` gets, with every one but `desktop` chosen to start with. Up and down (or `k` and `j`) move, space toggles, and Enter confirms. Without a terminal, devvm chooses every feature but `desktop`. devvm prints the choice and saves it in `~/.config/devvm/base-features`, one name per line, in the order they run.

To change the choice:

```sh
devvm setup                     # asks again, starting from the current choice
devvm setup --features fzf,omb  # without asking; --features '' chooses none
```

In a terminal, `devvm setup` first asks for the [default backend](../backends/_index.md#choosing-a-backend), then for the features and then for the [CPUs and memory](ports-size.md#choosing-new-vms-size) that new VMs get, but `--features` asks for neither the backend nor the size. Without a terminal, `devvm setup` keeps the choice. devvm refuses a name that isn't a feature, and then nothing changes. You can also edit `base-features` yourself. An empty file chooses none, and devvm skips a name without a script, with a warning.

A new choice makes `base` and the images built on it rebuild the next time they're used, and `devvm setup` says so. The old images are then stale, and [`devvm gc`](images.md#cleaning-up) deletes them.

### Your own features

A script of yours in `~/.config/devvm/features/<name>.sh` is also a feature, listed after devvm's own. Its name is made of lowercase letters, digits and dashes, and starts with a letter or digit.

- It runs with bash in `base`'s VM as the VM's user, who has sudo, like a [layer](#your-own-layers).
- Line 2 is a comment that says what it adds, which the picker shows.
- Put interactive shell setup in `~/.bashrc.d/<name>.sh`, and PATH changes in `~/.profile`.
- Download files with `devvm-fetch URL SHA256 OUT`, which `base` installs. It fails when the file's checksum doesn't match.
- Editing a chosen feature's script rebuilds `base` the next time it's used.

devvm ignores a feature of yours that has the name of one of devvm's, and warns you to rename it.
