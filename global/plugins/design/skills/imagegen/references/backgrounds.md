# Background textures and patterns

Full-bleed textures, gradients, and patterns that sit behind page content: section backgrounds, page washes, card backdrops, dark-mode atmospheres. This family is defined by one property that overrides everything else:

## The prime directive: a background must recede

A background succeeds when nobody notices it. The single most common failure of generated backgrounds is not bad geometry — it's a texture or pattern prominent enough to compete with the content in front of it. A technically perfect dot grid at normal contrast is a *failed background* because it grabs the eye.

Enforce recession in the prompt with all three levers:

1. **Contrast**: "extremely low contrast", "barely perceptible", "tone-on-tone" — pattern elements only a few percent darker/lighter than the base color, never a different hue at full strength.
2. **Scale**: texture elements tiny relative to the frame ("very fine grain", "small marks"); large elements read as subject matter, not texture.
3. **Uniformity**: "uniform across the entire frame, no focal point, no vignetting, no lighting variation corner to corner" — any local variation becomes a focal point.

And verify with the **squint test**: shrink the image or squint at it — it should read as a single flat color with a faint quality, not as "a picture of something". If any region draws the eye, reject it.

## First ask: should this be generated at all?

Flat geometric patterns — dot grids, line grids, checkers, simple waves — are better authored in CSS or SVG: deterministic, seamlessly tileable, a few hundred bytes, and trivially recolorable. Generate a background only when it needs organic or material qualities code can't produce: paper fiber, film grain, watercolor wash, fabric weave, brushed metal, atmospheric gradients with irregular blooms. If an agent can draw it with `background-image: radial-gradient(...)`, it should.

Generated backgrounds are single full-bleed images, NOT seamless tiles — never promise tileability; cover the full viewport width with one image instead.

## Sub-type recipes

### Material texture (paper, grain, fabric) — tested

The restraint problem: models add "interest" unless every escape hatch is closed. Close them all explicitly:

> An extremely subtle background texture: warm off-white paper with a very fine, even grain and faint fiber flecks, uniform across the entire frame with no focal point, no objects, no shapes, no vignetting, no lighting variation from edge to edge. The texture is barely perceptible, like premium uncoated stationery photographed flat and evenly lit. Consistent tone corner to corner, extremely low contrast. No text, no letters, no logos, no watermark.

Swap the material: "fine linen weave", "smooth concrete with faint pores", "soft film grain on deep charcoal" (dark mode). Keep the full no-list every time — it is what makes restraint work.

### Mesh / atmospheric gradient — tested

Named hues + a named calm zone + no hard edges is a stable recipe:

> A smooth abstract mesh gradient background: soft indigo, warm peach, and pale rose hues melting into each other in broad diffuse blooms, like colored light blended on frosted glass. The upper left two thirds settle into a calm, nearly uniform pale indigo field; the warmer blooms gather along the bottom right edge. Silky seamless transitions with no hard edges, no objects, fine film grain across the whole image to prevent banding. No text, no letters, no logos, no watermark.

Notes: saturation renders hotter than words like "pale" suggest — undershoot ("very pale", "desaturated", "whisper of color"). Always request fine grain to prevent banding on large screens. For a hero (not a section background) with text overlay, see the `abstract-gradient` hero pattern — same technique, hero-specific composition.

### Geometric pattern — highest-risk sub-type, use with care

Verified failure: a clean dot grid at normal contrast completely dominated the frame. If a geometric pattern must be generated (rather than CSS, see above):

- Tone-on-tone only: "dots barely a few percent darker than the background, same hue family, extremely low contrast, almost invisible".
- Tiny elements, generous spacing.
- **Uniform only.** Asking for graduated variation inside a pattern (a size ramp, a fading zone) corrupts it locally — the model produces blotches, mixed sizes, and smudges where the transition should be. Do emphasis zones in CSS on top of a uniform pattern.
- Force flatness: "perfectly flat 2D marks, no depth, no holes, no shadows inside shapes" — flat elements otherwise acquire physical depth (dots become drilled holes).

### Dark mode

Dark textures behave like light ones with the same rules: "deep charcoal with barely perceptible fine grain", "near-black with a faint warm vignette-free tone". Avoid pure black (banding); keep any glow or color bloom to one corner and whisper-subtle.

## Recurring model behaviors (counter in every prompt)

- **Base-hue drift** (~1 in 3–4 generations): grey drifts blue, cream drifts dark. State the base color at the start AND end of the prompt; regenerate on drift — it's a dice roll.
- **Escalating contrast**: the model's default texture strength is 2–3× too strong for a background. "Barely perceptible" and "extremely low contrast" are the load-bearing words; repeat them.
- **Interest creep**: unprompted focal points, vignettes, and lighting gradients appear unless explicitly banned.

## Generation parameters

- `aspect_ratio`: `16:9` for section/page backgrounds; `9:16` for mobile-first full-height sections.
- `type`: `illustration` for gradients/patterns, `photo` for material textures.
- `count`: 2–4 — hue drift makes variants especially valuable here.
- Files run large (4–6 MB PNG at high resolution); convert to WebP/AVIF before shipping.

## Review checklist (reject → regenerate)

- Squint test: reads as one flat color with a faint quality; nothing draws the eye.
- No objects, focal points, vignettes, or lighting variation anywhere.
- Base hue matches the request (check against the named color, not vibes).
- Pattern (if any): uniform, flat, tone-on-tone; no corrupted zones.
- No banding in gradients at full size; grain present if requested.
- No letter-like artifacts (check corners and texture flecks).
