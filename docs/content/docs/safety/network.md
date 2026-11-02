---
title: Network policy
weight: 1
---

A VM with network policy reaches only the hosts on its allowlist, through a proxy that devvm runs on this machine, and it never holds your secrets. A secret is a credential devvm keeps on this machine for VMs to use: your GitHub token, Claude Code's token, an agent's login or an API key. New VMs get network policy by default. Without it, a VM reaches anything this machine can, including this machine's own services on `localhost`.

```mermaid
sequenceDiagram
  participant P as The VM
  participant X as Its proxy,<br>on this machine
  participant H as api.github.com
  P->>X: GET /user, with<br>a placeholder
  Note over X: on the allowlist
  X->>H: GET /user, with<br>your GitHub token
  H-->>X: the response
  X-->>P: the response
  P->>X: CONNECT<br>example.com:443
  X-->>P: 403: not on<br>the allowlist
```

This page covers what you do with network policy and what you see. [How it works › Network policy](../internals/network.md) explains how devvm confines a VM to its proxy and how the proxy handles each connection.

## Turning it on or off

A new VM gets network policy unless you turn it off. When devvm creates a VM in a terminal, it shows what each choice means and asks whether to turn network policy on, with yes as the default:

```text
Network policy for devvm-some-project-71a99b1076d92c:
  on:  it reaches only the hosts you allow, and your secrets stay on this machine (recommended)
  off: it reaches anything this machine can
https://devvm.dev/docs/safety/network/#turning-it-on-or-off
Turn it on? [Y/n]
```

`--network-policy` and `--no-network-policy` answer the question up front, and when there's no terminal to ask in, network policy is on. A VM keeps its choice. On an existing VM devvm ignores both flags and warns you.

