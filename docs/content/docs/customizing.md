---
title: Base image features
weight: 23
---

Features are optional extras built into [`base`](images.md#base), after `layers/base.sh`, so every VM has them.

| Feature | What it adds |
| --- | --- |
| `omb` | [oh-my-bash](https://ohmybash.github.io/): a themed prompt, completions and aliases (auto-update off) |
| `fzf` | fuzzy Ctrl-R history and Ctrl-T file search |
| `cli-tools` | bat, fd, eza, zoxide (`z`), btop, tree, tldr |

Like `layers/base.sh`, each feature pins its downloads to a version and SHA-256 at the top of its script, `features/<name>.sh` next to devvm's binary.

## Choosing features

The first time devvm builds anything, it asks which features `base` gets, with every one chosen to start with: up and down (or `k` and `j`) move, space toggles, Enter confirms. Without a terminal, every feature is chosen. devvm prints the choice and saves it in `~/.config/devvm/base-features` (`$XDG_CONFIG_HOME/devvm`), one name per line, in the order they run.

To change the choice, edit that file: an empty file chooses none. The chosen features are part of `base`'s [key](images.md#caching), so a new choice makes `base` and the images built on it rebuild on next use. A name without a script is skipped, with a warning.

## Your own features

A script of yours in `~/.config/devvm/features/<name>.sh` is a feature too, listed after devvm's own. Its name is lowercase letters, digits and dashes, starting with a letter or digit.

- It runs with bash in `base`'s VM as its user, who has sudo, like a [layer](images.md#layers).
- Line 2 is a comment saying what it adds: the picker shows it.
- Put interactive shell setup in `~/.bashrc.d/<name>.sh`, and PATH changes in `~/.profile`.
- Download with `devvm-fetch URL SHA256 OUT`, which `base` installs: it fails when the checksum doesn't match.
- Editing a chosen feature's script rebuilds `base` on next use.

A feature of yours named like one of devvm's is ignored, with a warning: rename it.
