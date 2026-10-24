# internal/proxy

The network policy proxy: one process per VM with [network policy](../../docs/content/docs/network.md), on the host, run as `devvm _proxy`. Go, standard library only (depguard enforces it).

The VM can reach nothing but this proxy (on macOS its Lima hostagent runs under a Seatbelt profile, on Linux QEMU's network is restricted). Other programs on the host can reach the port too, so the port alone doesn't identify the VM: every request must carry the VM's password (`Proxy-Authorization`, user `devvm`; the VM's proxy URL has it), and the VM's placeholders end in a random value of its own (not the password), so no other VM's or program's placeholder gets a token. It refuses to start without `-auth-file`. For each connection it:

- answers `407` without the VM's password;
- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy files, then tunnels or forwards allowed ones untouched;
- resolves names itself, and refuses the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10`) unless a rule names the address;
- requires a TLS ClientHello in a tunnel to name the CONNECT host (SNI);
- for hosts with a bound secret, swaps the placeholder in one header for the real value: it intercepts their `:443` tunnels, and does the same for absolute-form `https://` requests (everything else passes unread);
- logs every decision as JSON lines.

It returns when the VM's hostagent is gone.

## Files

| File | What |
|---|---|
| `proxy.go` | `Main` and the flags, the listener, CONNECT and absolute-form handling, the address floor, the log (`Event`, `AllowRule`: `devvm log` reads it), the `devvm.proxy` endpoints, `-init-ca` |
| `policy.go` | the rule syntax and matching (`ValidRule`: `devvm policy` takes only these); policy files are re-read when they change |
| `sni.go` | reads a tunnel's ClientHello to check its server name |
| `secrets.go` | interception for hosts with a bound secret: certificates from devvm's CA, header substitution, values from `-secret-cmd` (cached briefly, never logged) |
| `logins.go` | Codex and opencode logins, and Claude Code's MCP secrets (`~/.claude.json`): the VM's copy with placeholders, bindings, and OAuth refresh on the host |
| `redact.go` | agents' settings files (`settings.json`, `config.toml`, opencode's config) for VMs with network policy: values under secret-looking keys replaced |
| `*_test.go` | unit tests, and fuzz targets for policy rules (`FuzzParseRule`), request targets (`FuzzRequest`) and ClientHellos (`FuzzClientHello`) |

## How devvm runs it

`internal/cli/proxy.go` is the hidden `devvm _proxy` command: it hands its arguments to `Main` with devvm's streams (`IO`: the event log and data on `ui.Stdout`, the proxy's own log lines on `ui.Stderr`), warns through `ui` about logins copied as they are (`IO.Kept`, `secrets.LoginKept`), and ends an error with `ui.Die`. The package itself prints nothing.

```text
devvm _proxy -listen 127.0.0.1:PORT -vm NAME -policy FILE,FILE,… -log LOG.jsonl -lifeline HA.pid \
  -ca ca.pem -cakey ca.key -secrets BINDINGS.json -secret-cmd PATH/TO/devvm -auth-file KEY
```

Flags take one dash or two.

| Flag | What |
|---|---|
| `-listen` | the VM's proxy port, on loopback |
| `-vm` | the VM's name, for the log |
| `-policy` | the policy files, comma-separated, in order; a missing file has no rules |
| `-log` | the JSON-lines event log; default stdout |
| `-lifeline` | the hostagent's pid file: return when that process is gone |
| `-ca`, `-cakey` | devvm's CA, for interception |
| `-secrets` | the VM's secret bindings (JSON, no values), re-read when they change |
| `-secret-cmd` | the `devvm` binary, run as `devvm secret get NAME` for a value |
| `-auth-file` | a file (mode 600) with the VM's key: requests need it as the proxy password, user `devvm` (in a file because a process's arguments are visible to other users) |

Other modes, each doing one thing and returning:

| | |
|---|---|
| `-init-ca DIR` | write a new CA (`ca.pem`, `ca.key`, each written whole; valid 30 days; no host name in it) into DIR unless one with over a week left is there |
| `-login claude -login-file F -login-secrets` | print the MCP servers in `~/.claude.json` whose settings hold env values or headers |
| `-login codex\|opencode\|claude -login-file F -login-vm` | print the VM's copy of the login file, with placeholders (`-login-salt KEY`, or `$DEVVM_LOGIN_SALT`, which keeps it out of the arguments: the VM's random salt, which ends them) |
| `-login … -login-bindings` | print the login's secret bindings (JSON, no values; `-login-salt` as above); with `-login-hosts`, their hosts as allowlist lines (`host:443`) |
| `-login … -login-get NAME` | print one value, refreshing an OAuth token when it's due |
| `-redact json\|toml` | copy stdin to stdout with values under secret-looking keys replaced by `devvm-redacted`, their paths on stderr |

Inside the VM, `http://devvm.proxy/blocked` lists recent blocks, `/blocked/new` those since the last prompt, and `/ca.pem` serves the CA certificate.

## Testing

`go test ./internal/proxy` runs the unit tests and the fuzz targets' seeds; `go test -fuzz FuzzRequest ./internal/proxy` fuzzes one. `testdata/script/proxy.txtar` runs `devvm _proxy` itself.
