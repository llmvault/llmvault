---
name: logo-design
description: Use when a user asks to design, create, generate, redesign, refine, or explore a logo, wordmark, monogram, brand mark, symbol, lockup, app icon, favicon, or logo variants — or to turn a raster logo into a clean SVG. This skill produces a complete, production-ready logo pack: one generated vector symbol becomes a full set of lockups, app icon, monochrome, reverse, and favicon assets, delivered as scalable SVGs and transparent PNGs at every size. It also teaches logo judgment (mark architecture, typography, restraint, small-size discipline) for exploring and presenting directions.
---

# Logo design: generate one mark, deliver a whole logo pack

This skill turns a brand into a **complete logo pack** the user can actually use:
a symbol, horizontal and stacked lockups, an app icon, monochrome and reverse
versions, and a favicon — each as an infinitely scalable **SVG** and a set of
transparent **PNGs**. The heavy lifting is done by two bundled scripts, so you
mostly gather inputs, generate a mark, and run commands.

## The one principle that makes this work

**Generate the symbol ONCE. Build everything else from it — never regenerate it.**

Image generation is creative: every call redraws from scratch and drifts, so
generating "the mark", then "the mark inside a lockup", then "the mark as an app
icon" would give you three *different* marks. That is unacceptable for a logo.

Because the output is SVG (editable vector), the `logo_pack.mjs` script reuses the
**exact same `<path>` geometry** everywhere — recoloring or repositioning it, never
redrawing — so the mark is byte-identical across the whole pack. The script prints
`GEOMETRY IDENTITY: PASS` to prove it. This is the only way to guarantee a
consistent mark.

You only ever make a fresh generation call to explore *different mark concepts*
before one is chosen. Once chosen, the mark is frozen.

---

## Procedure (follow exactly)

### 1. Gather the brand inputs

You need three things. If any are missing and would change the design, ask **one**
`request_user_input` with concrete options; otherwise proceed and state assumptions.

- **Brand name** — exact spelling and casing (e.g. `Meadowlark`, `blip`).
- **Brand color** — a hex value (e.g. `#3F7D4E`). If none given, pick a defensible
  one from the category/tone and say what you chose.
- **Vibe / category** — a few words (e.g. "organic farm", "Gen-Z chat app",
  "enterprise SaaS"). Drives the symbol idea and typographic register.

### 1b. Choose the right direction for the brand

Before writing any prompt, decide what *kind* of logo fits this brand — do not
default to one look. Read `references/brand-logo-playbook.md` and pick, for the
brand's category: the **architecture** (symbol+lockup, wordmark, lettermark, or
monogram), the **symbol approach**, the **typographic register**, and the **color
direction**. A dev tool, a farm, and a kids' app should not get the same treatment.

### 2. Generate 2–3 mark options

Call `hivy_generate_vector_image` **once per option** (2–3 calls), varying the idea
and/or style so the user has a real choice. Each call returns JSON with a
`public_url` and `drive_asset_id` — keep them. Set `type: logo`.

Choose the mark type from step 1b and prompt it precisely:

- **Pictorial symbol** — the default. Symbol only, **no text, no letters**. Use the
  template and validated examples in `references/mark-prompts.md`.
- **Lettermark / monogram / custom wordmark** — when the brand is best carried by
  type (short/distinctive name, premium, institutional). Describe the letterforms
  *specifically* — construction, weight, terminals, corners, one distinctive detail —
  and give exact hex colors. Vague prompts produce generic type; precise ones produce
  a designed mark. Full vocabulary, templates, and worked examples are in
  `references/type-and-color-prompts.md`.

Whatever the type, enforce: one subject, flat vector, one brand color (exact hex),
transparent background, centered, no background shapes.

### 3. Let the user choose the mark

You cannot reliably judge a mark visually — the **user** is the judge. Post each
option's `public_url` in your reply (the chat renders them inline), then use
`request_user_input` to ask which mark to build the pack from. (Only skip this if
the user explicitly wants a single fast result.)

If the user likes a candidate but wants a change ("this one, but thicker"), refine
it by calling the tool again with that mark's `drive_asset_id` in
`reference_asset_ids` plus a revised prompt — see "Refining a near-miss" in
`references/mark-prompts.md`. Repeat until they approve; only then is the mark
frozen and the pack built.

### 4. Download the chosen mark into the workspace

Use the chosen option's `public_url`. The URL redirects, so `-L` is required:

```bash
mkdir -p /workspace/logo
curl -fL --retry 3 --retry-all-errors --connect-timeout 10 --max-time 120 \
  -o /workspace/logo/mark.svg "<PUBLIC_URL_OF_CHOSEN_MARK>"
test -s /workspace/logo/mark.svg || { echo "download failed"; exit 1; }
```

### 5. Build the complete pack (one command)

```bash
node .skills/logo-design/scripts/logo_pack.mjs \
  --mark /workspace/logo/mark.svg \
  --name "BrandName" \
  --color "#3F7D4E" \
  --out /workspace/logo/pack
```

Optional flags: `--font "Georgia, serif"` (wordmark typeface; default is a clean
sans), `--weight 600`, `--wordmark /workspace/logo/wordmark.svg` (use a generated
custom wordmark instead of typeset text), `--recolor true` (force every non-white
fill to the exact `--color` hex — use whenever the brand color must be exact, and
always when combining a generated mark with a generated wordmark, since separate
generations never match hues), `--sizes 1024,512,256,128,64` (PNG ladder for
square assets).

