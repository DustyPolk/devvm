---
title: devvm completion
weight: 5
---

Print the shell completion script for bash, zsh or fish

## Synopsis

Print the script that completes devvm's commands, flags and image names in SHELL: bash, zsh or fish. Load it from your shell's startup file: source <(devvm completion bash) in ~/.bashrc (it needs the bash-completion package), source <(devvm completion zsh) in ~/.zshrc (after compinit), devvm completion fish | source in ~/.config/fish/config.fish.

```text
devvm completion SHELL [flags]
```

## Options

```text
  -h, --help   help for completion
```

## Options inherited from parent commands

```text
  -v, --verbose   show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line
```

## See also

- [devvm](devvm.md): Open a shell in this folder's VM, creating it first
