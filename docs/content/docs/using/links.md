---
title: Links between VMs
weight: 5
---

A link lets a program in one of your VMs reach a TCP service in another by name, as in `postgres://db:5432/app` or `fetch("http://api:3000")`. The VM with the service is the target, and you publish its port in its folder. The VM that reaches it is the client, and you add the link in the client's folder. Links work on every backend, with or without network policy on either VM. devvm carries each connection over ssh through this machine, so neither VM's network changes, and [How it works › Links between VMs](../internals/ports.md#links-between-vms) explains how.

For a database in `~/code/db` and an app in `~/code/app`, both with VMs:

```sh
cd ~/code/db && devvm link publish 5432    # offer the database's port to links
cd ~/code/app && devvm link add db 5432    # programs in app's VM reach it as db:5432
```

Programs in the app's VM then reach the database as `db:5432` and as `db.devvm.internal:5432`. A link joins the two VMs' trust, so add one only between VMs you'd trust with each other's reach ([below](#a-link-joins-the-two-vms-trust)).

## Publishing a port

Links reach only the ports a VM publishes, so you publish a port in the target's folder before any VM can link to it:

```sh
devvm link publish             # what this folder's VM publishes: none, or a list
devvm link publish 5432        # offer port 5432 to links
devvm link publish 8000-8010   # offer a range
devvm link unpublish 5432      # stop offering it
```

- `devvm link publish` takes ports and ranges as `devvm ports` does, such as `5432,8000-8010`, but not `all` or `none`. A port you publish is offered to links from any of your VMs, and each link is still made in its client's folder with `devvm link add`.
- `devvm link unpublish` takes ports as `devvm link publish` lists them, and it refuses a port that isn't published. The server in the VM keeps running. When other VMs link to a port you take out, devvm names them, and their next connections to that port are refused, while connections that are open stay until they close. To remove those links, run `devvm link rm` in their folders.
- A change applies at once, whether the VM is running or stopped.
- Publishing forwards nothing to this machine, and it doesn't change [the ports the VM forwards](ports-size.md#ports).

A link connects to the target's own `127.0.0.1`, so the server in the target has to listen there or on all its addresses. A server that listens on `::1` alone isn't reached.

## Adding a link

In the client's folder, `devvm link add TARGET PORT` links its VM to PORT of the VM that TARGET names, once that VM publishes the port:

```sh
devvm link add db 5432                          # reach port 5432 of the VM for the folder db as db:5432
devvm link add db 5432 --as pg                  # reach it as pg:5432 instead
devvm link add ../api 8080                      # name the target by its folder's path
devvm link add devvm-api-71a99b1076d92c 8080    # or by its VM's name
```

TARGET takes one of three forms:

- **A folder's name,** as VM names have it, such as `db` for `devvm-db-71a99b1076d92c`. When more than one VM has a folder of that name, devvm refuses and lists each VM with its folder, so you can give the VM's name or the folder's path instead.
- **A folder's path,** which starts with `.` or holds a `/`, such as `../api` or `/Users/you/code/api`.
- **A VM's name,** as `devvm ls` lists it.

devvm refuses a target that has no VM, the client's own VM, and a port that the target doesn't publish. It also refuses a link that [network policy](#links-and-network-policy) doesn't allow. When devvm makes the link, it says what the client reaches it as, and that the link joins the two VMs' trust.

### Names

Programs in the client reach a link at `NAME:PORT` and `NAME.devvm.internal:PORT`. NAME is the target's folder name, as VM names have it, unless you give `--as NAME`. `.internal` is reserved for private networks, so `NAME.devvm.internal` never names a host on the internet, and it's the name to use when the short one could clash with a name your network resolves.

- A client has one name for each VM it links to. The first link to a target sets the name, and later links to that target take it too, so after `devvm link add db 5432 --as pg`, `devvm link add db 8000` reaches `pg:8000`. To change the name, remove that target's links and add them again.
- A name is one DNS label, made of lowercase letters, digits and dashes, which starts and ends with a letter or a digit and isn't all digits. devvm refuses `localhost`, the client's own hostname and a name that another of the client's links has. When the target's folder name isn't a valid name, or it's taken, give another with `--as NAME`.
- A link is reached only by its names, never at the client's `localhost`, so `localhost:5432` in the client stays the client's own.
- A link's name hides the same name from anywhere else in the client, such as a host `db` under your network's search domain.

### Ports

The client reaches the target's port on the same port number, so `devvm link add web 80` gives `web:80`, and ports below 1024 work like any other. Two targets can publish the same port, because the client reaches each one by its own name.

Links carry TCP over IPv4 alone. Each name resolves to an IPv4 address, so a program that connects only over IPv6 doesn't reach it, and UDP isn't carried.

### Which VM a link reaches

A link stays with the VM it was made for, because devvm records the target VM's identity with the link. A VM made again in the target's folder, or another VM with the same folder name that appears later, isn't the linked target.

### A link joins the two VMs' trust

Code in the client can often run code in the target through the target's own service, such as Postgres's `COPY … TO PROGRAM`. It then has whatever the target has: its network, the secrets it may use and its project folder. devvm doesn't stop that, because it's what the service itself allows. So add a link only between VMs you'd trust with each other's reach, and publish only the ports that you mean other VMs to use. [Network policy](../safety/network.md#links-between-vms) explains what this means for a VM with network policy.

## Links and network policy

A client with network policy links only to a target with network policy, because a link to a target without it would let the client reach the internet through the target. A client without network policy may link to any target.

devvm checks this rule when you add a link, at each launch of the client and on every connection. On every connection, the target's network policy must also be in force as the target runs. If you turn it on while the target is running, `devvm ls` shows it as `unenforced` until the target starts again behind its proxy, and devvm refuses the client's connections to it until then, so start the target again with `devvm restart` in its folder. devvm also warns you of such a target when you add a link to it, when you publish a port in its folder that the client links to, and each time the client starts. The warning names the target and refuses nothing, and devvm doesn't warn of a stopped target, because its network policy applies when it next starts. While such a link exists, devvm refuses `devvm network off` in the target's folder and names the clients that link to it, and it refuses `devvm network on` for a client that links to a VM without network policy. Remove those links with `devvm link rm` first, or turn on network policy for the target.

With network policy, a program that uses the VM's proxy settings, such as curl, sends its requests for a link to the VM's proxy, and the proxy carries a request for one of the link's names and its port, such as `db:5432` or `db.devvm.internal:5432`, to the link. Other programs reach the link by its name, as they do in a VM without network policy. The proxy passes a link's connection on as it is, so it doesn't look into it and puts none of your secrets into it.

Network policy's rules don't apply to links. The allowlist doesn't allow or block a link's connections, and `devvm network deny db` doesn't stop the link `db`. The rule above and the published ports decide which connections a link carries, and `devvm network log` shows them ([the link log](#the-link-log)). To stop a link, remove it with `devvm link rm`, or unpublish its port in the target's folder.

## Listing links

`devvm link` lists this folder's VM's links, with one row for each port:

```text
NAME  PORT  TARGET                   FOLDER              STATUS   APPLIES
db    5432  devvm-db-71a99b1076d92c  /Users/you/code/db  Running  yes
db    8000  devvm-db-71a99b1076d92c  /Users/you/code/db  Running  unpublished
```

`STATUS` is the target's status as its backend reports it, such as `Running` or `Stopped`, and it's empty when devvm can't tell. `APPLIES` is `yes` for a link that works while the target runs, or else it says why the link doesn't apply:

- `gone`: the target VM no longer exists.
- `replaced`: the target's folder has another VM, made again since the link was.
- `unpublished`: the target doesn't publish the port.
- `policy`: the client has network policy and the target doesn't.

`devvm ls` shows how many links each VM has, in its `LINKS` column.

### devvm link --json

`devvm link --json` prints the same list as one JSON object, as [output for scripts](../getting-started/commands.md#output-for-scripts) describes. Each of the `links` has its `name`, its `port`, the `target` VM, the target's folder as `target_folder` and its status as `target_status`, both `""` when devvm doesn't know them, `applies`, which is `true` or `false`, and `reason`, which is why the link doesn't apply, as `APPLIES` shows it, or `""` when it does.

```json
{
  "version": 1,
  "links": [
    {
      "name": "db",
      "port": 5432,
      "target": "devvm-db-71a99b1076d92c",
      "target_folder": "/Users/you/code/db",
      "target_status": "Running",
      "applies": true,
      "reason": ""
    }
  ]
}
```

### devvm link publish --json

`devvm link publish --json` prints what the VM publishes as one JSON object, and it only shows, so devvm refuses a list of ports with it. `ports` is `none` or a list of ports and ranges, written as `devvm link publish` takes it.

```json
{
  "version": 1,
  "ports": "5432,8000-8010"
}
```

## Removing a link

`devvm link rm TARGET PORT` removes one of this folder's VM's links, where TARGET is the link's name or anything that `devvm link add` takes:

```sh
devvm link rm db 5432   # stop reaching db:5432
devvm link rm pg 5432   # by the name given with --as
```

The link's next connection is refused, while connections that are open stay until they close.

## When links change

A change of links reaches a running client at once and a stopped one when it next starts, and devvm says which. On a running client, the names resolve at once, because devvm rewrites the client's `/etc/hosts`, and a link's port answers within a few seconds. With network policy, the VM's proxy reaches a new link as soon as its port answers, so the programs that use the VM's proxy settings also reach it, including those that were already running. Nothing in the VM restarts, and the VM's environment doesn't change, with or without network policy.

## When a target stops or goes

devvm never starts a VM for a link. A connection to a stopped target fails, and each launch of the client warns about each stopped target, until you start it with `devvm` in its folder. The client's links then work again without any change to the client. When the target stops, the connections that were open through it end. A link also needs both VMs' backends to carry links, which every backend of devvm's does, and when one can't, each launch of the client says so.

`devvm rm` of a target removes every link to it from your other VMs and names the clients that lost one. A VM that you make again in that folder isn't linked, so publish its ports and add the links again. `devvm rm` of a client removes its links, and `devvm stop` of a client ends its links' connections.

A link can't listen when a program in the client already holds its port in a way that covers the link's address, such as a server of the client's own on all its addresses with the same port. That link alone doesn't work, devvm says so, and the client's other links work. Stop that program, then run `devvm restart`.

## Containers in the VM

A container that the [docker layer](images.md#built-in-layers) runs in the client has a network of its own, so it neither resolves a link's names nor reaches its address. Run it with `--network host`, as in `docker run --network host IMAGE`, so it shares the VM's network and reaches the links as the VM does.

## The link log

`devvm network log` in the client's folder shows its links' connections, with or without network policy. A line has the time, `link` for a connection devvm carried or `REFUSED` for one it refused, the link's name and port, and the target VM, with why devvm refused it:

```text
2026-11-01T10:00:00Z  link     db:5432  to devvm-db-71a99b1076d92c
2026-11-01T10:05:00Z  REFUSED  db:5432  to devvm-db-71a99b1076d92c: it isn't running
```

When the client has network policy and the target's isn't in force yet, the line says `unenforced`, as [Links and network policy](#links-and-network-policy) describes.

A client has at most 256 of its links' connections carried at once, and devvm refuses and logs the next one. [devvm network log --json](../safety/network.md#devvm-network-log---json) describes the events in JSON.
