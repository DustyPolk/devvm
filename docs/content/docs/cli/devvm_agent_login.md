---
title: devvm agent login
weight: 5
---

Log an agent in, in this folder's VM, and save the login in devvm's store

## Synopsis

Log AGENT, codex or opencode, in to ChatGPT inside this folder's VM, in a browser on this machine, or with a code with --device-code, or save an API key with --api-key. devvm's store keeps the login, so every VM with the agent gets it, and the VMs get only placeholders.

```text
devvm agent login AGENT [flags]
```

## Examples

```text
  devvm agent login codex                                    # log Codex in to ChatGPT in a browser on this machine
  devvm agent login codex --device-code                      # log in with a code, in a browser on any machine
  devvm agent login opencode --api-key --provider anthropic  # save opencode's Anthropic API key, asking for it
```

## Options

```text
      --api-key             save an API key instead, asking for it
      --device-code         log in with a code to type into a browser on any machine, so nothing is forwarded
  -h, --help                help for login
      --here                use this folder's own VM, never that of a folder it's inside
      --parent              use the VM of the nearest folder this one is inside, even across a git repo of its own
      --provider PROVIDER   with opencode's --api-key, the PROVIDER it's for: anthropic, openai or openrouter
      --stdin               read the secret from a line of input, without asking
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm agent login](../using/coding-agents.md#logging-in-with-devvm-agent-login) explains it in full.

## See also

- [devvm agent](devvm_agent.md): Fill devvm's store of the agents' settings and logins that every VM gets
