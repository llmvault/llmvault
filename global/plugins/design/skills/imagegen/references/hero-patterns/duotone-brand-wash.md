# Duotone brand-wash hero

A photograph graded so hard into two brand colors that it reads as branded texture rather than a photo. Text overlays anywhere. For brands with strong color identities: sports, streaming, events, bold B2C, political/cause campaigns.

## What the layout does to the image

- The image behaves like a colored background; copy (usually the brand's third color — white or black) sits anywhere on top.
- Because tonal range is compressed into two hues, text contrast is uniform and forgiving across the whole frame.
- Often combined with heavy typographic heroes — the image is atmosphere, not information.

## Image requirements

- A subject with strong shapes and clear silhouettes (grading eats fine detail — pick bold geometry, figures, architecture).
- Shadows fall into brand color A (dark), highlights into brand color B (light/accent). Hex codes steer intent, not exact values — always ALSO name the colors in words.
- Overall mid-to-low contrast so no region blocks text.
- Avoid faces needing natural skin tones — duotone stylizes skin, which suits energetic brands but not trust-critical ones (healthcare, finance advice).

## Aspect ratio

`16:9`.

## Prompt recipe

[photographic subject with bold shapes] + [explicit duotone grade: "shadows fall into <color word + hex>, highlights pick up <color word + hex>, almost duotone but still photographic"] + [mood] + [no text]. The "almost duotone but still photographic" phrasing reliably produces the branded-texture look without losing photographic detail.

## Example prompts

Construction tech brand, navy + yellow:

> Corporate photograph of a tower crane and building steel framework at dusk, low angle, bold graphic silhouettes. Color graded so the shadows fall into a deep navy blue #0F1E3C and the highlights pick up a warm construction yellow #FFC400 — almost duotone but still photographic. Large areas of clean gradient sky around the structures, minimal and high-end. Slightly soft contrast throughout. No people, no text, no letters, no logos.

Fitness brand, deep purple + coral:

> Dynamic photograph of a sprinter mid-stride on an outdoor track, shot from a low side angle with dramatic body silhouette. Color graded into a bold two-tone wash: shadows fall into deep violet #2D1B4E, highlights and rim light pick up hot coral #FF6B5A — almost duotone but still photographic, with soft compressed contrast. Slight motion blur on the trailing leg, gritty energetic mood, film grain. No visible brand marks on clothing, no text, no letters, no logos.

## Pitfalls

- Promising exact hex reproduction to a client will disappoint — the model approximates hue; final color-lock happens with a CSS blend/filter on top if precision matters.
- High-contrast grading defeats the purpose (text becomes unreadable in highlight patches); ask for "soft compressed contrast".
- Busy multi-subject scenes turn to mud in two colors; one bold subject wins.
