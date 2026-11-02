# internal/proxy

The network policy proxy: one process per VM with [network policy](../../docs/content/docs/internals/network.md), on the host, run as `devvm _proxy`. Go, standard library only (depguard enforces it).

The VM can reach nothing but this proxy. On macOS its Lima hostagent runs under a Seatbelt profile, on Linux QEMU's network is restricted, and on smolvm the proxy decides each connection. Other programs on the host can reach the port too, so the port alone doesn't identify the VM. Every request must carry the VM's password in `Proxy-Authorization`, with the user `devvm`, and the VM's proxy URL has it. The VM's placeholders end in a random value of its own, which isn't the password, so no other VM's or program's placeholder gets a secret. The proxy refuses to start without `-auth-file`. For each connection it:

- answers `407` without the VM's password;
- with `-links`, sends a request for one of the VM's links (`CONNECT db:5432`, `GET http://db:5432/`) to that link's socket on the host before any rule (below);
- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy files, then tunnels or forwards allowed ones untouched;
- resolves names itself, and refuses the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10`) unless a rule names the address; through a company proxy (`-upstream`), which resolves names itself, it refuses a name when the host's resolver answers with such an address, and a name the host can't resolve when only `**` allows it (unless `-upstream-unresolved`, which builds on Lima and a Linux Docker engine get), and it reads at most 64 KiB of the company proxy's answer to `CONNECT`;
- requires a TLS ClientHello in a tunnel to name the CONNECT host (SNI);
- for hosts with a bound secret, swaps the placeholder in one header for the real value: it intercepts their `:443` tunnels, and does the same for absolute-form `https://` requests (everything else passes unread);
- with `-nix-cache`, answers `http://devvm.proxy/nix/` itself, as a Nix binary cache that VMs share and only the host fills (below);
- logs every decision as JSON lines.

On smolvm, the VM's allow list is the host alone, and smolvm asks the proxy about each connection before making it, which smolvm calls mediated egress. With `-decider`, the proxy is that decider too, on a loopback port of its own. It takes over connections to its own address in the VM (`-decider-to`) and serves them as any other, password and policy included. It refuses the rest and logs them as `blocked`. A connection without the decider token that smolvm was started with (`-decider-token-file`) gets no answer.

On Docker, whose VMs can't reach the host's loopback, it also listens on a unix socket (`-listen-unix`), only the user's: a relay (`devvm _proxy -relay`, in a container on the VM's own network) passes each connection the VM makes to it there, unread, and the proxy serves it as any other.

For a VM whose image has the nix layer, devvm passes `-nix-cache`, and the VM's Nix tries `http://devvm.proxy/nix` before `https://cache.nixos.org`. The proxy takes only `GET` and `HEAD` of `nix-cache-info`, `<32 nix-base32>.narinfo` and `nar/<52 nix-base32>.nar` (bare, `.xz`, `.zst` or `.bz2`), with no query, and answers `400` to anything else. It makes up `nix-cache-info` itself, at priority 30, and fetches the rest from `https://cache.nixos.org` alone, only when the VM's policy allows `cache.nixos.org:443`, through the floor or the company proxy, never following a redirect and sending nothing of the VM's request. Before keeping a narinfo, it checks its signature by the pinned `cache.nixos.org-1` key over its fingerprint (`1;StorePath;NarHash;NarSize;References`), that its StorePath is the one asked for, and that its URL is named by its own `NarHash` or `FileHash`. It fetches a NAR only after a narinfo it checked names it, never for a `HEAD`, each one once however many ask and at most four at once, and keeps it only when its SHA-256 and size are that narinfo's `FileHash` and `FileSize`. Files are written whole and renamed into the folder, mode 600 in folders of 700. Anything that fails answers `404`, and nothing of it is kept, so Nix goes to cache.nixos.org itself. Under a lock (`flock` on `.lock`), the proxy takes each file's room in whole blocks before it writes it (`reserve`), deleting the least recently used files (by modification time, which a hit updates) when the file would take the cache past `-nix-cache-max`, down to nine tenths of it, and `.size` keeps the running total. A NAR being fetched is made its full size at once and counts against the cap, so a fetch that would pass it with them is refused. devvm gc and the retention settings trim it with `TrimNixCache`, under the same lock.

