---
title: Port forwarding
weight: 4
---

This page explains how devvm's forwarder forwards a VM's ports to this machine.

devvm's forwarder forwards the ports by the same rules on every backend.

- **What it forwards.** When a VM forwards every port, the forwarder forwards each TCP port from 1024 up that a server in the VM listens on, on the VM's loopback or on all its addresses. When a VM lists its ports, it forwards those ports alone, however many there are, ports below 1024 included. Each port goes to the same port on this machine's `127.0.0.1` and `::1` and nowhere else, within a few seconds of the server starting, and the forward goes away when the server stops. At most 256 ports are forwarded at once.
- **What it never takes.** The forwarder never takes a port that a program on this machine has on any address. That program keeps the port and its `localhost` connections, and the forward waits until the VM's server stops and starts again. The forwarder also never takes a port that devvm records for a VM, even while that port is free. Those are the ssh, proxy and decider ports devvm chooses for VMs, the VM's own ssh port, and other VMs' listed ports. A Lima VM without network policy has an ssh port that Lima picks and devvm doesn't record, but Lima holds that port, so the forwarder can't take it.
- **A listed port that a program on this machine has** isn't forwarded when a start, or a change of a running VM's ports, finds it in use, and devvm warns about it (`ports_taken`). The VM starts or runs on all the same, and the next start or change that finds the port free forwards it again.
- **A program that starts on a port after it's forwarded** can still bind it on all addresses, as most servers do, and then that program's `localhost` connections reach the VM.
- **How it works.** The forwarder reaches the VM over the VM's ssh, with a key and a host key that devvm pins, and it asks for no forwarding into this machine. Every connection starts on this machine, and the VM opens none to it. The forwarder runs as a hidden `devvm _lima-ports`, `_smolvm-ports` or `_docker-ports` process, one per VM, which devvm starts after each start of the VM and again when you change a running VM's ports, and stops with `devvm stop`. Its log is `<vm>.ports.log` beside the backend's records, which on Lima are in `~/.local/share/devvm/lima-vms`. When a forwarder doesn't start, devvm warns with its log (`ports_forward_failed`), and the VM runs on.
