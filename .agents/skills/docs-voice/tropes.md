# Tropes

<!-- Each example's before and after are two quotes, one after the other. -->
<!-- markdownlint-disable MD028 -->

What to look for, with what `scan.sh` calls it. Each has a before and after from devvm's own docs. A hit is a candidate: fix it only if the sentence reads better without it.

## Compressed writing

These are the common ones in devvm's docs: prose packed so tight that the reader has to unpack it.

### Clauses chained with colons (`colon`)

A colon that joins two clauses makes the reader guess the relation: cause, example, consequence or definition. Say which one it is. Keep colons that introduce a list, a code example or a definition list's value.

> A VM keeps its choice: it's in its state, `net=on` or `net=off`.

> A VM keeps its choice, which devvm records in its state as `net=on` or `net=off`.

### Semicolons between clauses (`semicolon`)

Split them into two sentences, or join them with "and", "but" or "so" when one follows from the other.

> `devvm rm -f` skips the checks; it still keeps fetched branches.

> `devvm rm -f` skips these checks, but it still keeps the branches you fetched.

### Parentheticals (`paren`)

Allow one short parenthetical per sentence at most, for an example or a literal value (`(macOS)`, `` (`.env`, `node_modules`) ``). An aside that's a clause, holds a colon or changes the meaning goes into the sentence, or into a sentence of its own.

