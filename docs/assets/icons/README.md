# Icons

The docs site's pixel icons, which `{{< icon "name" >}}` inlines into a page.

Every icon here except three comes from [Dinkie Icons](https://github.com/atelier-anchor/dinkie-icons) 0.4.6, the version of the `@dinkie-icons/react` npm package that the path data was copied from. Dinkie Icons is MIT licensed, and its license, with its copyright notice, is in [LICENSE](LICENSE). Each file is named after its Dinkie icon in kebab case, without the size, so `DinkieIconRocket12` is `rocket.svg`.

`penguin.svg`, for Linux, `prompt.svg`, a terminal window with a `>_` prompt, and `warning.svg`, a triangle with an exclamation mark, are devvm's own. They're drawn in the same style as Dinkie's icons, with 1-pixel lines on the same grid.

Each icon is a 12 by 12 pixel grid (`viewBox="0 0 12 12"`) whose drawing is 11 pixels wide and tall, in the bottom left 11 by 11 pixels. It has one path filled with `currentColor` and `shape-rendering="crispEdges"`, so it takes the text's color and its pixel edges stay sharp.
