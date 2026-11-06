# devvm

One VM for every project: an isolated sandbox with your project's toolchain, ready for you or an agent to start coding.

Each project folder gets a VM of its own. devvm detects the project's toolchain from its files, so a folder with `package.json` gets node, and the VM mounts only the project folder, at the same path as on your machine. New VMs get the allowlist network tier by default, so they reach only the hosts you allow, and on every tier your secrets, such as your GitHub token and Claude Code token, stay on your machine. Claude Code, Codex and opencode run in the VM, with your settings, skills and MCP servers, and you start them in the VM's shell. You don't install them on your machine, because devvm keeps their settings and logins in a store of its own, and `devvm agent import` copies them in from an agent you already use there. devvm runs its VMs on [Lima](https://lima-vm.io).

## Install

devvm runs on macOS 13 or newer and on Linux, on arm64 or x86_64, with Lima 2.0 or newer. It's tested on Apple silicon Macs and on Linux on both arm64 and x86_64, but not yet on Intel Macs. Run the one command for your system.

On macOS, install devvm with Homebrew, which also installs Lima:

```sh
brew install devvmhq/tap/devvm
```

On Linux, install it with the install script, which also works on macOS and asks before it installs Lima, QEMU or access to `/dev/kvm`:

```sh
curl -fsSL https://github.com/devvmhq/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor`, which checks what devvm needs on your machine and says how to get anything that's missing. [Installing](https://devvm.dev/docs/getting-started/install/) has the full list of requirements, the install script's options, and how to upgrade and uninstall devvm.

## Your first VM

Go to a project folder and run devvm:

```sh
cd ~/some-project
devvm
```

The first time, devvm asks a few questions that set its defaults for every new VM and its base image: the size and the coding agents that new VMs get, and the optional extras the base image gets. Then it shows what this folder's new VM gets, with the toolchains your project's files call for and those agents, and asks `Customize it? [y/N]`. Press Enter to make the VM as shown, or type y to check or uncheck its toolchains, coding agents, such as Claude Code, and layers, then choose its network tier and size. On that first run, devvm then builds the base image, which takes a few minutes once. Later VMs start in seconds.

devvm then opens a shell in the VM, at the same path as on your machine, and its prompt starts with `(devvm)`. Start the agent there, so it runs in the VM. Claude Code needs your Claude login before its first start, which `devvm agent login claude`, run in the folder on your machine, makes from the VM and keeps on your machine ([your first VM](https://devvm.dev/docs/getting-started/first-vm/#claude-codes-login)):

```sh
claude
```

Running `devvm` in the folder again opens a shell in the same VM, and `devvm tools` adds an agent to it. [Your first VM](https://devvm.dev/docs/getting-started/first-vm/) has the rest.

## Working on a clone

By default, the VM mounts your project folder, so the VM and your editor share the same files. With `devvm --clone`, the VM gets a clone of the repo of its own instead, so nothing in it can touch your working tree or `.git`, and its work reaches your repo only when you fetch it:

```sh
devvm --clone                 # make the VM with its own clone, and open a shell in it
git fetch devvm               # later, on this machine, fetch the VM's work as devvm/<branch>
git log -p main..devvm/main   # review it before you merge it
```

[Working on a clone](https://devvm.dev/docs/using/clone/) has the details.

## Staying safe

A few habits close most of what devvm can't close on its own, so read [Staying safe](https://devvm.dev/docs/safety/) before you trust a VM with a project.

## Docs

The docs are at [devvm.dev](https://devvm.dev/), and their source is in `docs/`:

- [Getting started](https://devvm.dev/docs/getting-started/): installing devvm, your first VM and the commands you use most.
- [Using devvm](https://devvm.dev/docs/using/): images and tools, clones, links between VMs, ports and size, coding agents, SSH and editors, and troubleshooting.
- [Staying safe](https://devvm.dev/docs/safety/): the habits that keep you safe, network tiers, `.git` protection and attack vectors.
- [Backends](https://devvm.dev/docs/backends/): Lima, which runs the VMs, and what it gives them.
- [CLI reference](https://devvm.dev/docs/cli/): every command, with its arguments and flags.
- [How it works](https://devvm.dev/docs/internals/): how devvm does what it does, for checking a claim or reviewing its security.
- [Contributing](https://devvm.dev/docs/contributing/): building, testing and writing docs.

## License

[MIT](LICENSE)
