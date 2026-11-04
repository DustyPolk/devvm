---
title: Troubleshooting
weight: 12
---

Each section below starts with what you see, then says what to do, with a link to the details.

## A command fails

devvm prints its own steps, and it sends the output of the commands it runs to a log instead of the terminal. When a step fails, devvm prints the end of the log.

To see the commands' output as they run, add `-v` or `--verbose` to any command, as in `devvm -v build base` or `devvm -v`. devvm then shows each line on standard error as the log gets it, without the status line, and it still writes the log. The lines are the log's own, so a VM's proxy key is hidden in them as it is in the log, and devvm drops the control characters a command prints, so its output can't move the cursor or change your terminal. When a step fails with `-v`, devvm names the log instead of printing its end again.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, which only you can read, and devvm keeps the newest 20. In the log, each command appears as a `+` line with its arguments, followed by its output, and a numbered step adds a `# [2/5] <step>` line first. A VM's proxy key and the build proxy's password are written as `devvm:<key>@`. Any other password in a URL, in a command's arguments or its output, is written as `user:<password>@`, so a company proxy's password from your shell's `HTTPS_PROXY`, which Lima can print as it starts a VM without network policy, stays out of the log. This covers the output of the commands devvm runs in a VM too, such as a VM's setup or a `devcontainer.json`'s commands.

Lima keeps a log of its own for each VM, `ha.stderr.log` in the VM's folder in devvm's Lima home, and it writes a proxy password into that log as it starts the VM. Once Lima has started the VM, devvm replaces each password there with as many `*`, so the log keeps its length and Lima goes on writing to it. If devvm can't change the file, it warns you and names it, and you can delete the file, which Lima makes again at the VM's next start.

In a terminal and without `-v`, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

## A command is refused before it runs

devvm checks a command's arguments and flags before it does anything else, so a mistyped one never reaches your VM. When one is wrong, such as a missing argument, one too many, a value the command doesn't take or a flag it doesn't have, devvm says what the command expected, then names that command's own help on the last line:

```console
$ devvm ports 5173 8080
devvm: The command devvm ports takes [all|none|LIST], not 5173 8080.
devvm: https://devvm.dev/docs/using/troubleshooting/#a-command-is-refused-before-it-runs
Run devvm ports --help to see how to use it.
```

Nothing has changed at that point, so you can fix the command and run it again.

## Exit codes

devvm exits with one of four codes, so a script can tell what happened:

