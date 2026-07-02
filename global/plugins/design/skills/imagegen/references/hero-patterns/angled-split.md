# Angled split hero

Text and image regions divided by a diagonal line or sweeping curve instead of a vertical edge — the image is clipped by CSS (`clip-path` or an SVG mask). Conveys motion and energy: sports, fitness, delivery, automotive, youth brands.

## What the layout does to the image

- The image sits on one side (usually right) and is clipped along a diagonal running roughly from top-center to bottom-right, or a curve.
- The clip removes a wedge of the image — anything near the clipped edge gets cut at an angle.
- The diagonal itself becomes a strong compositional line on the page; the image should echo it, not fight it.

## Image requirements

- Subject placed toward the outer (unclipped) side of the image, fully intact in the safe zone — assume the inner 30–40% will be shaved off diagonally.
- Internal diagonals that parallel the expected clip direction: a leaning runner, a crane's jib, a road receding at an angle, a slope.
- Energetic, directional composition; static symmetrical subjects waste this pattern.
- Full-bleed scene fill so the clip never reveals plain background.

## Aspect ratio

`4:3` or `1:1` — the clip consumes width, so wider ratios waste pixels.

## Prompt recipe

[dynamic subject positioned toward the right of the frame] + [strong diagonal element running through the composition] + [environment filling every edge] + [energetic lighting/palette] + [no text].

## Example prompts

Fitness / sports brand:

> High-energy photograph, square composition filled to every edge. A boxer mid-training leans into a heavy bag in the right half of the frame, body angled along a strong diagonal from lower left to upper right, sweat catching dramatic rim light. Chalk dust drifts through a shaft of hard warm light that cuts the frame along the same diagonal. Dark gritty gym environment fills the background completely, deep shadows with punchy orange highlights. Motion, intensity, grain. No text, no letters, no logos.

Delivery / logistics startup (illustration):

> Bold flat vector illustration filling the entire canvas edge to edge, no empty margins. A courier on a scooter races up a diagonal street that runs from the bottom left corner to the upper right corner of the frame, packages strapped on the back, motion lines trailing. Stylized buildings lean along the same diagonal on both sides, clouds and birds fill the remaining sky areas. Vivid palette: cobalt blue, warm yellow, coral, cream. Energetic, modern, clean shapes. No text, no letters, no logos.

## Pitfalls

- A subject centered in the image lands exactly where the clip cuts — keep it in the outer half.
- Horizontal/vertical compositions look accidentally cropped when clipped diagonally; the internal diagonal is what makes the clip look intentional.
- Coordinate with the actual clip direction used in the page's CSS before generating; a mismatch (image diagonal vs clip diagonal) looks broken.
