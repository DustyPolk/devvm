---
title: devvm secret set
explainer: "Save a secret on this machine"
---

Save a secret on this machine

## Synopsis

Save a secret on this machine: claude for the Claude Code token that claude setup-token prints, github for a GitHub token, proxy for the password of the company proxy that config.toml names, or one of your own, which needs --host and --env the first time and keeps what it has of them after. devvm asks for it without showing it, and --stdin reads a line instead. Proxies that are running put a new value in at once, and a VM's proxy reads proxy as it starts.

```text
devvm secret set NAME [flags]
```

## Examples

```text
  devvm secret set claude                                                          # save the Claude Code token, asking for it
  gh auth token | devvm secret set github --stdin                                  # save GitHub CLI's token without asking
  devvm secret set stripe --host api.stripe.com --env STRIPE_API_KEY --prefix sk_  # save a Stripe key, sent to api.stripe.com alone
  devvm secret set openrouter --host openrouter.ai --env OPENROUTER_API_KEY        # save an API key, sent as Authorization: Bearer
  devvm secret set stripe                                                          # save a new value for a secret of yours, keeping its hosts
  devvm secret set proxy                                                           # save the company proxy's password, asking for it
```

## Options

```text
      --basic-user      let a secret of your own go in as the username of Basic credentials, as Stripe takes a key
      --env VARIABLE    the VARIABLE that holds the placeholder of a secret of your own in VMs, such as STRIPE_API_KEY
      --header HEADER   the HEADER that a secret of your own goes in, such as X-Api-Key (default: Authorization)
  -h, --help            help for set
      --host HOST       a HOST that a secret of your own goes to, such as api.example.com: repeat it, or join hosts with commas
      --path PATH       the PATH below which a secret of your own goes in, such as /api/, or none for every path
      --prefix PREFIX   the PREFIX that the placeholder of a secret of your own starts with, such as sk_, for a client that checks how a key starts
      --scheme SCHEME   the SCHEME of a secret of your own in Authorization besides Bearer and Token, such as ApiKey, or none
      --stdin           read the secret from a line of input, without asking
```

## Options inherited from parent commands

```text
      --no-input   never ask, and fail where devvm would ask, naming the flag that answers it
  -v, --verbose    show the output of the commands devvm runs instead of a status line
```

## Docs

[The docs page for devvm secret set](../safety/network.md#saving-and-binding-secrets) explains it in full.

## See also

- [devvm secret](devvm_secret.md): List the secrets kept on this machine, for VMs to use
