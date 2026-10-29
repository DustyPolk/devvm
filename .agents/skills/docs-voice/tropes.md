# Tropes

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

Allow one short parenthetical per sentence at most, for an example or a literal value (`(macOS)`, `(`.env`, `node_modules`)`). An aside that's a clause, holds a colon or changes the meaning goes into the sentence, or into a sentence of its own.

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

## Not tropes

Leave these alone:

- Transition words used for their meaning ("so", "but", "then", "also").
- Clean grammar, plain or formal register, and long sentences that read easily.
- Precise lists of flags, paths, messages and limits: that precision is the content.
- Colons before lists, code, examples, and in "**Term:** definition" reference lists.
- Negatives that state a guarantee or a limit the reader must know.
