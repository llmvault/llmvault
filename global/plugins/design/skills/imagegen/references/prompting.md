# Fallback prompting guide

This file is the detailed method for image requests that map to no documented pattern family in `SKILL.md`, and the shared depth for intents every pattern uses (edits, reference images, text-in-image, transparent cutouts, iteration). The hard rules in `SKILL.md` apply to everything here.

## Contents

- [Intent rules](#intent-rules)
- [Use-case taxonomy](#use-case-taxonomy)
- [Specificity and augmentation policy](#specificity-and-augmentation-policy)
- [Prompt structure](#prompt-structure)
- [Composition](#composition)
- [Lighting, camera, and materials](#lighting-camera-and-materials)
- [Text in images](#text-in-images)
- [UI mockups and wireframes — the layout-vocabulary exception](#ui-mockups-and-wireframes--the-layout-vocabulary-exception)
- [Input images and references](#input-images-and-references)
- [Edits and invariants](#edits-and-invariants)
- [Transparent and cutout prompts](#transparent-and-cutout-prompts)
- [Iteration method](#iteration-method)
- [Use-case tips](#use-case-tips)

## Intent rules

- If the user wants to modify an existing image while preserving parts of it, treat the request as an **edit**.
- If the user provides images for style, composition, mood, or subject guidance but does not ask to modify them, treat the request as **generation with references**.
- If the user provides no image, treat the request as **generation**.
- If the request is for an icon, logo, or UI graphic that should match existing repo-native vector/code assets, prefer direct native editing over generated-image prompting.
- If the request is a landing-page or marketing-site visual, STOP — go back to `SKILL.md` and route through the pattern tables first. This fallback is for requests that genuinely fit no pattern.

## Use-case taxonomy

Classify each request into one bucket; keep the slug consistent across prompts and notes.

Generate:

- `photorealistic-natural` — candid/editorial lifestyle scenes with real texture and natural lighting.
- `product-mockup` — product/packaging shots, catalog imagery, merch concepts.
- `ui-mockup` — app/web interface mockups and wireframes; see the exception section below.
- `infographic-diagram` — diagrams/infographics with structured layout and rendered labels.
- `scientific-educational` — explainers and learning visuals with required labels and accuracy constraints.
- `ads-marketing` — campaign creatives with audience, brand position, scene, and exact tagline.
- `productivity-visual` — slides, charts, and data-heavy business visuals.
- `logo-brand` — logo/mark work; routed to the `logo-design` skill, not prompted here.
- `illustration-story` — comics, children's book art, narrative scenes.
- `stylized-concept` — style-driven concept art, 3D/stylized renders, game assets.
- `historical-scene` — period-accurate scenes; state location, date, and accuracy constraints.

Edit:

- `text-localization` — translate/replace in-image text while preserving layout.
- `identity-preserve` — try-on, person-in-scene, character continuation; lock identity.
- `precise-object-edit` — remove or replace a specific element.
- `lighting-weather` — time-of-day, season, or atmosphere changes only.
- `background-extraction` — transparent background, clean cutout, silhouette isolation.
- `style-transfer` — apply a reference style while changing subject or scene.
- `compositing` — multi-image insert/merge with matched lighting and perspective.
- `sketch-to-render` — line art to photoreal or polished render.

## Specificity and augmentation policy

Use the user's prompt specificity to decide how much augmentation is appropriate:

- If the prompt is already specific and detailed, preserve that specificity and only normalize or structure it.
- If the prompt is generic, add tasteful concrete detail only where it materially improves the result.

Good augmentation: composition and framing cues; polish-level hints; scene concreteness that supports the request; material, texture, lighting, and camera detail matched to the goal.

Never invent: extra characters, props, or objects not implied; brand names, slogans, palettes, or story beats not supplied; arbitrary left/right placement the consuming layout doesn't call for; unrequested style shifts or decorative clutter.

## Prompt structure

Compose in a consistent order — each clause concrete, no quality-word piles ("ultra detailed", "masterpiece", "8k" add nothing):

1. **Medium/style** — "photograph", "flat vector illustration", "soft 3D render", "risograph print". Set this first; everything else is interpreted through it.
2. **Subject and action** — specific nouns and verbs. "A welder crouched on a steel deck sending up orange sparks", not "a worker working".
3. **Environment** — where, what surrounds the subject, what fills the frame's remaining regions.
4. **Composition/framing** — close-up/medium/wide, viewpoint, subject placement in the frame, what happens at the edges.
5. **Lighting/mood** — light source, time of day, emotional temperature.
6. **Palette/materials** — up to five named colors; surface textures.
7. **Constraints** — what must be true (blank packaging, one subject only, horizon position).
8. **Negatives** — "no text, no letters, no logos, no watermark" plus request-specific exclusions.

For complex requests, keep the same clause order but split the paragraph into two or three sentences (subject and scene; composition and lighting; constraints and negatives). Never submit the prompt as a labeled key-value list — the clause order is a composition method, not a form.

## Composition

- Specify framing and viewpoint whenever it matters: close-up, medium, wide, top-down, low angle, isometric, centered, symmetrical, rule-of-thirds.
- **Negative space must be described as scenery, never as a reservation** (hard rule 2): name the medium that fills the quiet region and where it sits — "the upper two thirds is smooth gradient sky", "the left half is a plain walnut wall in soft shadow". Never say what the space is *for*.
- Direct placement only when the consuming layout justifies it, and phrase it as frame regions: "in the right third of the frame", "spanning the bottom quarter edge to edge".
- For people: body framing, scale, gaze direction relative to the frame, pose, and hand/object interactions — "full body visible", "gaze directed just past the left of the frame", "hands naturally gripping the handlebars".
- For illustrations: full-bleed language per hard rule 3, or a deliberate named backdrop if a floating composition is intended.
- For sets of images that must match, write one shared style sentence (medium + palette + lighting) and reuse it verbatim in every prompt; chain `reference_asset_ids` from the first accepted image for the rest.

## Lighting, camera, and materials

- Lighting defines mood and clarity: soft daylight, overcast, warm indoor practicals, hard studio rim light, golden hour, clean shadowless product lighting. Name a direction when it matters ("key light from the upper left").
- Camera language for photos: lens length (35mm environmental, 50mm neutral, 85mm portrait), aperture (f/1.8 melts backgrounds, f/8 keeps scenes sharp), viewpoint, candid/editorial/studio treatment, film stock for grade ("Kodak Portra 400 warmth").
- Material language prevents generic plastic-looking surfaces: matte ceramic, brushed steel, worn cotton, rough paper, translucent glass, weathered wood, full-grain leather. For photorealism add believable imperfection: pores, fabric wear, dust, fingerprints, asymmetry.

## Text in images

Default: exclude text entirely (hard rule 4) and add copy in HTML/design tools. When the asset's job requires rendered text (ads, diagrams, slides, logos with wordmarks):

- Quote the exact text: `Text (verbatim): "Yours to Create."` and require exact spelling with no extra words.
- Keep it short. Dense paragraphs, tables, legends, and tiny labels are high-risk; every additional string multiplies failure odds.
- Specify typography intent (clean sans-serif, engraved serif), size, color, and placement.
- Spell tricky words letter by letter when accuracy matters.
- Inspect rendered text at full zoom before accepting; a single wrong glyph is a reject.

## UI mockups and wireframes — the layout-vocabulary exception

Hard rule 1 bans web/layout vocabulary for imagery that is not a UI. For `ui-mockup` and wireframe requests, the interface IS the subject, so layout vocabulary is required — with these controls:

- State fidelity first: "low-fi grayscale wireframe" vs "realistic product UI, not concept art".
- Name the blocks in reading order: "top nav; hero area with heading and button; three feature cards; footer".
- For wireframes, ask for labeled blocks and no real text beyond the labels; for high-fidelity mockups, keep visible text minimal and quoted verbatim.
- Expect and tolerate some pseudo-text in low-priority regions; regenerate only when it hits focal areas.
- Never mix modes: a photographic scene *containing* a laptop screen should have its screen described as abstract shapes ("soft colored dashboard tiles"), not as a real UI — see the `device-frame` hero pattern.

## Input images and references

- Do not assume every provided image is an edit target.
- Label each image by index and role: `Image 1: edit target`, `Image 2: style reference`, `Image 3: subject reference`, `Image 4: insert element`, `Image 5: mood reference`.
- Style/composition/mood references guide generation; the prompt should say what to take from each: "match Image 2's palette and brushwork".
- For compositing, describe the interaction: "place the subject from Image 2 into Image 1, matching Image 1's lighting and camera perspective, with a soft contact shadow".
- For character/product consistency across a series, always pass the anchor image as a reference and add "do not redesign; preserve proportions, palette, and defining features".

## Edits and invariants

- State the change AND the invariants in the same prompt: "change only X; keep Y unchanged".
- Enumerate invariants concretely: "preserve face, pose, lighting, camera angle, and background".
- Repeat invariants on every iteration — drift compounds across retries.
- For text-localization edits, preserve layout, typography, spacing, and hierarchy; change only the strings.

## Transparent and cutout prompts

Ask for the cleanest possible isolation:

```text
Create the requested subject isolated on a clean transparent background.
If true transparency is not available, use a perfectly flat, solid, high-contrast background color that does not appear in the subject.
Keep the subject fully separated from the background with crisp edges and generous padding.
No cast shadow, contact shadow, reflection, watermark, extra text, background texture, gradient, floor plane, or lighting variation.
```

For hair, fur, feathers, glass, smoke, liquids, and reflective objects, say what the edge must preserve ("keep individual hair strands crisp, no halo") — these subjects are where cutouts fail.

## Iteration method

1. Start from the closest recipe (pattern file or `sample-prompts.md`), not from scratch.
2. When a result fails, name the failure mode before editing: wrong subject, weak composition, quiet zone violated, garbled text, identity drift, style mismatch, missing invariant, unwanted object.
3. Change ONE dimension per retry — composition, lighting, medium, palette, or mood — and keep everything else verbatim.
4. Re-state critical constraints every iteration.
5. Two failed retries on the same dimension means the approach is wrong: switch technique (different style keywords, different composition strategy), don't keep re-rolling.

## Use-case tips

Generate:

- `photorealistic-natural`: prompt as if a real photo was captured in the moment; photography language, natural texture, believable imperfection; no over-polish.
- `product-mockup`: product shape, materials, clean silhouette, named backdrop, studio lighting, contact shadow; blank packaging unless text is required verbatim. For hero placement use the `product-object` pattern file instead.
- `ui-mockup`: see the exception section above.
- `infographic-diagram`: define audience and reading flow; name every label verbatim; require readable spacing and strong contrast.
- `scientific-educational`: audience, lesson objective, required labels, accuracy constraints, arrows, scan-friendly whitespace.
- `logo-brand`: do not prompt logos from this file — load the `logo-design` skill, which generates native-SVG marks with constructive-geometry prompts and builds the full pack deterministically.
- `ads-marketing`: write like a creative brief — audience, brand position, scene, vibe, exact tagline once; check the tagline glyph by glyph.
- `productivity-visual`: name the artifact (slide, chart), canvas, hierarchy, real labels/data verbatim, polished spacing.
- `illustration-story`: concrete scene beats or panels, one action per panel; consistent character description repeated in every panel's prompt.
- `stylized-concept`: style cues, material finish, rendering approach; don't invent story elements the user didn't give.
- `historical-scene`: location and date, period-accurate clothing/props/environment, "no modern objects".

Edit: follow the invariant rules above; each edit category's recipe is in `references/sample-prompts.md`.
