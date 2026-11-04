---
title: Network policy
weight: 5
---

This page explains how devvm confines a VM to its proxy, what the proxy does with each connection, and how your secrets get into the VM's requests without the VM holding them. [Network policy](../safety/network.md) covers turning it on, the allowlists and their rules, the secrets and the logs as you use them.

## Starting a VM with network policy

A VM's state records its choice in its `net` section, as `"on": true` or `"on": false`. When devvm starts a VM with network policy, it takes these steps in order:

1. It readies the VM's own CA and the next one ([the VM's CA](#the-vms-ca)), and on macOS it keeps them out of Time Machine.
2. It writes the VM's [secrets](#secrets) as the VM's proxy sees them.
3. It starts the VM's [proxy](#the-proxy) on a free port on the host's loopback, and waits for the proxy to say it's listening.
4. It starts the VM [confined](lima.md#confinement), with the VM's proxy settings pointing at the proxy.
5. It runs [the self-check](#the-self-check) before the VM's setup and before its shell.
6. It sets the VM up, as [the VM's setup](#the-vms-setup) describes. The setup puts the VM's CA and the next one in the VM, points the tools that don't read the environment at the proxy, and writes the VM's placeholders and its [agents' logins and settings](#agents-logins-and-settings) with your keys taken out.

## VMs that can't have it

devvm makes a VM without network policy, and warns you, when the VM's backend can't confine it to its proxy (`net_off_backend`).

## The proxy

One proxy runs for each VM. It runs on the host as `devvm _proxy`, a hidden command, and listens on a loopback port. The VM reaches nothing else over its network, so every connection the VM makes goes through its proxy. The one other way out of the VM is the [links](ports.md#links-between-vms) you add to it, which this machine carries over ssh to the ports your other VMs publish, so they don't use the VM's network. The proxy is a second entrance to those links, for the VM's HTTP clients ([links through the proxy](#links-through-the-proxy)).

Other programs on the host can also reach that port, so the port alone doesn't identify the VM. Every request must carry the VM's password in `Proxy-Authorization`, with the user `devvm`, or it gets `407`. Each VM has its own random password in its proxy settings. The proxy reads the password from its config, a file only you can read, never from its arguments, which other users can see, and it refuses to start without one ([devvm _proxy](#devvm-_proxy)). The VM's settings also get the password through files ([Lima](lima.md#a-lima-home-of-its-own)).

For each connection, the proxy does the following:

- It sends a request for one of the VM's own links to that link before any rule, and passes its bytes on as they are ([links through the proxy](#links-through-the-proxy)).
- It checks `CONNECT host:port` and absolute-form requests (`GET http://host/…`) against the VM's [policy files](#policy-files), and then tunnels or forwards the allowed ones untouched.
- It takes only host names, made of letters, digits, `-`, `_` and `.`, and addresses. It lowercases a name and drops one trailing dot, and the name that's left may have no empty label, so `evil.test..`, `evil..test` and `.evil.test` get `400` too, because a company proxy that drops trailing dots would otherwise reach a host that a deny of `evil.test` never matched. Anything else gets `400` before the policy or the log sees it.
- It resolves names itself and dials the addresses it checked. It refuses the host's own addresses and internal ones unless a rule names the address. Internal addresses are loopback, private, link-local, multicast, unspecified, CGNAT `100.64.0.0/10` and the other ranges that aren't the public internet, and a NAT64 address `64:ff9b::a.b.c.d` counts as its IPv4 address.
- It requires a TLS ClientHello in a tunnel to name the `CONNECT` host in its SNI. The proxy doesn't see the `Host` inside the encrypted traffic, so a CDN that routes by it can reach other sites it serves.
- For a host with a bound secret, it replaces the placeholder in one header with the real value, as [how the proxy puts them in](#how-the-proxy-puts-them-in) describes. Everything else passes unread.
- It logs every decision ([log files](#log-files)).

A `CONNECT` tunnel meets the proxy's checks in this order:

```mermaid
flowchart TD
  conn["CONNECT host:port from the VM"] --> pw["the VM's password?<br>no: 407"]
  pw --> name["a host name or an address?<br>no: 400"]
  name --> link{{"one of the VM's links?"}}
  link -- "yes" --> linked["the link's socket,<br>tunneled untouched"]
  link -- "no" --> allow["on the allowlist, and not denied?<br>no: 403"]
  allow --> floor["public and not this machine's,<br>or an address a rule names?<br>no: 403"]
  floor --> bound{{"a bound host, on port 443?"}}
  bound -- "no" --> sni["its TLS SNI names<br>the CONNECT host?<br>no: closed"]
  sni -- "yes, or not TLS" --> tunnel["tunneled untouched"]
  bound -- "yes" --> intercept["intercepted, with<br>the VM's CA"]
  intercept --> named["each request names<br>the bound host?<br>no: 421"]
  named --> swap["the placeholder swapped<br>for the real value"]
```

A tunnel that the proxy doesn't intercept carries the bytes both ways as they are. When the VM finishes sending, the proxy closes its side of the connection to the far end, so the far end sees the end of what the VM sent and can still answer. The tunnel ends when the far end's bytes end, or when the proxy can't pass them on because the VM's connection has gone, and then the proxy closes both connections. In a tunnel to a host the policy allows, the far end's bytes reach the VM at once, so a protocol whose server speaks first, such as SMTP, gets its greeting before the VM sends anything. The VM's bytes wait until the proxy has checked them: when the first one starts a TLS record, the proxy reads the whole ClientHello and checks its SNI before any of it reaches the far end, and any other first byte passes at once.

### Links through the proxy

A VM's `NO_PROXY` names only the VM itself, so an HTTP client that uses the VM's proxy settings sends a request for a link, such as `http://db:5432/`, to the proxy. A running VM can't take a new environment, so the proxy carries those requests itself, and a link added while the VM runs works for every HTTP client at once.

`devvm _links` writes every link the VM has, carried or not, to a list, `links.json` in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/`, mode 600 in a folder of mode 700, whenever they change or the file is gone. Each entry has the link's names (`NAME` and `NAME.devvm.internal`), its address and port in the VM, its target, and its socket on this machine. The list stays when `devvm _links` ends, and devvm removes it with the VM, so a link that isn't carried, which has no socket, is refused rather than asked of the rules. devvm puts the list's path in the proxy's config as `links`, and its run folder, where the links' sockets are, as `link_sockets`. The proxy checks the list for every request and reads it again when it has changed, as it does the policy files. A list it can't read has no links, which its log says once. The list must be a regular file, not a symbolic link, that only you can write, and at most 1 MiB.

The proxy checks a request's host against the list after the VM's password and before any rule, as it does for `devvm.proxy`. It lowercases the host and drops one trailing dot, and then a host and port match only an entry that names them exactly, so `DB.:5432` matches `db:5432`, while `db..:5432`, `db.devvm:5432` and `127.77.0.1:5432` don't. The request names a host and a port, never a socket. An entry counts only when its socket is the one that `devvm _links` names for this VM's link at that address and port, in the run folder of devvm's data folder, so the proxy never connects to another VM's link or to any other socket, whatever the list holds. Its address must also be in `127.77.0.0/16`. Any other name under `devvm.internal` is refused with `403` and logged as a refused block, whatever the rules allow, so the rules, the host's resolver and a company proxy never see one. A host and port on no entry are otherwise the policy's, as before.

For a match, the proxy connects to the link's socket, which is the same socket that the link's remote forward reaches, so `devvm _links` checks the connection as it checks every other ([each connection](ports.md#links-between-vms)), and the 256-connection cap counts both. A `CONNECT` is tunneled as it is, with no SNI check and no interception, and an absolute-form request, such as `GET http://db:5432/`, goes to the socket in origin form, without `Proxy-Authorization` or `Proxy-Connection`, over a new connection for each request, which ends when the client gives up the request. A kept-alive connection to the proxy that asks for a link and then for another host has each request handled on its own. HTTPS in absolute form gets `400`, because `CONNECT` carries it. The proxy puts no secret into a link's connection, even when a secret's hosts would match the link's name, and its rules don't apply, so `devvm network deny db` doesn't stop the link `db`.

The proxy logs nothing for a link's connection that it passes on, because `devvm _links` logs it as `link`, once. When the socket doesn't answer, because the link was removed since the proxy read the list or `devvm _links` isn't running, the proxy answers `502` and logs `link_refused` with the reason `not carried`. A socket whose backlog is full refuses a connection, so the proxy tries again for under a second.

### Through a company proxy

When devvm starts a VM's proxy, it reads `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` from its own environment, each in capitals first and then in lowercase, as Go's clients do. When its environment sets neither `HTTPS_PROXY` nor `HTTP_PROXY`, it reads `config.toml`'s `[proxy]` table instead, its `https`, `http` and `no-proxy`, and never a mix of the two. It writes them into the proxy's config, `proxy.json` in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/`, as `upstream`, so a password in the environment's URL is in a file only you can read and in no process's arguments. devvm checks the settings first: a proxy URL is `http://user:password@host:port`, or `host:port` alone, and a proxy without a port is on port 80. devvm refuses an `https://` or `socks5://` proxy (`upstream_proxy_scheme`) and a setting it can't read (`upstream_proxy_invalid`), so a VM with network policy doesn't start until you change it.

A `config.toml` URL never holds a password (`proxy_password_in_config`, as it's read), because people copy that file between machines and keep it with their dotfiles. When its URL names a user, `upstream` gets `"secret": "proxy"`, and the proxy looks the `proxy` secret up as it starts, through the same lookup as the [secrets it puts in](#where-secrets-are-saved), the Keychain or `~/.local/share/devvm/secrets/proxy`, and uses it as the password of each URL that names a user and none of its own. So `proxy.json` never holds that password. A proxy whose lookup fails doesn't start, and devvm checks that the secret is saved before it makes a new VM, starts a VM's proxy, or builds an image or the shared tools (`proxy_password_missing`). Each of these checks is `settings.CompanyProxy.Upstream`.

devvm's own requests on this machine go through the same company proxy. They are the download of its own mise, the list of Ubuntu's cloud images, and the registries `--from` asks for digests. When the environment sets the proxy, their clients use Go's own environment proxy. When `config.toml` sets it, `settings.CompanyProxy.Route` is their transport's proxy. At the first request, it looks the `proxy` secret up and puts it in each URL that names a user and none of its own. It routes each request as a VM's proxy does: `HTTPS_PROXY`'s for HTTPS, `HTTP_PROXY`'s for plain HTTP, and directly for `NO_PROXY`'s hosts, `localhost` and loopback addresses. Go sends the user and password only to the company proxy, in `Proxy-Authorization`. A missing password fails the request without a retry. The mise that devvm runs on this machine inherits devvm's environment, so it uses the environment's proxy as it is. With `config.toml`'s proxy, devvm adds `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` to mise's environment, in capitals and in lowercase, with the password in each URL that names a user. mise takes a proxy's password only in its URL, so its environment is the one place it can go. It's never in mise's arguments or in a file.

A VM's proxy refreshes Codex's and opencode's OAuth logins through the company proxy it sends the VM's requests through, its `upstream`, whether that came from the environment or from `config.toml`. As it starts, it makes that upstream's route for its own requests, with the `proxy` secret it looked up as the password, as `proxy.Upstream.WithPassword` and `proxy.Upstream.Route` do for devvm's own requests. It gives the route to each lookup in the lookup's context (`proxy.WithRoute`), and `agentlogin` sets it as the refresh's transport proxy (`proxy.RouteOf`). A VM's proxy without an upstream sends the refresh directly, as it does the VM's requests.

A VM without network policy gets only the proxy settings of devvm's environment, through Lima's `propagateProxyEnv`, and never `config.toml`'s, because the guest would need the password. When the environment sets no proxy and `config.toml` sets one, each start of such a VM warns (`proxy_needs_policy`).

devvm warns (`upstream_proxy_gone`) when the config the VM's proxy last started with has an `upstream` and the one it starts it with now has none, as happens when a shell without the settings starts it.

The proxy decides each request as it does without a company proxy, by the host name the VM asked for, and only then picks the way out:

- A host that `NO_PROXY` names, `localhost`, a name under `.localhost` and a loopback or unspecified address go directly, through the checks above.
- A tunnel, and the connection the proxy makes to a bound host to put a secret in, go through `HTTPS_PROXY`'s proxy. The proxy sends `CONNECT host:port` with the name the VM asked for, and `Proxy-Authorization: Basic` from the URL's user and password, and it reads the answer. Anything but `200` refuses the VM's request, naming the company proxy by its host and port. Over that tunnel, the proxy's TLS connection to a bound host checks the server's certificate against the system's roots and the [CAs in your CA folder](#a-companys-ca), as it does directly.
- A plain HTTP request goes to `HTTP_PROXY`'s proxy in absolute form, with its `Proxy-Authorization` in place of the VM's. The company proxy's address in the request has no password, so no error the proxy logs holds one.
- Without `HTTPS_PROXY` or `HTTP_PROXY`, those requests go directly.

`NO_PROXY` is a comma-separated list, which the proxy reads as Go's clients read it. In it, `*` matches every host, and an address or a CIDR matches addresses. A name matches itself and its subdomains, while a name with a leading `.` or `*.` matches only its subdomains. Any entry can end in a port, which limits it to that port.

The company proxy resolves the names it's sent, so the proxy can't hold it to the addresses it checked, and the floor changes for those requests. An address in the VM's request is checked as before, and a host whose last label is all digits or starts with `0x`, such as `127.1`, gets `400`, because a company proxy may read it as an address. For a name, the proxy asks this machine's resolver, for at most three seconds, and refuses the request when any answer is internal or denied, unless a rule names the address. A name this machine can't resolve, which is common behind a company proxy, goes to the company proxy unchecked when a rule names it, as itself or under `*.domain`. When only `**` allows it, the proxy refuses it with `403`, and the event log says that this machine can't resolve it and only `**` allows it. The proxy reads at most 64 KiB of the company proxy's answer to `CONNECT`. [The design note](design.md#a-company-proxy) explains what that leaves open, and how builds reach the company proxy through a proxy of their own.

The event log's `upstream` field names the company proxy, by host and port, for each tunnel and request that went through it, and `devvm network log` adds `through host:port` to the line. The proxy's own log names both company proxies by host and port when it starts. Neither holds the user or the password.

### How long a proxy runs

The proxy exits when the VM's Lima hostagent does, which it learns from the hostagent's pid file, the VM's lifeline (`lifeline` in its config). The proxy starts before the VM, so it waits for the lifeline to name a process that runs. What the file held when the proxy started is an earlier start's, and the proxy waits past it, but when a new start writes the file and its process is already gone, the proxy exits. Once it follows a start, it runs while the file holds what it held then and that process runs, so a VM started again gets a new proxy. If the hostagent never appears, the proxy exits 15 minutes after it started, and `devvm stop` stops it at once.

A VM that starts again gets a new proxy on the same port. A running VM also gets a new proxy when the config devvm would start it with differs from the one it started with, which devvm tells by the SHA-256 of each (`proxy.Config.Sum`). The config holds everything that, changed, starts the proxy again: the VM's list of policies, the VM's CA and the next one, by the SHA-256 of each certificate, so a CA that was replaced counts, the SHA-256 of devvm's binary, so a proxy from another devvm binary counts, the [CAs in your CA folder](#a-companys-ca), the [company proxy](#through-a-company-proxy) settings in devvm's environment and the [shared Nix cache](#the-shared-nix-cache)'s cap. A config that's gone, or that the proxy wouldn't read, counts as one that differs. To start a new proxy, devvm stops the old one with `SIGTERM`, and with `SIGKILL` when it's still running two seconds later. The old proxy counts as stopped only once its process is gone, not once its arguments can no longer be read, because an ending process loses its memory before its files, and for that moment its port still looks taken by another program, which would fail the launch with `proxy_port_taken`. `SIGKILL` goes only to a process whose arguments devvm can still read as the proxy's, so a proxy still ending gets none, and neither does another process given its pid since, even one whose arguments devvm can't read. If it still runs two seconds after that, devvm stops with `proxy_stop_failed` and starts nothing, because the old proxy would go on with its old settings.

devvm counts a proxy as started only once the proxy says it's listening on the VM's port and is still running, because a program that took the port first would otherwise get the VM's traffic. The proxy says so in a status file of its own in the VM's run folder (`vms/<hash>/run/proxy.status`), which devvm names to it when it starts it and removes before each start. The file holds the proxy's pid and the address it listens on, so a status that an earlier proxy left, or that another program wrote, never counts. While devvm waits, the proxy counts as running as long as its pid file still names it, it hasn't ended, and its arguments are the VM's proxy's, `_proxy -config` with the VM's `proxy.json`, or can't be read yet. On Linux, a process that has just started shows no arguments for a moment, because the kernel lets devvm go on as the new program replaces devvm's copy and shows the program's arguments only once it has loaded. If the proxy doesn't start, devvm stops and names the proxy's log (`proxy_start_failed`). When a stopped VM's port was taken by another program, devvm chooses a new free port. A running VM's backend holds the port it started with, so when a running VM's proxy is gone and another program took its port, devvm can't start the proxy again (`proxy_port_taken`). devvm then stops the VM (`net_proxy_down_stopped`), and it does the same when the proxy fails to start for any other reason, because a VM with network policy never runs on without its proxy. The next `devvm` starts both. An interrupt during the start of a VM with network policy stops the VM and its proxy too (`interrupted_start`, a cleanup that `Env.start` registers around the backend's `Start`), because a backend can leave the VM running while its start still waits for it: Lima's hostagent keeps running the VM after `limactl start` ends, and devvm records the confined start only once `limactl start` returns. Without the stop, the VM would run with no record of its confined start, so `devvm ls` would show it as `unenforced` and its next launch would offer to restart it.

## Policy files

A VM's proxy reads these policies, in this order, as one list. [Allowlists](../safety/network.md#allowlists) describes what they allow and the rules they hold.

| Policy | Rules for |
| --- | --- |
| `layers/base.policy` (built in) | every VM: OS packages, mise's version lists, GitHub |
| `shared-tools/<name>.policy` (built in) | each shared tool in the VM's image (`node`, `claude`, …): its registries, downloads and APIs |
| `layers/<name>.policy` (built in) | each built-in layer in the VM's image: `docker`'s has Docker's packages and the container registries Docker Hub and ghcr.io, and `nix`'s has Nix's binary cache, channels and flake registry |
| `~/.config/devvm/layers/<name>.policy` | each of your layers in the VM's image |
| `~/.config/devvm/policy` | every VM: your own rules (`devvm network allow`, `deny` and `rm` with `-g`) |
| `~/.local/share/devvm/vms/<hash>/policy` | that VM only (`devvm network allow`, `deny` and `rm`) |
| `~/.local/share/devvm/vms/<hash>/run/secrets.policy` | the hosts of the VM's secrets and logins, on port 443 (devvm writes it) |

The built-in policies are paths in devvm's own files, `assets/` in the repo, which are [built into the binary](install.md#devvms-own-files), and no copy of them is ever on disk. `netpolicy.Policies` lists both kinds for a VM: the built-in ones go in the proxy's config as names in `builtin_policies`, and your files as paths in `policies`. The proxy reads the built-in ones from the binary once, as it starts, ahead of your files in the list, and never again, so only an upgrade of devvm changes them, and an upgrade starts every VM's proxy again because the binary changed. A built-in policy that's missing, or that holds a line that isn't a rule, is a bug in devvm, so the proxy refuses to start rather than run with fewer rules than devvm meant, and devvm stops the VM as for any proxy that doesn't start. A missing file of yours, on the other hand, has no rules. `devvm network` reads the built-in policies from the binary too, and it labels each with its name (`# devvm: layers/base.policy`, and `"builtin"` with `--json`) where it shows your files' paths. Both lists are in the config that a proxy started with, so a change to either starts the proxy again.

The proxy, `devvm network` and `devvm doctor` read every policy with one parser, `proxy.ParsePolicy`, which returns each line that holds more than a comment, with its number and whether it's a rule. A line is a rule only when `proxy.ValidRule` takes it, which is exactly what `devvm network allow` and `deny` take. Each time the proxy reads a file, it logs each line that isn't a rule, with its file, number and text, quoted, to its own log, `proxy.log` in the VM's `logs/` folder. It skips such a line, unless the line starts with `!` (`proxy.BrokenDeny`). A broken deny fails closed: while any of the VM's policy files has one, the proxy's policy denies every name and every address, and each request gets `403` and a refused block whose reason names the file and the line, so neither the VM's prompt nor `devvm network log` suggests an allow rule. The VM's links and the proxy's own answers at `devvm.proxy` come before the rules, so they still work, and tunnels already open stay open, as they do for any new deny. Fixing the line lifts it at the proxy's next read of the file, which is the next connection. The proxy reads a file again when its mtime, size or inode changes, or when it comes back after it was missing, whatever its mtime. It also reads a file again at every connection while its mtime is less than 2 seconds old, because some filesystems keep mtimes only to the second or two, so a second write within that time could leave the stamp as it was. An error other than a missing file keeps its last rules and its broken deny, which the proxy's log says once. The proxy, `devvm network` and `devvm doctor` read a policy file only when it's a regular file, through a symbolic link if it's one, of at most 1 MiB (`proxy.ReadRegular`). They open it without blocking and check the file they opened, so a FIFO or a link to `/dev/zero` in a policy file's place can't hang them, and each refuses it with a named error, `proxy.ErrNotRegular` or `proxy.ErrTooLong`. A leading byte-order mark isn't part of the first line. `netpolicy.PolicyWarnings` warns about each line that isn't a rule, with `policy_broken_deny` for a broken deny and `policy_not_rule` for the others. `netpolicy.Proxy.Prepare` uses it for the VM's policy files at each launch with network policy, `devvm network allow`, `deny` and `rm` for the file they change, and `devvm doctor` for the files you edit: `~/.config/devvm/policy`, your layers' and each VM's own. `devvm network` also marks the line, and `--json` lists it in `not_rules`. A unit test parses every policy file that devvm ships and fails on any line that isn't a rule.

Each change that `devvm network allow`, `deny` or `rm` makes rewrites the file whole, through a link if the file is one, so a proxy never reads half of it.

## The VM's setup

On every launch, the VM's setup does the following:

- It installs the VM's own CA and the next one into the VM's trust store, as `devvm-proxy.crt` and `devvm-proxy-next.crt`, so the proxy's certificates for hosts with a bound secret verify, in this VM only. It never gets their keys.
- It points the programs that ignore the proxy variables at the proxy. Those are apt, sudo (which keeps the variables), every ssh connection, Docker's daemon and containers with the [docker layer](../using/images.md#docker), and Nix's daemon with the [nix layer](images.md#the-nix-layer). Under systemd, each daemon gets a drop-in, `devvm-proxy.conf`, that holds the VM's own proxy address and nothing else, and without systemd, Docker's settings in `/etc/conf.d/docker` get a block instead, and Nix's daemon gets `/etc/conf.d/nix-daemon`. When network policy is off, the setup removes them, and it restarts a daemon whose settings changed. An ssh connection gets a `ProxyCommand` that tunnels through the proxy, and the proxy checks its host and port against the allowlist.
- With the nix layer, it points Nix at the [shared Nix cache](#the-shared-nix-cache) first: `/etc/nix/devvm-cache.conf`, which `/etc/nix/nix.conf` includes with `!include`, sets `substituters = http://devvm.proxy/nix https://cache.nixos.org`. When network policy is off, the setup removes that file, so Nix goes to cache.nixos.org alone, and it restarts Nix's daemon when the file changed.
- It sets `NO_PROXY` and `no_proxy` to the VM itself, `localhost,127.0.0.1,::1`, in the daemons' settings above and in Docker's client settings (`~/.docker/config.json`), as the VM's backend does in the environment it sets at each start. The VM's [links](ports.md#links-between-vms) aren't in it, because HTTP clients reach them [through the proxy](#links-through-the-proxy), so a change of links changes no daemon's settings and restarts nothing.
- It puts the VM's placeholders in `~/.config/devvm/env`, which login shells read, as `CLAUDE_CODE_OAUTH_TOKEN` and `GH_TOKEN`. When `github` is bound, it also sets up git's credential helper for GitHub over HTTPS.
- It adds `devvm-why`, the note before each prompt, and the notes for Claude Code and Codex ([inside the VM](../safety/network.md#inside-the-vm)).

Each placeholder ends in a random value of the VM's own, its salt, which isn't its password.

## The shared Nix cache

VMs with network policy and the [nix layer](images.md#the-nix-layer) share their downloads from Nix's binary cache, so each package comes from cache.nixos.org once per machine instead of once per VM. Each VM keeps its own Nix store and daemon, and only the host writes the cache, so no VM can change what another VM gets. The [design note](design.md#the-shared-nix-cache) says why it's a cache that the host fills rather than a shared store.

**In the VM.** devvm starts the VM's proxy with `nix_cache` set to `~/.local/share/devvm/cache/nix-cache` in its config when the VM's image has the nix layer, and the VM's setup puts `http://devvm.proxy/nix` before `https://cache.nixos.org` in Nix's `substituters` ([the VM's setup](#the-vms-setup)). It leaves `trusted-public-keys` as Nix has it, which is cache.nixos.org's key alone. Nix's daemon reaches `devvm.proxy` through its own proxy settings, so its requests are absolute-form `GET`s with the VM's password, and the proxy answers them itself, as it does `/blocked` and `/ca.pem`, and never forwards them. When the cache answers with anything but a file, Nix goes on to cache.nixos.org, through the proxy as any other download.

**What a VM can ask for.** The proxy takes only `GET` and `HEAD` requests with no query, for three kinds of path under `/nix/`. These are `nix-cache-info`, a narinfo named `<hash>.narinfo` with a 32-character hash in Nix's base 32, and a NAR named `nar/<hash>.nar` with a 52-character hash and no suffix or `.xz`, `.zst` or `.bz2`. Anything else gets `400`. The proxy makes up `nix-cache-info` itself, with priority 30, so Nix asks the cache before cache.nixos.org, whose priority is 40. It serves the other files from the folder, or fetches them from `https://cache.nixos.org` when the VM's policy allows `cache.nixos.org:443`. That's the only host it fetches from, and its fetches go through the same address floor and company proxy as the VM's own requests. It follows no redirect, and nothing of the VM's request goes with it, so nothing a VM sends is ever stored.

**What the host checks.** Before it keeps a narinfo, the proxy checks that its `StorePath` is the path the VM asked for, and that `cache.nixos.org-1`, whose Ed25519 key devvm pins, signed its fingerprint, `1;StorePath;NarHash;NarSize;References`. The signature doesn't cover the NAR's URL, so the proxy also checks that the URL is named by the narinfo's own `NarHash` or `FileHash`, and no narinfo can put a file in another NAR's place. It fetches a NAR only after it checked a narinfo that names it, never for a `HEAD`, and it keeps the NAR only when its SHA-256 and size are that narinfo's `FileHash` and `FileSize`. The host can't check a NAR's `NarHash`, which is the hash of the archive before compression, because the proxy uses Go's standard library alone, and that has no xz or zstd. Each VM's Nix checks the signature and the `NarHash` again, so a narinfo with an added reference, or a corrupted NAR, fails there too. A file that fails a check, a not-found and an error all get `404`, and the proxy keeps nothing of them, so Nix goes to cache.nixos.org itself.

**The folder and its cap.** The proxy writes each file whole into a temporary file in the folder, then renames it into place, mode 600 in folders of 700. The cache's cap is the `nix-cache.size` [retention setting](../using/settings.md#retention), 10 GiB when it's off, which devvm puts in the proxy's config as `nix_cache_max` when the proxy starts. The cap counts the disk each file takes, in whole blocks, and a NAR being fetched counts at its full size from the start. When a new file would take the cache past its cap, the proxy deletes the least recently used files, down to nine tenths of the cap, and when the NARs being fetched leave no room, it refuses the fetch. One proxy fetches each NAR once, however many requests ask for it, and at most four at once. A hit marks a file used by setting its modification time. Every VM's proxy and devvm gc take one lock on the folder before they change it, so the cap holds while many VMs fill it, and a running total in the folder saves counting it for each file. devvm gc and the daily retention run trim the cache to its cap too, and `devvm gc --nix-cache` empties it.

**Logs.** Each request is a `nix-cache` event in the VM's own [log](#log-files), with its path and what came of it: `hit`, `fetched`, or why it wasn't found or was refused. `devvm network log` shows these as `cache`.

**VMs without network policy** have no proxy, so they get no shared cache. Their Nix goes to cache.nixos.org directly, and nothing runs or listens on the host for them.

**What stays open.** A VM can tell by timing whether some VM fetched a public path, and it can fill the cache up to its cap, which pushes other VMs' files out, so they fetch those again. Private caches, such as Cachix or one behind a `netrc`, are never shared, because only root in a VM can add a substituter, and Nix reaches it directly through the proxy's allowlist.

## A company's CA

The CA certificates in `~/.config/devvm/ca` ([a company's CA](../using/settings.md#a-companys-ca)) reach every project VM, with network policy or without, and the VM's proxy. Builds trust them too, as [Images and caching](images.md#a-companys-ca-in-builds) describes.

**Reading them.** `trust.Read` reads the folder's `*.pem` and `*.crt` files at each launch, each build and each proxy start, and nothing outside the folder. It checks the folder and each of those files with `os.Lstat` and follows no link: a folder that is a link fails with `ca_folder_link`, and a file that is one, dangling or not, with `ca_link`. A link into a project folder would otherwise let that project's VM write the CA that every VM, build and proxy trusts. A file or folder that it can't read fails with `ca_unreadable` or `ca_folder_unreadable`, which name it and carry the error. It decodes every PEM block and refuses a block that isn't `CERTIFICATE` (`ca_not_certificate`), a certificate that `crypto/x509` can't parse (`ca_invalid`) and a certificate without CA basic constraints (`ca_not_ca`). It also refuses a file with no certificate in it. It tells a private key apart by its PEM type, not by the file's name, and a refusal fails the whole launch or build. What leaves the host is each certificate encoded again from its parsed bytes, once each, so the text around a block never does.

**Installing them.** `assets/guest/ca.sh DIR` runs as root and makes devvm's CAs in `/usr/local/share/ca-certificates` exactly DIR's `devvm-*.crt` files: the VM's own CA and its next one, `devvm-proxy.crt` and `devvm-proxy-next.crt`, and the user's `devvm-extra-N.crt`. When anything changed, it runs `update-ca-certificates --fresh`, so a CA that went leaves neither the bundle nor its hashed link. `update-ca-certificates` tells the hooks in `/etc/ca-certificates/update.d` only about the CAs it links, never about a file that was deleted, so `ca.sh` tells them itself. It runs each hook that `run-parts` would run, with a `-/etc/ssl/certs/NAME.pem` line on its input for each CA it removed, as Debian's `update-ca-certificates` does for a CA it deselects. That way Java's `cacerts` (ca-certificates-java's `jks-keystore` hook) and Mono's store drop a removed CA, in a project VM and in an image whose layer installed Java. A hook that rebuilds its store from the bundle ignores that input. A hook that fails stops nothing, and `ca.sh` says so in the run log. It also writes them all to `/etc/ssl/devvm.pem`, for Node. In a system without `update-ca-certificates`, such as a container image before its `ca-certificates` package, it adds them to `/etc/ssl/certs/ca-certificates.crt` itself. It keeps the bundle as it was in `/var/lib/devvm-ca`, or a mark that there was none, and puts it back once they go, and a package that arrives in between makes the bundle again from the same folder. Run without DIR, `ca.sh` removes every one of devvm's files. Its unit tests run it in a folder that stands for `/` (`DEVVM_CA_ROOT`).

**Project VMs.** The setup that devvm sends at each launch carries `ca.sh` and the certificates in `ca/`, and `sync.sh` runs it, so a change applies at the next launch and a running VM needs no restart. Every project VM starts with `trust.Env` in its environment, whatever its network policy and whether there are CAs or not, so adding a CA changes nothing that its backend sets at a start. On Lima, the clone that makes the VM sets the environment too (`VMSpec.Env`), so a start without network policy needs no `limactl edit` and the VM keeps its host keys. The variables are `NODE_EXTRA_CA_CERTS=/etc/ssl/devvm.pem`, which is empty when there are none (Node warns only about a missing file), `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE` and `PIP_CERT`, set to the system's bundle, and `UV_SYSTEM_CERTS=1`.

**The proxy.** `netpolicy.Proxy.Prepare` reads the folder too, and writes the bundle into the proxy's config, `proxy.json` in the VM's run folder, as `upstream_cas`, which the proxy adds to the system's roots for its own TLS connections. A VM can't change that file or the folder, so the proxy's roots grow only by what you put there. A proxy whose bundle differs from the folder's starts again, as one whose CA was replaced does.

**End-to-end checks** (`tests/e2e.sh`): in `project`, a VM trusts a test CA from its next launch (`openssl verify` against the system's bundle, and curl reaching an HTTPS server on the host that the CA signed), has the variables, fails its launch on a file that isn't a certificate, and trusts the CA no more once the folder is emptied; in `tools`, Node fetches from that server.

## Secrets

[Secrets](../safety/network.md#secrets) lists each secret with its hosts and header. This section describes where devvm keeps them and how the proxy puts them into requests.

### Where secrets are saved

On macOS, devvm saves a secret in the Keychain, under the service `devvm` and the account `claude` or `github`. It saves it through `security -i`, giving the value on its input, so the value is in no process's arguments. `DEVVM_SECRET_SERVICE` names another service, made of letters, digits, `.`, `_` and `-` only. On other systems, devvm saves a secret in `~/.local/share/devvm/secrets/<name>`, a file only you can read.

### How the proxy puts them in

A secret's hosts join the VM's allowlist on port 443, whether a value is saved or not.

For each secret that has a value, devvm writes the VM's bindings to `secrets.json` in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/`, as one JSON object of `"version": 1`, whole and mode 600. A binding holds the secret's name, its kind, its hosts, header and placeholder, and never its value. The kind is `secret` for one that devvm keeps and `login` for one of an agent's logins, with the agent whose login file holds it, so where a value comes from never depends on its name. The proxy reads the file only when it's a regular file, not a symbolic link, that only you can read or write, and a file it can't read that way has no bindings until devvm writes it again. For a bound host, the proxy intercepts `:443` tunnels instead of passing them through. It terminates TLS with a certificate from [the VM's own CA](#the-vms-ca). Each certificate it makes for a host lasts a day, but never past the CA's own end or before its start, because a client checks the whole chain. It then replaces the placeholder in the binding's header with the real value, also inside `Basic` credentials, and forwards the request over a new TLS connection. Absolute-form `https://` requests get the same. Each request must name the bound host, and one that names another host gets `421`. The proxy checks the bindings file for every request and reads it again when it has changed, so removing a binding applies to tunnels that are already open.

The proxy is devvm itself, so it looks a value up in its own process, from the Keychain, devvm's secrets folder or the agent's login file, by the binding's kind. It runs no other devvm for it, no devvm command prints a secret, and no value is in any process's arguments. On macOS it asks the Keychain through `security find-generic-password`, whose output is the value. A lookup takes at most 20 seconds, and the proxy makes one at a time, so two requests never refresh a login at once. The proxy caches values for 5 minutes, or 30 seconds for agents' logins, and never logs them. A lookup that fails puts nothing in, and the proxy's own log says why, without the value, such as `codex_refresh_failed` when refreshing a Codex login failed. It refuses with `403` the GitHub API calls that would make a credential of their own with the GitHub token, which are adding an account or deploy key, registering a runner and adding a webhook, whether the repo or organization is named or given by its ID. Every HTTPS request it forwards is checked, inside an intercepted tunnel and in absolute form alike, before it goes anywhere.

### The VM's CA

Each VM with network policy has a CA of its own, which only its proxy signs with and only that VM trusts, so a copy of one VM's key can impersonate sites to that VM alone, and no VM's proxy can make a certificate that another VM accepts. A CA is ECDSA P-256, valid for 30 days from an hour before it's made, and it names no host. Its common name holds its serial number, so no two CAs share a name. devvm keeps the VM's CAs in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/ca/`: the current CA as `ca.pem` and its key `ca.key`, and the next CA as `next.pem` and its key `next.key`. The folder and both keys are yours alone (modes 700 and 600), devvm writes each file whole, a key before its certificate, and on macOS it keeps the folder out of Time Machine. The keys never leave this machine: the VM gets the two certificates and nothing else. The proxy reads each certificate and key only when it's a regular file, not a symbolic link, and refuses a key that anybody but you may read or write.

At each launch with network policy, `trust.InitCA` readies both CAs before the proxy starts, holding the folder's lock, `lock`, so two launches of one VM at once take turns. The current CA stays while it has over a week left. When it has less, or there's none, the next CA takes its place if it has started and has over a week left, and otherwise devvm makes a new current CA. The next CA starts a week and an hour before the current one ends, and lasts 30 days too. When it doesn't follow the current one that way, by ending after it and starting by the time it's replaced, devvm makes it again. A key without its certificate, or a CA that lasts longer than 30 days, is made again too. Each launch's setup installs both certificates in the VM, so the VM trusts the next CA weeks before the proxy uses it.

The proxy holds both CAs, and it signs with the next one from an hour after that CA's start, which is a week before the current one ends and the moment from which the next launch would make it the current one. The hour lets a VM whose clock is behind see that the CA has started. The switch needs no launch and no new proxy, because the VM already trusts the next CA, so a VM that you don't open again keeps verifying the proxy's certificates until the next CA ends, which is at least 30 days less an hour after its last launch. A proxy keeps the CAs it started with, because the VM trusts no others, and a launch that replaced either one starts the proxy again with the new ones, as [how long a proxy runs](#how-long-a-proxy-runs) describes. `/ca.pem` at `devvm.proxy` serves both certificates.

A VM that runs past its next CA's end without a launch can't verify the proxy's certificates any more. From then on, its proxy answers each `CONNECT` to a bound host's `:443` with `403` before any tunnel opens, and it logs a refused block with the reason `devvm's CA has ended: open the VM again (devvm in its folder) for a new one`, so neither the VM's prompt nor `devvm network log` suggests an allow rule. Absolute-form `https://` requests involve no certificate for the VM, so they go on getting their secrets. The VM's other connections work until you open it again. Running `devvm` in the VM's folder then makes a new CA and a next one and starts the proxy again with them, which ends the connections open through the old one, and the launch's setup installs both in the VM.

The CAs live as long as the VM's folder, or as its network policy. A remake keeps the folder, so the VM keeps its CAs. `devvm rm`, `devvm gc --orphans`, and devvm deleting a VM whose creation it interrupted or whose folder an earlier VM of the project left, delete the folder, and the CAs and their keys with it. A VM whose network policy is turned off keeps its CAs only until its next start, which stops its proxy and then deletes the CA folder, keys and all, under the folder's lock, and its setup removes both certificates from the VM. When its proxy doesn't stop, the folder stays, because that proxy may still sign with the keys, and a later start deletes it. Turned on again, the VM gets new CAs at its next launch.

**Unit tests:** `TestInitCA`, `TestInitCASwitch` and `TestInitCAReplaced` (`internal/trust`) check the dates, the modes, the switch and each CA that's made again; `TestInitCATogether` and `TestInitCACancelled` check that launches at once take turns on the lock; `TestUnconfined` (`internal/netpolicy`) checks that a start without network policy deletes the CAs only once their proxy has stopped; `TestNextCASigns` checks that a client trusting both CAs verifies the proxy's certificates before the switch and after it, with the same proxy; `TestLoadCA` checks that the proxy refuses a key others may read and a symbolic link; `TestServersApart` checks that two proxies' certificates come from their own CAs. **CLI tests** (`network.txtar`, `sync.txtar`, `layout.txtar`): each VM's CA files and modes, two VMs' CAs differ, the setup sends both certificates and no key, and a VM turned off loses its CAs at its next start and gets new ones once it's on again. **End-to-end checks** (`tests/e2e.sh`, `network`): a second VM with network policy, whose CAs differ from the first's; each VM trusts its own two and none of the other's; no key is anywhere in the VM; each proxy serves its own CAs at `/ca.pem`, and its certificates verify with its own VM's CAs and not with the other's; `devvm rm` deletes a VM's CA with its folder.

## Agents' logins and settings

Agents' login files also stay on the host. When a VM's image has the agent and the host has a login file for it, the login's bindings join the VM's, and their hosts join its allowlist on port 443.

Codex's login is `~/.codex/auth.json` and opencode's is `~/.local/share/opencode/auth.json`. The VM's copy holds placeholders, shaped like a JWT where the tools read claims, and the bindings put the host's API key or access token into requests for the provider's hosts:

- For Codex, those are OpenAI's hosts, `api.openai.com`, and `chatgpt.com` for a ChatGPT login.
- For opencode, they're the hosts of Anthropic, OpenAI and OpenRouter keys, and of OpenAI and GitHub Copilot logins, but not Copilot Enterprise.

Each VM's placeholders end in its own random value, so no other VM's placeholder gets its secrets. devvm reads a login file only when it's a regular file, so a FIFO or a device in its place can't hold devvm. devvm refreshes OAuth logins on the host, one lookup at a time, and writes them back to the host's file, so a VM never holds a refresh token. A refresh that has started finishes and is written back even when the request that needed it ends, because OpenAI may already have replaced the refresh token. When a refresh fails, or the host's login no longer has what a binding names, the proxy puts nothing in, and its own log says to log in again with the agent on this machine (`codex_refresh_failed`, `opencode_refresh_failed`, `codex_no_key`, `codex_no_chatgpt`), or names the login file it couldn't read (`login_read_failed`). devvm copies logins it doesn't know as they are, and warns you (`codex_login_kept`, `opencode_login_kept`). If devvm can't make the VM's copy of a login, it stops the VM's setup (`login_copy_failed`).

[Coding agents](../using/coding-agents.md) lists which files each agent shares.

### MCP servers

On each launch, devvm merges this machine's settings and MCP servers from `~/.claude.json` into the VM's own `~/.claude.json`:

- Each of your servers replaces the VM's server of that name whole.
- devvm removes the servers it copied before that you no longer have.
- Servers added in the VM stay.

The copy holds only your settings, onboarding and account state, and MCP servers. The VM keeps its own per-project entries, such as trust and allowed tools, and yours never go in.

With network policy, an HTTPS server's headers become bindings for its host, which must be a DNS name on port 443. devvm replaces these with `devvm-redacted`: env values, plain-HTTP servers' headers, user info and secret-looking values in URLs, and secret-named or key-shaped arguments. Without network policy, devvm copies the servers' env values and headers as they are, and names the servers that have them (`claude_mcp_secrets_copied`).

### Keys in settings files

With network policy, devvm also takes the keys out of the VM's copies of the agents' settings files, which are `~/.claude/settings.json`, `~/.codex/config.toml` and opencode's `.json`, `.jsonc` and `.toml` files. Those are JSON, JSONC or TOML, and devvm parses each file whole and writes the VM's copy from what it parsed, so a JSONC file's copy is JSON and a TOML file's copy has no comments. devvm replaces these with `devvm-redacted`, and names what it replaced (`config_redacted`):

- Every value under a key that looks like a secret's (`*key*`, `*token*`, `*secret*`, `*password*`, `Authorization`), and every string in a list or a table under such a key, but key bindings, such as opencode's `keybinds`.
- Any string that starts like a known key or token, such as `sk-`, `ghp_` or `eyJ`, wherever it is.
- Every value in an env or headers table, whatever its name: `env`, `environment`, `headers`, `http_headers`, and Codex's `shell_environment_policy.set`. A number of up to five digits, `true` and `false` stay, because they're switches and ports. In Claude Code's own `env`, the top-level one of `settings.json`, its [configuration variables](https://code.claude.com/docs/en/env-vars) stay too: `ANTHROPIC_MODEL`, `ANTHROPIC_SMALL_FAST_MODEL`, the `ANTHROPIC_DEFAULT_*_MODEL` and `ANTHROPIC_CUSTOM_MODEL_OPTION` families, `CLAUDE_CODE_*`, `DISABLE_*`, `MAX_*` and `*_TIMEOUT_MS`. A variable whose name has `KEY`, `TOKEN` as a word, `SECRET`, `PASS`, `AUTH`, `CREDENTIAL`, `COOKIE` or `HEADER` in it is redacted all the same, such as `CLAUDE_CODE_OAUTH_TOKEN`, and so is its value when it's a URL.
- In a URL anywhere in the file, its user info and its secret-looking query values and path segments.
- In every list, the arguments that MCP servers' arguments in `~/.claude.json` lose: the value after a secret-named flag, such as `--api-key X`, the value of `--token=X`, and any that looks like a key.

Values that only name where a secret is stay as they are. Those are an environment variable's name (`env_key = "OPENAI_API_KEY"`), `{env:X}`, `${X}` and a path. devvm refuses a file that doesn't parse as its format, instead of copying any of it unread, so the VM doesn't get that file (`config_not_copied`). An MCP server or a provider that needs one of those keys in the VM has to get it another way.

## The self-check

Network policy holds only while the VM runs on the process that devvm started confined. A process can't leave its sandbox, but `limactl start` starts a new Lima process, unconfined. So on every launch, and whenever an export or `devvm rm`'s check of a `--clone` VM needs the VM running, whether devvm started the VM or found it running, devvm checks two things, from the host:

1. That the VM still runs on the process devvm started. The VM can't fake this check.
2. That a direct connection from the VM to the internet fails. The VM answers this check, so it catches only the honest case.

devvm records the result in the VM's state, as `ok` or `unenforced` in the `net` section's `made.check`. When the check fails, what devvm does depends on why:

- **The VM wasn't started confined,** because it was started outside devvm or has been running since before `devvm network on`. In a terminal, devvm offers to restart the VM confined (`net_unenforced_restart`), and if you decline, it warns you and goes on (`net_unenforced`). Without a terminal, or if the VM is still unenforced after the restart, devvm stops the VM and ends without opening a shell (`net_unenforced_stop`). The next `devvm` starts the VM confined.
- **The VM was started confined, yet it gets out,** because it reaches the internet through something devvm didn't set up, such as a second network. A restart wouldn't help, so devvm stops the VM and ends (`net_direct_route`).

This is the check, and what devvm does when it fails:

```mermaid
flowchart TD
  launch["every launch"] --> proc["on the process<br>devvm started?"]
  proc -- "yes" --> direct["a direct connection<br>from the VM fails?"]
  direct -- "yes" --> ok["check: ok"]
  proc -- "no" --> un["check: unenforced"]
  direct -- "no" --> un
  un --> confined["started confined?"]
  confined -- "yes: another<br>way out" --> route["stops the VM<br>and ends"]
  confined -- "no" --> term["a terminal?"]
  term -- "no" --> stop["stops the VM, no shell;<br>the next devvm starts<br>it confined"]
  term -- "yes" --> offer["restart it confined?"]
  offer -- "no" --> warn["warns, goes on"]
  offer -- "yes" --> again["enforced now?"]
  again -- "yes" --> ok2["check: ok"]
  again -- "no" --> stop
```

`devvm ls` and `devvm network` show such a VM's network policy as `unenforced`. That's a VM whose last self-check failed, or a running VM that isn't on the process devvm started confined, which devvm checks on the host each time.

On Lima, "the process devvm started" is the VM's Lima process, which devvm starts [confined](lima.md#confinement).

## Log files

The proxy writes every decision (`tunnel`, `intercept`, `forward`, `blocked`, or `nix-cache` for a request to the [shared Nix cache](#the-shared-nix-cache)) as a JSON line in `network.jsonl` in the VM's logs folder, `~/.local/share/devvm/vms/<hash>/logs/`. A line holds the time, the VM, the host and port, the method and path when the proxy sees them, the rule or the reason, and the names of the secrets it put in, never their values.

The proxy's own lines, such as what it loaded and what failed upstream, go to `proxy.log` next to it. An upstream error stays there, because it can name addresses that the VM shouldn't learn. Only you can read the VM's logs folder and its run folder, and devvm's run logs never hold a VM's password. The VM chose the hosts and paths, so `devvm network log` drops control characters in them.

The event log also holds the VM's [links](ports.md#links-between-vms)' connections, which `devvm _links` writes in the same format: `link` for a connection it carried, and `link_refused`, `link_target_stopped` or `link_limit` for one it refused, with the link's name as the host and the target VM in `target`. A VM without network policy has an event log for those alone.

The proxy is the only program that writes `proxy.log`, and the event log has `devvm _links` as its only other writer. The proxy rotates each log before a line would take it past 10 MiB, and `devvm _links` writes the event log with the proxy's own code, so it rotates the log by the same rule and a VM that runs for months keeps at most about 20 MiB of each. A line never spans two files, and lines from the proxy's connections at the same time are written one after another, so a rotation loses none and splits none. Before each line, each writer checks that the file it has open is still `network.jsonl`, and when the other writer has rotated it, the writer opens the new `network.jsonl` and writes there, so its lines go on reaching the file that `devvm network log --follow` follows. Each writer takes the log's size from the file, so it counts the other writer's lines too. A writer rotates the log while it holds a lock on the file and checks the name again first, so when both reach the limit at once, only one renames the file and the other writes to the new one. A rotation renames `network.jsonl` to `network.jsonl.1`, which replaces the one that was there, and starts a new `network.jsonl`. `proxy.log` is also the proxy process's output, where its last error goes when it stops, so the proxy copies it to `proxy.log.1` and empties it instead, and that error still reaches the current file. Every log file and its `.1` file is readable only by you (mode 600).

`devvm network log` reads `network.jsonl.1` before `network.jsonl`, so the history it shows and the blocks that `devvm network log --blocked` counts carry on across a rotation. `devvm network log --follow` reads the renamed file to its end and then follows the new one from its start. `devvm rm` deletes the `.1` files with the rest of the VM's logs.

## devvm _proxy

devvm runs the proxy as itself, `devvm _proxy -config FILE`, and nothing else is in its arguments. Its flags take one dash or two, and its errors end in one line, `network policy proxy: …`.

The config is one JSON file, `proxy.Config`, which a VM's proxy and a build's proxy both run with. For a VM it's `proxy.json` in the VM's run folder, `~/.local/share/devvm/vms/<hash>/run/`, and for a build it's `proxy.json` in the build proxy's folder. devvm writes it whole, mode 600, before each start. It holds the VM's password and the company proxy's credentials from the environment, though never `config.toml`'s password, which the proxy looks up itself, so it's never in a process's arguments, which other users can see. The proxy reads it once, as it starts, and refuses to start unless it's a regular file, not a symbolic link, owned by the user the proxy runs as and readable and writable by nobody else, at most 1 MiB, of `"version": 1`, with no field the proxy doesn't know. It also refuses a config without a password, one whose `listen` isn't a loopback address and port, one with `ca` and no `ca_key` or the other way around, and one with a `nix_cache_max` below 0. A config of another version is never read, so a proxy and the devvm that started it always agree on what each field means.

| Field | What |
| --- | --- |
| `version` | `1` |
| `listen` | the loopback address and port to listen on, port 0 for any free one |
| `vm` | the VM's name, for the log |
| `password` | the VM's password, which every request carries in `Proxy-Authorization` with the user `devvm`; required |
| `builtin_policies` | the built-in policies, paths in devvm's own files, read from the binary once as the proxy starts, ahead of `policies`; a missing one refuses to start |
| `policies` | your policy files, in order, each read again when it changes; a missing one has no rules |
| `log` | the JSON-lines event log (none: standard output), [rotated](#log-files) past 10 MiB |
| `proxy_log` | the file that the proxy's output goes to, which is its own log, rotated past 10 MiB by copying |
| `lifeline` | the hostagent's pid file: exit once it no longer holds what it held when the proxy first saw its process running, or that process is gone |
| `ca`, `ca_key` | the [VM's own CA](#the-vms-ca), for interception: its certificate and key |
| `ca_next`, `ca_next_key` | with `ca`: the VM's next CA, which signs from an hour after its start |
| `secrets` | the VM's bindings file, read again when it changes; the proxy looks each binding's value up in its own process |
| `upstream_cas` | the [CAs in your CA folder](#a-companys-ca) (PEM), which the proxy's own TLS connections trust besides the system's |
| `upstream` | the [company proxy](#through-a-company-proxy): `https`, `http` and `no_proxy`, as `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` or `config.toml`'s `[proxy]` had them, and `secret`, the secret the proxy looks up as the password of a URL that names a user and none of its own; allowed hosts are reached through it |
| `upstream_unresolved` | with `upstream`: names this machine can't resolve go there even when only `**` allows them, which builds' proxies get |
| `nix_cache` | serve the [shared Nix cache](#the-shared-nix-cache) at `http://devvm.proxy/nix/` from this folder, which it makes, only yours (mode 700) |
| `nix_cache_max` | with `nix_cache`: the cache's cap, in bytes (none: 10 GiB) |
| `links` | the VM's links' list, which `devvm _links` writes: reach each link through its socket, before any rule ([links through the proxy](#links-through-the-proxy)); a missing list has no links |
| `link_sockets` | with `links`: the folder of the links' sockets, devvm's run folder; without it, no link is reached |
| `devvm_sha256`, `ca_sha256`, `ca_next_sha256` | the SHA-256 of devvm's binary, of the VM's CA certificate and of its next CA's that the proxy started with; the proxy doesn't read them, and they're there so that a new binary or a new CA makes another config ([how long a proxy runs](#how-long-a-proxy-runs)) |

`-config` is the proxy's only flag, and running the proxy is its only mode. What only this machine does for network policy runs inside devvm itself, never through the proxy's arguments. devvm makes each VM's CAs with `trust.InitCA`, and `internal/agentlogin` makes the VM's copies of agents' logins and settings files, their bindings and their values.
