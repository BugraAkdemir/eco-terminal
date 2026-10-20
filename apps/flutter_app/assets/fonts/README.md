# Bundled typography

Source: https://github.com/google/fonts/tree/main/ofl (downloaded 2026-10-20).
Upstream families: `fraunces`, `atkinsonhyperlegiblenext`, `ibmplexmono`.
Original binaries are unmodified; every family directory includes its upstream `OFL.txt`.
Flutter aliases: Fraunces, AtkinsonHyperlegible, IBMPlexMono.
Atkinson Hyperlegible Next replaces the original version because the original lacks U+0130 (İ).
Fraunces and Atkinson Next are variable fonts; declared weights map to the same binaries.

## Turkish glyph coverage

Checked every binary with `fc-query --format=%{charset}`; all contain ğĞşŞıİöÖçÇüÜ.
This verifies cmap coverage; rendered text is checked separately during visual QA.

## SHA-256

```
ce9cffed32742ad2d9238c561a93220385e5934cdc02b8eb4097a50efa957dc6  atkinson/AtkinsonHyperlegibleNext-Italic-Variable.ttf
5a455d1cfa099b601ab70751bb9673e8fe1854dc4500c80e1a220d0d75e31745  atkinson/AtkinsonHyperlegibleNext-Variable.ttf
b24448c43702fac4ee856781d461a0dfba8d8e594b6e8e190234b75fed2c0e01  fraunces/Fraunces-Italic-Variable.ttf
177ff6c0f14e5550a3c624247cd1189611d4eb65d000b14944c63d967958abbb  fraunces/Fraunces-Variable.ttf
ac27abd6450a64dd94467580a02fe6235156d5b92f2926ebbc8e7489df64e0be  plex/IBMPlexMono-Bold.ttf
af4e05a761e98c1adf064c48a6352c9bec1a6ad70982cd2a544149323391f98e  plex/IBMPlexMono-BoldItalic.ttf
3362fc791b0652193328b862c1c5f23a789bc7288b1617fa63302f88689a2a34  plex/IBMPlexMono-Italic.ttf
6a3412f058c7d8dfd9170c41e85ade48e5156ecb89356110ca57a0a27734af46  plex/IBMPlexMono-Regular.ttf
```
