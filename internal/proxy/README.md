# internal/proxy

The network policy proxy: one process per VM with [network policy](../../docs/content/docs/internals/network.md), on the host, run as `devvm _proxy`. Go, standard library only, with `internal/daemon`, devvm's background-process helper, which is standard library only too (depguard enforces both).

The VM can reach nothing but this proxy. On macOS its Lima hostagent runs under a Seatbelt profile, and on Linux QEMU's network is restricted. Other programs on the host can reach the port too, so the port alone doesn't identify the VM. Every request must carry the VM's password in `Proxy-Authorization`, with the user `devvm`, and the VM's proxy URL has it. The VM's placeholders end in a random value of its own, which isn't the password, so no other VM's or program's placeholder gets a secret. The proxy refuses to start without `-auth-file`. For each connection it:

- answers `407` without the VM's password;
- with `-links`, sends a request for one of the VM's links (`CONNECT db:5432`, `GET http://db:5432/`) to that link's socket on the host before any rule (below);
- checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's policy, which is devvm's built-in policies, read from the binary, and then the user's policy files, and tunnels or forwards allowed ones untouched;
- resolves names itself, and refuses the host's own addresses and internal ones (loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10`) unless a rule names the address; through a company proxy (`-upstream`), which resolves names itself, it refuses a name when the host's resolver answers with such an address, and a name the host can't resolve when only `**` allows it (unless `-upstream-unresolved`, which builds on Lima get), and it reads at most 64 KiB of the company proxy's answer to `CONNECT`;
- requires a TLS ClientHello in a tunnel to name the CONNECT host (SNI);
- for hosts with a bound secret, swaps the placeholder in one header for the real value: it intercepts their `:443` tunnels, and does the same for absolute-form `https://` requests (everything else passes unread);
- with `-nix-cache`, answers `http://devvm.proxy/nix/` itself, as a Nix binary cache that VMs share and only the host fills (below);
- logs every decision as JSON lines.

For a VM whose image has the nix layer, devvm passes `-nix-cache`, and the VM's Nix tries `http://devvm.proxy/nix` before `https://cache.nixos.org`. The proxy takes only `GET` and `HEAD` of `nix-cache-info`, `<32 nix-base32>.narinfo` and `nar/<52 nix-base32>.nar` (bare, `.xz`, `.zst` or `.bz2`), with no query, and answers `400` to anything else. It makes up `nix-cache-info` itself, at priority 30, and fetches the rest from `https://cache.nixos.org` alone, only when the VM's policy allows `cache.nixos.org:443`, through the floor or the company proxy, never following a redirect and sending nothing of the VM's request. Before keeping a narinfo, it checks its signature by the pinned `cache.nixos.org-1` key over its fingerprint (`1;StorePath;NarHash;NarSize;References`), that its StorePath is the one asked for, and that its URL is named by its own `NarHash` or `FileHash`. It fetches a NAR only after a narinfo it checked names it, never for a `HEAD`, each one once however many ask and at most four at once, and keeps it only when its SHA-256 and size are that narinfo's `FileHash` and `FileSize`. Files are written whole and renamed into the folder, mode 600 in folders of 700. Anything that fails answers `404`, and nothing of it is kept, so Nix goes to cache.nixos.org itself. Under a lock (`flock` on `.lock`), the proxy takes each file's room in whole blocks before it writes it (`reserve`), deleting the least recently used files (by modification time, which a hit updates) when the file would take the cache past `-nix-cache-max`, down to nine tenths of it, and `.size` keeps the running total. A NAR being fetched is made its full size at once and counts against the cap, so a fetch that would pass it with them is refused. devvm gc and the retention settings trim it with `TrimNixCache`, under the same lock.

