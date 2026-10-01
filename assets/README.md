# Images

The SVG files are the source. The PNGs are rendered from them and committed, because
GitHub renders a PNG identically for everyone, where an SVG depends on the fonts the
reader happens to have.

| File | Used by | Size |
|---|---|---|
| `banner.svg` → `banner.png` | the README header | 1040×300, rendered at 2× |
| `social-preview.svg` → `../.github/social-preview.png` | the repository's social preview | 1200×630 |

## Re-rendering

```bash
rsvg-convert -w 2080 -h 600 assets/banner.svg         -o assets/banner.png
rsvg-convert -w 1200 -h 630 assets/social-preview.svg -o .github/social-preview.png
```

Any SVG rasteriser does: `inkscape --export-type=png`, `magick`, or a headless browser.
The banner renders at twice its nominal size so it stays sharp on a high-density display.

## Setting the social preview

`.github/social-preview.png` is only a file in the repository; committing it changes
nothing on its own. GitHub exposes no API for the social preview, so it is uploaded by
hand: **Settings → General → Social preview → Edit → Upload an image**.

To check which image is live:

```bash
gh api graphql -f query='{repository(owner:"markosAMO",name:"Alfred"){openGraphImageUrl usesCustomOpenGraphImage}}'
```

## Fonts

The SVGs name font stacks rather than a single family, so a missing font degrades to the
next one instead of to a default. Nothing is embedded, and no file is fetched at render
time.
