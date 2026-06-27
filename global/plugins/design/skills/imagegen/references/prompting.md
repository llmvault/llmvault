# Prompting best practices

This file contains provider-neutral image prompting principles. It covers prompt structure, specificity, composition, constraints, text handling, references, iteration, transparent/cutout requests, and use-case tips.

It intentionally does not cover model selection, execution tools, credentials, provider parameters, local post-processing, storage, billing, or network setup.

## Contents

- [Structure](#structure)
- [Specificity policy](#specificity-policy)
- [Allowed and disallowed augmentation](#allowed-and-disallowed-augmentation)
- [Composition and layout](#composition-and-layout)
- [Lighting, camera, and texture](#lighting-camera-and-texture)
- [Constraints and invariants](#constraints-and-invariants)
- [Text in images](#text-in-images)
- [Input images and references](#input-images-and-references)
- [Transparent and cutout prompts](#transparent-and-cutout-prompts)
- [Iterate deliberately](#iterate-deliberately)
- [Use-case tips](#use-case-tips)
- [Where to find copy/paste recipes](#where-to-find-copypaste-recipes)

## Structure

- Use a consistent order: scene/backdrop -> subject -> key details -> constraints -> output intent.
- Include intended use, such as ad, UI mockup, product shot, slide, sprite, or infographic, to set the level of polish.
- For complex requests, use short labeled lines instead of one long paragraph.
- Prefer concrete visual nouns and relationships over generic quality piles.

## Specificity policy

- If the user prompt is already specific and detailed, normalize it into a clean spec without adding creative requirements.
- If the prompt is generic, add tasteful detail when it materially improves the output.
- Treat examples in `sample-prompts.md` as fully authored recipes, not as permission to expand every prompt to that level.
- For photorealism, include `photorealistic` directly when that is the goal, plus concrete real-world texture such as pores, wrinkles, fabric wear, material grain, fingerprints, dust, or imperfect everyday detail.

## Allowed and disallowed augmentation

Allowed augmentation for generic prompts:

- Composition and framing cues.
- Intended-use or polish-level hints.
- Practical layout guidance.
- Reasonable scene concreteness that supports the request.
- Material, lighting, and texture details that make the output less generic.

Do not add:

- Extra characters, props, or objects that are not implied.
- Brand palettes, slogans, or story beats that are not supplied.
- Arbitrary side-specific placement unless the surrounding layout supports it.
- Unrequested style shifts, new narrative stakes, or decorative clutter.

## Composition and layout

- Specify framing and viewpoint when it matters: close-up, medium shot, wide shot, top-down, low angle, isometric, centered, symmetrical, or rule-of-thirds.
- Call out negative space if the asset needs room for UI or copy.
- Avoid left/right layout decisions unless the user or consuming layout supports them.
- For people, describe body framing, scale, gaze, pose, and object interactions when they matter: `full body visible`, `looking down at the book`, `hands naturally gripping the handlebars`.
- For diagrams and slides, specify reading order, hierarchy, whitespace, and label placement.

## Lighting, camera, and texture

- Use lighting to define mood and clarity: soft daylight, overcast natural light, warm indoor practicals, hard studio rim light, golden hour, or clean shadowless product lighting.
- Use camera language for photos: lens length, depth of field, crop, viewpoint, candid/editorial/studio treatment.
- Use material language to prevent generic surfaces: matte ceramic, brushed steel, worn cotton, rough paper, translucent glass, weathered wood.
- Avoid stacking vague phrases such as `ultra detailed`, `masterpiece`, and `8k` unless they are part of the user's desired style.

## Constraints and invariants

- State what must not change: `keep background unchanged`, `preserve face, pose, lighting, and camera angle`.
- For edits, say `change only X; keep Y unchanged`.
- Repeat invariants on every iteration to reduce drift.
- Include `no watermark`, `no extra text`, or `no logos` when those failures would matter.

## Text in images

- Put literal text in quotes and specify typography, size, color, and placement.
- Require exact spelling and no extra words when text accuracy matters.
- Spell uncommon words letter by letter if needed.
- Keep in-image text short. Dense text, long paragraphs, tables, legends, and tiny labels are high-risk.
- For diagrams or slides, name every required label and ask for clear hierarchy and readable spacing.

## Input images and references

- Do not assume every provided image is an edit target.
- Label each image by index and role: `Image 1: edit target`, `Image 2: style reference`, `Image 3: subject reference`.
- If the user provides images for style, composition, or mood guidance and does not ask to modify them, treat the request as generation with references.
- If the user asks to preserve an existing image while changing specific parts, treat the request as an edit.
- For compositing, describe how the images interact: `place the subject from Image 2 into Image 1`, `match Image 1 lighting and camera perspective`.

## Transparent and cutout prompts

- Ask for the subject isolated on a transparent background when that is the desired output.
- If downstream cutout work may be needed, prompt for a perfectly flat, solid, high-contrast background color that does not appear in the subject.
- Explicitly prohibit shadows, gradients, floor planes, reflections, texture, and lighting variation in the background.
- Ask for crisp edges, generous padding, and no use of the background color inside the subject.
- For hair, fur, feathers, glass, smoke, liquids, translucent materials, and reflective objects, describe what the edge should preserve and avoid heavy halos or lost fine detail.

## Iterate deliberately

- Start with a clean base prompt, then make small single-change edits.
- Re-specify critical constraints when iterating.
- Prefer one targeted follow-up at a time over rewriting the whole prompt.
- When a result fails, identify the failure mode first: wrong subject, weak composition, poor text, identity drift, style mismatch, missing invariant, or unwanted object.

## Use-case tips

Generate:

- `photorealistic-natural`: Prompt as if a real photo is captured in the moment; use photography language, natural texture, and believable imperfections; avoid over-stylized polish unless requested.
- `product-mockup`: Describe product shape, packaging, materials, label clarity, clean silhouette, and lighting; require exact label text if needed.
- `ui-mockup`: Describe target fidelity first, then layout, hierarchy, practical UI elements, and screen context; avoid concept-art language.
- `infographic-diagram`: Define the audience and layout flow; label parts explicitly; require verbatim text and readable spacing.
- `logo-brand`: Keep it simple and scalable; ask for strong silhouette, balanced negative space, and flat color unless another treatment is requested.
- `ads-marketing`: Write like a creative brief; include audience, brand position, desired vibe, scene, and exact tagline when text must appear.
- `productivity-visual`: Name the exact artifact, define canvas and hierarchy, provide real labels/data, and ask for readable typography and polished spacing.
- `scientific-educational`: Define audience, lesson objective, required labels, scientific constraints, arrows, and scan-friendly whitespace.
- `illustration-story`: Define panels or scene beats; keep each action concrete.
- `stylized-concept`: Specify style cues, material finish, and rendering approach without inventing new story elements.
- `historical-scene`: State the location/date and required period accuracy; constrain clothing, props, and environment to match the era.

Edit:

- `text-localization`: Change only the text; preserve layout, typography, spacing, imagery, and hierarchy; no extra words or reflow unless needed.
- `identity-preserve`: Lock identity, face, body, pose, hair, expression, and character-defining features; change only the specified elements.
- `precise-object-edit`: Specify exactly what to remove or replace; preserve surrounding texture, lighting, shadows, and everything else.
- `lighting-weather`: Change only environmental conditions: light, shadows, atmosphere, precipitation, season, or time of day.
- `background-extraction`: Request a clean isolated subject, crisp silhouette, generous padding, no shadows, no halos, no restyling, and exact preservation of label text.
- `style-transfer`: Specify style cues to preserve, such as palette, texture, brushwork, or rendering approach, and add `no extra elements` to prevent drift.
- `compositing`: Reference inputs by index; specify what moves where; match lighting, perspective, scale, and contact shadows.
- `sketch-to-render`: Preserve layout, proportions, and perspective; choose materials and lighting that support the supplied sketch without adding new elements.

## Where to find copy/paste recipes

For reusable prompt recipes, see `references/sample-prompts.md`. This file focuses on principles, specificity, and iteration patterns.
