# Split 50/50 hero

Text column left, image column right (occasionally mirrored), each in its own container — text never overlaps the image. The safe default for SaaS, services, and agencies. Because no text overlays it, the image can be rich and busy.

## What the layout does to the image

- The image is displayed in a bounded container (often rounded corners), roughly the right 50% of the hero.
- It will be cropped to the container's aspect by `object-fit: cover` — the subject must sit center-ish and survive moderate crops on all sides.
- On mobile it stacks below the text at a wide short crop, so the subject shouldn't rely on full height.

## Image requirements

- One clear subject, centered or slightly off-center, occupying 50–80% of the frame.
- Full-bleed scene fill — no plain margins around the subject (the container's own whitespace does that job).
- Can be dense and detailed: this is where the busy, characterful images that would ruin overlay patterns belong.
- Style should echo the page's design language: photographic for corporate, illustrated for friendly SaaS.

## Aspect ratio

`4:3` or `3:2` for landscape containers, `1:1` for square, `3:4` for tall column images. Match the actual container; don't default to 16:9.

## Prompt recipe

[medium/style] + [single subject and action] + [environment filling the frame to every edge] + [lighting/palette] + [no text]. For photos, camera language; for illustrations, full-bleed language (hard rule 3 in the main skill).

## Example prompts

HR / team SaaS (photo):

> Warm candid photograph, 4:3 composition filling the whole frame. A small diverse team of four collaborates around a wooden table scattered with sticky notes and laptops in a sunlit loft office, one person standing and gesturing at a whiteboard just visible at the frame edge. Golden late-afternoon window light, genuine laughing expressions, shallow depth with the background office softly blurred but still filling every corner of the image. Kodak Portra warmth, editorial not stock. No visible screen content, no text, no letters, no logos.

Construction management SaaS (illustration — note the full-bleed composition language, hard rule 3):

> Flat vector illustration of a construction site scene that fills the entire canvas edge to edge with no empty margins — every part of the frame contains scenery. A yellow tower crane reaches from bottom to top of the frame, geometric buildings under construction span the full width, small workers in hard hats review plans in the foreground, stacked materials and scaffolding occupy the corners, stylized clouds fill the sky area. Warm palette: cream, navy, construction yellow, coral accents. Clean flat shapes, modern and friendly. No text, no letters, no logos.

## Pitfalls

- Don't reuse a negative-space composition here — a half-empty image inside a half-width container looks broken.
- Check edge fill on illustrations: the floating-vignette failure mode shows up most in this pattern's containers.
- Screens/whiteboards in photos generate garbled pseudo-text; "no visible screen content" prevents rejects.
