---
title: Troubleshooting
weight: 10
---

This page explains where devvm logs what it runs and what happens when a run is interrupted or a build fails.

## Build logs and interrupts

The output of the commands devvm runs goes to a log instead of the terminal. devvm prints its own steps, and it prints the end of the log when a step fails. `DEVVM_VERBOSE=1` prints the commands' output instead.

Each run that runs a command has one log, `~/.local/share/devvm/logs/runs/<date>-<time>-<pid>.log`, which only you can read, and devvm keeps the newest 20. Each command appears as a `+` line with its arguments, followed by its output. A numbered step adds a `# [2/5] <step>` line first. A VM's proxy key is written as `devvm:<key>@`.

In a terminal, a status line shows the step, how long it has run, and the log's latest line. A step that takes more than a few seconds leaves a line with its time.

Ctrl+C, or closing the terminal, stops the command that's running, cleans up what the run left unfinished, and exits with code 130. A second Ctrl+C doesn't stop the cleanup.

devvm deletes an interrupted new VM, with its state, until its first shell opens, because nothing of yours is in it yet.

An interrupted build deletes the image it was building. So does a build whose create or clone fails, once devvm has shown the end of the log, because there's nothing in the image to look at.

A build that fails later, in its script or as the VM starts or stops, keeps the image's VM, stopped and without a record, so you can look inside. devvm's error names the VM and says how to open a shell in it with [`devvm limactl`](../backends/lima.md#running-limactl):

```sh
devvm limactl shell --start devvm-img-<key>
```

`--start` starts the VM first, outside the build's confinement. The next build of that image deletes the VM and starts over, and `devvm gc` also deletes it.

Either way, the images under it, which are already built, stay. devvm deletes and builds again any image it finds without its record, such as a failed build's or one devvm couldn't clean up after a crash or a power cut.
