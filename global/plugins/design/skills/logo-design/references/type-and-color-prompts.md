# Prompting custom letterforms & exact color

We generate vectors, so we can produce genuinely **designed type** — lettermarks,
monograms, and full wordmarks with deliberately shaped letters — not a font picked
at random. The prompts below were validated against the live API; both worked-out
examples are reproduced verbatim with the observed result.

The principle is the same as for symbols (`references/mark-prompts.md`): **describe
the letterform's construction**, the way a type designer would spec it. Vague
("modern wordmark") → generic. Constructed → designed.

## 1. The letterform vocabulary

Specify letters on these axes — every axis you skip becomes a generic default:

- **Skeleton/construction:** geometric (circles + straight stems) · humanist
  (calligraphic, modulated) · grotesque (neutral, uniform) · serif: transitional /
  old-style / Didone (high contrast) / slab.
- **Stroke:** monolinear (even weight) vs high-contrast (thick/thin); give the
  weight relative to the letter ("stroke weight one quarter of the letter width").
- **Terminals:** flat cut · fully rounded · sheared · pointed.
- **Corners/joins:** sharp · softly rounded · notched.
- **Proportions:** x-height (tall/classical), width (wide/condensed).
- **Weight:** light → black. **Case:** exact ("all lowercase exactly 'blip'").
- **The one intervention:** a single distinctive move with a *job* — an extended
  stroke, an enlarged dot, a shared stem, one cut. One intervention is ownable;
  three are noise (see the IBM-bars principle in the playbook).

Always: state the exact letters and casing, and demand **NO other text**. Add to
the `instructions` field: *"A designed letterform, not a typed font character.
Consistent optical stroke weight throughout."* — this line measurably changed the
output from font-like to designed in testing.

## 2. Validated examples (verbatim, with results)

**Lettermark — enterprise "N" with a compass-needle extension.** Result: exactly as
constructed — an N whose diagonal continues past the stem into an upward arrow.
Distinctive, name-derived (North), works as an app icon:

> A logomark of the single capital letter N only — NO other letters, NO words, NO
> other text. Construction: geometric grotesque skeleton built from two vertical
> stems and one diagonal; monolinear stroke weight exactly one quarter of the
> letter width; flat cut terminals; sharp exterior corners; the diagonal extends
> slightly above the right stem into a small upward point, like a compass needle.
> Solid deep enterprise blue #1E4FD1 on a transparent background, flat vector,
> centered with generous padding.

**Custom wordmark — "blip" with an enlarged floating i-dot.** Result: rounded
geometric black-weight lowercase, the i-dot enlarged and floated exactly as
specified — a real designed wordmark:

> A wordmark of the word blip only, all lowercase exactly 'blip' — NO tagline, NO
> other words. Construction: rounded geometric sans letterforms, black weight,
> monolinear strokes, fully rounded terminals, tall x-height, tight even letter
> spacing; the dot of the i is enlarged to one and a half times the stroke width
> and floats slightly higher than normal, like a radar blip. Solid vivid purple
> #7C3AED on a transparent background, flat vector, baseline aligned, centered.

## 3. Color: prompt the hex, enforce it in the pipeline

Always put an exact hex in the prompt — but know that **the model approximates
color** (we requested `#7C3AED` and received `#6F2BF5`; two generations in one
session returned two *different* purples). Brand color is enforced
deterministically instead:

- Pass `--recolor true` to `logo_pack.mjs`: every non-white fill in the mark (and
  `--wordmark`, if given) is forced to the exact `--color` hex. White fills are
  preserved as negative space.
- Use `--recolor` whenever the brand hex must be exact, and **always** when
  combining a generated mark with a generated wordmark — separate generations never
  match hues on their own.
- One solid color per mark remains the default ask. Gradients only for expressly
  playful consumer brands, with both stops specified — and know they complicate
  mono/reverse variants.

## 4. How type marks flow through the pipeline

- A **lettermark or monogram** is just a mark: it is the `--mark` input and gets
  the full pack (app icon, favicon, mono, reverse) like any symbol.
- A **custom wordmark** rides alongside a symbol: generate it separately, download
  it, and pass `--wordmark wordmark.svg`. The script normalizes its viewBox,
  flattens its styles (generated SVGs reuse the same CSS class names — composing
  two without flattening makes one invisible; the script handles this), sizes the
  lockup from its real aspect ratio, and recolors it with `--recolor`.
- **The user must see and approve any generated letterforms** before the pack is
  built — generated type is distinctive but can occasionally distort a letter.
  Typeset text (the script's default wordmark) is the safe fallback for plain
  names; generated custom type is the distinctive upgrade for brands that want it.
- Match register to category via `references/brand-logo-playbook.md`, Decision 3.
