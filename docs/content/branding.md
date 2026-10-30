---
title: Branding
layout: branding
description: "devvm's logo, icons, social card, colors and fonts, to download and use when you write about devvm."
sidebar:
  exclude: true
---

{{< hextra/hero-headline >}}
  Branding
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
  devvm's logo, icons, colors and fonts, ready to use when you write about devvm or link to it.
{{< /hextra/hero-subtitle >}}

## The logo

The logo is the name devvm after a lime prompt, in a box with a lock on its bottom-right corner. This version is made of box-drawing characters, so it lines up in any monospace font, and you can paste it into a terminal, a README or a code comment. The copy button at the top right of the block copies it.

```text
╭─────────╮
│ ❯ devvm │
│        ╭┤
╰────────██
```

The SVG versions are drawn on a pixel grid in Departure Mono, the site's pixel font, as the favicon is. Use the first one on dark backgrounds and the second one on light backgrounds, where the prompt is a darker lime so that it stays readable on white. Each one also comes as a PNG at four times its size, with a transparent background.

{{< brand-assets >}}
{{< brand-asset title="For dark backgrounds" src="branding/devvm-logo-on-dark.svg" bg="dark" width="250" height="86" alt="The devvm logo in light gray and lime" files="branding/devvm-logo-on-dark.svg branding/devvm-logo-on-dark.png" >}}
{{< brand-asset title="For light backgrounds" src="branding/devvm-logo-on-light.svg" bg="light" width="250" height="86" alt="The devvm logo in black and dark lime" files="branding/devvm-logo-on-light.svg branding/devvm-logo-on-light.png" >}}
{{< /brand-assets >}}

## Icons

The app icon is the logo drawn on a 16-pixel grid, with the box, the prompt and a small lock on black. Each PNG is the SVG at a whole multiple of its size, so its pixels stay sharp at any of them. The second set holds the icons that the site itself uses, for browser tabs, home screens and bookmarks.

{{< brand-assets >}}
{{< brand-asset title="App icon" src="branding/devvm-icon-128.png" bg="dark" width="128" height="128" alt="The devvm app icon" files="favicon.svg branding/devvm-icon-64.png branding/devvm-icon-128.png branding/devvm-icon-256.png branding/devvm-icon-512.png branding/devvm-icon-1024.png" >}}
{{< brand-asset title="The site's icons" src="apple-touch-icon.png" bg="dark" width="180" height="180" alt="The devvm icon with a black border, as a home screen shows it" files="favicon.ico favicon-16x16.png favicon-32x32.png apple-touch-icon.png android-chrome-192x192.png android-chrome-512x512.png" >}}
{{< /brand-assets >}}

## The social card

Sites and chat apps show the social card next to a link to devvm.dev. It has the logo and the home page's headline and tagline, in the site's colors.

{{< brand-assets wide="true" >}}
{{< brand-asset title="Social card, 1200 by 630" src="images/social-card.png" bg="dark" alt="The devvm social card: the logo, One VM for every project, and the tagline" files="images/social-card.png" >}}
{{< /brand-assets >}}

## Colors

The site is black with one lime accent, from the JustBlack theme for VS Code. The blue marks commands, and the red marks only what an attack has reached in the home page's drawings. Click a swatch to copy its hex.

{{< palette >}}

## Fonts

The site uses three fonts, and all of them are free under the SIL Open Font License.

<!-- markdownlint-disable MD034 -- the font-sample shortcodes' parameters, not bare URLs -->

{{< font-sample name="Departure Mono" font="pixel" link="https://departuremono.com/" license="fonts/departure-mono/LICENSE" >}}
A pixel font by Helena Zhang, which the site uses for the logo, the headings and the buttons. It's sharp at multiples of 11px and has one weight. The site serves its own copy of the font, together with its license.
{{< /font-sample >}}

{{< font-sample name="Instrument Sans" font="sans" link="https://fonts.google.com/specimen/Instrument+Sans" >}}
The font for the site's text, which comes from Google Fonts.
{{< /font-sample >}}

{{< font-sample name="Martian Mono" font="mono" link="https://fonts.google.com/specimen/Martian+Mono" >}}
The font for code, which also comes from Google Fonts. The site sets it narrower than its default and without ligatures, so that flags look as you type them.
{{< /font-sample >}}

<!-- markdownlint-enable MD034 -->

## Using the logo

- Write the name in lowercase, as devvm, even at the start of a sentence.
- Keep the lock with the box, because together they're the logo.
- Leave clear space around the logo, at least as wide as the lock.
- Use the colors that the logo comes in, with the version for your background. Don't recolor, stretch or outline it.

## Credits

The site's pixel icons come from [Dinkie Icons](https://github.com/atelier-anchor/dinkie-icons), which is MIT licensed. The site is built with [Hugo](https://gohugo.io) and the [Hextra](https://imfing.github.io/hextra/) theme.
