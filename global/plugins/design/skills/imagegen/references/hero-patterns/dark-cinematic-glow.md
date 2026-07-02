# Dark cinematic glow hero

Near-black scene with one luminous focal element — the Linear/Vercel/dev-tool aesthetic. White text sits anywhere because almost the whole frame is dark. The default for developer tools, AI products, fintech infrastructure, and any dark-UI brand.

## What the layout does to the image

- The section background is dark (near-black or deep brand hue); the image either fills it or blends into it at the edges.
- Text can be centered or left-aligned — the image's glow element should be positioned to balance it, not collide with it.
- Often the image's edges are faded into the section background with CSS masks, so the image should already be darkest at its edges.

## Image requirements

- 70–85% of the frame in deep shadow (not pure black — subtle texture/gradient keeps it alive).
- Exactly ONE light source / glowing focal element: a welder's sparks, a lit window grid, a glowing horizon line, neon-edged structure, luminous cube on a crane hook.
- The glow placed off-center (rule of thirds) opposite where the copy will sit.
- Restrained palette: one or two hues in the glow against neutral darkness. Teal/amber, violet/magenta, or brand hue.

## Aspect ratio

`16:9`.

## Prompt recipe

[dark scene, time of night] + [single named glow element and its position in the frame] + [what the darkness is made of — texture, silhouettes] + [palette of the glow] + [vignetted/darkest at the edges] + [no text].

## Example prompts

Developer tools / AI product:

> Cinematic night photograph of a construction site after rain, almost entirely in deep shadow. In the right third of the frame, a lone welder crouched on a steel deck sends up a small shower of glowing orange sparks, the only bright light in the scene, its warm glow reflecting in wet concrete panels below. Silhouettes of scaffolding and a distant crane are barely visible against a deep teal-black sky. The edges of the frame fade to near black. Anamorphic widescreen feel, deep teal shadows with amber highlights, quiet and epic. No text, no letters, no logos.

Fintech / data platform (abstract):

> Dark abstract 3D render, near-black frame with subtle depth. A single thin line of luminous cyan light traces a horizon across the lower left of the frame, illuminating faint geometric ridges below it like a topographic surface fading into darkness. Soft volumetric glow rises from the line, everything else dissolves into deep charcoal-blue shadow, darkest at the frame edges. Minimal, precise, futuristic. No text, no letters, no logos.

## Pitfalls

- Two competing light sources destroy the pattern — "the only bright light in the scene" is load-bearing.
- Pure black crushes to banding on screens; ask for "deep shadow with subtle texture", not "black background".
- Keep the glow off-center; a centered glow fights centered headlines exactly like a centered subject does.
