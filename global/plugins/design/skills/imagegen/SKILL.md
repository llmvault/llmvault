---
name: "imagegen"
description: "Prompting guide for high-quality image generation and image editing. Routes each image request to a detailed per-pattern reference file (landing-page hero patterns today; more image-type families over time) with hard rules that prevent common image-model failures, plus a fallback recipe for requests that map to no documented pattern. Use when an agent needs to generate or edit photos, illustrations, hero images, product mockups, UI mockups, diagrams, ads, logos, transparent cutouts, or reference-based visual variants. Pick the specific reference file for your image type and load only that file. Do not use this skill for credentials, billing, or network configuration."
---

# Image Generation

This skill turns an image request into a strong prompt. It works pattern-first: identify what *kind* of image the layout or task needs, load the one reference file documenting that pattern, and adapt its tested recipe. Requests that map to no documented pattern fall back to the general prompting method at the end of this file.

## Core workflow

1. **Classify the intent**: generate a new image, edit an existing image, or generate with reference images. (Edit and reference-image work: see `references/prompting.md`.)
2. **Identify the image's job**: where will it live? A landing-page hero, a section illustration, a product shot, a diagram, a game asset?
3. **Match it to a documented pattern family** (tables below). If it matches, load ONLY that pattern's reference file — it contains layout anatomy, image requirements, the right aspect ratio, tested example prompts, and pitfalls. Never load more than two pattern files in one task.
4. **If nothing matches**, use the fallback method (bottom of this file) plus `references/prompting.md` for depth.
5. **Generate, then review** every result against the quality checklist before using it.

## Hard rules — common image-model failure modes

These failure modes recur across image models. Violating these rules produces broken images.

1. **Never use layout or web vocabulary in a prompt** for imagery that is not itself a UI. Words like "website", "hero section", "landing page", "headline", "button", "CTA", or "space for text overlay" make the model render a literal page mockup with garbled placeholder text. Describe a *photograph* or *artwork*, never its intended use. (Exception: `ui-mockup` and wireframe requests, where rendering an interface IS the goal — see `references/prompting.md`.)
2. **Get negative space photographically, not by instruction.** Wrong: "leave the left 50% empty for a headline." Right: "a vast expanse of soft dawn sky fills the left of the frame; all structures sit in the right third." Name what occupies the quiet zone (sky, water, wall, fog, bokeh) — emptiness must be made of *something*.
3. **Flat/vector illustration prompts must force full-bleed composition.** Without it the model floats a small centered vignette in a sea of empty background. Include language like: "the scene fills the entire canvas edge to edge, no empty margins, every part of the frame contains scenery."
4. **Add "no text, no letters, no writing, no logos" to prompts for text-free assets.** The model leaks letter-like artifacts, especially in line art and anything resembling signage. Text belongs in HTML, not in the image (exception: assets whose job includes rendered text, e.g. ads and diagrams — then quote it verbatim, keep it short, and see the text rules in `references/prompting.md`).

## Reliable levers

