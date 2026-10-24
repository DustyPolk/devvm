---
title: Coding agents
weight: 22
---

When a VM's image has `claude`, `codex` or `opencode`, the VM gets your config for that agent from this machine, but never your history: transcripts, sessions and databases stay where they are, and each VM's own stay in that VM.

| | Mounted read-only, linked into `~` | Copied in on every launch |
| --- | --- | --- |
| claude | `~/.claude/{skills,agents,commands,output-styles,plugins}` | `~/.claude/settings.json`, `~/.claude/CLAUDE.md`, `~/.claude.json` ([merged](#mcp-servers), without per-project entries) |
| codex | `~/.codex/{prompts,skills}` | `~/.codex/{config.toml,AGENTS.md,auth.json}` |
| opencode | the subfolders of `~/.config/opencode` (not `node_modules`) | the files in `~/.config/opencode` (not lock files), `~/.local/share/opencode/auth.json` |

## Mounts

The folders that exist when devvm creates the VM are mounted read-only, at the same path as on this machine, so absolute paths in them (Claude Code's plugin paths) still work, and linked from the VM's home. This machine enforces read-only: even root in the VM can't write to them. A folder whose path has a comma isn't mounted, with a warning ([Lima's limits](lima.md#limits)).

## Copies

Files are copied into the VM's home on every launch, over the VM's own. With [network policy](network.md#agents-logins-and-settings), the logins and settings files are copies with your keys and tokens taken out.

### Claude Code login

Claude Code's login on macOS is in the Keychain and can't be shared, so VMs use a long-lived token instead: run `claude setup-token`, and devvm puts it in the VM's environment as `CLAUDE_CODE_OAUTH_TOKEN`. devvm reads it from its own environment (`CLAUDE_CODE_OAUTH_TOKEN`), else from where it keeps [secrets](network.md#secrets). A token that isn't a setup token (`sk-ant-oat01-…`) isn't sent. With network policy the VM gets a placeholder of its own, and its proxy puts the token into requests; without it, the token itself. A `~/.claude/.credentials.json` is never copied.

### Codex and opencode logins

With network policy the VM's `auth.json` holds placeholders, and its proxy holds the real keys and tokens. Without it, the VM gets the file as it is, and a VM that refreshes an OAuth login can rotate its refresh token and sign this machine out (an API key avoids this).

### MCP servers

On each launch the VM's `~/.claude.json` gets this machine's settings and MCP servers, merged into its own: each of your servers replaces the VM's server by that name whole, servers devvm copied before that you no longer have are removed, and servers added in the VM stay. The VM keeps its own per-project entries (trust, allowed tools); yours never go in. Without network policy, the servers' env values and headers are copied as they are, and devvm names the servers that have them.

### Keys in settings files

With network policy, the VM's copies of `~/.claude/settings.json`, `~/.codex/config.toml` and opencode's `.json`, `.jsonc` and `.toml` files have every value under a key that looks like a secret's (`*key*`, `*token*`, `*secret*`, `*password*`, `Authorization`) replaced with `devvm-redacted`, and devvm names what it replaced. Values that only point at a secret stay: an environment variable's name (`env_key = "OPENAI_API_KEY"`), `{env:X}`, `${X}`, a path. A file devvm can't read that way (a TOML multi-line string) isn't copied. An MCP server or provider that needs one of those keys in the VM has to get it another way.

## The VM's setup

Besides the agents' config, every launch sets the VM up from this machine's settings:

- its hostname: the folder's part of the VM's name;
- the timezone (`TZ`, else this machine's) and, when it's UTF-8, the locale (`LC_ALL`, else `LANG`);
- git's `user.name` and `user.email`, as git has them in the project folder;
- telemetry off where an environment variable can't do it: Go's, and Codex's analytics and metrics;
- `(devvm)` at the start of the prompt.

With network policy it also installs devvm's CA, points apt, sudo, ssh and Docker at the proxy, adds `devvm-why` (what the proxy blocked lately), and tells the agents, in `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, why a request may fail ([Inside the VM](network.md#inside-the-vm)). Tokens go in `~/.config/devvm/env`, which login shells read.
