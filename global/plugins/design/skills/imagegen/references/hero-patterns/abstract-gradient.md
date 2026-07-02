# Abstract gradient hero

No literal subject at all: soft gradients, blurred color fields, glass or fluid 3D forms as pure atmosphere behind the copy. The easiest pattern for text overlay and the default when the brand is the typography. Common in AI, fintech, developer tools, and premium B2B. (For non-hero *section* backgrounds and material textures, use the backgrounds family — `references/backgrounds.md` — which trades this pattern's single accent event for total recession.)

## What the layout does to the image

- The image is a full-section background; the headline (often large, centered or left) sits directly on it with no scrim.
- Because there's no subject, text can go anywhere — but contrast still matters: the copy area needs consistently light or dark tone.
- Often blended into the page with CSS masks/fades at the section edges; images that already darken or simplify toward the edges blend seamlessly.

## Image requirements

- 2–4 named hues maximum, flowing smoothly — more turns to rainbow noise.
- Very soft transitions: "out of focus", "diffused", "like light through frosted glass". Hard edges fight the type.
- Tonal plan: decide where the copy sits and make that region the calmest, most uniform area (name it: "the upper left dissolves into near-solid deep indigo").
- Optional single accent event (one bright bloom, one light streak) placed off-center, opposite the copy.
- Subtle grain/noise texture prevents banding on large screens — ask for it.

## Aspect ratio

`16:9`. Generate `9:16` separately for mobile if the section is full-height.

## Prompt recipe

[medium: "abstract gradient background" / "soft 3D fluid forms" / "blurred color field photograph"] + [2–4 named hues and where each lives in the frame] + [texture: grain, frosted glass, silk] + [the calm region and its tone] + [optional single accent and its position] + [no text].

## Example prompts

AI / developer platform (dark):

> Abstract gradient artwork, very soft and diffused, like colored light seen through frosted glass. Deep indigo and near-black fill most of the frame, dissolving to an almost solid calm dark field across the left half. In the lower right, a single soft bloom of electric violet melting into warm coral rises like slow smoke, gently out of focus. Fine film grain across the whole image to prevent banding, smooth silky transitions, no hard edges anywhere. Premium, quiet, futuristic. No text, no letters, no logos, no objects.

Wellness / consumer fintech (light):

> Soft abstract background of flowing translucent silk-like forms in pale peach, warm cream and blush pink, drifting diagonally across the frame, heavily diffused and dreamlike. The upper two thirds settle into a nearly uniform light cream field, calm and even; gentle rose-tinted folds gather along the bottom edge with soft shadows giving faint depth. Subtle fine grain, airy and premium, morning-light warmth. No text, no letters, no logos, no objects.

## Pitfalls

- Skipping the tonal plan produces beautiful images with nowhere readable to put text — always name the calm region and its tone.
- More than four hues or any "vibrant rainbow" language collapses into noise.
- Banding is the technical killer: "fine film grain" is cheap insurance.
- Watch for accidental objects (the model loves sneaking in horizons and orbs); "no objects" belongs in the negative clause.
