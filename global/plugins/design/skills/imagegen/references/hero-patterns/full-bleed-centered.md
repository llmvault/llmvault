# Full-bleed centered hero

The image fills the entire hero section edge to edge; headline, subhead, and CTA are centered on top, usually over a darkening overlay. Used for bold single-message pages: hospitality, real estate, travel, events, agencies, restaurants.

## What the layout does to the image

- Text sits dead center, typically white, over a 20–50% dark overlay applied in CSS.
- The image is cropped responsively on the sides (mobile shows roughly the center 40%).
- Nothing in the image should compete with centered text: the *center* must be the calmest area.

## Image requirements

- Visual interest at the edges and top/bottom thirds; a calm, low-detail center (sky, water, fog, open floor, distant horizon).
- Mid-to-dark overall tone, or tones that survive a dark overlay. Avoid busy high-contrast texture in the middle.
- Symmetry works beautifully here — centered one-point perspective reads as intentional.
- Subject must survive side-cropping: keep essential elements within the central 60% *vertically*, but keep the center *horizontally* quiet.

## Aspect ratio

`16:9` for desktop-first. Generate a `3:4` companion for mobile if the section is full-viewport-height.

## Prompt recipe

Scene with symmetrical or edge-weighted composition + calm center + cinematic lighting + explicit tone. Structure: [scene, wide shot] + [what fills the edges] + [what keeps the center quiet] + [lighting/mood] + [grade/palette] + [no text].

## Example prompts

Hotel / hospitality:

> Wide symmetrical photograph of an infinity pool at dusk merging into a calm ocean horizon, shot from the center of the pool deck. Tall palm silhouettes and warm lantern light frame the left and right edges of the image; the center of the frame is pure gradient sky in deep teal fading to warm apricot at the horizon line. Long exposure smoothness on the water, one-point perspective, luxurious and serene. Cinematic color grade, slightly dark overall exposure. No people, no text, no letters, no logos.

Construction / real estate development:

> Cinematic wide establishing shot of a construction site at blue hour after rain. Tower cranes and floodlit scaffolding rise along the left and right edges of the frame, framing a calm open center of wet reflective concrete and deep blue evening sky. Warm tungsten work lights glow at the frame edges, teal-and-amber color grade, quiet epic mood like a prestige documentary frame. Dark, moody exposure throughout. No text, no letters, no logos.

## Pitfalls

- A single centered subject (building, person, product) fights the centered headline — this pattern wants a centered *void*, not a centered subject.
- If the model puts detail in the center anyway, strengthen the "center of the frame is …" clause by naming the empty medium (sky, water, fog) and retry.
- Don't request "dark overlay" in the prompt (layout vocabulary); request a dark *exposure* or *dusk/night scene* instead.
