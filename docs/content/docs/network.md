---
title: Network policy
weight: 24
---

A VM with network policy reaches only an allowlist of hosts, through a proxy devvm runs on the host, and never holds your tokens. Network policy isn't available for VMs yet: devvm creates them only with `--no-network-policy` ([commands](commands.md)). The proxy itself is in place, and [Lima's confinement](lima.md#confinement) is what makes it a VM's only way out. This page describes the proxy.

## The proxy

One proxy runs per VM, on the host, listening on a loopback port: `devvm _proxy`, a hidden command. The VM reaches nothing else, so every connection it makes goes through the proxy. Other programs on the host can reach the port too, so the port alone doesn't identify the VM: every request must carry the VM's password (`Proxy-Authorization`, user `devvm`), or gets `407`. The proxy refuses to start without one. For each connection it:

- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy files, then tunnels or forwards allowed ones untouched;
- takes only host names (letters, digits, `-`, `_`, `.`) and addresses: anything else gets `400` before the policy or the log sees it;
- resolves names itself and dials the addresses it checked, refusing the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10` and other ranges that aren't the public internet; a NAT64 address `64:ff9b::a.b.c.d` counts as its IPv4 address) unless a rule names the address;
- requires a TLS ClientHello in a tunnel to name the `CONNECT` host (SNI). The `Host` inside isn't seen, so a CDN that routes by it can reach other sites it serves;
- for hosts with a bound secret, swaps the placeholder in one header for the real value ([Secrets](#secrets)); everything else passes unread;
- logs every decision.

It exits when the VM's Lima hostagent does (`-lifeline`, the hostagent's pid file), or 15 minutes after starting if the hostagent never appears.

## Rules

Policy files hold one rule per line; `#` starts a comment, and matching ignores case.

| Rule | Matches |
| --- | --- |
| `example.com` | that host only |
| `*.example.com` | any subdomain, at any depth, not `example.com` itself |
| `**` | any host |
| `10.0.0.5`, `10.0.0.0/8`, `fd00::/8` | addresses, IPv4 or IPv6 (also what lets a connection past the internal-address refusal) |
| `RULE:22`, `[fd00::1]:22`, `10.0.0.0/8:8080` | only that port; without a port, 443 and 80 |
| `!RULE` | deny: a deny beats any allow |

A denied address is never dialed, even for an allowed name that resolves to it, or to its NAT64 form. The proxy reads several policy files (`-policy`, comma-separated) as one list, and reads each again when it changes. A missing file has no rules; one that changed but can't be read keeps its last rules, so a deny never lapses.

## Inside the VM

A blocked request gets `403`, naming the host, the reason and the rule that would allow it. The proxy also answers at `http://devvm.proxy/`:

- `/blocked`: the last 50 blocks;
- `/blocked/new?after=N`: the number of blocks so far, then those after the Nth, each rule once (for a shell prompt to show what a command hit);
- `/ca.pem`: devvm's CA certificate, never its key.

## Secrets

For hosts with a bound secret (`-secrets`, a JSON list of bindings: a name, host patterns, a header and the placeholder the VM sends; no values), the proxy intercepts `:443` tunnels instead of passing them through. It terminates TLS with a certificate from devvm's own CA (`-ca`, `-cakey`; ECDSA P-256, valid 30 days, no host name in it), replaces the placeholder in the binding's header (inside `Basic` credentials too) with the real value, and forwards the request over a new TLS connection. Absolute-form `https://` requests get the same. Each request must name the bound host: one that names another gets `421`. The bindings are read again for every request, so removing one applies to tunnels already open.

Values come from `PROGRAM secret get NAME` (`-secret-cmd`), are cached for 5 minutes (30 seconds for agents' logins) and are never logged. GitHub API calls that would make a credential of their own with the token (an account or deploy key, a runner registration, a webhook) are refused with `403`.

## Agents' logins and settings

The proxy also prepares the VM's copies of agents' files on the host, so the VM holds placeholders instead of keys:

- **Codex** (`~/.codex/auth.json`) **and opencode** (its `auth.json`): the VM's copy holds placeholders (shaped like a JWT where the tools read claims), and the bindings put the host's API key or access token into requests for the provider's hosts: OpenAI (`api.openai.com`, `chatgpt.com` for a ChatGPT login), and for opencode Anthropic, OpenAI and OpenRouter keys and OpenAI and GitHub Copilot logins (not Copilot Enterprise). OAuth tokens are refreshed on the host, one lookup at a time, and written back to the host's file, so a VM never holds a refresh token. Logins devvm doesn't know are copied as they are, with a warning.
- **Claude Code's MCP servers** (`~/.claude.json`): an HTTPS server's headers become bindings for its host (a DNS name on port 443 only); env values, plain-HTTP servers' headers, user info and secret-looking values in URLs, and secret-named or key-shaped arguments become `devvm-redacted`. The copy keeps only settings, onboarding and account state, and MCP servers; per-project entries never go.
- **Settings files** (JSON, JSONC or Codex's TOML): values under secret-looking keys become `devvm-redacted`, except values that name where a secret is (an environment variable's name, `{env:X}`, `${X}`, a path). TOML multi-line strings are refused rather than copied unread.

## Logs

Every decision (`tunnel`, `intercept`, `forward`, `blocked`) is a JSON line with the time, the VM, the host and port, the method and path when seen, the rule or reason, and the names of secrets put in, never their values (`-log`, or standard output). The proxy's own lines (what it loaded, what failed upstream) go to standard error: an upstream error stays there, since it can name addresses the VM shouldn't learn.

## devvm _proxy

devvm runs the proxy as itself. Flags take one dash or two; errors end in one line, `network policy proxy: …`.

| Flag | What |
| --- | --- |
| `-listen` | the address to listen on (default `127.0.0.1:0`) |
| `-vm` | the VM's name, for the log |
| `-policy` | the policy files, comma-separated |
| `-log` | the JSON-lines event log (default: standard output) |
| `-lifeline` | the hostagent's pid file: exit when that process is gone |
| `-auth-file` | a file holding the VM's password (in a file because a process's arguments are visible to other users); required |
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
