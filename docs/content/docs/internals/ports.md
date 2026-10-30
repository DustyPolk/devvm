---
title: Port forwarding
weight: 4
---

This page explains how devvm's forwarder forwards a VM's ports to this machine, which ports it never takes, and how it runs on each backend. [Ports and size](../using/ports-size.md#ports) covers choosing a VM's ports with `--ports` and `devvm ports`.

devvm's forwarder forwards ports by the same rules on every backend, and devvm uses no backend's own forwards for a VM's ports. [Lima](lima.md#ports), [smolvm](smolvm.md#ports) and [Docker](docker.md#ports) each explain why, and what devvm sets on the backend instead.

## What it forwards

When a VM forwards every port (`--ports all`, the default), the forwarder forwards each TCP port from 1024 up that a server in the VM listens on, whether on the VM's loopback (`127.0.0.1` or `::1`) or on all its addresses.

When a VM lists its ports (`--ports 5173,8000-8010`), the forwarder forwards those ports alone, however many there are, ports below 1024 included, and servers on the VM's loopback also reach this machine through them. You can list port 22, which forwards the VM's sshd to port 22 on this machine, while ssh by name keeps using the VM's own ssh port.

Each port goes to the same port on this machine's `127.0.0.1` and `::1` and nowhere else. A port is forwarded within a few seconds of its server starting, and the forward goes away when the server stops. The forwarder forwards at most 256 ports at once.

## What it never takes

The forwarder never takes a port that a program on this machine has on any address. That program keeps the port and its `localhost` connections, and the forward waits until the VM's server stops and starts again.

The forwarder also never takes a port that devvm records for a VM, even while that port is free. Those are the ssh, proxy and decider ports that devvm chooses for VMs, the VM's own ssh port, and the ports other VMs list. A Lima VM without network policy has an ssh port that Lima picks and devvm doesn't record, but Lima holds that port, so the forwarder can't take it.

When a start, or a change of a running VM's ports, finds that a program on this machine has a listed port, devvm doesn't forward that port and warns you (`ports_taken`). The VM starts or runs on all the same, and the next start or change that finds the port free forwards it again.

A program that starts on a port after the forwarder has forwarded it can still bind the port on all addresses, as most servers do. That program's `localhost` connections then reach the VM.

## How it reaches the VM

The forwarder reaches the VM over the VM's ssh, with a key and a host key that devvm pins, and it asks for no forwarding into this machine. Every connection starts on this machine, and the VM opens none to it. The VM sees each forwarded connection only as one to its own loopback, and with network policy the VM still reaches only its proxy.

How the forwarder logs in differs by backend:

- **Lima:** it connects to Lima's ssh port as Lima's user, with Lima's key (`_config/user` in devvm's Lima home) and the host key devvm pinned for the VM's ssh config.
- **smolvm:** it connects over the VM's [ssh](smolvm.md#ssh-for-editors), with the VM's own key and the host key that devvm read through smolvm.
- **Docker:** it connects over the VM's [ssh](docker.md#ssh-for-editors), through `devvm _docker-connect`, with the VM's own key and its pinned host key.

## Its process and files

The forwarder runs as a hidden devvm command, one process for each VM. devvm starts it after each start of the VM and again with the new ports when you change a running VM's ports, and `devvm stop` stops it. devvm keeps which ports a VM forwards in its own record for the VM, so `devvm ports` changes that record and never the backend's config. The forwarder's log is `<vm>.ports.log` beside that record.

| Backend | Process | Ports record | Log |
| --- | --- | --- | --- |
| Lima | `devvm _lima-ports` | `~/.local/share/devvm/lima-vms/<vm>.ports`, which holds `all`, `none` or a list | `~/.local/share/devvm/lima-vms/<vm>.ports.log` |
| smolvm | `devvm _smolvm-ports` | `~/.local/share/devvm/smolvm/vms/<vm>.json` | `~/.local/share/devvm/smolvm/vms/<vm>.ports.log` |
| Docker | `devvm _docker-ports` | `~/.local/share/devvm/docker/vms/<vm>.json` | `~/.local/share/devvm/docker/vms/<vm>.ports.log` |

When the forwarder ends also differs by backend:

- **Lima:** devvm starts it after each start of a project VM, and it ends when the VM's hostagent does (`ha.pid`). `devvm rm` deletes its files with the record.
- **smolvm:** it starts after the VM's sshd and ends with it. When you change a running VM's ports, devvm starts it again with the new ones, unless the VM was started outside devvm since devvm last started it. Such a VM runs no sshd of devvm's, so devvm warns you instead (`ports_no_ssh`).
- **Docker:** devvm starts it after each start with ssh, and when you change a running VM's ports, again for the same start of the container. It ends once Docker no longer runs that start of the container, which it asks every 30 seconds and whenever its connection ends.

## When it can't forward

- A VM without ssh forwards no ports, and devvm warns you (`ports_no_ssh`). On smolvm, for example, that's a VM whose image has no sshd.
- When a forwarder doesn't start, devvm warns you and names its log (`ports_forward_failed`), and the VM runs on without forwarded ports.