The script automatically repairs two common generation defects: it **recrops the
viewBox** to the content bounding box (generated marks often arrive cropped or
off-center — never discard a mark for bad framing) and **flattens CSS class styles
to inline fills** (generated SVGs reuse the same class names; composing two without
flattening silently recolors or hides one).

**Confirm the script prints `GEOMETRY IDENTITY: PASS ✅`.** If it prints `FAIL`, the
mark SVG was malformed — regenerate the mark and retry. The pack lands in
`/workspace/logo/pack/` and contains, for each asset, an `.svg` plus transparent
`.png`s at multiple sizes:

- `mark` — the symbol alone
- `lockup-horizontal`, `lockup-stacked` — symbol + typeset wordmark
- `app-icon` — white mark on the brand-color rounded square
- `mono-black` — single-ink version
- `reverse-white` — for dark backgrounds
- `favicon` — square mark at 16–128px
- `manifest.json` — the asset list + mark id

### 6. Deliver the pack to the user (one command)

```bash
bash .skills/logo-design/scripts/deliver.sh /workspace/logo/pack logos/brandname
```

This uploads every file to the org drive and prints `"<relative-path>  <asset_url>"`
for each. (It uses the sandbox-injected `HIVY_DRIVE_UPLOAD_URL` /
`HIVY_DRIVE_UPLOAD_BEARER` — no setup needed.)

### 7. Present the result — link EVERY asset

Files are only visible to the user if their URL is in your reply. **You must provide
a link to every single asset in the pack — never a subset, never "and the rest."**
`deliver.sh` prints one `asset_url` per file; every one of those URLs goes in your
reply.

Structure the reply so it stays readable:

- **Show the hero previews inline** by pasting these three `asset_url`s on their own
  lines so the chat renders them: `lockup-horizontal-1000.png`, `app-icon-512.png`,
  `mark-512.png`.
- **Then list every remaining file** as a labeled link, grouped by asset (mark,
  lockups, app icon, monochrome, reverse, favicon), including each size and each
  `.svg`. Do not omit any — a logo pack is only useful if the user can reach the
  whole stock. If the list is long, group it under short headings; still include
  all URLs.
- Tell the user: **SVGs are the master files** (infinitely scalable, editable);
  **PNGs are transparent** and provided at each size for direct use.

Never describe a file you did not link. If a file was delivered, its URL is in the
reply.

---

## Bonus: turn an existing raster logo into an SVG

If the user uploads a PNG/JPEG logo (a drive asset), vectorize it instead of
generating:

1. Call `hivy_vectorize_image` with `reference_asset_ids: ["<the raster asset id>"]`.
   It returns a clean SVG (`public_url`).
2. `curl -fL` it to `/workspace/logo/mark.svg`, then run `logo_pack.mjs` from step 5
   to produce the full pack, or use the converter below for just PNGs.

## Bonus: convert any SVG to transparent PNGs

```bash
node .skills/logo-design/scripts/svg_to_png.mjs input.svg --out /workspace/logo/png \
  --sizes 1024,512,256,128,64
```

Transparent by default. Add `--background "#ffffff"` for a solid background.

## The scripts

All under `.skills/logo-design/scripts/` (materialized automatically):

- `logo_pack.mjs` — builds the full pack from one mark, reusing exact geometry, and
  self-verifies consistency. Rasterizes with `rsvg-convert` if present, else
  auto-installs `@resvg/resvg-js`. No setup required.
- `svg_to_png.mjs` — standalone SVG→PNG converter (same rasterizer).
- `deliver.sh` — uploads a directory to the org drive and prints each file's URL.

---

## Logo judgment (for good marks and confident presentation)

The pack pipeline handles production; these principles make the *mark itself* good.
Load the reference files only as needed.

- **Pick the right direction per brand** — architecture, symbol, register, and color
  by category, with exemplars. → `references/brand-logo-playbook.md`
- **Prompt precise type & color** — custom letterforms, monograms, wordmarks, and
  exact hex color. → `references/type-and-color-prompts.md`
- **Architecture** — most new brands want a **primary lockup** (symbol + wordmark)
  plus a square symbol/letterform fallback, monochrome, and reverse. Symbol-only is
  a secondary mark, earned after recognition. → `references/architectures-explained.md`
- **Typography** — choose the register for audience-side perception, not trend:
  humanist sans for trust+warmth, geometric sans for modern optimism, transitional
  serif for premium/editorial, monogram/slab for institutional. → `references/typographic-registers.md`
- **Symbol** — one primary idea (literal drawn with specificity, abstract gesture
  with formal logic, geometric reduction, or letterform-derived). Two can coexist;
  three become committee work. → `references/symbol-approaches.md`, `references/mark-prompts.md`
- **Restraint & small-size** — it must pass silhouette, sketchability, single-color,
  and 16px/32px legibility. Most logo failure is over-design. → `references/application-contexts.md`
- **Category fit** — avoid the clichés of the category (globes, swooshes, three
  circles) unless subverting them deliberately. → `references/category-conventions.md`
- **Variant rationale** — when writing up why a direction works. → `references/example-variant-spec.md`

## Optional: a presentation board in Canvas

The steps above deliver real files directly, which is usually what the user wants.
If the user asks for an **exploration board or a presentation** of directions, use
the `canvas` skill to build a `web_page` artifact (brief → territories → variants →
application tests → shortlist), embedding the delivered asset URLs, then
`canvas artifact verify` and `canvas artifact sync`. Run `canvas brands list` first
and design against the org's default brand when one exists. Canvas is for
presentation; the logo pack is the deliverable.
