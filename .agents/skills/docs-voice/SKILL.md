---
name: docs-voice
description: Rewrites devvm's prose (the docs site, READMEs, command help) into a plain, easy-to-follow guide, finding and fixing compressed writing and AI writing tropes without losing any fact. Use when writing or editing docs, or when asked to clean up the wording, voice or readability of a page.
---

# Docs voice

devvm's docs should read like a guide a colleague wrote: full sentences that say what devvm does and what you do, in the order you'd need them. Every fact stays, and only the wording changes.

[tropes.md](tropes.md) lists what to fix, with examples. [scan.sh](scan.sh) flags candidates, and [facts.sh](facts.sh) checks that a rewrite kept every fact.

## The voice

- **Full sentences.** Each one has a subject, a verb and its articles. Name the actor: devvm, you, the VM, the proxy.
- **Joined, not chopped.** Connect related facts with "and", "but", "so", "because", "when" or "if". Vary sentence length, and avoid runs of sentences under eight words.
- **One idea per sentence.** A sentence that needs a colon, a semicolon and a parenthetical holds two or three ideas, so split it.
- **What it does first.** Describe the behavior, then its limits and exceptions. Use negatives for guarantees.
- **The reader's order.** Lead a page or section with what the reader wants to do, then how, then the details and edge cases.
- **Exact.** Keep every flag, path, command, message, number and limit. Plain wording never means vaguer wording.
- **Present tense, current state.** Describe how devvm works now. Don't mention older behavior, what changed or what moved.
- **House rules.** Write commands as devvm x or in backticks, never in quotes. Never write "please", "note that" or "simply". Use sentence-case headings. Address the reader as "you".

## Scope

All prose in the repo:

- `docs/content/**/*.md`, except `docs/content/docs/cli/`, which is generated.
- The command help that generates those CLI pages: `Short` and `Long` in `internal/cli/*.go`. Run `mise run cli-docs` after editing them.
- `README.md`, `SECURITY.md`, `CONTRIBUTING.md`, `assets/oci/README.md` and `internal/proxy/README.md`.

These follow their own rules and are out of scope:

- Messages in `locales/en.toml`, which follow AGENTS.md's Messages rules.
- `AGENTS.md`, which stays short.
- `.agents/skills/security-audit/`, which is vendored.

## Workflow

Work on one page at a time.

1. **Scan.** `.agents/skills/docs-voice/scan.sh FILE` shows the page's counts, and `scan.sh -v FILE` (or `-p NAME`) shows each hit. Run it on every in-scope file to choose the densest page first.
2. **Read the whole page** before changing it, together with the pages it links to and the pages that link to its headings (`grep -rn 'page.md#' docs/content internal/msg/docs.go`).
3. **Rewrite** paragraph by paragraph with the Edit tool, following the voice and [tropes.md](tropes.md). Restructure a section when its order is the problem, but keep its headings: other pages, `internal/msg/docs.go` and messages link to them. Rename a heading only when you update every link to it.
4. **Check the facts.** `.agents/skills/docs-voice/facts.sh FILE` lists the code spans, links, headings and numbers that changed. Each change must be one you meant. When a change moves text between pages, run `facts.sh --all REV` instead, which compares the whole docs tree at `REV` with your working tree, so a fact that moved to another page doesn't show as removed. It counts each fact's copies and ignores headings and link targets, because pages and their anchors move. Then reread the old and new versions side by side (`git diff --word-diff FILE`) for meaning: conditions, who does what, "only" and "never".
5. **Rescan.** The counts should drop. Hits that remain are allowed by [tropes.md](tropes.md)'s "Not tropes".
6. **Gate.** Run `mise run fmt && mise run lint && mise run test`. Vale's `devvm.AIWords` rule fails on chatbot vocabulary, and `TestDocLinks` fails on a broken message link.