With `-links`, the proxy is a second entrance to the VM's [links between VMs](../../docs/content/docs/internals/network.md#links-through-the-proxy), beside the ssh remote forward that carries every link. `devvm _links` writes every link the VM has, carried or not, to the list (`LinkList`, JSON, version 1) whenever they change, and leaves it when it ends, so a link with no socket is refused rather than asked of the rules, and the proxy reads it for every request: a regular file, not a symbolic link, that only its owner may write, at most 1 MiB. After the password and before any rule, it lowercases the host, drops one trailing dot, and looks for an entry that names that host (`NAME` or `NAME.devvm.internal`) and port exactly. It takes an entry only when its socket is `LinkSocket` of the list's folder, the VM (`-vm`), the entry's address and its port, which is how `vmssh.Links` names it, and its address is in `127.77.0.0/16`, so the proxy never connects to another VM's link or to any other socket. Any other host under `devvm.internal` gets `403`, logged as a refused block, before the rules. A `CONNECT` is tunneled to the socket as it is, with no SNI check, no interception and no secret, and an absolute-form `http://` request goes there in origin form, without the proxy's headers, over a new connection for each request, which ends when the client gives it up. An `https://` request in absolute form gets `400`. The proxy logs nothing for a link it carried, because `devvm _links` logs it, and when the socket doesn't answer, after a few tries for a full backlog, it answers `502` and logs `link_refused` (`not carried`).

It returns when the VM's hostagent is gone.

## Files

| File | What |
| --- | --- |
| `proxy.go` | `Main` and the flags, the listener, CONNECT and absolute-form handling, the address floor, the log (`Event`, `AllowRule`: `devvm network log` reads it), the `devvm.proxy` endpoints, `-init-ca` |
| `policy.go` | the rule syntax and matching (`ValidRule`: `devvm network allow`, `deny` and `rm` take only these); policy files are re-read when they change |
| `sni.go` | reads a tunnel's ClientHello to check its server name |
| `decider.go` | `-decider`: smolvm's mediated-egress protocol (version 2), connections taken over or refused |
| `relay.go` | `-listen-unix`, and `-relay`: Docker's relay, bytes passed unread to the proxy's socket |
| `secrets.go` | interception for hosts with a bound secret: certificates from devvm's CA, header substitution, values from `-secret-cmd` (cached briefly, never logged) |
| `upstream.go` | `-upstream`: a company proxy that allowed requests go through (`CONNECT` by name for tunnels and interception, absolute form for plain HTTP), `NO_PROXY`, and the floor for names it resolves (`floorUpstream`) |
| `rotate.go` | the logs' rotation past `LogLimit` (10 MiB): the event log renamed to `<log>.1` under the file's lock, the proxy's own log, which is also its stderr, copied there and emptied; `OpenEventLog`, the event log's writer, which netpolicy's `AppendEvent` uses too, and which opens the log again before a line once the other writer has rotated it |
| `logins.go` | Codex and opencode logins, and Claude Code's MCP secrets (`~/.claude.json`): the VM's copy with placeholders, bindings, and OAuth refresh on the host |
| `nixcache.go` | `-nix-cache`: the shared Nix cache at `http://devvm.proxy/nix/`, its path pattern (`nixPath`), narinfo checks (`parseNarinfo`), NAR checks, the cap and its trim (`NixCacheUsage`, `TrimNixCache`) |
| `links.go` | `-links`: the VM's links' list (`LinkList`, `LinkEntry`), their sockets' names (`LinkSocket`, `LinkHash`), matching (`linkFor`), `CONNECT` and absolute-form requests to a link |
| `redact.go` | agents' settings files (`settings.json`, `config.toml`, opencode's config) for VMs with network policy: values under secret-looking keys replaced |
| `*_test.go` | unit tests, and fuzz targets for policy rules (`FuzzParseRule`), request targets (`FuzzRequest`), ClientHellos (`FuzzClientHello`), the decider's preludes (`FuzzPrelude`) and the Nix cache's paths (`FuzzNixPath`); `testdata/nix/` holds a narinfo and its NAR from cache.nixos.org |

## How devvm runs it

`internal/cli/proxy.go` is the hidden `devvm _proxy` command: it hands its arguments to `Main` with the run UI's streams (`IO`: the event log and data on its `Stdout`, the proxy's own log lines on its `Stderr`), warns through it about logins copied as they are (`IO.Kept`, `secrets.LoginKept`), and ends an error with its `Die`. The package itself prints nothing.