> A copy that fails (the VM can't be reached, say) is tried again the next time you run devvm in the folder.

> If the copy fails, for example because the VM can't be reached, devvm tries again the next time you run it in the folder.

### "say" as an aside (`say`)

"(a branch named `saved`, say)" is a clipped "for example". Write "for example" or "such as".

### Dropped articles and telegraphic fragments (`short`)

Sentences without their articles, subjects or verbs read like notes. Strings of short sentences read like a checklist. Write full sentences and join the closely related ones. Keep a short sentence when it lands a point.

> git on this machine. The VM's images have git.

> git installed on this machine. The VM's images already have it.

> Without a terminal to ask in, either one refuses, and the VM stays. Then devvm deletes the VM and…

> If there's no terminal to ask in, devvm refuses and keeps the VM. Otherwise it deletes the VM and…

### Inverted or noun-first sentences

Put the actor first and the action second: "devvm does X when Y", not "X is done by devvm on Y". Name who does it (devvm, you, the VM, the proxy) instead of a passive.

> Nothing else is copied: not tags, not uncommitted changes, not untracked or ignored files.

> devvm copies only your branches. Tags, uncommitted changes, and untracked or ignored files stay on this machine.

### Defining by negation (`negative`)

Say what something does before what it doesn't. Negatives belong in guarantees ("the VM never holds your tokens"), not in every description. Several in a row ("not X, not Y, nothing but Z") are a sign the sentence should be turned around.

> The VM mounts nothing from this machine but the shared tools and its agents' config folders.

> The VM mounts only the shared tools and its agents' config folders from this machine.

### "too" tacked on (`too`)

A trailing "too" makes the reader look back for what it adds to. Name the addition with "also", or merge the two facts into one sentence.

### Bold-label bullets (`boldlabel`)

`- **Label.** Explanation` lists are fine for reference material the reader scans: modes, flags, guarantees. When the bullets explain a sequence or build on each other, write paragraphs. Don't bold a whole sentence as the label.

### Undefined shorthand

Internal names (`net=`, "the share", "an overlay", "set-id files") used before the page says what they are. Define a term the first time a page uses it, or link to where it's defined.

## Chatbot tropes

From [Wikipedia: Signs of AI writing](https://en.wikipedia.org/wiki/Wikipedia:Signs_of_AI_writing). These are rare here, so don't introduce them while fixing the tropes above.

- **AI vocabulary** (`aiword`, Vale's `devvm.AIWords`): additionally, crucial, delve, enhance, foster, garner, intricate, landscape, meticulous, pivotal, robust, seamless, showcase, tapestry, testament, underscore, vibrant, leverage, streamline, effortless, comprehensive, powerful, essential, ensure. Use the plain word: "also", "important", "has", "use", "checks", or nothing.
- **Avoiding "is"** (`copula`): "serves as", "stands as", "acts as", "functions as", "boasts", "plays a key role". Write "is" or "has".
- **Negative parallelism** (`notbut`): "not just X but Y", "it's not X, it's Y", "Y rather than X" used to correct a misconception nobody had. State Y.
- **Trailing "-ing" phrases** (`ingtail`): ", ensuring…", ", allowing…", ", making it…" tacked on to add significance. Make it its own sentence with a subject, or drop it.
- **Significance and promotion**: "a key part of", "powerful", "effortlessly", "best-in-class", "out of the box". Say what it does.
- **Rule of three**: lists padded to three items for rhythm. List what's true, whether that's one item or five.
- **Elegant variation**: different names for the same thing in nearby sentences ("the VM", "the guest", "the sandbox", "the machine"). Pick one name and keep it.
- **False ranges**: "from X to Y" where X and Y aren't ends of a scale.
- **Summaries and closers** (`summary`): "In short", "Overall", "In other words", a closing paragraph that repeats the page. End when the content ends.
- **Hedges and filler** (`hedge`): "generally", "typically", "essentially", "simply", "just", "really", "of course". Keep "usually" when it's true and the exceptions are stated.
- **Em dashes** (`dash`): use a comma, parentheses or a new sentence.
- **Title Case Headings**: headings are sentence case.
- **Chat leftovers**: "Here's", "Let's", "I hope this helps", "Feel free to".

## Terms

- **A bare "token"** (`token`): agents use "tokens" for model tokens, so name the secret instead, such as "GitHub token", "Claude Code token" or "secrets". A hit on "GitHub token" or "Claude Code token" is fine. See [SKILL.md](SKILL.md#terms) for the terms.

## Messages

What `scan.sh` flags in `locales/en.toml`, besides the tropes above. Status steps, which stay a few words and lowercase, are allowed hits.

### Lowercase starts and missing full stops (`lower`, `end`)

A message is one or two full sentences, so it starts with a capital letter and ends with a full stop or, for a question, a question mark. devvm and a template field may start a sentence as they are.

> couldn't adopt this folder's VM: {{.reason}}

### Warnings and errors that start with devvm

devvm prints "devvm: " before every warning and error, so one that starts with devvm reads "devvm: devvm couldn't…". Lead with what failed or the thing it's about.

> devvm couldn't add the git remote {{.remote}}. Run devvm again to retry.

> Adding the git remote {{.remote}} failed. Run devvm again to retry.

### Clauses chained with colons, and the mechanism in the message

A colon or a semicolon between clauses packs a cause, a mechanism and an action into one line. Say what happened, then the one thing to do, and leave how devvm works to the message's doc link.

> {{.layer}} is a layer, installed into {{.vm}}'s disk, so it can't be removed in place: save your work outside the project folder, then devvm rm and create the VM again

> You can't remove a layer from a VM. To drop {{.layer}}, save your work outside the project folder, then run devvm rm and devvm again.

> network policy isn't enforced for {{.vm}}: devvm started it confined, but it reaches the internet without its proxy (a network devvm didn't set up?), so devvm stopped it

> devvm stopped {{.vm}} because it reached the internet without going through its proxy. Something outside devvm may have given it another network.

### Passive and noun-first messages

Name who did it, as on a page: devvm, you, the VM or something else.

> WARNING: {{.git}} was replaced since devvm locked it, maybe from inside the VM, and its config and hooks run on this machine: check them before running git here (and look for the old .git, renamed)

> WARNING: Something replaced {{.git}} after devvm locked it, possibly the VM. Check its hooks and config before you run git here.

### Claims that aren't exact

Shorter never means less accurate. A message says what devvm checks and what it doesn't, and how to check the rest.

> gpgv isn't installed, so only the HTTPS download vouches for Ubuntu's checksums: install it to check their signature (brew install gnupg, or apt install gpgv)

> gpgv isn't installed, so devvm can't check the signature on Ubuntu's checksums. devvm still checks the image against its pinned checksum. To check the signature too, run brew install gnupg, or apt install gpgv.

## Not tropes

Leave these alone:

- Transition words used for their meaning ("so", "but", "then", "also").
- Clean grammar, plain or formal register, and long sentences that read easily.
- Precise lists of flags, paths, messages and limits: that precision is the content.
- Colons before lists, code, examples, and in "**Term:** definition" reference lists.
- Negatives that state a guarantee or a limit the reader must know.
