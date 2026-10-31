---
title: Troubleshooting
weight: 11
---

Each section below starts with what you see, then says what to do, with a link to the details.

## A command fails

devvm prints its own steps, and it sends the output of the commands it runs to a log instead of the terminal. When a step fails, devvm prints the end of the log. To see the commands' output as they run, set `DEVVM_VERBOSE=1`.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, which only you can read, and devvm keeps the newest 20. In the log, each command appears as a `+` line with its arguments, followed by its output, and a numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

## A build fails

When an image's build fails in its script, or as the build VM starts or stops, devvm keeps the build VM, stopped and without a record, so you can look inside. devvm's error names the VM and says how to open a shell in it. On Lima, that's [`devvm limactl`](../backends/lima.md#running-limactl):

```sh
devvm limactl shell --start devvm-img-<key>
```

`--start` starts the VM first, outside the build's confinement. On smolvm, start and open the machine with smolvm's own commands ([looking inside](../backends/smolvm.md#looking-inside)). On Docker, start the container and open a shell in it with Docker's own commands ([looking inside](../backends/docker.md#looking-inside)).

The next build of that image deletes the VM and starts over, and `devvm gc` also deletes it. A build whose create or clone fails deletes the image, once devvm has shown the end of the log, because there's nothing in the image to look at.

Either way, the images under it, which are already built, stay. devvm deletes and builds again any image it finds without its record, such as a failed build's or one devvm couldn't clean up after a crash or a power cut.

## Not enough free space

Before each build and each new VM, devvm checks the free space on the disk that will hold it, and it refuses when there's less than the step needs. With Lima on macOS, that's 8 GiB for base, 2 GiB for another image or a VM, and from 4 GiB for one shared tool to 8 GiB for all ten, and [free space](ports-size.md#free-space) gives the numbers for Linux hosts and smolvm. The message says which folder's disk it checked, how much is free and how much the step needs. Nothing has been built or made yet, so you can make room and run the same command again:

- `devvm gc` deletes stale and unfinished images and unused shared tool images ([cleaning up](images.md#cleaning-up)).
- `devvm images` lists the images and shared tool images with the space each takes, and `devvm images rm` deletes those you don't need ([deleting images](images.md#deleting-images)).
- `devvm ls` lists your VMs, and `devvm rm` deletes one. devvm never deletes a VM to make room, because a VM may hold work you haven't saved.

When devvm says it couldn't read the free space, the error after the message says why, such as a folder it can't reach.

## You pressed Ctrl+C

Ctrl+C, or closing the terminal, stops the command that's running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.

devvm deletes an interrupted new VM, with its state, until its first shell opens, because nothing of yours is in it yet. An interrupted build deletes the image it was building.

## base can't build on Lima

On Lima, base starts from a pinned, dated release of Ubuntu's cloud image. Ubuntu removes its dated releases after a few months, and once the pinned one is gone, devvm can't find it and can't build base. Check that you're online, and if you are, update devvm to a version with a newer release ([Ubuntu's cloud image](../internals/images.md#ubuntus-cloud-image)).

## A file devvm can't read

When devvm can't read one of its settings or state files, it names the file and the line. Fix the line or delete the file. [Settings and files](settings.md) lists what each file holds.

## devvm names set-id files

On a Linux host, root in a VM can set the set-user-id and set-group-id bits on files in the project folder, and those bits reach this machine, so such a file runs as its owner for anyone who runs it. devvm names these files at each launch and stop, and it names the ones that root owns apart from the rest. Those come from a Docker VM, where root writes files as this machine's root, so they run as root.

- For a file that root owns, don't run it, and delete it with `sudo rm FILE`.
- For any other file, `chmod u-s,g-s FILE` clears its bits.

Either way, don't run programs from the project folder outside the VM ([what devvm doesn't protect](../internals/security.md#what-it-doesnt-protect)).

## HOME isn't an absolute path

devvm finds its folders from `HOME`, so it stops when `HOME` is empty or isn't an absolute path. Set `HOME` to your home folder, such as `/Users/you` on macOS or `/home/you` on Linux, and run devvm again.

## A VM shows as unenforced

`devvm ls` and `devvm network` show a VM's network policy as `unenforced` when the VM has network policy but devvm didn't start it confined. That happens when it was started outside devvm, such as with `devvm limactl start`, or when it has been running since before `devvm network on`. The VM then reaches the network without its proxy.

On the VM's next launch, devvm offers in a terminal to restart it confined, and if you decline, it warns and goes on. Without a terminal, devvm stops the VM instead, and the next `devvm` starts it confined ([the self-check](../internals/network.md#the-self-check)).

## A request is blocked

With network policy, a VM reaches only the hosts on its allowlist. To see what the proxy blocked, run `devvm log --blocked` on this machine, or `devvm-why` in the VM. `devvm log --blocked` also shows the `devvm policy allow` command that allows each host, and the rule applies at once, to the next connection ([logs](../safety/network.md#logs)).

## Behind a company proxy

When this machine reaches the internet only through a company proxy and nothing in a VM or a build gets through, check that the shell you run devvm in has the proxy settings, as `env | grep -i _proxy` shows. devvm reads `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` from that shell, so set them there, then run devvm again in the folder, which starts the VM's proxy again with them ([network policy behind a company proxy](../safety/network.md#behind-a-company-proxy)). A shell without the settings, such as an editor's terminal that didn't load your profile, starts the VM's proxy without them.

When devvm says that `HTTPS_PROXY` or `HTTP_PROXY` names an `https://` or `socks5://` proxy, ask your IT team for the proxy's `http://` address, and set that. A company proxy that inspects TLS also needs its CA, as the next section describes. On macOS, image builds can't reach this machine's loopback, so a proxy on `localhost`, such as one a VPN client runs, works for VMs with network policy but not for builds on Lima.

## TLS errors behind a company proxy

When apt, `npm install`, `pip install`, `git clone` over HTTPS or an image's build fails with an error such as `certificate verify failed`, `self-signed certificate in certificate chain` or `unable to get local issuer certificate`, the network you're on may inspect TLS. Ask your IT team for the CA's certificate, or export it from this machine's store, such as Keychain Access on macOS. Put it in `~/.config/devvm/ca` and open the VM again ([a company's CA](settings.md#a-companys-ca)).

`devvm doctor` says so when `HTTPS_PROXY` is set and the folder has no CA certificates. Many proxies don't inspect TLS, so that line isn't a problem in itself. When devvm refuses a file in the folder, it names the file, and you take out anything that isn't a CA certificate, such as a private key or a server's certificate.

## A listed port isn't forwarded

When a program on this machine already has a port that the VM lists, devvm warns that the VM doesn't forward it, and the VM runs on without it. Stop that program, then run `devvm restart`, or change the VM's ports with `devvm ports`, and devvm forwards the port once it's free ([ports](ports-size.md#ports)).

## A VM won't boot

A Lima VM that doesn't answer over ssh within a minute of starting is stopped. Read its console log before you start it again, because every start overwrites the log. The log is `serialv.log`, or another `serial*.log`, in the VM's folder in devvm's Lima home, which is `~/.local/share/devvm/lima/<vm>/` unless you set `DEVVM_LIMA_HOME` ([Lima's limits](../backends/lima.md#limits)).

## The folder was made again

When you delete a project folder and make a new one at the same path, such as by cloning another repo there, devvm refuses the old folder's VM. Run `devvm rm` to delete the old VM, or `devvm adopt` to give it to the new folder ([a folder made again](../getting-started/commands.md#a-folder-made-again)).

## An outdated VM, or newer shared tools

devvm warns on each launch when an image the VM was made from has changed, and it leaves the VM as it is ([outdated VMs](images.md#outdated-vms)). It also warns when there are newer shared tools than the VM's, and `devvm update` moves the VM to them ([adding and removing tools](images.md#adding-and-removing-tools)).

## A shared tool doesn't mount

When a VM can't mount one of its shared tools, devvm stops the launch with an error that names the VM and the tools, and the VM keeps running. That happens when the tool's image is missing from the folder that devvm keeps the shared tools in, such as after you deleted it by hand, or when the VM can't mount the image. devvm doesn't let the VM download the tool in its place, because that copy would be the VM's own, a download you didn't ask for, and one that network policy may block.

Run `devvm update` in the project folder. It builds the tools that are missing and then opens the shell. When the VM has the newest versions, devvm builds those, and otherwise it builds the newest versions and moves the VM to them. If the launch still stops, the run log says why the VM couldn't mount the image ([a command fails](#a-command-fails)).

A tool that doesn't mount never keeps the VM from booting. The VM mounts the tools it can and boots as usual, and only devvm's launch stops ([the share](../internals/images.md#the-share)).

## Docker on macOS can't get devvm's Linux build

On macOS, the Docker backend runs devvm's Linux build in containers, and a release fetches it the first time a Docker VM or build needs it. When the download fails, check your connection and run devvm again. When the download doesn't match the SHA-256 that devvm has of it, devvm doesn't use it, so reinstall devvm ([requirements](../backends/docker.md#requirements)).
