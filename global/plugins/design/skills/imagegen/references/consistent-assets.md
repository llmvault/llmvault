# Consistent assets — remixing with references

Some assets are useless unless they stay *identical* across images: a mascot that changes faces between panels is not a mascot. This file is the method for generating **raster** assets whose identity must persist — recurring characters/mascots, a specific product, and image sets that must read as siblings.

**Logos are handled elsewhere.** The `logo-design` skill owns logo work end to end: it generates the mark as a native SVG and composes every variant from the same vector geometry, giving *byte-identical* consistency that no remix workflow can match. The recipes below apply the same "creative exactly once" principle to assets that must stay raster.

The core fact, verified by direct testing: **within one generation, elements are self-consistent; across generations, the model reinvents everything it redraws.** Generating a mark and a lockup as two independent images produces two different marks, every time. Consistency is therefore an *architecture* problem, not a prompting problem — you get it by controlling how many times the model is allowed to be creative.

## The master-asset workflow

1. **Let the model be creative exactly once.** Generate the richest single asset as the master — for a character, a clear full-body anchor pose; for a product, the clean three-quarter studio shot. Generate several candidates (`count: 3–4`) and have the user pick the winner.
2. **Derive mechanically whenever possible.** Cropping a detail out of the winning master is pixel-exact and free. Recolors, reversals, and rearrangements of flat assets can be done in SVG/CSS/an image editor with zero drift. Never regenerate what you can crop or compose.
3. **Use `remix_image` only for variants that require redrawing** — the character in a new pose, the product from a new angle, the next sibling in a set.

## The `remix_image` tool

`remix_image` generates a new image *guided by reference images*, preserving the identity of what the references show:

- `prompt` — what to produce, written with invariant language (below).
- `reference_asset_ids` — 1–10 drive asset IDs; pass the master asset (and only what's needed — extra references dilute identity).
- `aspect_ratio`, `count` — as with `generate_image`. Use `count: 2–3` and pick the most faithful, not the prettiest.

Use `generate_image` when creating something new; use `remix_image` whenever the new image must contain something that already exists.

## Invariant prompt language — what makes identity hold

Tested template:

> Using the exact [logo / character / product] from the reference image, completely unchanged — do not redesign the [enumerate its defining features: "the rounded hexagon outline, the amber dot straddling its upper right edge, the lowercase letterforms, the spacing"] — [the single change: "reproduce the identical lockup with every dark element changed to pure white, on a flat deep charcoal background"]. [Restate style constraints for anything new: "Perfectly flat, crisp vector edges, no gradients, no shadows."] No other text, no watermark.

The load-bearing parts, in order of importance:

1. **"Completely unchanged — do not redesign"** framing. State it before describing the change.
2. **Enumerate the defining features** of the referenced asset in words, even though the image is attached. The verbal description anchors what the reference shows; identity fidelity measurably improves.
3. **One change per remix.** Recolor OR rearrange OR extract — a variant needing two changes is two sequential remixes (or one remix plus mechanical editing).
4. **Restate style constraints for every NEW element.** Verified failure mode: the referenced mark survives perfectly while a newly-added container (an app-icon square) comes back embossed/3D with shadows. Drift lives in what the model draws fresh, not in what it copies — so everything fresh needs the full flatness/style clause again. Better still: only remix the referenced element itself and build containers in SVG/CSS.

## Logos: superseded by the vector pipeline

This file previously carried a raster logo-package recipe (generate a lockup,
crop the mark, remix each variant, vectorize at the end — topping out around
~95% visual identity). It is retired: the `logo-design` skill generates the mark
as a native SVG and derives every lockup and variant from the *same* vector
geometry, so identity is byte-exact by construction and no verification pass is
needed. For any logo, brand mark, monogram, wordmark, app icon, or favicon
request, load `logo-design` and follow its procedure.

## Sets that must read as siblings

For **flat vector illustration sets in one style**, use the `illustration-system`
skill instead — it holds a style across subjects natively in SVG with a written
style contract plus reference conditioning. The levers below are for raster sets
(photographic, 3D, painterly) and recurring characters:

- **Shared style contract**: one verbatim sentence (medium, palette, lighting, background treatment) prefixed to every prompt in the set.
- **Anchor + remix**: generate the first/largest asset with `generate_image`, then produce the rest with `remix_image` referencing the anchor — "same character, same style, same palette… now [new scene/pose/concept]". For characters add: "do not redesign the character; preserve facial features, proportions, outfit, and color palette."

## Review checklist (reject → re-remix)

- Referenced element: proportions, feature positions, colors, and (for lockups) letterforms match the master at a side-by-side comparison.
- Nothing was "improved": no added details, no style upgrades on the referenced element.
- New elements obey the restated style constraints (flatness is the usual casualty).
- The single requested change happened — and nothing else changed.