To answer the question for every new VM, save `on` or `off` with `devvm network -g on` or `devvm network -g off`, or choose it in [devvm setup](../using/settings.md#devvm-setup), and devvm makes each new VM that way without asking. `devvm network -g ask` brings the question back, and `devvm network -g` shows what new VMs get: `ask`, which is the default, `on` or `off`. devvm reads the saved answer only when it creates a VM, so the VMs you already have keep their own network policy, and the two flags still choose for one new VM. devvm keeps the answer in `~/.config/devvm/network`, and when that file holds anything else, devvm names the file and refuses to create a VM without one of the flags.

Some VMs can't have network policy. Those are VMs on a backend that can't confine a VM to its proxy, such as a [Docker](../internals/docker.md#network-policy) engine on another machine, and Docker VMs with the [docker layer](../backends/docker.md#the-docker-layer). devvm doesn't ask about those, and it makes the VM without network policy and warns you. If you pass `--network-policy` for such a VM, devvm refuses before it builds anything, because it never starts a VM you asked network policy for with an open network. For the same reason, it refuses `devvm network on` for a VM on a backend that can't confine it to its proxy.

| | |
| --- | --- |
| `devvm network` | print this folder's VM's network policy as `devvm ls` shows it, `on`, `off` or `unenforced`, and then its [allowlist](#changing-rules) |
| `devvm network on\|off` | change it from the VM's next start, which `devvm restart` gives you |
| `devvm network -g [on\|off\|ask]` | show or change what new VMs get |

Until the VM starts again, the change waits. If you turn network policy on while the VM is running, the VM isn't confined yet, so `devvm ls` shows it as `unenforced`, and its next launch offers to restart it confined, as it does for a VM started outside devvm. If you turn it off while the VM is running confined, its launches keep it on its proxy with your secrets still out of it, and they warn you that network policy turns off when the VM next starts.

Secrets the VM got while network policy was off still work wherever the VM kept them, so rotate them if that matters.

### When it isn't enforced

On every launch devvm checks on this machine that the VM still runs on the process devvm started confined, and that a direct connection from the VM to the internet fails. A VM that fails either check shows as `unenforced`, and devvm offers to restart it confined or stops it, as [the self-check](../internals/network.md#the-self-check) describes.

## Allowlists

The VM's proxy reads several policy files, in order, as one list:

- devvm's own rules for every VM, which allow OS packages, mise's version lists and GitHub;
- devvm's rules for each shared tool in the VM's image (`node`, `claude` and the others), which allow its registries, downloads and APIs;
- devvm's rules for each [built-in layer](../using/images.md#built-in-layers) in the VM's image, so the `docker` layer allows Docker's packages and the container registries Docker Hub and ghcr.io, and the `nix` layer allows Nix's binary cache, channels and flake registry;
- `~/.config/devvm/layers/<name>.policy`, for each of your own layers in the VM's image;
- `~/.config/devvm/policy`, your own rules for every VM, which `devvm network allow`, `deny` and `rm` change with `-g`;
- `~/.local/share/devvm/vms/<vm>.policy`, the rules for that VM only, which [`devvm network allow`, `deny` and `rm`](#changing-rules) change;
- the hosts of the VM's secrets and logins, on port 443, which devvm adds.

devvm leaves storage and paste sites out of its defaults on purpose, because they're easy ways to send data out. Rules never come from the project folder, because the VM can write it. The proxy reads each file again when it changes, so an edit applies at once, from the next connection. [How it works](../internals/network.md#policy-files) lists where devvm keeps its own files.

### Rules

A policy file holds one rule per line. `#` starts a comment, and matching ignores case.

| Rule | Matches |
| --- | --- |
| `example.com` | that host only |
| `*.example.com` | any subdomain, at any depth, not `example.com` itself |
| `**` | any host |
| `10.0.0.5`, `10.0.0.0/8`, `fd00::/8` | addresses, IPv4 or IPv6 (also what lets a connection past the internal-address refusal) |
| `RULE:22`, `[fd00::1]:22`, `10.0.0.0/8:8080` | only that port; without a port, 443 and 80 |
| `!RULE` | deny: a deny beats any allow |

A rule is plain ASCII with no spaces in it, and a port is a number from 1 to 65535. A host's one trailing dot is dropped, so `example.com.` matches `example.com`. Brackets hold only an IPv6 address, and an address can't follow `*.`. A line that holds anything else, such as `example.com:x`, `a*.example.com` or `! example.com` with a space after the `!`, isn't a rule.

When a line that isn't a rule starts with `!`, it reads as a deny, but the proxy can't tell what it would deny. So while the line is there, the proxy of every VM that reads the file denies every new connection apart from the VM's [links](#links-between-vms), whatever the other rules allow. Connections that are already open stay open. Each refusal names the file and the line, and it suggests no `devvm network allow` command, because no rule would help. Any other line that isn't a rule is skipped, and the file's other lines still apply. The proxy names each such line in its own log.

devvm warns about each line that isn't a rule, with the file and the line's number, and the warning for a broken deny says that the proxies deny everything until it's fixed. It warns when you open a VM with network policy whose proxy reads the file, when `devvm network allow`, `deny` or `rm` changes the file, and in `devvm doctor`. `devvm network` also marks the line. Fix the line or remove it, and the change applies from the next connection.

The proxy never dials a denied address, even for an allowed name that resolves to it or to its NAT64 form. A missing file has no rules. A file that changed but can't be read keeps its last rules, so a deny never lapses.

### Changing rules

`devvm network` shows this folder's VM's allowlist after its network policy. It prints each of the VM's policy files that holds more than comments, with the file's path after `#`, and then its rules without their comments. A line that isn't a rule is marked `# not a rule, skipped`, with its line number:

```text
on
# /Users/you/.local/share/devvm/assets/7fdd969856e644e8/layers/base.policy
  ports.ubuntu.com
  github.com
# /Users/you/.local/share/devvm/vms/devvm-some-project-71a99b1076d92c.policy
  example.com
  example.com:x  # not a rule, skipped (line 2)
```

| | |
| --- | --- |
| `devvm network allow RULE...` | add the rules to the VM's own file |
| `devvm network deny RULE...` | add them as denies, adding `!` when it's missing |
| `devvm network rm RULE...` | remove the lines with these rules, a deny with its `!`, and warn about one that isn't there |
| `-g`, `--global` | with `allow`, `deny` or `rm`, change `~/.config/devvm/policy`, for every VM, instead, which needs no VM |

A rule is what [Rules](#rules) lists, without spaces or a comment. devvm refuses anything else before it adds any rule, and it also refuses `allow !RULE`. A rule that's already in the file, in any case, isn't added again. Each change applies at once, from the next connection. For a VM without network policy, devvm notes that the rules don't apply. When `devvm network log --blocked` or a blocked request in the VM suggests a `devvm network allow` command, its rule is one that `devvm network allow` takes.

If a VM was deleted outside devvm, a new VM with its name never gets its rules.

### devvm network --json

`devvm network --json` prints the VM's network policy and its allowlist as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes. `policy` is `on`, `off` or `unenforced`, and `allowlist` has each policy file that holds more than comments, in the order the proxy reads them, with its path, its rules and, in `not_rules`, each line that isn't a rule, with its `line` number and its `text`. With `-g`, the object has only `policy`, which is what new VMs get: `ask`, `on` or `off`. With `--json` the command only shows, so devvm refuses `on`, `off` or `ask` with it.

```json
{
  "version": 1,
  "policy": "on",
  "allowlist": [
    {
      "file": "/Users/you/.local/share/devvm/assets/7fdd969856e644e8/layers/base.policy",
      "rules": [
        "ports.ubuntu.com",
        "github.com"
      ],
      "not_rules": []
    },
    {
      "file": "/Users/you/.local/share/devvm/vms/devvm-some-project-71a99b1076d92c.policy",
      "rules": [
        "example.com"
      ],
      "not_rules": [
        {
          "line": 2,
          "text": "example.com:x"
        }
      ]
    }
  ]
}
```

## Inside the VM

Every login in the VM has the proxy settings in its environment (`HTTP_PROXY`, `HTTPS_PROXY` and their lowercase forms, `NO_PROXY` for the VM itself, and `NODE_USE_ENV_PROXY`), and the [CA variables](../using/settings.md#a-companys-ca) that every VM has point the tools that don't read the system's store at devvm's CA. devvm also points apt, sudo, ssh and Docker at the proxy ([how](../internals/network.md#the-vms-setup)). A direct connection fails at once, even as root, and so does DNS, because names resolve in the proxy.

A blocked request gets `403`, and the response names the host, the reason and the rule that would allow it. The proxy also answers at `http://devvm.proxy/`:

- `/blocked` lists the last 50 blocks.
- `/blocked/new?after=N` gives the number of blocks so far, then those after the Nth, with each rule once, so a shell prompt can show what a command hit.
- `/ca.pem` is devvm's CA certificate, never its key.
- `/nix/` is the shared Nix cache, in a VM with the `nix` layer ([below](#nix)).

In the VM, `devvm-why` prints what the proxy blocked lately, and a note before each prompt tells you when something new was blocked. devvm also tells Claude Code and Codex, in `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, why a request may fail and to run `devvm-why`.

### Nix

In a VM with the [`nix` layer](../using/images.md#nix), Nix's daemon makes most of Nix's downloads, and devvm also points it at the proxy, so its downloads go through the same allowlists. The layer's allowlist has `cache.nixos.org` for Nix's binary cache, and `channels.nixos.org` and `releases.nixos.org` for its channels and flake registry, and GitHub, where most flakes' inputs come from, is in devvm's rules for every VM.

Nix asks the proxy's shared cache, `http://devvm.proxy/nix`, before cache.nixos.org, so every VM with network policy and the layer shares the packages that any of them downloaded ([shared downloads](../using/toolchains.md#nix-projects)). The proxy fetches what the cache doesn't have from cache.nixos.org alone, and only while the VM's allowlist allows `cache.nixos.org`. It keeps a package only when cache.nixos.org's signature on it checks, and the VM's Nix checks that signature again. A VM can only make the proxy fetch from cache.nixos.org: the proxy refuses anything a VM sends to store, and every request that isn't for one of Nix's cache files. Each request shows in `devvm network log` as `cache`, with whether it was a `hit`, `fetched`, or why it wasn't found. A VM can tell by how fast the cache answers whether another VM downloaded a public package, and a VM that downloads many packages can push other VMs' packages out of the cache, which only makes them download those again. A VM without network policy has no proxy, so it gets no shared cache: its Nix downloads from cache.nixos.org directly, and nothing runs or listens on this machine for it.

When a package isn't in the binary cache, Nix builds it from source, and the build downloads its sources from their own hosts, such as `ftpmirror.gnu.org`. The proxy blocks those hosts until you allow them, so the build fails, and the error, `devvm-why` and `devvm network log --blocked` name the host and the `devvm network allow` command that allows it.

## Behind a company proxy

When this machine reaches the internet only through a company proxy, the VM's proxy sends the requests it allows through that proxy. devvm reads `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY`, or their lowercase forms, from the shell you run it in. It passes them to the VM's proxy when the proxy starts, and when you run devvm again with different settings, devvm starts the VM's proxy again with the new ones. So run devvm from a shell that has your company's proxy settings, and after you change them, run devvm again in the folder or `devvm restart`.

The VM's proxy still decides each request by the host name the VM asked for, against the same allowlists, and only then passes it on. It sends tunnels, which carry HTTPS and the requests it puts your secrets into, through `HTTPS_PROXY`'s proxy, and plain HTTP requests through `HTTP_PROXY`'s. It reaches the hosts that `NO_PROXY` names directly, and also `localhost` and loopback addresses. In `NO_PROXY`, `example.com` matches that host and its subdomains, `.example.com` matches only its subdomains, an address or a CIDR matches addresses, any entry can end in a port, and `*` matches every host. `devvm network log` adds `through host:port` to each request that went through the company proxy.

The company proxy needs an `http://` address, such as `http://user:password@proxy.example.com:8080`, and a proxy without a port is on port 80. devvm doesn't start a VM with network policy while `HTTPS_PROXY` or `HTTP_PROXY` names an `https://` or `socks5://` proxy, and it tells you which setting to change. The company proxy's password stays on this machine. The VM never gets it, and devvm writes it into no log, no image and no process's arguments.

The company proxy sees the name and port of every host the VM reaches, and if it inspects TLS, the [CA you add for it](../using/settings.md#a-companys-ca) also lets it read the traffic. The company proxy also looks up each name itself, so the VM's proxy can't check every address a name leads to. It still refuses an address that's this machine's own or internal, unless a rule names it, and it refuses a name when this machine's lookup gives an internal or denied address for it. A name that this machine can't look up goes to the company proxy unchecked only when an allowlist names it, as `example.com` or `*.example.com`. When only `**` allows a name that this machine can't look up, the VM's proxy refuses it, so `**` never lets a VM send the company proxy a name of its own making. Under a name you allow by name, a VM can still reach whatever the company proxy reaches, such as hosts on your company's network.

### Without network policy

A Lima VM without network policy gets your proxy settings in its environment when it starts and uses the company proxy directly, while smolvm and Docker VMs don't get them.

Builds never get your company proxy's settings, and images never keep them. When devvm builds an image, builds the shared tools or converts a `--from` container image on Lima or Docker behind a company proxy, the build goes through a proxy of devvm's on this machine, made for that build alone, which goes through your company proxy. The build's commands get only that proxy's address, with a password devvm makes for the build, while they run, and devvm stops the proxy when the build ends. So the code a build runs, which includes a container image's own, never sees your company proxy's address or password. That proxy also runs on this machine, so a company proxy on `localhost`, such as one a VPN client runs, works for builds too. smolvm builds don't get your proxy settings at all. The Docker client puts the proxies in `~/.docker/config.json` into every container it makes, so devvm makes its containers with those settings empty.

A build's proxy allows any host, as `**` does. On Lima and on a Linux Docker engine, it sends a name that this machine can't look up on to the company proxy, so builds work where only the company proxy looks up outside names. On Docker Desktop, it refuses such a name, because Desktop's builds go through devvm's proxy to keep them off this machine's own services, and a company proxy running on this machine could reach them. So on Docker Desktop, builds behind a company proxy reach a host by name only when this machine can look the name up.

## Secrets

Secrets stay on this machine. A VM gets a placeholder of its own for each one, and its proxy puts the real value into requests for that secret's hosts, in one header:

| Secret | Hosts | Header | Used by |
| --- | --- | --- | --- |
| `claude` | `api.anthropic.com` | `Authorization` | every VM whose image has `claude` |
| `github` | `api.github.com`, `github.com`, `codeload.github.com`, `raw.githubusercontent.com` | `Authorization` | VMs it's bound to |

Agents' logins work the same way. When a VM's image has Codex or opencode and you're logged in on this machine, the VM gets placeholders for the login, and so do Claude Code's MCP servers' secrets ([agents' logins and settings](../internals/network.md#agents-logins-and-settings)).

### Saving and binding secrets

| Command | What it does |
| --- | --- |
| `devvm secret set claude` or `github` | asks for the Claude Code token or the GitHub token without showing it, and saves it (`--stdin` reads a line instead) |
| `devvm secret rm NAME` | deletes a saved secret |
| `devvm secret` | lists the secrets, whether each is saved, and the VMs using each |
| `devvm secret use NAME` | lets this folder's VM use the secret |
| `devvm secret unuse NAME` | stops this folder's VM using it, though a VM whose image has `claude` always uses `claude` |

devvm saves a secret in the macOS Keychain, or elsewhere in `~/.config/devvm/secrets/<name>`, a file only you can read ([details](../internals/network.md#where-secrets-are-saved)). It drops whitespace in a secret's value. It refuses a value with a quote, a backslash or a control character, and a `claude` value that isn't a setup token (`sk-ant-oat01-…`, from `claude setup-token`). A `CLAUDE_CODE_OAUTH_TOKEN` in devvm's environment wins over a saved one.

Saving, deleting, binding and unbinding apply at once to the proxies that are running. From its next launch, the VM gets the placeholder in `CLAUDE_CODE_OAUTH_TOKEN` or `GH_TOKEN`, and with `github` a git credential helper for `https://github.com`. A VM without network policy gets the real value instead.

To put a secret in, the proxy shows the VM a certificate from devvm's own CA, which each launch installs in the VM and which lasts 30 days. devvm makes a new CA when a launch finds that the current one has under a week left, so a VM that you open with `devvm` in its folder at least once a week always trusts the current one. Connecting with `ssh` alone isn't a launch. When a VM runs past its CA's end without a launch, its proxy refuses its HTTPS connections to secrets' hosts, and `devvm network log --blocked` gives the reason `devvm's CA has ended`. The VM's other requests keep working. Run `devvm` in the VM's folder to install a new CA, which also starts the VM's proxy again and ends the connections open through it ([how](../internals/network.md#how-the-proxy-puts-them-in)).

`devvm secret unuse` and `devvm secret rm` take nothing back from a VM without network policy, because the value it got still works wherever it kept it, and devvm says so on `unuse`. Neither command revokes what a VM made with the secret ([what devvm doesn't protect](../internals/security.md#what-it-doesnt-protect)), so rotate it if that matters. Use a fine-grained GitHub token, scoped to the repos your VMs need.

#### devvm secret --json

`devvm secret --json` prints the list as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes. Each of the `secrets` has its name, `saved`, which is `true` when it's saved or, for `claude`, in devvm's environment, and `used_by`, the names of the VMs using it. The JSON never holds a secret's value.

```json
{
  "version": 1,
  "secrets": [
    {
      "name": "claude",
      "saved": true,
      "used_by": [
        "devvm-some-project-71a99b1076d92c"
      ]
    },
    {
      "name": "github",
      "saved": false,
      "used_by": []
    }
  ]
}
```

## Logs

`devvm network log` shows the proxy's decisions for this folder's VM, oldest first, one line each. A line has the time, `allowed`, `BLOCKED` or `cache`, and the host and port. An allowed line then has the rule that allowed it (`, decrypted` for an intercepted tunnel), the request (`GET /user`) and the secrets put in (`, token: github`), a blocked line has why it was blocked, and a `cache` line, a request to the [shared Nix cache](#nix), has the request and what came of it:

```text
2026-10-09T10:00:00Z  BLOCKED  example.com:443  not on the allowlist
2026-10-09T10:00:05Z  allowed  github.com:443  rule github.com
2026-10-09T10:00:06Z  allowed  api.github.com:443  GET /user, token: github
2026-10-09T10:00:07Z  cache    cache.nixos.org:443  GET /nix/yv1zf3y6all4j99aad30cb8i55dw9zmp.narinfo: hit
```

| | |
| --- | --- |
| `-n N`, `--lines N` | the last N (default 50) |
| `--follow` | then each new one as it happens, until Ctrl+C |
| `--blocked` | the blocks instead, one row per host and port, by when last blocked: `LAST`, `COUNT`, `HOST` and `ALLOW WITH`, the `devvm network allow` command that allows it |

`ALLOW WITH` is `-` for a request that the proxy refuses whatever the allowlist says, such as a GitHub call that makes a credential, and for a host that isn't a host name or an address. A VM without network policy logs only the connections of its links to your other VMs, and `devvm network log` says so. The proxy keeps each VM's log to its last 10 to 20 MiB by starting a new file once the current one reaches 10 MiB and keeping one older file, which `devvm network log` reads too. `devvm rm` deletes the VM's logs, and [How it works](../internals/network.md#log-files) describes the files.

### devvm network log --json

`devvm network log --json` prints the last N events as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes, and devvm refuses it with `--follow`. Each of the `events` has what the proxy logged, without the VM's name. That's the `time`, the `event`, the `host` and the `port`, and, when the event has them, the request's `method` and `path`, the `rule` that allowed it, the `reason` it was blocked or what came of a cache request, the request's `User-Agent` header as `agent`, the `secrets` put in, `refused` for a request refused whatever the allowlist says, the company proxy it went through as `upstream`, and the VM that a link's connection went to as `target`. The `event` is `tunnel` for an allowed connection, `intercept` for an allowed tunnel that the proxy decrypted, `forward` for an allowed request, `blocked`, or `nix-cache` for a request to the shared Nix cache. A link's connection is `link` when devvm carried it, and `link_refused`, `link_target_stopped` or `link_limit` when devvm refused it, with the link's name as its `host`. A log with nothing in it gives `"events": []`.

```json
{
  "version": 1,
  "events": [
    {
      "time": "2026-10-09T10:00:00Z",
      "event": "blocked",
      "host": "example.com",
      "port": 443,
      "method": "CONNECT",
      "reason": "not on the allowlist"
    },
    {
      "time": "2026-10-09T10:00:06Z",
      "event": "forward",
      "host": "api.github.com",
      "port": 443,
      "method": "GET",
      "path": "/user",
      "secrets": [
        "github"
      ]
    }
  ]
}
```

With `--blocked`, each of the `blocked` has the `last` time it was blocked, the `count`, the `host` and the `port`, and `allow`, the rule that `devvm network allow` takes to allow it. A block that `ALLOW WITH` shows as `-` has no `allow`.

```json
{
  "version": 1,
  "blocked": [
    {
      "last": "2026-10-09T10:01:00Z",
      "count": 2,
      "host": "example.com",
      "port": 443,
      "allow": "example.com"
    }
  ]
}
```

## Links between VMs

A [link between VMs](../using/links.md) lets a program in one VM, the client, reach a port that another VM, the target, publishes. devvm carries a link's connections over ssh through this machine, so they use neither VM's network, and with network policy a VM reaches only the hosts on its allowlist and the links you added to it. A client's proxy is a second way into its links: it carries a request for one of the client's own links, by the link's name and port, to the same link on this machine, and passes it on as it is, without looking into it or putting a secret into it. It reaches no other socket on this machine and no other VM's links. The allowlist's rules don't apply to links, so `devvm network deny` doesn't stop one, and `devvm link rm` does.

A link joins the two VMs' trust. Code in the client can often run code in the target through the target's service, such as Postgres's `COPY … TO PROGRAM`, and it then has whatever the target has: its network, the secrets it may use and its project folder. So devvm keeps two rules:

- **A client with network policy links only to a target with network policy.** A link to a target without it would let the client reach the internet through the target, around its own proxy. devvm checks this when you add a link, at each launch of the client and on every connection, and on every connection the target's network policy must be in force as it runs, so devvm refuses the connection while the target shows as `unenforced`, and it warns you of such a target when you add the link, when you publish a port in its folder that the client links to, and when the client starts. While such a link exists, devvm refuses `devvm network off` for the target, and it refuses `devvm network on` for a client that links to a target without network policy. A client without network policy may link to any target, and it then has the target's secrets and project folder within reach as well as its own network.
- **The target publishes.** A link reaches only a port that the target's folder publishes with `devvm link publish`, and only on the target's own loopback. Publishing offers the port to any of your VMs, and each link is still added in the client's folder, so a VM reaches another's service only when you've run a command in both folders.

The first rule keeps a client's network policy whole, but it doesn't keep the target's data in the target. A client with network policy can read whatever the target's service gives it and send that out through the hosts on its own allowlist. Link two VMs only when you'd trust each with what the other has.

A [group](../using/links.md#groups) links each of its VMs to the ports that the others publish, once you've run `devvm group join` in each VM's folder. Its VMs all have network policy or none do, so every link it makes keeps the first rule, and devvm refuses `devvm network on` and `devvm network off` for a VM while it's in a group. A group joins the trust of all its VMs, so put VMs in one only when you'd trust each with what the others have.

## Not covered

Anything an allowed host accepts can carry data out, including whatever your secrets allow, such as pushes and API calls.

A [company CA](../using/settings.md#a-companys-ca) that you put in `~/.config/devvm/ca` can read the TLS traffic of every VM, with network policy or without, as it can this machine's, and the proxy trusts it for its own connections. Only that folder adds CAs to the proxy: nothing a VM sends can.