- **Prompt adherence is strong.** Specific concrete nouns reliably appear (a welder's sparks, a woman pointing, blueprints). Say exactly what you want; vagueness is what produces generic results.
- **Style keywords execute with high fidelity:** "risograph print", "paper-cut collage", "impasto oil painting", "line art with spot color", "flat vector", "isometric". These produce distinctive, non-stock results.
- **Camera language works for photos:** focal length, aperture (f/1.8 for creamy bokeh, f/8 for sharp scenes), "golden hour", film stocks ("Kodak Portra 400"), "anamorphic widescreen".
- **Hex codes set palette intent, not exact values.** "Deep navy #0F1E3C with amber #FFC400 accents" steers the grade convincingly; don't promise exact brand-color reproduction.
- **Mood/cinematic language beats keyword stacking** for premium feel. Keyword soup ("8k, sharp focus, professional") yields competent but stock-looking images.
- **Centered-diorama gravity:** isometric, soft-3D, and clay styles pull toward a centered object on a plain background even when told otherwise. Accept it where a contained image is fine, or fight it with "cropped by the frame on all four sides".
- **"Claymorphism" renders as tilt-shift photography of miniatures,** not a 3D render. Ask for "soft 3D render, matte clay material" for the render look.

## Generating with the image tools

Use `hivy_generate_image` (raster) or `hivy_generate_vector_image` (SVG) for new images:

- `prompt` — up to 4000 chars; 400–900 chars is the sweet spot.
- `aspect_ratio` — supported: `16:9`, `9:16`, `3:2`, `2:3`, `4:3`, `3:4`, `1:1`. Take it from the pattern file; don't default to one ratio for everything.
- `count` — generate 2–4 variants and pick the best; single generations gamble on one dice roll.
- `type` — hint such as `photo`, `illustration`, `logo`, `icon`.
- `reference_asset_ids` — up to 10 drive assets for loose style guidance.

Use `hivy_remix_image` when the new image must contain something that already exists — a recurring character, a specific product, or the next sibling in a set. It takes `prompt` + `reference_asset_ids` (the master asset) and preserves the identity of what the references show. Elements are only consistent *within* one generation; across generations the model reinvents whatever it redraws — so identity-critical raster work MUST go through `hivy_remix_image` with the master-asset workflow in `references/consistent-assets.md`.

**Vector illustration systems are also an exception: load the `illustration-system` skill.** When the user wants a *set* of flat vector illustrations in one consistent style (spot/section illustrations, empty states, "more like these samples"), that skill holds the style across subjects with a written style contract plus reference conditioning, natively in SVG. The raster sibling-set workflow below remains correct for photos, 3D, and painterly sets.

**Logos are the exception: load the `logo-design` skill and do the work there.** It generates the mark as a native SVG (`hivy_generate_vector_image`), refines with reference-conditioned regeneration until the user approves, then composes every lockup and variant deterministically from the same vector geometry — byte-identical, no remix drift, delivered as a full SVG + transparent-PNG pack. Never build a logo by generating a raster lockup and remixing variants from it; that workflow is superseded.

## Pattern families

### Hero images (landing pages)

A hero image's requirements are dictated by the hero section's *layout* — where the headline sits, whether text overlays the image, how it crops. Pick exactly one pattern, then load `references/hero-patterns/<slug>.md`.

Image-as-background (text overlays the image — image must reserve quiet space):

| Slug | Pick when |
|------|-----------|
| `full-bleed-centered` | Bold single message, dramatic photo, centered headline. Hospitality, real estate, events. |
| `full-bleed-text-left` | Full-width photo, copy column left. The most common marketing hero. |
| `full-bleed-text-right` | Mirror of the above; subject anchored left, copy right. |
| `bottom-anchored` | Headline in the upper half, scene detail along the bottom (skyline, crowd, product row). |
| `dark-cinematic-glow` | Dev tools, fintech, AI; dark-UI brands (Linear/Vercel aesthetic). White text anywhere. |
| `duotone-brand-wash` | Strong brand colors; image reads as branded texture, not photo. |

Split layouts (image in its own column/region — text never overlaps it):

| Slug | Pick when |
|------|-----------|
| `split-50-50` | Classic text-left image-right. Safe default for most SaaS and services. |
| `split-breakout` | Image column bleeds off the viewport edge for energy/scale. |
| `angled-split` | Diagonal/curved boundary between copy and image; sporty, dynamic brands. |
| `device-frame` | Visual shown inside a laptop/phone/browser frame. |
| `card-hero` | Image floats in a rounded card over a plain/gradient background. |
| `bento-grid` | Headline plus a mosaic of 3–6 tiles; needs a consistent image set. |

Illustration and abstract:

| Slug | Pick when |
|------|-----------|
| `flat-illustration` | Friendly SaaS, HR, education. Flat vector scenes. |
| `isometric-scene` | Technical/logistics/infrastructure; explanatory diorama feel. |
| `abstract-gradient` | No literal subject; pure atmosphere behind text. Easiest text overlay. |
| `textured-editorial` | Brands wanting craft: risograph, paper collage, painterly. |

Subject-led:

| Slug | Pick when |
|------|-----------|
| `human-portrait` | Trust and human connection: coaching, healthcare, local services. |
| `product-object` | A physical product is the star: e-commerce, hardware, CPG. |
| `lifestyle-context` | Product/service in real use; aspirational e-commerce. |
| `aerial-establishing` | Scale and scope: construction, real estate, logistics, agriculture. |

### Product UI vignettes (feature sections, product visuals)

The stylized fake-screenshot style (Stripe/Intercom/Kit): simplified UI fragments — stat cards, flow diagrams, chat bubbles, kanban boards — as floating white cards on flat pastel backgrounds. Use for feature-section visuals, split-hero image columns, bento/card tiles, and `device-frame` content. Load `references/product-ui-vignette.md`.

This family is the sanctioned exception to hard rule 4: up to ~6 short verbatim UI strings render reliably in this flat style (with a restricted character set and a mandatory glyph-by-glyph review). The dominant failure mode is geometry (tilted "sticker" badges, background color drift), not spelling — the reference file documents the counters and a tested archetype menu.

### Background textures and patterns

Full-bleed textures, gradients, and patterns behind page content: paper grain, mesh gradients, dark-mode atmospheres. Load `references/backgrounds.md`. The family's prime directive: **a background must recede** — tone-on-tone, barely perceptible, no focal points; a beautiful pattern that grabs attention is a failed background. Note the file's "should this be generated at all?" gate: flat geometric patterns belong in CSS/SVG; generate only organic/material qualities code can't produce.

More families (section illustrations, object/e-commerce sets, blog headers, and others) will be added under `references/` over time; check the reference map before falling back.

## Fallback — requests that map to no pattern

When the request fits no documented pattern (a game sprite, a diagram, an ad, a one-off scene), build the prompt from this method. Read `references/prompting.md` for the full treatment; the short version:

1. Classify the use case with the taxonomy in `references/prompting.md` (photorealistic-natural, product-mockup, ui-mockup, infographic-diagram, ads-marketing, logo-brand, illustration-story, stylized-concept, and the edit categories).
2. Respect the user's specificity: a detailed request gets normalized, not embellished; a generic request gets tasteful concrete detail — never invented characters, brands, palettes, or story beats.
3. Compose in this order: **medium/style → subject and action → environment → composition/framing → lighting/mood → palette/materials → constraints → negatives.** Every clause concrete, no quality-word piles.
4. Apply the hard rules above — they hold for every image, pattern or not.
5. Copy a matching recipe from `references/sample-prompts.md` and adapt it rather than writing from scratch.

Write the prompt as **one flowing paragraph** in that clause order — the same
format as every example in `references/hero-patterns/` and
`references/sample-prompts.md`. Do not submit the prompt as a labeled key-value
list; the clause order is a composition method, not a form to fill in. For
example:

> Photorealistic candid photograph of an elderly sailor on a small fishing boat,
> hands mid-pull adjusting a net, worn oilskin jacket. Coastal water with soft
> haze behind him. Medium close-up at eye level, the sailor in the right two
> thirds of the frame. Soft coastal daylight, shallow depth of field. Natural
> color balance, no glamorization. No text, no letters, no logos, no watermark.

## Quality review checklist

Before accepting any generated image:

- Subject correct and unambiguous; nothing important invented or missing.
- Composition honors the pattern's quiet-zone/full-bleed/margin requirement.
- No letter-like artifacts anywhere (inspect signage, screens, spines, line art closely).
- Hands, teeth, and eyes intact on any people, at full zoom.
- Style matches the brand direction, not drifted to generic stock.
- Tone/contrast in the text zone (if any) is uniform enough for readable copy.
- For sets: siblings share medium, palette, lighting, and detail level.

If a result fails, change ONE prompt dimension and retry — don't rewrite the whole prompt, and don't accept a near-miss for a load-bearing image.

## Reference map

Load only the file(s) relevant to the current task.

- `references/hero-patterns/<slug>.md` — one file per hero-section pattern (slugs in the tables above): layout anatomy, image requirements, aspect ratio, example prompts, pitfalls. Load after picking a pattern; never more than two.
- `references/product-ui-vignette.md` — the product-UI-vignette family: visual system contract, text and geometry rules, tested archetype menu (stat cards, flows, chat, kanban, dark mode), set-consistency method, review checklist.
- `references/backgrounds.md` — the backgrounds family: the recede directive, generate-vs-CSS gate, tested recipes (material texture, mesh gradient, tone-on-tone patterns, dark mode), recurring model behaviors, review checklist.
- `references/consistent-assets.md` — identity-critical assets (logos, characters, products, sibling sets): the master-asset workflow, `hivy_remix_image` usage, invariant prompt language, worked logo-package example, review checklist. REQUIRED reading before any logo generation or multi-image set.
- `references/prompting.md` — the detailed fallback guide: intent rules, use-case taxonomy, specificity and augmentation policy, composition, lighting/camera/material language, text-in-image rules, reference-image roles, transparent/cutout prompts, edit invariants, iteration method.
- `references/sample-prompts.md` — copy/paste recipes by use case for generation and edits, plus asset-type templates (game assets, wireframes, logos, diagrams).
