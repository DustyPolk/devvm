---
title: devcontainer.json
weight: 4
---

A new VM gets what the project's `.devcontainer/devcontainer.json`, else `.devcontainer.json` at the folder's root, says, as far as devvm does it: its image, the features devvm has layers for, its ports, its environment and its commands. devvm says so in one line naming the file. `devvm --no-devcontainer` makes the VM without reading it.

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/javascript-node:22",
  "features": { "ghcr.io/devcontainers/features/docker-in-docker:2": {} },
  "forwardPorts": [3000],
  "remoteEnv": { "PATH": "${containerEnv:PATH}:${containerWorkspaceFolder}/bin" },
  "postCreateCommand": "npm ci",
  "postStartCommand": "npm run db:start"
}
```

## When it's read

- **Only when the folder's VM is made**, on every backend. Comments and trailing commas are allowed. devvm reads it within the folder: a link out of the folder isn't followed, and only a regular file of up to 1 MiB is read. Nothing is ever written to the project folder.
- **What you give on the command line wins:** `--from` over `image` (which wins over `$DEVVM_FROM`), `--ports` over `forwardPorts`, each said in one line. The layers IMAGE names go on top of the features' (`devvm claude`); with a devcontainer.json, a folder whose toolchain devvm can't tell gets `base`.
- **An existing VM never changes.** Its state records the file's SHA-256. A launch whose file differs from it, is gone or appeared since warns once: `devvm rm`, then `devvm`, makes the VM again from the file as it is. A VM made with `--no-devcontainer` isn't checked.

Anything devvm refuses stops it in one line naming the file, before anything is made; invalid JSON names the line and column.

## Properties

| Property | What devvm does |
| --- | --- |
| `image` | the VM's base starts from it, as with [`--from`](from.md), where the backend can: only an image its registry gives anyone, asked anonymously, pinned to that digest; never one only Docker has, one only your registry login reaches, or one on this machine or its network (`localhost`, or a registry whose address is loopback, private or link-local, by name or as it resolves) |
| `features` | `ghcr.io/devcontainers/features/docker-in-docker` is devvm's [docker layer](images.md#built-in-layers), with the backend's own rules for it: on Docker, where it means no network policy, devvm still asks about network policy, and a yes refuses the VM; `node`, `python`, `rust` and `go` under `ghcr.io/devcontainers/features/` are the [shared toolchains](images.md#shared-tools). Their options are ignored, said in one line. `docker-outside-of-docker` (this machine's Docker) and every other feature are refused. No feature's script is fetched or run |
| `forwardPorts` | numbers, `localhost:N` and `127.0.0.1:N` are forwarded as far as your ports allow ([ports](images.md#ports)): with every port forwarded, the default, they're among them; ones your ports leave out are said in one line (`devvm ports add` forwards them), never added. Other hosts' are ignored, said in one line |
| `containerEnv`, `remoteEnv` | the VM's environment: login shells and the commands below get them. `remoteEnv` wins over `containerEnv`, and devvm's own variables (`LANG`, tokens) over both. A `remoteEnv` of `null` sets nothing |
| `onCreateCommand`, `updateContentCommand`, `postCreateCommand` | run once, in that order, once the VM is made and set up ([commands](#commands)) |
| `postStartCommand` | runs at each start: on the first launch after the VM started |
| `initializeCommand` | never runs, said in one line: it would run on this machine |
| `build`, `dockerFile`, `dockerfile`, `dockerComposeFile` | refused: devvm starts a VM only from an image |
| `postAttachCommand`, `privileged`, `capAdd`, `securityOpt`, `mounts`, `runArgs`, `workspaceMount`, `workspaceFolder`, `remoteUser`, `containerUser`, `updateRemoteUserUID`, `init`, `shutdownAction`, `overrideCommand`, `hostRequirements`, `appPort`, `portsAttributes`, `otherPortsAttributes`, `userEnvProbe`, `waitFor`, `overrideFeatureInstallOrder`, `context`, `service`, `runServices` | ignored, named in one line |
| `name`, `customizations`, `$schema` | ignored |
| anything else | ignored, named in one line |

## Commands

A command is a string, run with `sh -c`; an array, run as it is; or an object of named commands, each run, sorted by name. They run in the VM, as its user, in the project folder, in a login shell, never on this machine. Their output goes to the [run log](images.md#build-logs-and-interrupts).

A command that fails is said in one line naming it; those after it among the ones that run once, or at that start, don't run. The VM is kept and opened: the command is the project's, not devvm's. With [network policy](../safety/network.md), commands reach only what the VM's allowlist allows, like anything else in it.

## Variables

| Variable | |
| --- | --- |
| `${containerWorkspaceFolder}`, `${containerWorkspaceFolderBasename}` | the project folder, at the same path in the VM, and its name |
| `${containerEnv:NAME}` | in a string command and in `containerEnv` and `remoteEnv`, the VM's own `NAME` |
| `${localEnv:NAME}`, `${env:NAME}` | refused: devvm never puts this machine's environment in a VM |
| `${localWorkspaceFolder}`, `${localWorkspaceFolderBasename}`, `${devcontainerId}`, `${containerEnv:NAME}` elsewhere | refused |

Any other `${…}` is left as it is, for the VM's shell (`${HOME}` in a command).

## Security

The file is the project's, and so the VM's to change: a VM that writes it changes what the folder's next VM gets. So nothing in it runs on this machine, reaches its files or environment, adds a mount, or weakens confinement: it shapes only what runs inside a new VM ([security model](../safety/security.md#a-projects-devcontainerjson)). An `image` from it can still be anything public: its own code runs as root in the build VM, as with `--from` ([the image is untrusted](from.md#the-image-is-untrusted)).
