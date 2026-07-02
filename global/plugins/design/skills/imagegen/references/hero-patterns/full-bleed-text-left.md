# Full-bleed hero, text left

The most common marketing hero: an edge-to-edge background image with the copy block (headline, subhead, CTA) occupying the left third to half. The image's subject must live on the right; the left must be genuinely quiet.

## What the layout does to the image

- Copy is left-aligned inside a container roughly the left 40–50% of the viewport; text is usually white or dark depending on the quiet zone's tone.
- A left-to-right gradient scrim is often applied in CSS (dark on the left, transparent right), so the left side should tolerate darkening.
- On mobile the layout usually stacks: the image becomes a shorter band or background, center-cropped — so the subject should sit right-of-center but not in the extreme right edge.

## Image requirements

- All visual weight (subject, detail, contrast) in the right 50–60% of the frame.
- The left 40–50% is a continuous low-detail field: sky, wall, water, fog, out-of-focus environment. It must be *one thing*, not scattered small objects.
- Consistent tone in the quiet zone so text contrast is predictable — either clearly dark or clearly light, not patchy.
- Subject facing or moving *toward* the left (into the copy) creates a natural eye-line loop from image to headline.

## Aspect ratio

`16:9`. For very tall heroes use `3:2` and let CSS crop vertically.

## Prompt recipe

CRITICAL: never say "space for text" (see the hard rules in the main skill). Describe the quiet zone as scenery: [what fills the left of the frame, named medium, soft/plain] + [subject positioned in the right third, facing left] + [lighting/mood] + [grade] + [no text].

## Example prompts

SaaS / technology company (photographic):

> Professional photograph, wide composition. A vast expanse of soft dawn sky in muted warm grey and pale amber fills the left half of the frame — smooth gradient clouds, no objects. In the right third, a modern glass office tower under construction rises with a yellow tower crane beside it, crisp silhouette catching golden rim light, angled slightly toward the left of the frame. Low camera angle, calm and premium corporate mood, subtle atmospheric haze. Shot on a 35mm lens at f/8, clean and sharp. No people in the left half, no text, no letters, no logos.

Coffee brand / e-commerce (photographic):

> Moody product-environment photograph. The left half of the frame is a smooth dark walnut café wall in soft shadow, evenly lit, completely plain. On the right side, a barista's hands pour steaming milk into a ceramic cup on a wooden counter, warm window light from the right edge, rich browns and creams, shallow depth of field with the steam drifting leftward across the scene. Kodak Portra warmth, intimate and artisanal. No text, no letters, no logos.

## Pitfalls

- This pattern is the highest-risk one for the page-mockup failure mode: prompting with layout language ("space for a headline") makes the model render a literal web page with garbled placeholder text. Re-read hard rule 1 in the main skill before writing the prompt.
- If small objects sneak into the quiet zone, add "the left half contains only [sky/wall/water], nothing else" and retry.
- Check the quiet zone's tone in the result: patchy light/dark means unreadable text; regenerate rather than accept.
