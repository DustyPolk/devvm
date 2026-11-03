# devvm

One VM for every project: an isolated sandbox with your project's toolchain, ready for you or an agent to start coding.

Each project folder gets a VM of its own. devvm detects the project's toolchain from its files, so a folder with `package.json` gets node, and the VM mounts only the project folder, at the same path as on your machine. New VMs get network policy by default, so they reach only the hosts you allow, and your secrets, such as your GitHub token and Claude Code token, stay on your machine. Claude Code, Codex and opencode run in the VM, with your settings, skills and MCP servers, and you start them in the VM's shell. devvm runs its VMs on [Lima](https://lima-vm.io) by default, or on the experimental smolvm backend.

## Install

devvm runs on macOS 13 or newer and on Linux, on arm64 or x86_64, with Lima 2.0 or newer. Run the one command for your system.

On macOS, install devvm with Homebrew, which also installs Lima:

```sh
brew install w3cj/tap/devvm
```

On Linux, install it with the install script, which also works on macOS and asks before it installs Lima, QEMU or access to `/dev/kvm`:

```sh
curl -fsSL https://github.com/w3cj/devvm/releases/latest/download/install.sh | bash
```

Then run `devvm doctor`, which checks what devvm needs on your machine and says how to get anything that's missing. [Installing](https://devvm.dev/docs/getting-started/install/) has the full list of requirements, the install script's options, and how to upgrade and uninstall devvm.

## Your first VM

Go to a project folder and run devvm:

```sh
cd ~/some-project
devvm
```

The first time, devvm asks for the size and the coding agents that new VMs get. Then it lists what this VM can get, with the toolchains your project's files call for and those agents already checked. Check or uncheck a coding agent, such as Claude Code, with the arrow keys and space, then press enter. devvm asks whether the VM gets network policy (answer yes, the default). On that first run, it also asks which optional extras every VM gets, then builds the base image, which takes a few minutes once. Later VMs start in seconds.

devvm then opens a shell in the VM, at the same path as on your machine, and its prompt starts with `(devvm)`. Start the agent there, so it runs in the VM. Claude Code needs a Claude Code token before its first start, which [your first VM](https://devvm.dev/docs/getting-started/first-vm/#claude-codes-token) shows how to make on your machine and give to devvm:

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
- [Staying safe](https://devvm.dev/docs/safety/): the habits that keep you safe, network policy, `.git` protection and attack vectors.
- [Backends](https://devvm.dev/docs/backends/): what runs the VMs, and choosing one.
- [CLI reference](https://devvm.dev/docs/cli/): every command, with its arguments and flags.
- [How it works](https://devvm.dev/docs/internals/): how devvm does what it does, for checking a claim or reviewing its security.
- [Contributing](https://devvm.dev/docs/contributing/): building, testing and writing docs.

## License

[MIT](LICENSE)