| Code | Meaning |
| --- | --- |
| 0 | The command succeeded. |
| 1 | The command failed, and devvm's error says why. |
| 2 | devvm refused the command line, such as a bad argument or flag or an unknown command, before anything ran. |
| 130 | The command was interrupted by Ctrl+C or a closed terminal ([you pressed Ctrl+C](#you-pressed-ctrlc)). |

Two programs can take devvm's place and exit with their own codes. Once devvm opens a shell in your VM, the shell replaces devvm, so the code is the shell's, and `devvm limactl` exits with limactl's code.

## A build fails

When an image's build fails in its script, or as the build VM starts or stops, devvm keeps the build VM, stopped and without a record, so you can look inside. devvm's error names the VM and says how to open a shell in it. On Lima, that's [`devvm limactl`](../backends/lima.md#running-limactl):

```sh
devvm limactl shell --start devvm-img-<key>
```

`--start` starts the VM first, outside the build's confinement.

The next build of that image deletes the VM and starts over, and `devvm gc` also deletes it. A build whose create or clone fails deletes the image, once devvm has shown the end of the log, because there's nothing in the image to look at.

Either way, the images under it, which are already built, stay. devvm deletes and builds again any image it finds without its record, such as a failed build's or one devvm couldn't clean up after a crash or a power cut.

## Not enough free space

Before each build and each new VM, devvm checks the free space on the disk that will hold it, and it refuses when there's less than the step needs. With Lima on macOS, that's 8 GiB for base, 2 GiB for another image or a VM, and from 4 GiB for one shared tool to 8 GiB for all ten, and [free space](ports-size.md#free-space) gives the numbers for Linux hosts. The message says which folder's disk it checked, how much is free and how much the step needs. Nothing has been built or made yet, so you can make room and run the same command again:

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

When devvm can't read one of its settings or state files, it names the file, and the line when the problem is on one. Fix the line or delete the file. [Your settings](settings.md#your-settings) lists what each file you edit holds, and [devvm's data folder](space.md#devvms-data-folder) what each file devvm writes holds.

## A newer devvm changed devvm's folders

devvm's data folder, `~/.local/share/devvm`, and its settings folder, `~/.config/devvm`, each record their layout version in a file named `version`, which says how devvm keeps its files there, and devvm checks both before each command. When either one has a newer version than this devvm's, a newer devvm has changed the folders, so this one refuses to run rather than misread them, and it names the file and both versions. Run the newer devvm, or upgrade this one to it, for example after going back to an older version by mistake. `devvm --version`, help and completion still run. An older version is never refused, because devvm changes the folders to its own layout before the command runs, and a folder without a `version` file gets this devvm's ([the layout version](../internals/overview.md#the-layout-version)).

## devvm names set-id files

On a Linux host, root in a VM can set the set-user-id and set-group-id bits on files in the project folder, and those bits reach this machine, so such a file runs as its owner for anyone who runs it. devvm names these files at each launch and stop, and it names the ones that root owns apart from the rest, because those run as root.

- For a file that root owns, don't run it, and delete it with `sudo rm FILE`.
- For any other file, `chmod u-s,g-s FILE` clears its bits.

Either way, don't run programs from the project folder outside the VM ([what devvm doesn't protect](../internals/security.md#what-it-doesnt-protect)).

## HOME isn't an absolute path

devvm finds its folders from `HOME`, so it stops when `HOME` is empty or isn't an absolute path. Set `HOME` to your home folder, such as `/Users/you` on macOS or `/home/you` on Linux, and run devvm again.

## A VM shows as unenforced

`devvm ls` and `devvm network` show a VM's network policy as `unenforced` when the VM has network policy but devvm didn't start it confined. That happens when it was started outside devvm, such as with `devvm limactl start`, or when it has been running since before `devvm network on`. The VM then reaches the network without its proxy.

On the VM's next launch, devvm offers in a terminal to restart it confined, and if you decline, it warns and goes on. Without a terminal, devvm stops the VM instead, and the next `devvm` starts it confined ([the self-check](../internals/network.md#the-self-check)).

If you press Ctrl+C while devvm starts a VM with network policy, devvm stops the VM and its proxy before it exits, so the VM never runs on without the proxy, and the next `devvm` starts both again confined.

## The proxy doesn't start

A VM with network policy runs only with its proxy, so when the proxy doesn't start, devvm stops the VM, and the next `devvm` starts both again. When devvm says the proxy didn't start, read the log it names for why. When it says another program took the proxy's port, devvm picks another port the next time it starts the VM, so run `devvm restart`. When the proxy doesn't stop for new settings, run `devvm stop`, then `devvm` ([the proxy](../internals/network.md#the-proxy)).

## A request is blocked

With network policy, a VM reaches only the hosts on its allowlist. To see what the proxy blocked, run `devvm network log --blocked` on this machine, or `devvm-why` in the VM. `devvm network log --blocked` also shows the `devvm network allow` command that allows each host, and the rule applies at once, to the next connection ([logs](../safety/network.md#logs)).

## Behind a company proxy

When this machine reaches the internet only through a company proxy and nothing in a VM or a build gets through, check where devvm finds the proxy. devvm reads `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` from the shell you run it in, as `env | grep -i _proxy` shows, and when that shell sets neither `HTTPS_PROXY` nor `HTTP_PROXY`, it reads the `[proxy]` table of `~/.config/devvm/config.toml` instead ([network policy behind a company proxy](../safety/network.md#behind-a-company-proxy)). A shell without the settings, such as an editor's terminal that didn't load your profile, starts the VM's proxy without a company proxy, unless `config.toml` names one. So set the proxy in `config.toml`, and it works from every shell:

```toml
[proxy]
https = "http://alice@proxy.example.com:8080"
http = "http://alice@proxy.example.com:8080"
no-proxy = "localhost,.example.com"
```

`config.toml` never holds the company proxy's password. Put only the user in the URL, as above, and save the password with `devvm secret set proxy`, which asks for it. Then run devvm again in the folder, which starts the VM's proxy again with the new settings.

When a VM's proxy went through a company proxy the last time it started and devvm finds none this time, devvm warns you, names `config.toml` and starts the proxy without one, so the VM's requests go directly. Set the proxy in that shell or in `config.toml`, then run `devvm restart` in the folder.

When devvm says that `HTTPS_PROXY`, `HTTP_PROXY` or one of `config.toml`'s `[proxy]` keys names an `https://` or `socks5://` proxy, ask your IT team for the proxy's `http://` address, and set that. A company proxy that inspects TLS also needs its CA, as the next section describes.

Builds behind a company proxy go through a proxy of devvm's, which allows any host. It sends a name that this machine can't look up on to the company proxy, which resolves it.

## TLS errors behind a company proxy

When apt, `npm install`, `pip install`, `git clone` over HTTPS or an image's build fails with an error such as `certificate verify failed`, `self-signed certificate in certificate chain` or `unable to get local issuer certificate`, the network you're on may inspect TLS. Ask your IT team for the CA's certificate, or export it from this machine's store, such as Keychain Access on macOS. Put it in `~/.config/devvm/ca` and open the VM again ([a company's CA](settings.md#a-companys-ca)).

`devvm doctor` says so when `HTTPS_PROXY` or `config.toml`'s `proxy.https` is set and the folder has no CA certificates. Many proxies don't inspect TLS, so that line isn't a problem in itself. When devvm refuses a file in the folder, it names the file, and you take out anything that isn't a CA certificate, such as a private key or a server's certificate.

## A listed port isn't forwarded

When a program on this machine already has a port that the VM lists, devvm warns that the VM doesn't forward it, and the VM runs on without it. Stop that program, then run `devvm restart`, or change the VM's ports with `devvm ports`, and devvm forwards the port once it's free ([ports](ports-size.md#ports)).

## A VM won't boot

A Lima VM that doesn't answer over ssh within a minute of starting is stopped. Read its console log before you start it again, because every start overwrites the log. The log is `serialv.log`, or another `serial*.log`, in the VM's folder in devvm's Lima home, which is `~/.local/share/devvm/lima/<vm>/` unless you set `DEVVM_LIMA_HOME` ([Lima's limits](../backends/lima.md#limits)).

## A VM whose creation didn't finish

When devvm stops after it made a VM but before the VM was ready, such as when you press Ctrl+C at the wrong moment, devvm never starts that VM. The VM never opened a shell, so nothing of yours is in it. In a terminal, devvm asks whether to delete the VM and make it again, and the answer is no unless you type y. Without a terminal, with `--no-input` or when you answer no, devvm keeps the VM and says so, and you run `devvm rm` to delete it, then `devvm` to make it again.

## The folder is gone

When you delete or move a project folder, its VM stays and keeps its disk, and `devvm ls` writes `(gone)` after its folder. To keep the VM, move the folder back to its path. To delete it, run `devvm rm VM` with the name that `devvm ls` lists, in any folder. devvm asks first, and when the VM is stopped and has an agent, it needs `--no-export`, because starting the VM to export the history would make the folder again ([a VM whose folder is gone](../getting-started/commands.md#a-vm-whose-folder-is-gone)).

## The folder is on a disk that isn't connected

When a project folder is on an external disk, a USB stick or a network share that isn't connected, `devvm ls` writes `(not connected)` after its folder, and `devvm space` names the VM. Connect the disk and the VM works as before. If you mean to delete the VM, run `devvm rm VM` in a terminal and answer yes, because devvm refuses without one, even with `--force`, as the disk may still hold the project's work ([a folder on a disk that isn't connected](../getting-started/commands.md#a-folder-on-a-disk-that-isnt-connected)).

## The folder was made again

When you delete a project folder and make a new one at the same path, such as by cloning another repo there, devvm refuses the old folder's VM. Run `devvm rm` to delete the old VM, or `devvm adopt` to give it to the new folder ([a folder made again](../getting-started/commands.md#a-folder-made-again)).

## An outdated VM, or newer shared tools

devvm warns on each launch when an image the VM was made from has changed, and when there are newer shared tools than the VM's, and it leaves the VM as it is. Both warnings name `devvm update`, which moves the VM to the newest shared tools first, without asking. Then, only when the VM's image has changed, it makes the VM again from the new image, keeping its home folder, the project folder and its settings, after asking ([outdated VMs](images.md#outdated-vms)). `devvm tools update` moves the VM to the newest shared tools alone ([adding and removing tools](toolchains.md#adding-and-removing-tools)).

## A download doesn't check out

devvm checks Ubuntu's cloud image and its own mise against checksums before it uses them, and it refuses a download that doesn't match. Ubuntu's checksums must also carry a valid signature from Ubuntu's cloud image key. When devvm refuses one, try again later, and if it keeps failing, the mirror or the network between may be serving something else. Checking the signature needs `gpgv`, so without it devvm checks only the pinned checksum and says so, and `brew install gnupg` or `apt install gpgv` adds it ([Ubuntu's cloud image](../internals/images.md#ubuntus-cloud-image)).

## A new tool release isn't there yet

A shared tool's newest release is sometimes not published for every platform yet, such as for a few days after a Node release. devvm then uses the newest older version that is, and says so, and the next daily check tries the newest again. When a release has no download for a VM's platform yet, try again the next day ([versions](../internals/images.md#versions)).

## A shared tool doesn't mount

When a VM can't mount one of its shared tools, devvm stops the launch with an error that names the VM and the tools, and the VM keeps running. That happens when the tool's image is missing from the folder that devvm keeps the shared tools in, such as after you deleted it by hand, or when the VM can't mount the image. devvm doesn't let the VM download the tool in its place, because that copy would be the VM's own, a download you didn't ask for, and one that network policy may block.

Run `devvm tools update` in the project folder. It builds the tools that are missing and then opens the shell. When the VM has the newest versions, devvm builds those, and otherwise it builds the newest versions and moves the VM to them. If the launch still stops, the run log says why the VM couldn't mount the image ([a command fails](#a-command-fails)).

A tool that doesn't mount never keeps the VM from booting. The VM mounts the tools it can and boots as usual, and only devvm's launch stops ([the share](../internals/images.md#the-share)).
