---
title: devvm build
weight: 2
---

Build an image and the images under it, unless they're built

## Synopsis

Build an image and the images under it, unless they're built. IMAGE is base, or your layers joined with +, bottom first (foo+bar is bar on foo on base).

```text
devvm build IMAGE [flags]
```

## Options

```text
  -h, --help   help for build
```

## See also

- [devvm](devvm.md): One isolated VM per project folder
