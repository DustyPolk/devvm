<!-- markdownlint-disable-file MD041 -- a pull request's body, under GitHub's own title -->
## What this changes

<!-- What the change does and why. Link the issue it fixes, as in "Fixes #12". -->

## How it was tested

<!-- The tests you added or ran. For a change that touches VMs, the e2e sections you ran (mise run e2e -- --only SECTION) and on which OS. -->

## Checklist

- [ ] The title is a Conventional Commit, `type(scope): summary`, because it becomes the squashed commit's subject.
- [ ] `mise run fmt && mise run lint && mise run test` passes.
- [ ] New behaviour has tests, and every behaviour change updates its page in `docs/content/docs/`.
- [ ] A changed command follows the cli skill, and `mise run cli-docs` is rerun when its help changed.
- [ ] Nothing here belongs in a private security advisory instead (see SECURITY.md).