With `-links`, the proxy is a second entrance to the VM's [links between VMs](../../docs/content/docs/internals/network.md#links-through-the-proxy), beside the ssh remote forward that carries every link. `devvm _links` writes every link the VM has, carried or not, to the list (`LinkList`, JSON, version 1) whenever they change, and leaves it when it ends, so a link with no socket is refused rather than asked of the rules, and the proxy reads it for every request: a regular file, not a symbolic link, that only its owner may write, at most 1 MiB. After the password and before any rule, it lowercases the host, drops one trailing dot, and looks for an entry that names that host (`NAME` or `NAME.devvm.internal`) and port exactly. It takes an entry only when its socket is `LinkSocket` of the sockets' folder (`-link-sockets`, devvm's run folder; without it, no link is reached), the VM (`-vm`), the entry's address and its port, which is how `vmssh.Links` names it, and its address is in `127.77.0.0/16`, so the proxy never connects to another VM's link or to any other socket. Any other host under `devvm.internal` gets `403`, logged as a refused block, before the rules. A `CONNECT` is tunneled to the socket as it is, with no SNI check, no interception and no secret, and an absolute-form `http://` request goes there in origin form, without the proxy's headers, over a new connection for each request, which ends when the client gives it up. An `https://` request in absolute form gets `400`. The proxy logs nothing for a link it carried, because `devvm _links` logs it, and when the socket doesn't answer, after a few tries for a full backlog, it answers `502` and logs `link_refused` (`not carried`).

Once it listens, it says so with the address it listens on (`daemon.Ready`) in the status file that devvm names in its environment (`DEVVM_DAEMON_STATUS`), which is what devvm waits for as it starts the proxy. It returns when the VM's hostagent is gone.

## Files

| File | What |
| --- | --- |
| `proxy.go` | `Main` and the flags, the listener, CONNECT and absolute-form handling, the address floor, the log (`Event`, `AllowRule`: `devvm network log` reads it), the `devvm.proxy` endpoints, `-init-ca` |
| `policy.go` | the rule syntax and matching (`ValidRule`: `devvm network allow`, `deny` and `rm` take only these); `ParsePolicy`, the one strict parser of policy files, which `devvm network` and doctor use too: each line that holds more than a comment, with its number and whether it's a rule; the built-in policies are read once (`readBuiltins`), every line a rule, and come first in the union; the user's policy files are re-read when they change, and each line that isn't a rule is logged and skipped, except one that starts with `!` (`BrokenDeny`), which makes the policy deny everything, as refused blocks naming the file and line, until it's fixed |
| `sni.go` | reads a tunnel's ClientHello to check its server name |
| `splice.go` | every tunnel the proxy carries itself: `hijack` takes a `CONNECT`'s connection from the HTTP server and answers `200`, and `splice` passes the bytes both ways as they are, passes on the end of what the client sent, and ends once the server's bytes end or the client has gone |
| `secrets.go` | interception for hosts with a bound secret: certificates from devvm's CA (a day each, never past the CA's end; once the CA has ended, `CONNECT`s to be intercepted answered `403` and logged as refused with `caEndedReason`), header substitution, values from `-secret-cmd` (cached briefly, never logged) |
| `upstream.go` | `-upstream`: a company proxy that allowed requests go through (`CONNECT` by name for tunnels and interception, absolute form for plain HTTP), `NO_PROXY`, and the floor for names it resolves (`floorUpstream`) |
| `rotate.go` | the logs' rotation past `LogLimit` (10 MiB): the event log renamed to `<log>.1` under the file's lock, the proxy's own log, which is also its stderr, copied there and emptied; `OpenEventLog`, the event log's writer, which netpolicy's `AppendEvent` uses too, and which opens the log again before a line once the other writer has rotated it |
| `logins.go` | Codex and opencode logins, and Claude Code's MCP secrets (`~/.claude.json`): the VM's copy with placeholders, bindings, and OAuth refresh on the host |
| `nixcache.go` | `-nix-cache`: the shared Nix cache at `http://devvm.proxy/nix/`, its path pattern (`nixPath`), narinfo checks (`parseNarinfo`), NAR checks, the cap and its trim (`NixCacheUsage`, `TrimNixCache`) |
| `links.go` | `-links`: the VM's links' list (`LinkList`, `LinkEntry`), their sockets' names (`LinkSocket`, `LinkHash`), their addresses and domain, which devvm's other packages use too (`IsLinkAddr`, `LinkAddr`, `LinkDomain`), matching (`linkFor`), `CONNECT` and absolute-form requests to a link |
| `redact.go` | agents' settings files (`settings.json`, `config.toml`, opencode's config) for VMs with network policy: values under secret-looking keys replaced |
| `*_test.go` | unit tests, and fuzz targets for policy rules (`FuzzParseRule`), request targets (`FuzzRequest`), ClientHellos (`FuzzClientHello`) and the Nix cache's paths (`FuzzNixPath`); `testdata/nix/` holds a narinfo and its NAR from cache.nixos.org |

## How devvm runs it

`internal/cli/proxy.go` is the hidden `devvm _proxy` command: it hands its arguments to `Main` with the run UI's streams (`IO`: the event log and data on its `Stdout`, the proxy's own log lines on its `Stderr`) and devvm's own files (`IO.FS`, which is `assets.FS`, so the package never imports `assets`), warns through it about logins copied as they are (`IO.Kept`, `secrets.LoginKept`), and ends an error with its `Die`. The package itself prints nothing.

