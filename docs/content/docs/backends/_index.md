---
title: Backends
weight: 40
---

A backend is what runs devvm's VMs and builds their images. devvm decides what goes into a VM, such as its image, its folders, its size and its network policy, and the backend runs it. [Lima](lima.md) is the default backend, and [smolvm](smolvm.md) and [Docker](docker.md) are experimental. Docker runs containers, not VMs ([isolation](docker.md#isolation-a-container-is-not-a-vm)).

Each backend has its own images. An image's key includes its backend, so one backend never uses another's images.

- [Lima](lima.md): the default backend, what it needs, and its limits.
- [smolvm (experimental)](smolvm.md): the microVM backend, and what it needs.
- [Docker (experimental)](docker.md): the container backend, which runs containers, not VMs, and what that means.

## Choosing a backend

A new VM gets the first of:

1. `--backend`.
2. The default, which devvm keeps in `~/.config/devvm/backend` as one line with the backend's name.
3. `lima`.

To set the default:

```sh
devvm setup                  # asks, then for base's features
devvm setup --backend NAME   # without asking; the features stay as they are
```

In a terminal, `devvm setup` lists every backend, with the experimental ones marked, starting from the current default. The up and down keys (or `k` and `j`) move, and Enter confirms. devvm saves and prints the choice, then asks for [base's features](../using/customizing.md#choosing-features). With `--features` alone, or without a terminal, it doesn't ask for the backend.

A VM's state records its backend, and the VM stays on that backend, so devvm ignores `--backend` on an existing VM. When the backend file or a VM's state names a backend devvm doesn't have, devvm stops and lists the ones it has.

A launch asks only the backends that some VM's state names for their VMs, which it does for the [ssh config](../using/ssh.md) and the SSH agent's folder checks. A backend that no VM uses never runs during a launch, so it can't slow one down, even when it isn't running or isn't installed.
