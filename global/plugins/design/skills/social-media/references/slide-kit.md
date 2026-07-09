# Slide kit — the HTML/CSS system for fixed-size social assets

Every social asset is a fixed-pixel HTML page. No responsive breakpoints, no fluid units for the frame itself — the body IS the canvas. This file is the construction kit; artifact mechanics (validation, `data-canvas-id` rules, sync) are the `canvas` skill's contract and apply unchanged.

## Boilerplate

One file per slide (presentation) or `index.html` (web_page). Substitute the export pixels from `references/formats.md`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>IG carousel — launch — slide 1</title>
    <style>
      :root {
        /* tokens from `canvas brands view` — exact values, not approximations */
        --bg: #0A0B0D;
        --fg: #FCFCFD;
        --muted: #A6A8AC;
        --border: #2A2C2F;
        --accent-a: #0171FF;
        --accent-b: #00DFFC;
        --grad: linear-gradient(100deg, var(--accent-a) 10%, var(--accent-b) 90%);
        --font-display: 'Bricolage Grotesque', sans-serif;
        --font-mono: 'Geist Mono', monospace;
      }
      * { margin: 0; padding: 0; box-sizing: border-box; }
      html, body {
        width: 1080px;
        height: 1350px;
        overflow: hidden;
        background: var(--bg);
        color: var(--fg);
        font-family: var(--font-display);
        -webkit-font-smoothing: antialiased;
      }
      main {
        position: relative;
        width: 100%;
        height: 100%;
        background-color: var(--bg); /* paints while the image loads and behind transparent edges */
        background-image:
          linear-gradient(180deg, rgba(10, 11, 13, 0.35) 0%, rgba(10, 11, 13, 0) 40%), /* scrim: blends image into --bg */
          url('https://…/generated-backdrop.png'); /* the role's public_url */
        background-size: cover;
        background-position: center;
      }
    </style>
  </head>
  <body>
    <main data-canvas-id="slide-01-hook">
      <header data-canvas-id="slide-01-brand"><!-- brand chrome --></header>
      <section data-canvas-id="slide-01-copy"><!-- the message --></section>
      <footer data-canvas-id="slide-01-cue"><!-- swipe cue / handle --></footer>
    </main>
  </body>
</html>
```

Rules baked into this shape:

- `html, body` fixed to export pixels, `overflow: hidden`. Everything inside is positioned absolutely or flowed within that frame.
- Backgrounds are LAYERED in one declaration: scrim gradient(s) first, then the generated image, with `background-color` as the loading/fallback ground. The scrim is how the image's slightly-drifted background meets the exact brand `--bg` without a visible seam.
- Semantic structure and `data-canvas-id` on `main` + every `section`/`article`/`figure`/page-level `header`/`footer`, per `canvas` validation. Prefix IDs with the slide (`slide-03-steps`) so comments are unambiguous across the deck.
- One `<h1>` per slide — the slide's single message.

## Brand tokens

Populate `:root` from `canvas brands view <brand-id>` — colors, fonts, radius. Prompt hexes were *intent*; these are *truth*. Never invent one-off colors per slide: if a slide needs a new tint, derive it (`color-mix(in oklab, var(--accent-a) 20%, var(--bg))`).

Fonts: remote font URLs are allowed when intentional and stable (per the `canvas` skill). Load the brand's faces once per file:

```html
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,300..800&family=Geist+Mono:wght@400;500&display=swap" />
```

If the brand font isn't on a stable CDN, fall back to the closest system stack and note the substitution in your report.

## Type scale (at 1080px frame width)

Scale linearly for other widths (×1.48 for 1600px, ×0.93 for 1000px):

| Role | Size | Weight | Tracking | Notes |
|---|---|---|---|---|
| Display (hook headline) | 110–130px | 550–650 | -0.035em | line-height 0.98–1.02; 3–6 words max |
| Section headline | 84–96px | 550 | -0.03em | middle-slide h1s |
| Big step word / stat | 100–120px | 550 | -0.035em | the `recipes.md` step/stat pattern |
| Row/item title | 42–48px | 600 | -0.02em | list rows |
| Body / description | 28–34px | 400 | normal | line-height 1.4–1.5; ≤ 42ch |
| Mono label (eyebrow, index, footer) | 22–24px | 400–500 | +0.14em | UPPERCASE, `--font-mono` |
| CTA pill | 32–36px | 650 | -0.01em | |

Margins: 80px side gutters, 72–96px top/bottom. Vertical structure comes from a hairline system (`1px solid var(--border)`) between repeated rows — not from cards. Watch italic/descender clipping on display type; give display lines `padding-bottom: 0.06em` when descenders sit on a hairline.

Gradient text — spend it on ONE span per slide (the payoff word), never whole paragraphs:

```css
.grad-text {
  background: var(--grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
```

## Brand chrome

Consistent across every slide of a set — chrome is what makes slide 7 recognizably the same post as slide 1:

- **Logo lockup**, top-left: brand mark at 52–60px + wordmark. Use the brand's real logo asset URL from `canvas brands view` — never a redrawn approximation, never a generated logo.
- **Slide index**, top-right, mono label: `03 / 10`. Only on multi-slide sets.
- **Footer**, bottom (left or right per layout): domain or @handle as a mono label.
- **Swipe cue**, hook slide only: `swipe →` mono label, bottom-left.

Keep chrome positions identical across slides — pin them with the same absolute coordinates in every file (a shared `<style>` block copied per file, or a `shared.css` listed in the manifest `files` and linked relatively).

## Text over image energy

Text belongs in the backdrop's quiet zone by plan. Where copy must approach an energy zone:

- Prefer moving/shrinking the text block or re-anchoring the image (`background-position`) over darkening the whole image.
- A local `text-shadow: 0 2px 28px rgba(<bg-rgb>, 0.9)` buys ~1 line of safety at body sizes.
- A directional scrim (`linear-gradient` toward the energy) buys more, but flattens the image — keep it under 50% opacity.
- Contrast bar: body copy over image regions must stay clearly readable at a glance on a phone — when in doubt, screenshot at viewport size and squint-test; if you must inspect to read it, fix it.

## Vector accents (optional layer)

For the vector-accent style (`references/styles.md`): `hivy_generate_vector_image` produces SVG decoration (confetti, sparkle clusters, badges). Reference the returned `public_url` in an `<img>` for one-off placement, and request the exact brand hexes in the prompt so no recoloring is needed. Position absolutely; keep decoration OUT of the text column.

## Per-slide discipline

- One slide, one message. If a slide needs two headlines, it is two slides.
- Real copy only — no lorem, no fake metrics, no invented testimonials (per `design-taste`).
- Copy is design material: hooks are 3–6 words; body lines break where the *sense* breaks (author the `<br>`s on display type); CTAs say exactly what happens.
- Vary the layout system across a set (hook: display type; content: rows; steps: big numbered words; CTA: pill) — but never vary the chrome, tokens, or type scale.
