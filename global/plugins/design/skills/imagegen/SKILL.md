---
name: "imagegen"
description: "Provider-neutral prompting guide for high-quality image generation and image editing. Use when an agent needs to craft, critique, refine, or structure prompts for photos, illustrations, sprites, product mockups, UI mockups, diagrams, ads, logos, transparent cutouts, or reference-based visual variants. This skill covers prompt strategy only; do not use it for runtime setup, credentials, billing, network configuration, execution instructions, or model-parameter guidance."
---

# Image Generation Prompting Guide

This skill teaches how to write strong prompts for image generation and image editing. It is intentionally provider-neutral and prompt-only.

It does not include runtime instructions, provider-specific model notes, integration examples, credentials, environment variables, network setup, or local post-processing guidance.

## Use this skill for

- Turning a rough visual request into a clear production prompt.
- Refining prompts for higher visual quality, clearer composition, better text rendering, or fewer unwanted artifacts.
- Writing prompts for new images: photos, illustrations, concept art, product mockups, UI mockups, game assets, ads, diagrams, and logos.
- Writing prompts for edits: object replacement, background changes, lighting/weather changes, text replacement, compositing, style transfer, sketch-to-render, and identity-preserving edits.
- Using reference images by clearly labeling each image's role.
- Creating prompt variants for exploration without changing the user's core intent.

## Do not use this skill for

- Running an image model or choosing a provider.
- Installing tools, wiring integrations, or configuring credentials.
- File movement, asset storage, or post-processing implementation.
- Creating deterministic SVG, HTML, CSS, canvas, or vector-native assets when those are better authored directly.
- Extending an existing icon, logo, or illustration system where repo-native source files should be edited instead.

## Core workflow

1. Identify the intent: generate a new image, edit an existing image, or generate with reference images.
2. Identify the asset's job: hero image, product shot, ad, slide, game sprite, diagram, icon, logo exploration, etc.
3. Label every input image by role: edit target, subject reference, style reference, composition reference, insert element, or mood reference.
4. Classify the request using the taxonomy below.
5. Preserve user-provided constraints, exact text, subject identity, brand requirements, and avoid-list items.
6. Rewrite the prompt as a concise visual brief using only the schema fields that help.
7. Add negative constraints only when they prevent likely failure.
8. For edits, state the intended change and the invariants in the same prompt.
9. For prompt variants, change one dimension at a time: composition, lighting, medium, palette, or mood.
10. Before finalizing, check that the prompt names the subject, setting, style, composition, lighting, constraints, and any exact text.

## Intent rules

- If the user wants to modify an existing image while preserving parts of it, treat the request as an edit.
- If the user provides images for style, composition, mood, or subject guidance but does not ask to modify those images, treat the request as generation with references.
- If the user provides no image, treat the request as generation.
- If the request is for an icon, logo, or UI graphic that should match existing repo-native vector/code assets, prefer direct native editing instead of generated-image prompting.

## Specificity policy

Use the user's prompt specificity to decide how much augmentation is appropriate:

- If the prompt is already specific and detailed, preserve that specificity and only normalize or structure it.
- If the prompt is generic, add tasteful detail only when it materially improves the result.

Good augmentation:

- Composition and framing hints.
- Intended-use or polish-level hints.
- Practical layout guidance.
- Reasonable scene concreteness that supports the user's request.
- Material, texture, lighting, and camera details when they match the goal.

Avoid augmentation that invents:

- Extra characters, props, or objects not implied by the request.
- Brand names, slogans, palettes, or story beats not supplied by the user.
- Arbitrary left/right placement unless the surrounding layout supports it.
- Unrequested style shifts, decorative flourishes, or dramatic narratives.

## Use-case taxonomy

Classify each request into one of these buckets and keep the slug consistent across prompts and notes.

Generate:

