---
title: Network policy
weight: 5
---

This page explains how devvm confines a VM to its proxy, what the proxy does with each connection, and how secrets get into requests.

## Starting a VM with network policy

For a VM with network policy, devvm:

1. makes its CA, if it has none with over a week left, and keeps it out of Time Machine (macOS);
2. writes the VM's [secrets](../safety/network.md#secrets) as its proxy sees them;
3. starts the VM's [proxy](#the-proxy) on a free port on the host's loopback, and waits for it to say it's listening;
4. starts the VM [confined](lima.md#confinement) (on smolvm, [emulated](smolvm.md#network-policy); on Docker, [emulated](docker.md#network-policy)), its proxy settings pointing at the proxy;
5. runs [the self-check](#the-self-check), before the VM's setup and its shell;
6. sets the VM up: devvm's CA, the proxy for the tools that don't read the environment, the VM's placeholders and its [agents' logins and settings](#agents-logins-and-settings) with your keys taken out.

## The proxy

One proxy runs per VM, on the host, listening on a loopback port: `devvm _proxy`, a hidden command. The VM reaches nothing else, so every connection it makes goes through the proxy. Other programs on the host can reach the port too, so the port alone doesn't identify the VM: every request must carry the VM's password (`Proxy-Authorization`, user `devvm`), or gets `407`. Each VM has its own random password, in its proxy settings; the proxy reads it from a file only you can read, never from its arguments, which other users can see, and the VM's settings get it through files too ([Lima](lima.md#a-lima-home-of-its-own), [smolvm](smolvm.md#what-devvm-sets-on-a-vm)). The proxy refuses to start without one. For each connection it:

- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy files, then tunnels or forwards allowed ones untouched;
- takes only host names (letters, digits, `-`, `_`, `.`) and addresses: anything else gets `400` before the policy or the log sees it;
- resolves names itself and dials the addresses it checked, refusing the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10` and other ranges that aren't the public internet; a NAT64 address `64:ff9b::a.b.c.d` counts as its IPv4 address) unless a rule names the address;
- requires a TLS ClientHello in a tunnel to name the `CONNECT` host (SNI). The `Host` inside isn't seen, so a CDN that routes by it can reach other sites it serves;
- for hosts with a bound secret, swaps the placeholder in one header for the real value ([Secrets](../safety/network.md#secrets)); everything else passes unread;
- logs every decision.

On smolvm it also decides, before smolvm makes it, each connection the VM opens: it takes over those to itself and refuses the rest ([details](smolvm.md#network-policy)). On Docker, whose VMs can't reach the host's loopback, it also listens on a unix socket only you can reach, which a relay on the VM's own network passes the VM's connections to, unread ([details](docker.md#network-policy)).

It exits when the VM's Lima hostagent does (`-lifeline`, the hostagent's pid file; on smolvm, the machine's process; on Docker, a `docker container wait` on the VM's container), or 15 minutes after starting if the hostagent never appears; `devvm stop` stops it at once. A VM that starts again gets a new proxy, on the same port. So does a running VM whose allowlist files changed, whose CA was replaced, or whose proxy is from another devvm binary.

devvm counts a proxy as started only once it says it's listening on the VM's port and still runs: a program that took the port first would otherwise get the VM's traffic. If it doesn't, devvm stops and names the proxy's log. A VM already running whose proxy can't start again (gone, and another program took its port, or it fails to start) is stopped too (`net_proxy_down_stopped`): a VM with network policy never runs on without its proxy. The next `devvm` starts both.

## The self-check

Network policy holds only while the VM runs on the Lima process devvm started confined: a process can't leave its sandbox, but `limactl start` starts a new one, unconfined. So on every launch, whether devvm started the VM or found it running, devvm checks, on the host, that the VM still runs on the process it started, then that a direct connection from the VM to the internet fails. The VM answers the second check, so it only catches the honest case; the first is the one the VM can't fake. The result is in the VM's state: `net_check=ok` or `net_check=unenforced`.

- **Not started confined** (started outside devvm, or running since before `devvm network on`): in a terminal, devvm offers to restart the VM confined; declined, it warns and goes on. Without a terminal, or if the VM is still unenforced after the restart, devvm stops the VM and ends without opening a shell; the next `devvm` starts it confined.
- **Confined, yet it gets out:** the VM reaches the internet through something devvm didn't set up, such as a second network. A restart wouldn't help: devvm stops the VM and ends.

On smolvm, the process devvm started is the machine's, through the proxy's decider; `smolvm machine start` refuses the VM without a decider, and one started with someone else's isn't confined. On Docker, it's that start of the container, on its own network alone, with nothing else on it ([details](docker.md#network-policy)).

`devvm ls` shows such a VM's network policy as `unenforced`: one whose last self-check failed, or one running that isn't on the process devvm started confined (checked on the host each time). So does `devvm network`.

## How the proxy puts them in

A token's hosts join the VM's allowlist on port 443 whether a value is saved or not.

devvm writes the VM's bindings (`~/.local/share/devvm/run/<vm>.secrets.json`: each secret's name, hosts, header and placeholder, never its value) for those with a value. For a bound host the proxy intercepts `:443` tunnels instead of passing them through. It terminates TLS with a certificate from devvm's own CA (`~/.local/share/devvm/ca`; ECDSA P-256, valid 30 days, no host name in it), replaces the placeholder in the binding's header (inside `Basic` credentials too) with the real value, and forwards the request over a new TLS connection. Absolute-form `https://` requests get the same. Each request must name the bound host: one that names another gets `421`. The bindings are read again for every request, so removing one applies to tunnels already open.

The proxy gets a value by running `devvm secret get NAME`, which prints it, never to a terminal. Values are cached for 5 minutes (30 seconds for agents' logins) and never logged. GitHub API calls that would make a credential of their own with the token (an account or deploy key, a runner registration, a webhook) are refused with `403`.

## Agents' logins and settings

Agents' login files stay on the host too. For a VM whose image has the agent, and a login file on the host, the login's bindings join the VM's and their hosts its allowlist, on port 443:

- **Codex** (`~/.codex/auth.json`) **and opencode** (`~/.local/share/opencode/auth.json`): the VM's copy holds placeholders (shaped like a JWT where the tools read claims), and the bindings put the host's API key or access token into requests for the provider's hosts: OpenAI (`api.openai.com`, `chatgpt.com` for a ChatGPT login), and for opencode Anthropic, OpenAI and OpenRouter keys and OpenAI and GitHub Copilot logins (not Copilot Enterprise). OAuth tokens are refreshed on the host, one lookup at a time, and written back to the host's file, so a VM never holds a refresh token. Logins devvm doesn't know are copied as they are, with a warning.
- **Claude Code's MCP servers** (`~/.claude.json`): an HTTPS server's headers become bindings for its host (a DNS name on port 443 only); env values, plain-HTTP servers' headers, user info and secret-looking values in URLs, and secret-named or key-shaped arguments become `devvm-redacted`. The copy keeps only settings, onboarding and account state, and MCP servers; per-project entries never go.
- **Settings files** (JSON, JSONC or Codex's TOML): values under secret-looking keys become `devvm-redacted`, except values that name where a secret is (an environment variable's name, `{env:X}`, `${X}`, a path). TOML multi-line strings are refused rather than copied unread.

Which files each agent shares: [Coding agents](../using/coding-agents.md).

## MCP servers

On each launch the VM's `~/.claude.json` gets this machine's settings and MCP servers, merged into its own: each of your servers replaces the VM's server by that name whole, servers devvm copied before that you no longer have are removed, and servers added in the VM stay. The VM keeps its own per-project entries (trust, allowed tools); yours never go in. Without network policy, the servers' env values and headers are copied as they are, and devvm names the servers that have them.

## Keys in settings files

With network policy, the VM's copies of `~/.claude/settings.json`, `~/.codex/config.toml` and opencode's `.json`, `.jsonc` and `.toml` files have every value under a key that looks like a secret's (`*key*`, `*token*`, `*secret*`, `*password*`, `Authorization`) replaced with `devvm-redacted`, and devvm names what it replaced. Values that only point at a secret stay: an environment variable's name (`env_key = "OPENAI_API_KEY"`), `{env:X}`, `${X}`, a path. A file devvm can't read that way (a TOML multi-line string) isn't copied. An MCP server or provider that needs one of those keys in the VM has to get it another way.

## devvm _proxy

devvm runs the proxy as itself. Flags take one dash or two; errors end in one line, `network policy proxy: …`.

| Flag | What |
| --- | --- |
| `-listen` | the address to listen on (default `127.0.0.1:0`) |
| `-listen-unix` | also listen on this unix socket, only you can reach (mode 600); a socket left there is replaced, anything else refused |
| `-vm` | the VM's name, for the log |
| `-policy` | the policy files, comma-separated |
| `-log` | the JSON-lines event log (default: standard output) |
| `-lifeline` | the hostagent's pid file: exit when that process is gone |
| `-auth-file` | a file holding the VM's password; required |
| `-ca`, `-cakey` | devvm's CA, for interception |
| `-secrets` | the VM's secret bindings, read again when they change |
| `-secret-cmd` | the program run as `PROGRAM secret get NAME` for a value |
| `-decider` | also decide each connection the VM opens, on this loopback address (smolvm's mediated egress) |
| `-decider-to` | with `-decider`: the proxy's address in the VM, the only destination taken over |
| `-decider-token-file` | with `-decider`: a file holding the token smolvm proves itself with (64 hex digits); required |

Modes that do one thing and exit:

| Mode | What |
| --- | --- |
| `-init-ca DIR` | write a new CA (`ca.pem`, and `ca.key` readable only by you, each written whole) into DIR, unless one with over a week left is there |
| `-login codex\|opencode\|claude -login-file F -login-vm` | print the VM's copy of the login file; placeholders end in `-login-salt KEY` (or `$DEVVM_LOGIN_SALT`, which keeps it out of the arguments), so each VM's are its own |
| `-login … -login-bindings` | print the login's bindings (JSON, no values); with `-login-hosts`, their hosts as rules (`host:443`) |
| `-login claude -login-file F -login-secrets` | print the MCP servers whose settings hold secrets |
| `-login … -login-get NAME` | print one value, refreshing an OAuth token when it's due |
| `-redact json\|toml` | copy standard input to standard output with secret-looking values replaced, their paths on standard error |

`-relay ADDR -relay-to SOCKET` is a relay: it listens on `ADDR` and passes each connection, both ways and unread, to the proxy's unix socket, until it's stopped; it decides nothing. Docker's relay containers run it.
