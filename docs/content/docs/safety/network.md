---
title: Network policy
weight: 1
---

A VM with network policy reaches only an allowlist of hosts, through a proxy devvm runs on the host, and never holds your secrets. A secret is a credential devvm keeps on this machine for VMs to use: your GitHub token, Claude Code's token, an agent's login or an API key. New VMs get network policy by default. Without it, a VM reaches anything this machine can, its own services on `localhost` included.

## Turning it on or off

A new VM gets network policy unless you turn it off. When devvm creates a VM in a terminal, it shows what each choice means and asks whether to turn network policy on, with yes as the default:

```text
Network policy for devvm-some-project-71a99b1076d92c:
on: it reaches only the hosts you allow, and your secrets stay on this machine (recommended)
off: it reaches anything this machine can
https://devvm.dev/docs/safety/network/#turning-it-on-or-off
Turn it on? [Y/n]
```

`--network-policy` or `--no-network-policy` answer up front, and without a terminal to ask in, network policy is on. A VM keeps its choice, which devvm records in its state as `net=on` or `net=off`. On an existing VM the flags are ignored, with a warning.

Some VMs can't have network policy: those on a backend that can't confine a VM to its proxy, such as a [Docker](../internals/docker.md#network-policy) engine on another machine, and Docker VMs with the [docker layer](../backends/docker.md#the-docker-layer). devvm doesn't ask about those. It makes the VM without network policy and warns you (`net_off_backend` or `net_off_docker_layer`). If you pass `--network-policy` for such a VM, devvm refuses it, because it never gives an open network to a VM you asked network policy for.

| | |
| --- | --- |
| `devvm network` | print this folder's VM's network policy, as `devvm ls` shows it: `on`, `off`, or `unenforced` ([the self-check](../internals/network.md#the-self-check)) |
| `devvm network on\|off` | change it, from the VM's next start: `devvm restart` |

`devvm network on` is refused for a backend that can't confine the VM to its proxy. Tokens and logins the VM got while it was off still work wherever it kept them: rotate them if that matters. Until the VM starts again:

- **Turned on while running:** it isn't confined, so `devvm ls` shows it `unenforced`, and its next launch offers to restart it confined, as for [a VM started outside devvm](../internals/network.md#the-self-check).
- **Turned off while running confined:** its launches keep it on its proxy, with your tokens still out of it, and warn that it turns off when it next starts.

A backend that can't confine a VM to its proxy is refused before anything is built: devvm never starts a VM that asked for network policy with an open network.

## Allowlists

A VM's proxy reads these policy files, in order, as one list:

| File | Rules for |
| --- | --- |
| `assets/layers/base.policy` (devvm's, [written out](../internals/install.md#devvms-own-files)) | every VM: OS packages, mise's version lists, GitHub |
| `assets/shared-tools/<name>.policy` (devvm's, written out) | each shared tool in the VM's image (`node`, `claude`, …): its registries, downloads and APIs |
| `assets/layers/<name>.policy` (devvm's, written out) | each [built-in layer](../using/images.md#built-in-layers) in the VM's image: `docker`'s has Docker's packages and the container registries Docker Hub and ghcr.io |
| `~/.config/devvm/layers/<name>.policy` | each of your layers in the VM's image |
| `~/.config/devvm/policy` | every VM: your own rules (`devvm policy -g`) |
| `~/.local/share/devvm/vms/<vm>.policy` | that VM only ([`devvm policy`](#changing-rules)) |
| `~/.local/share/devvm/run/<vm>.secrets.policy` | the hosts of the VM's secrets and logins, on port 443 (devvm writes it) |

Storage and paste-like hosts are left out of the defaults on purpose: they're easy ways to send data out. Rules never come from the project folder, which the VM can write. The proxy reads each file again when it changes, so an edit applies at once, to the next connection.

### Rules

Policy files hold one rule per line; `#` starts a comment, and matching ignores case.

| Rule | Matches |
| --- | --- |
| `example.com` | that host only |
| `*.example.com` | any subdomain, at any depth, not `example.com` itself |
| `**` | any host |
| `10.0.0.5`, `10.0.0.0/8`, `fd00::/8` | addresses, IPv4 or IPv6 (also what lets a connection past the internal-address refusal) |
| `RULE:22`, `[fd00::1]:22`, `10.0.0.0/8:8080` | only that port; without a port, 443 and 80 |
| `!RULE` | deny: a deny beats any allow |

A denied address is never dialed, even for an allowed name that resolves to it, or to its NAT64 form. A missing file has no rules; one that changed but can't be read keeps its last rules, so a deny never lapses.

### Changing rules

`devvm policy` shows this folder's VM's allowlist: each of its policy files that has rules, its path after `#`, then its rules, comments left out. For a VM without network policy devvm notes that they don't apply.

| | |
| --- | --- |
| `devvm policy allow RULE...` | add the rules to the VM's own file |
| `devvm policy deny RULE...` | add them as denies (`!` added when missing) |
| `devvm policy rm RULE...` | remove the lines with these rules (a deny with its `!`); one that isn't there gets a warning |
| `-g`, `--global` | change `~/.config/devvm/policy`, for every VM, instead; no VM needed |

A rule is what [Rules](#rules) lists, without spaces or a comment: anything else is refused before any is added, and so is `allow !RULE`. A rule already in the file, in any case, isn't added again. Each change rewrites the file whole (through a link if it is one), so a proxy never reads half of it, and applies at once, to the next connection. The `devvm policy allow` commands that `devvm log --blocked` and the VM's blocked requests suggest are rules it takes.

A new VM never gets the rules of an old one by its name that was deleted outside devvm.

## Inside the VM

The proxy settings are in the VM's environment, for every login (`HTTP_PROXY`, `HTTPS_PROXY` and their lowercase forms, `NO_PROXY` for the VM itself, `NODE_USE_ENV_PROXY`), with the CA files for tools that don't read the system's store. A direct connection fails at once, even as root, and so does DNS: names resolve in the proxy.

A blocked request gets `403`, naming the host, the reason and the rule that would allow it. The proxy also answers at `http://devvm.proxy/`:

- `/blocked`: the last 50 blocks;
- `/blocked/new?after=N`: the number of blocks so far, then those after the Nth, each rule once (for a shell prompt to show what a command hit);
- `/ca.pem`: devvm's CA certificate, never its key.

The VM's setup, on every launch:

- installs devvm's CA into the VM's trust store, so the proxy's certificates for hosts with a bound secret verify, in this VM only;
- points what ignores those variables at the proxy: apt, sudo (which keeps them), every ssh connection (a `ProxyCommand` that tunnels through the proxy, which checks the host and port against the allowlist), and, with the [docker layer](../using/images.md#built-in-layers), Docker's daemon and containers;
- puts the VM's placeholders in `~/.config/devvm/env`, which login shells read (`CLAUDE_CODE_OAUTH_TOKEN`, `GH_TOKEN`), and git's credential helper for GitHub over HTTPS when `github` is bound;
- adds `devvm-why`, which prints what the proxy blocked lately, and a note before each prompt when something new was blocked;
- tells Claude Code and Codex, in `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, why a request may fail and to run `devvm-why`.

## Secrets

Tokens stay on the host. A VM gets a placeholder of its own instead, ending in a random value of its own (its salt, not its password), and its proxy puts the real value into requests for that token's hosts, in one header:

| Secret | Hosts | Header | Used by |
| --- | --- | --- | --- |
| `claude` | `api.anthropic.com` | `Authorization` | every VM whose image has `claude` |
| `github` | `api.github.com`, `github.com`, `codeload.github.com`, `raw.githubusercontent.com` | `Authorization` | VMs it's bound to |

### Saving and binding tokens

| Command | What it does |
| --- | --- |
| `devvm secret set claude` or `github` | asks for the token without showing it (`--stdin` reads a line instead) and saves it |
| `devvm auth claude` | the same as `devvm secret set claude` |
| `devvm secret rm NAME` | deletes a saved token |
| `devvm secret ls` (or `devvm secret`) | lists the secrets, whether each is saved, and the VMs using each |
| `devvm secret use NAME` | lets this folder's VM use the token |
| `devvm secret unuse NAME` | stops this folder's VM using it; a VM whose image has `claude` always uses `claude` |

A token is saved in the macOS Keychain (service `devvm`, account `claude` or `github`, through `security -i` on its input, so the token is in no process's arguments; `DEVVM_SECRET_SERVICE` names another service, letters, digits, `.`, `_` and `-` only), or elsewhere in `~/.config/devvm/secrets/<name>`, a file only you can read. Whitespace in a token is dropped. A token with a quote, a backslash or a control character is refused, and so is a `claude` token that isn't a setup token (`sk-ant-oat01-…`, from `claude setup-token`). A `CLAUDE_CODE_OAUTH_TOKEN` in devvm's environment wins over a saved one.

Saving, deleting, binding and unbinding apply at once to the proxies running: devvm writes their bindings again. The VM gets the placeholder (without network policy, the token itself) in `CLAUDE_CODE_OAUTH_TOKEN` or `GH_TOKEN`, and with `github` a git credential helper for `https://github.com`, from its next launch. `devvm secret unuse` and `devvm secret rm` take nothing back from a VM without network policy: the token it got still works wherever it kept it, and devvm says so on `unuse`. Neither revokes what a VM made with the token ([what devvm doesn't protect](../internals/security.md#what-it-doesnt-protect)): rotate it if that matters. Use a fine-grained GitHub token, scoped to the repos VMs need.

## Logs

Every decision (`tunnel`, `intercept`, `forward`, `blocked`) is a JSON line in `~/.local/share/devvm/logs/<vm>.jsonl`, with the time, the VM, the host and port, the method and path when seen, the rule or reason, and the names of secrets put in, never their values. The proxy's own lines (what it loaded, what failed upstream) go to `<vm>.proxy.log` next to it: an upstream error stays there, since it can name addresses the VM shouldn't learn. Both folders are yours alone, and devvm's run logs never hold a VM's password.

`devvm log` shows this folder's VM's decisions, oldest first, one line each: the time, `allowed` or `BLOCKED`, the host and port, then the rule that allowed it (`, decrypted` for an intercepted tunnel), the request (`GET /user`) and the tokens put in (`, token: github`), or why it was blocked:

```text
2026-10-09T10:00:00Z  BLOCKED  example.com:443  not on the allowlist
2026-10-09T10:00:05Z  allowed  github.com:443  rule github.com
2026-10-09T10:00:06Z  allowed  api.github.com:443  GET /user, token: github
```

| | |
| --- | --- |
| `-n N`, `--lines N` | the last N (default 50) |
| `-f`, `--follow` | then each new one as it happens, until Ctrl+C |
| `--blocked` | the blocks instead, one row per host and port, by when last blocked: `LAST`, `COUNT`, `HOST` and `ALLOW WITH`, the `devvm policy allow` command that allows it |

The VM chose the hosts and paths, so control characters in them are dropped. `ALLOW WITH` is `-` for a request refused whatever the allowlist says (a GitHub call that makes a credential) and for a host that isn't a host name or address. A VM without network policy logs nothing new, and `devvm log` says so. `devvm rm` deletes the VM's logs.

## Not covered

Anything an allowed host accepts can carry data out, including what the tokens allow (pushes, API calls).
