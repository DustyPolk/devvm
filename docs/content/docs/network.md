---
title: Network policy
weight: 24
---

A VM with network policy reaches only an allowlist of hosts, through a proxy devvm runs on the host, and never holds your tokens. Without it, a VM reaches anything this machine can, its own services on `localhost` included.

## Turning it on

When devvm creates a VM it asks whether to turn network policy on; the answer defaults to no. `--network-policy` or `--no-network-policy` answer up front, and without a terminal to ask in it's off. A VM keeps its choice: it's in its state, `net=on` or `net=off`.

For a VM with network policy, devvm:

1. makes its CA, if it has none with over a week left, and keeps it out of Time Machine (macOS);
2. writes the VM's [secrets](#secrets) as its proxy sees them;
3. starts the VM's [proxy](#the-proxy) on a free port on the host's loopback, and waits for it to say it's listening;
4. starts the VM [confined](lima.md#confinement), its proxy settings pointing at the proxy;
5. runs [the self-check](#the-self-check), before the VM's setup and its shell;
6. sets the VM up: devvm's CA, the proxy for the tools that don't read the environment, the VM's placeholders and its [agents' logins and settings](#agents-logins-and-settings) with your keys taken out.

A backend that can't confine a VM to its proxy is refused before anything is built: devvm never starts a VM that asked for network policy with an open network.

## The proxy

One proxy runs per VM, on the host, listening on a loopback port: `devvm _proxy`, a hidden command. The VM reaches nothing else, so every connection it makes goes through the proxy. Other programs on the host can reach the port too, so the port alone doesn't identify the VM: every request must carry the VM's password (`Proxy-Authorization`, user `devvm`), or gets `407`. Each VM has its own random password, in its proxy settings; the proxy reads it from a file only you can read, never from its arguments, which other users can see. The proxy refuses to start without one. For each connection it:

- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy files, then tunnels or forwards allowed ones untouched;
- takes only host names (letters, digits, `-`, `_`, `.`) and addresses: anything else gets `400` before the policy or the log sees it;
- resolves names itself and dials the addresses it checked, refusing the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10` and other ranges that aren't the public internet; a NAT64 address `64:ff9b::a.b.c.d` counts as its IPv4 address) unless a rule names the address;
- requires a TLS ClientHello in a tunnel to name the `CONNECT` host (SNI). The `Host` inside isn't seen, so a CDN that routes by it can reach other sites it serves;
- for hosts with a bound secret, swaps the placeholder in one header for the real value ([Secrets](#secrets)); everything else passes unread;
- logs every decision.

It exits when the VM's Lima hostagent does (`-lifeline`, the hostagent's pid file), or 15 minutes after starting if the hostagent never appears. A VM that starts again gets a new proxy, on the same port. So does a running VM whose allowlist files changed, whose CA was replaced, or whose proxy is from another devvm binary.

devvm counts a proxy as started only once it says it's listening on the VM's port and still runs: a program that took the port first would otherwise get the VM's traffic. If it doesn't, devvm stops and names the proxy's log.

## Allowlists

A VM's proxy reads these policy files, in order, as one list:

| File | Rules for |
| --- | --- |
| `layers/base.policy` (next to devvm) | every VM: OS packages, mise's version lists, GitHub, container registries |
| `tools/<name>.policy` (next to devvm) | each shared tool in the VM's image (`node`, `claude`, …): its registries, downloads and APIs |
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
- points what ignores those variables at the proxy: apt, sudo (which keeps them), every ssh connection (a `ProxyCommand` that tunnels through the proxy, which checks the host and port against the allowlist), and Docker's daemon and containers;
- puts the VM's placeholders in `~/.config/devvm/env`, which login shells read (`CLAUDE_CODE_OAUTH_TOKEN`, `GH_TOKEN`), and git's credential helper for GitHub over HTTPS when `github` is bound;
- adds `devvm-why`, which prints what the proxy blocked lately, and a note before each prompt when something new was blocked;
- tells Claude Code and Codex, in `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, why a request may fail and to run `devvm-why`.

## The self-check

Network policy holds only while the VM runs on the Lima process devvm started confined: a process can't leave its sandbox, but `limactl start` starts a new one, unconfined. So on every launch, whether devvm started the VM or found it running, devvm checks, on the host, that the VM still runs on the process it started, then that a direct connection from the VM to the internet fails. The VM answers the second check, so it only catches the honest case; the first is the one the VM can't fake. The result is in the VM's state: `net_check=ok` or `net_check=unenforced`.

- **Started outside devvm:** in a terminal, devvm offers to restart the VM confined; declined, it warns and goes on. Without a terminal, or if the VM is still unenforced after the restart, devvm stops the VM and ends without opening a shell; the next `devvm` starts it confined.
- **Confined, yet it gets out:** the VM reaches the internet through something devvm didn't set up, such as a second network. A restart wouldn't help: devvm stops the VM and ends.

`devvm ls` shows such a VM's network policy as `unenforced`: one whose last self-check failed, or one running that isn't on the process devvm started (checked on the host each time).

## Secrets

Tokens stay on the host. A VM gets a placeholder of its own instead, ending in a random value of its own (its salt, not its password), and its proxy puts the real value into requests for that token's hosts, in one header:

| Secret | Hosts | Header | Used by |
| --- | --- | --- | --- |
| `claude` | `api.anthropic.com` | `Authorization` | every VM whose image has `claude` |
| `github` | `api.github.com`, `github.com`, `codeload.github.com`, `raw.githubusercontent.com` | `Authorization` | VMs it's bound to |

devvm reads a token from the macOS Keychain (service `devvm`, account `claude` or `github`; `DEVVM_SECRET_SERVICE` names another service, letters, digits, `.`, `_` and `-` only), or elsewhere from `~/.config/devvm/secrets/<name>`, a file only you can read. A `CLAUDE_CODE_OAUTH_TOKEN` in devvm's environment wins over a saved one. A token's hosts join the VM's allowlist on port 443 whether a value is saved or not.

devvm writes the VM's bindings (`~/.local/share/devvm/run/<vm>.secrets.json`: each secret's name, hosts, header and placeholder, never its value) for those with a value. For a bound host the proxy intercepts `:443` tunnels instead of passing them through. It terminates TLS with a certificate from devvm's own CA (`~/.local/share/devvm/ca`; ECDSA P-256, valid 30 days, no host name in it), replaces the placeholder in the binding's header (inside `Basic` credentials too) with the real value, and forwards the request over a new TLS connection. Absolute-form `https://` requests get the same. Each request must name the bound host: one that names another gets `421`. The bindings are read again for every request, so removing one applies to tunnels already open.

The proxy gets a value by running `devvm secret get NAME`, which prints it, never to a terminal. Values are cached for 5 minutes (30 seconds for agents' logins) and never logged. GitHub API calls that would make a credential of their own with the token (an account or deploy key, a runner registration, a webhook) are refused with `403`.

## Agents' logins and settings

Agents' login files stay on the host too. For a VM whose image has the agent, and a login file on the host, the login's bindings join the VM's and their hosts its allowlist, on port 443:

- **Codex** (`~/.codex/auth.json`) **and opencode** (`~/.local/share/opencode/auth.json`): the VM's copy holds placeholders (shaped like a JWT where the tools read claims), and the bindings put the host's API key or access token into requests for the provider's hosts: OpenAI (`api.openai.com`, `chatgpt.com` for a ChatGPT login), and for opencode Anthropic, OpenAI and OpenRouter keys and OpenAI and GitHub Copilot logins (not Copilot Enterprise). OAuth tokens are refreshed on the host, one lookup at a time, and written back to the host's file, so a VM never holds a refresh token. Logins devvm doesn't know are copied as they are, with a warning.
- **Claude Code's MCP servers** (`~/.claude.json`): an HTTPS server's headers become bindings for its host (a DNS name on port 443 only); env values, plain-HTTP servers' headers, user info and secret-looking values in URLs, and secret-named or key-shaped arguments become `devvm-redacted`. The copy keeps only settings, onboarding and account state, and MCP servers; per-project entries never go.
- **Settings files** (JSON, JSONC or Codex's TOML): values under secret-looking keys become `devvm-redacted`, except values that name where a secret is (an environment variable's name, `{env:X}`, `${X}`, a path). TOML multi-line strings are refused rather than copied unread.

Which files each agent shares: [Coding agents](coding-agents.md).

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

## devvm _proxy

devvm runs the proxy as itself. Flags take one dash or two; errors end in one line, `network policy proxy: …`.

| Flag | What |
| --- | --- |
| `-listen` | the address to listen on (default `127.0.0.1:0`) |
| `-vm` | the VM's name, for the log |
| `-policy` | the policy files, comma-separated |
| `-log` | the JSON-lines event log (default: standard output) |
| `-lifeline` | the hostagent's pid file: exit when that process is gone |
| `-auth-file` | a file holding the VM's password; required |
| `-ca`, `-cakey` | devvm's CA, for interception |
| `-secrets` | the VM's secret bindings, read again when they change |
| `-secret-cmd` | the program run as `PROGRAM secret get NAME` for a value |

Modes that do one thing and exit:

| Mode | What |
| --- | --- |
| `-init-ca DIR` | write a new CA (`ca.pem`, and `ca.key` readable only by you, each written whole) into DIR, unless one with over a week left is there |
| `-login codex\|opencode\|claude -login-file F -login-vm` | print the VM's copy of the login file; placeholders end in `-login-salt KEY` (or `$DEVVM_LOGIN_SALT`, which keeps it out of the arguments), so each VM's are its own |
| `-login … -login-bindings` | print the login's bindings (JSON, no values); with `-login-hosts`, their hosts as rules (`host:443`) |
| `-login claude -login-file F -login-secrets` | print the MCP servers whose settings hold secrets |
| `-login … -login-get NAME` | print one value, refreshing an OAuth token when it's due |
| `-redact json\|toml` | copy standard input to standard output with secret-looking values replaced, their paths on standard error |

## Not covered

Anything an allowed host accepts can carry data out, including what the tokens allow (pushes, API calls).