```text
devvm _proxy -listen 127.0.0.1:PORT -vm NAME -policy FILE,FILE,… -log LOG.jsonl -proxy-log PROXY.log -lifeline HA.pid \
  -ca ca.pem -cakey ca.key -secrets BINDINGS.json -secret-cmd PATH/TO/devvm -auth-file KEY -links VM.links.json \
  [-decider 127.0.0.1:PORT -decider-to 100.96.0.1:PORT -decider-token-file TOKEN] [-listen-unix SOCKET] \
  [-upstream-cas CAS.pem] [-upstream UPSTREAM] [-nix-cache DIR -nix-cache-max BYTES]
```

Flags take one dash or two.

| Flag | What |
| --- | --- |
| `-listen` | the VM's proxy port, on loopback |
| `-listen-unix` | Docker only: also listen on this unix socket, mode 600; a socket left there is replaced, anything else refused |
| `-vm` | the VM's name, for the log |
| `-policy` | the policy files, comma-separated, in order; a missing file has no rules |
| `-log` | the JSON-lines event log, mode 600, rotated past 10 MiB; default stdout |
| `-proxy-log` | the file the proxy's stderr appends to (devvm opens it for the process): the proxy's own log lines, and its HTTP servers' errors, go through it and rotate past 10 MiB by copying, so the last error, written to stderr after `Main` returns, reaches the current file |
| `-lifeline` | the hostagent's pid file: return when that process is gone |
| `-ca`, `-cakey` | devvm's CA, for interception |
| `-secrets` | the VM's secret bindings (JSON, no values), re-read when they change |
| `-secret-cmd` | the `devvm` binary, run as `devvm secret get NAME` for a value |
| `-upstream-cas` | a file of CA certificates (PEM) the proxy's own TLS connections trust besides the system's |
| `-upstream` | a file (mode 600) of the host's `HTTPS_PROXY=`, `HTTP_PROXY=` and `NO_PROXY=` lines: allowed requests go through that company proxy, which gets host names, and its credentials only in its own `Proxy-Authorization` (in a file because a process's arguments are visible to other users); logs name it by `host:port` |
| `-auth-file` | a file (mode 600) with the VM's key: requests need it as the proxy password, user `devvm` (in a file because a process's arguments are visible to other users) |
| `-decider` | smolvm only: also decide each connection the VM opens, on this loopback address |
| `-decider-to` | with `-decider`: the proxy's address in the VM, the only destination taken over |
| `-decider-token-file` | with `-decider`: a file (mode 600) with the decider token smolvm proves itself with, 64 hex digits, new at each start |
| `-nix-cache` | the shared Nix cache's folder (`~/.local/share/devvm/nix-cache`), made mode 700 if it isn't there: serve `http://devvm.proxy/nix/` from it; devvm passes it for VMs whose image has the nix layer |
| `-nix-cache-max` | with `-nix-cache`: the cache's cap, in bytes (devvm passes `nix-cache.size`, 10 GiB when it's off) |
| `-links` | the VM's links' list, which `devvm _links` writes (`~/.local/share/devvm/run/<vm>.links.json`): reach each link through its socket, before any rule; a missing list has no links |

Other modes, each doing one thing and returning:

| | |
| --- | --- |
| `-init-ca DIR` | write a new CA (`ca.pem`, `ca.key`, each written whole; valid 30 days; no host name in it) into DIR unless one with over a week left is there |
| `-login claude -login-file F -login-secrets` | print the MCP servers in `~/.claude.json` whose settings hold env values or headers |
| `-login codex\|opencode\|claude -login-file F -login-vm` | print the VM's copy of the login file, with placeholders (`-login-salt KEY`, or `$DEVVM_LOGIN_SALT`, which keeps it out of the arguments: the VM's random salt, which ends them) |
| `-login … -login-bindings` | print the login's secret bindings (JSON, no values; `-login-salt` as above); with `-login-hosts`, their hosts as allowlist lines (`host:443`) |
| `-login … -login-get NAME` | print one value, refreshing an OAuth token when it's due |
| `-redact json\|toml` | copy stdin to stdout with values under secret-looking keys replaced by `devvm-redacted`, their paths on stderr |
| `-relay ADDR -relay-to SOCKET` | listen on ADDR and pass each connection, both ways and unread, to the proxy's socket, until stopped (Docker's relay containers) |

Inside the VM, `http://devvm.proxy/blocked` lists recent blocks, `/blocked/new` those since the last prompt, `/ca.pem` serves the CA certificate, and with `-nix-cache`, `/nix/` is the shared Nix cache.

## Testing

`go test ./internal/proxy` runs the unit tests and the fuzz targets' seeds; `go test -fuzz FuzzRequest ./internal/proxy` fuzzes one. `testdata/script/proxy.txtar` runs `devvm _proxy` itself.
