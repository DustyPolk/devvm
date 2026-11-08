# devvm 0.1.0's state

This folder holds the state that devvm 0.1.0 left, made by 0.1.0 itself, so that the tests can check that today's devvm still reads it and upgrades it correctly. It is frozen. Never regenerate it from a newer devvm, and never edit its files. A newer release whose state differs gets its own folder, such as `testdata/v0.2.0/`.

`generate.sh` made it. The script checks out the `v0.1.0` tag in a temporary folder, sets the build's version to `0.1.0` as GoReleaser does, and runs `generate.txtar` with that version's own CLI test harness, which gives it a sandbox, the stubs and a test build. The script then copies what the run left into a folder named for the system it ran on: `darwin/` was made on an arm64 Mac and `linux/` on x86_64 Fedora. Each folder has these:

- `data/` and `config/`: devvm's data and config folders, without the run logs. They hold four VMs' states and run folders, `settings.json` and `config.toml`, the agent store and its logins, the image's and the shared tools' records, the ssh config and both layout `version` files. On Linux they also hold the github secret's file, which a Mac keeps in the Keychain.
- `lima/` and `stub/state.json`: the stubs' record of the VMs, so today's stubs know them, and on a Mac the stub Keychain's items. The VM's own files are left out, because the `--clone` VM's clone has a `.git` folder, which git can't keep in this repo.
- `json/`: what 0.1.0's `--json` printed for each command that has it.
- `root`: the sandbox's folder, as given and physical, which `fixture` in the CLI tests replaces.
- `modes`: each file's and folder's mode, which git doesn't keep. Git doesn't keep empty folders either, so the two empty ones 0.1.0 left in its run folder, `build-ports` and `builds`, are listed there but missing, and devvm makes them when it needs them.

The VMs are these, all stopped:

| VM | What it has |
| --- | --- |
| app | a git repo, node, Claude Code and Codex, the allowlist tier, ports 3000 and 8080, `.git` protection on (off on Linux, which 0.1.0 couldn't protect), the github secret, a link to db, the group backend |
| db | the allowlist tier, port 5432 published, the group backend |
| web | `--clone`, the open tier, go and Claude Code |
| scratch | the none tier, `.git` protection off, Claude Code |

Every token and login in it is a fake, such as `ghp_testfakev010` and `sk-testfake-codex`, and each VM's CA keys were made for the run.

To make a system's folder again, which only ever replaces it with the same version's state, run `testdata/v0.1.0/generate.sh` from the repo on that system.
