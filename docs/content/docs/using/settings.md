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
| `ports` | the [ports](ports-size.md#size-and-ports) that new VMs forward | `devvm ports -g` |
| `policy` | your [network policy](../safety/network.md#changing-rules) rules for every VM | `devvm policy -g` |
| `layers/` | your own [layers](customizing.md#your-own-layers) and their allowlists | `devvm layer new` |
| `features/` | your own [features](customizing.md#your-own-features) | you |
| `ca/` | your [company's CA certificates](#a-companys-ca), which VMs and builds trust | you |
| `agent-keys` | the keys in [devvm's own SSH agent](ssh.md#devvms-own-agent) and their options | `devvm ssh-agent add` |
| `ssh_config`, `known_hosts` | the [ssh config](ssh.md#ssh-access-by-name) for running VMs and their host keys | devvm, on each launch and stop |

On Linux hosts, devvm also keeps your [secrets](../safety/network.md#saving-and-binding-secrets) in files under `~/.config/devvm/secrets/`, which only you can read. On macOS they're in the Keychain.

Upgrading devvm keeps this folder as it is. Back it up to keep your settings, layers and features, and keep in mind that on Linux hosts it holds your secrets.

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
- the agents' history that `devvm export` and `devvm rm` save, in `exports/`;
- devvm's own Lima home (`lima/`), where the Lima backend's VMs and images live.

devvm's Lima home is its own, whatever `LIMA_HOME` says, so Lima settings meant for other VMs never reach devvm's. Set `DEVVM_LIMA_HOME` to move it, for example to a sandbox or to a shorter path.

Only you can read devvm's files. When devvm can't read one, it names the file and the line ([a file devvm can't read](troubleshooting.md#a-file-devvm-cant-read)).

## Project folders devvm refuses

A VM mounts its project folder read-write, so devvm refuses to make a VM for a folder that would give the VM more than the project. It refuses a folder that is or holds your home folder, `~/.config/devvm`, `~/.local/share/devvm` or devvm's Lima home. It also refuses a folder inside one of devvm's own folders, because a VM there could rewrite the scripts that every image build runs, or the files that devvm runs on this machine.

When devvm refuses a folder, make the VM in a project folder inside it, or keep the project somewhere else. A [`--clone`](clone.md) VM mounts nothing from the folder, so devvm doesn't refuse its folder.

> [!WARNING]
> If you mean to give the VM the whole folder, pass `--unsafe-folder` when you make the VM, and devvm warns you that the VM gets read-write access to it ([only the project folder](../internals/security.md#only-the-project-folder)).

## Environment

| | |
| --- | --- |
| `DEVVM_VERBOSE=1` | print the output of the commands devvm runs, instead of logging it ([details](troubleshooting.md#a-command-fails)) |
| `DEVVM_LIMA_HOME` | move devvm's own Lima home ([details](../internals/lima.md#a-lima-home-of-its-own)) |
| `DEVVM_FROM` | the container image that the base of new VMs and builds starts from when there's no `--from` ([details](from.md)) |
| `DEVVM_NO_TOOL_CHECK=1` | don't check for newer shared tools once a day ([details](images.md#shared-tools)) |
| `DEVVM_SECRET_SERVICE` | the Keychain service devvm saves and reads secrets in, instead of `devvm` ([details](../safety/network.md#saving-and-binding-secrets)) |
| `CLAUDE_CODE_OAUTH_TOKEN` | the Claude Code token that VMs get, over a saved one ([details](coding-agents.md#claude-code-login)) |
