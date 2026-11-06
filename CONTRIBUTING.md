# Contributing

Building from source, the tools and tasks, the tests and writing docs: [Contributing](https://devvm.dev/docs/contributing/) on the docs site (source: [docs/content/docs/contributing/_index.md](docs/content/docs/contributing/_index.md)).

The rules for working in this repo, for people and coding agents alike: [AGENTS.md](AGENTS.md).

Before sending a change, run:

```sh
mise run fmt && mise run lint && mise run test
```

Pull requests are squash-merged, and the pull request's title becomes the commit's subject, so write it as a [Conventional Commit](https://www.conventionalcommits.org/), `type(scope): summary`, with a type of `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci` or `chore`. A check fails a title that doesn't follow it.