```text
devvm _proxy -listen 127.0.0.1:PORT -vm NAME -builtin-policy NAME,NAME,… -policy FILE,FILE,… -log LOG.jsonl -proxy-log PROXY.log -lifeline HA.pid \
  -ca ca.pem -cakey ca.key -secrets BINDINGS.json -secret-cmd PATH/TO/devvm -auth-file KEY -links links.json -link-sockets RUN \
  [-upstream-cas CAS.pem] [-upstream UPSTREAM] [-nix-cache DIR -nix-cache-max BYTES]
```

Flags take one dash or two.

| Flag | What |
| --- | --- |
| `-listen` | the VM's proxy port, on loopback |
| `-vm` | the VM's name, for the log |
| `-builtin-policy` | devvm's built-in policies, comma-separated paths in `IO.FS` (`layers/base.policy`, `shared-tools/node.policy`), in order: read once as the proxy starts, ahead of `-policy`'s files, and never again; one that's missing, or holds a line that isn't a rule, is devvm's bug, so `Main` returns an error and the proxy never listens |
| `-policy` | the user's policy files, comma-separated, in order, each re-read when it changes; a missing file has no rules |
| `-log` | the JSON-lines event log, mode 600, rotated past 10 MiB; default stdout |
| `-proxy-log` | the file the proxy's stderr appends to (devvm opens it for the process): the proxy's own log lines, and its HTTP servers' errors, go through it and rotate past 10 MiB by copying, so the last error, written to stderr after `Main` returns, reaches the current file |
| `-lifeline` | the hostagent's pid file (`daemon.Lifeline`): wait up to 15 minutes for it to name a process that runs, past what it held when the proxy started, and return once it no longer holds what it held then or that process is gone |
| `-ca`, `-cakey` | devvm's CA, for interception |
| `-secrets` | the VM's secret bindings (JSON, no values), re-read when they change |
| `-secret-cmd` | the `devvm` binary, run as `devvm secret get NAME` for a value |
| `-upstream-cas` | a file of CA certificates (PEM) the proxy's own TLS connections trust besides the system's |
| `-upstream` | a file (mode 600) of the host's `HTTPS_PROXY=`, `HTTP_PROXY=` and `NO_PROXY=` lines: allowed requests go through that company proxy, which gets host names, and its credentials only in its own `Proxy-Authorization` (in a file because a process's arguments are visible to other users); logs name it by `host:port` |
| `-auth-file` | a file (mode 600) with the VM's key: requests need it as the proxy password, user `devvm` (in a file because a process's arguments are visible to other users) |
| `-nix-cache` | the shared Nix cache's folder (`~/.local/share/devvm/cache/nix-cache`), made mode 700 if it isn't there: serve `http://devvm.proxy/nix/` from it; devvm passes it for VMs whose image has the nix layer |
| `-nix-cache-max` | with `-nix-cache`: the cache's cap, in bytes (devvm passes `nix-cache.size`, 10 GiB when it's off) |
| `-links` | the VM's links' list, which `devvm _links` writes (`~/.local/share/devvm/vms/<hash>/run/links.json`): reach each link through its socket, before any rule; a missing list has no links |
| `-link-sockets` | with `-links`: the folder of the links' sockets, devvm's run folder (`~/.local/share/devvm/run`); without it, no link is reached |

Other modes, each doing one thing and returning:

| | |
| --- | --- |
| `-init-ca DIR` | write a new CA (`ca.pem`, `ca.key`, each written whole; valid 30 days; no host name in it) into DIR unless one with over a week left is there |
| `-login claude -login-file F -login-secrets` | print the MCP servers in `~/.claude.json` whose settings hold env values or headers |
| `-login codex\|opencode\|claude -login-file F -login-vm` | print the VM's copy of the login file, with placeholders (`-login-salt KEY`, or `$DEVVM_LOGIN_SALT`, which keeps it out of the arguments: the VM's random salt, which ends them) |
| `-login … -login-bindings` | print the login's secret bindings (JSON, no values; `-login-salt` as above); with `-login-hosts`, their hosts as allowlist lines (`host:443`) |
| `-login … -login-get NAME` | print one value, refreshing an OAuth token when it's due |
| `-redact json\|toml` | copy stdin to stdout with values under secret-looking keys replaced by `devvm-redacted`, their paths on stderr |

Inside the VM, `http://devvm.proxy/blocked` lists recent blocks, `/blocked/new` those since the last prompt, `/ca.pem` serves the CA certificate, and with `-nix-cache`, `/nix/` is the shared Nix cache.

## Testing

`go test ./internal/proxy` runs the unit tests and the fuzz targets' seeds; `go test -fuzz FuzzRequest ./internal/proxy` fuzzes one. `testdata/script/proxy.txtar` runs `devvm _proxy` itself.