- `photorealistic-natural` - candid/editorial lifestyle scenes with real texture and natural lighting.
- `product-mockup` - product/packaging shots, catalog imagery, merch concepts.
- `ui-mockup` - app/web interface mockups and wireframes; specify the desired fidelity.
- `infographic-diagram` - diagrams/infographics with structured layout and text.
- `scientific-educational` - classroom explainers, scientific diagrams, and learning visuals with required labels and accuracy constraints.
- `ads-marketing` - campaign concepts and ad creatives with audience, brand position, scene, and exact tagline/copy.
- `productivity-visual` - slide, chart, workflow, and data-heavy business visuals.
- `logo-brand` - logo/mark exploration, vector-friendly.
- `illustration-story` - comics, children's book art, narrative scenes.
- `stylized-concept` - style-driven concept art, 3D/stylized renders.
- `historical-scene` - period-accurate/world-knowledge scenes.

Edit:

- `text-localization` - translate/replace in-image text while preserving layout.
- `identity-preserve` - try-on, person-in-scene, character continuation; lock face/body/pose or character design.
- `precise-object-edit` - remove or replace a specific element.
- `lighting-weather` - time-of-day, season, or atmosphere changes only.
- `background-extraction` - transparent background, clean cutout, or silhouette isolation.
- `style-transfer` - apply a reference style while changing subject or scene.
- `compositing` - multi-image insert/merge with matched lighting and perspective.
- `sketch-to-render` - drawing/line art to photoreal or polished render.

## Shared prompt schema

Use this labeled spec as prompt scaffolding. Include only fields that help.

```text
Use case: <taxonomy slug>
Asset type: <where the asset will be used>
Primary request: <user's main prompt>
Input images: <Image 1: role; Image 2: role> (optional)
Scene/backdrop: <environment>
Subject: <main subject>
Style/medium: <photo/illustration/3D/vector-like/etc>
Composition/framing: <wide/close/top-down; placement; crop>
Lighting/mood: <lighting and emotional tone>
Color palette: <palette notes>
Materials/textures: <surface and detail notes>
Text (verbatim): "<exact text>"
Constraints: <must keep/must include/must avoid>
Avoid: <negative constraints>
```

Notes:

- `Asset type` and `Input images` are prompt scaffolding, not tool arguments.
- `Scene/backdrop` means the visual setting, not a runtime background parameter.
- Keep prompts compact. A complete visual brief beats a long pile of generic quality words.
- For edits, explicitly list invariants: `change only X; keep Y unchanged`.
- If critical information is missing and would block success, ask a question; otherwise make the most conservative reasonable assumption.

## Prompting best practices

- Structure prompts as scene/backdrop -> subject -> key details -> constraints -> output intent.
- Include intended use to set the mode and polish level.
- Use camera, lens, framing, and lighting language for photorealism.
- Use material and texture language to avoid plastic or generic surfaces.
- Quote exact text and specify typography, size, color, and placement.
- For tricky words, spell them letter by letter and require verbatim rendering.
- For multi-image inputs, reference images by index and describe how each image should be used.
- For edits, repeat invariants every iteration to reduce drift.
- Iterate with a single targeted change rather than rewriting the whole prompt.
- Use negative constraints sparingly: remove common failure modes without overloading the prompt.

## Transparent and cutout prompts

For transparent-background or cutout requests, prompt for the cleanest possible isolation:

```text
Create the requested subject isolated on a clean transparent background.
If true transparency is not available, use a perfectly flat, solid, high-contrast background color that does not appear in the subject.
Keep the subject fully separated from the background with crisp edges and generous padding.
No cast shadow, contact shadow, reflection, watermark, extra text, background texture, gradient, floor plane, or lighting variation.
```

For complex subjects such as hair, fur, feathers, glass, smoke, liquids, translucent materials, or reflective objects, be explicit about edge quality and what should remain visible.

## Quality review checklist

Before using a prompt, check:

- Subject: Is the main subject unambiguous?
- Setting: Is the environment clear enough without being over-specified?
- Style: Is the desired medium or realism level stated?
- Composition: Is framing, scale, and placement clear where it matters?
- Lighting: Does the prompt define mood and visibility?
- Text: Is all required text quoted exactly?
- Invariants: For edits, are preserved elements clearly named?
- Avoid list: Does it block likely problems without contradicting the request?

## Reference map

- `references/prompting.md` - deeper prompt principles, composition guidance, text handling, reference-image usage, transparent/cutout prompts, and use-case tips.
- `references/sample-prompts.md` - reusable prompt recipes by generation/edit category.
