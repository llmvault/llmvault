# Flat illustration hero

A flat vector-style illustrated scene as the hero visual — beside the copy, beneath it, or as a full-width band. The friendly-SaaS classic (HR, education, productivity, fintech-lite). Reads approachable, affordable, human.

## What the layout does to the image

- Usually placed in a split layout's image column or as a full-width band under the headline.
- The illustration's background color often extends into the section background — pick the canvas color deliberately so CSS can match it.
- No text overlays the busy areas, but generous internal sky/background regions help the composition breathe.

## Image requirements

- **Full-bleed composition is mandatory** (hard rule 3 in the main skill): "the scene fills the entire canvas edge to edge with no empty margins — every part of the frame contains scenery." Without this the model floats a tiny centered vignette in a sea of flat background — the most common flat-illustration failure.
- A specific scene with named elements and actions — "workers reviewing plans in the foreground", "a crane reaching from bottom to top of the frame". Generic "teamwork illustration" prompts produce clip-art.
- A limited named palette (4–5 colors) including the canvas color.
- Human figures small-to-medium, simplified, no detailed faces.

## Aspect ratio

`16:9` for full-width bands, `4:3` or `1:1` for split columns.

## Prompt recipe

"Flat vector illustration of [scene] that fills the entire canvas edge to edge with no empty margins" + [element spanning bottom-to-top or left-to-right] + [foreground subjects and actions] + [what fills corners and sky] + [named 4–5 color palette] + "clean flat shapes, modern" + [no text].

## Example prompts

Construction management SaaS (16:9 band):

> Flat vector illustration of a city construction scene that fills the entire canvas edge to edge with no empty margins — every part of the frame contains scenery. A tall yellow crane reaches from the bottom edge to the top edge of the frame, geometric buildings under construction span the full width, small workers in hard hats collaborate in the foreground, stacked materials and scaffolding occupy the corners, a warm sun and stylized clouds fill the sky area. Palette: cream sky, navy, construction yellow, coral accents. Clean flat shapes, no gradients, modern and friendly. No text, no letters, no logos.

Online education platform (4:3 column):

> Flat vector illustration filling the entire canvas edge to edge, no empty margins. A cozy study scene: a student at a desk by a large window in the right half, an oversized open book forming a hill in the left foreground with tiny figures climbing its pages, floating geometric shapes — a lightbulb, a paper plane, stars — filling the upper areas, plants anchoring both bottom corners. Palette: warm cream background, deep teal, mustard yellow, soft coral, charcoal. Clean flat shapes, playful but polished. No text, no letters, no logos.

## Pitfalls

- Omitting the full-bleed language is the known catastrophic failure for this style — never skip it.
- "Corporate Memphis" fatigue is real: anchor the scene in the customer's actual domain (cranes, classrooms, kitchens) instead of generic floating people.
- If a result still floats in background color, also add "the scene is cropped by the frame on all four sides" and retry.
