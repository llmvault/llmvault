# Sample prompts

Copy/paste prompt recipes for generation and edits, written in the house prompt
format: **one flowing paragraph** in the clause order from `references/prompting.md`
(medium/style → subject and action → environment → composition/framing →
lighting/mood → palette/materials → constraints → negatives) — the same format as
the example prompts in `references/hero-patterns/`. They are complete,
fully-authored examples — not the default amount of detail to add to every request
(see the specificity policy in `references/prompting.md`).

When adapting a recipe:

- Keep all user-provided requirements; adapt the recipe around them.
- Do not invent extra story elements, brands, or palettes.
- Keep exact text, constraints, and reference-image roles explicit.
- Drop clauses that don't apply; never pad a prompt with quality words.

**Landing-page and marketing-site visuals do not belong here** — route them through
the hero pattern tables in `SKILL.md` and use the example prompts inside
`references/hero-patterns/<slug>.md`. This file covers everything else.

## Generate

### photorealistic-natural

> Photorealistic candid photograph of an elderly sailor on a small fishing boat,
> hands mid-pull adjusting a net — weathered skin with deep wrinkles and sun
> texture, worn oilskin jacket. Coastal water with soft haze behind him. Medium
> close-up at eye level, the sailor in the right two thirds of the frame facing
> into it. Soft coastal daylight, shallow depth of field, subtle film grain; real
> skin texture, worn fabric, salt-worn wood. Natural color balance with no heavy
> retouching, no glamorization, no staged studio look. No text, no letters, no
> logos, no watermark.

### product-mockup

> Premium studio product photograph of a single matte black shampoo bottle with a
> satin cap and a blank label area, centered at a slight three-quarter angle on a
> clean studio gradient from light gray to white. Softbox lighting with clean
> highlights and controlled shadows, a soft contact shadow grounding the bottle
> and a subtle reflection beneath it, generous empty margin on all sides. Blank
> packaging with no logos or trademarks. No text, no letters, no watermark.

### ui-mockup

> Realistic mobile app home screen for a local farmers market — a practical
> product UI, not concept art. Clean vertical mobile layout with blocks in
> reading order: a search bar at the top, a featured vendor card, a scrolling
> list of daily specials, and a bottom tab bar. Practical spacing and clear
> typography with minimal visible text. No logos or trademarks, no watermark.

### infographic-diagram

> Clean vector-style infographic of an automatic coffee machine's flow on a light
> neutral background, vertical poster layout reading top to bottom: bean hopper,
> grinder, brew group, boiler, water tank, and drip tray connected by clear
> arrows with simple callouts. Render each label exactly once, spelled exactly:
> "Bean Hopper", "Grinder", "Brew Group", "Boiler", "Water Tank", "Drip Tray".
> Clear hierarchy and strong contrast, no extra words, no logos, no watermark.

### scientific-educational

> Flat scientific diagram for a high school classroom handout on a clean white
> background: glucose turning into energy inside a cell, showing glycolysis, the
> Krebs cycle, and the electron transport chain with consistent icons, arrows,
> and readable labels. Landscape slide-style layout with clear hierarchy and
> generous whitespace. Render these labels exactly, spelled exactly: "Cellular
> Respiration at a Glance", "Glucose", "Pyruvate", "ATP", "NADH", "FADH2", "CO2",
> "O2", "H2O". Scientifically plausible, no tiny text, no extra decoration, no
> watermark.

### logo-brand

Real logo work does not use these raster recipes — load the `logo-design` skill
instead. It generates the mark as a native SVG (`hivy_generate_vector_image`) with
constructive-geometry prompts and composes every variant (lockups, app icon,
monochrome, reverse, favicon) deterministically from the same geometry, so the
mark stays byte-identical across the whole pack. A raster logo generated here
would need vectorizing and re-deriving by remix, with drift at every step.

### illustration-story

> Comic illustration in four equal vertical panels with clear borders, one
> readable action per panel: a pet reacting to its owner leaving, sulking alone,
> relaxing, then returning to a composed pose as the owner comes home — the same
> cozy living room and the identical character design repeated in every panel.
> No text, no letters, no logos, no watermark.

### stylized-concept

> Cinematic concept art of a cavernous industrial hangar interior with tall
> support beams and drifting fog, a compact shuttle parked near the center.
> Wide-angle, low-angle composition with the scene filling the frame to every
> edge and a deep sense of scale. Volumetric light rays cutting through the fog,
> industrial realism. No text, no letters, no logos, no watermark.

### ads-marketing

> Polished youth streetwear campaign photograph with a 35mm editorial look: a
> group of friends hanging out in a stylish urban setting, natural poses and
> genuine expressions. Vertical composition; the upper quarter of the frame is a
> plain sunlit concrete wall above the group. Tasteful golden-hour warmth,
> contemporary and energetic. Render the tagline "Yours to Create." exactly once
> in clean legible typography in the upper plain area, spelled exactly — no
> extra text, no unrelated logos, no watermark.

### productivity-visual

> One clean modern pitch-deck slide in 16:9 with a white background and crisp
> sans-serif typography: a TAM/SAM/SOM concentric-circle diagram beside a small
> growth bar chart spanning 2021 to 2026, with clear data hierarchy and polished
> spacing. Render these strings exactly, spelled exactly: "Market Opportunity",
> "TAM: $42B", "SAM: $8.7B", "SOM: $340M", "AGI Research, 2024", "Internal
> analysis". Readable sizes throughout — no clip art, no stock photography, no
> decorative clutter, no watermark.

### historical-scene

> Photorealistic wide shot at eye level of an outdoor crowd scene in Bethel, New
> York on August 16, 1969: an open field filled with a crowd in period-accurate
> clothing and authentic staging throughout, rendered with era-appropriate film
> grain and color. Period-accurate details only, no modern objects. No text, no
> logos, no watermark.

## Website assets (non-hero)

Hero images: use `references/hero-patterns/` — not this section.

### Section illustration

> Flat illustration for a feature section: a loose cluster of rounded translucent
> shapes linked by thin flowing lines, slightly off-center on a soft light-gray
> field with a faint paper texture filling the frame to every edge. Muted neutral
> palette with one soft blue accent, soft shadows, restrained contrast. The scene
> reaches every edge of the canvas. No text, no letters, no logos, no watermark.

### Blog header image

> Photorealistic top-down photograph of a warm wooden tabletop filling the entire
> frame: an open blank notebook and a ceramic coffee cup arranged in the right
> two thirds, the left third plain empty tabletop in even light. Soft morning
> window light. Blank notebook pages. No text, no letters, no logos, no
> watermark.

## Game assets

Prompt recipe: [realistic/stylized] + [concept art / character render / UI icon /
tileable texture] + [subject with material specificity] + [backdrop or set
dressing] + [framing and focal point] + [time of day / mood / atmosphere] + [no
text, no letters, no logos, no watermark].

### Character concept

> Stylized-realism game character render of a desert scout in a neutral hero
> pose, full body visible: long weathered coat, leather satchel, practical
> layered travel clothing with visible fabric wear. Simple flat backdrop with
> generous padding around the figure. No text, no letters, no logos, no
> watermark.

### UI icon

> Painted game UI icon of a round shield with a subtle rune pattern, centered
> with generous padding and a clear silhouette, no background scene elements.
> No text, no letters, no logos, no watermark.

### Tileable texture

> Seamless tileable game texture of worn sandstone blocks under neutral even
> lighting, PBR-style surface detail, edges matching perfectly for tiling, no
> obvious focal elements. No text, no logos, no watermark.

## Wireframes

Wireframes are the sanctioned exception to the no-layout-vocabulary rule — see
`references/prompting.md`.

### Homepage desktop

> Low-fi grayscale website wireframe of a SaaS homepage in a landscape desktop
> layout, blocks in reading order: top nav, hero block with a heading and button,
> three feature cards, a testimonial strip, a pricing preview, and a footer.
> Label major blocks only. No color, no logos, no real photos, no watermark.

### Mobile onboarding flow

> Low-fi grayscale wireframe of a three-screen mobile onboarding flow, portrait
> screens side by side: screen one a heading and a button, screen two feature
> bullets, screen three form fields and a button. Label screens and major blocks
> only. No color, no logos, no real photos, no watermark.

## Logos

Logo work is owned by the `logo-design` skill — do not prompt logos with the
raster recipes in this file. That skill generates marks as native SVG via
`hivy_generate_vector_image`, prompts by *construction* (strokes, arcs, ratios,
terminals, negative space — see its `references/mark-prompts.md`), covers custom
letterforms and monograms (`references/type-and-color-prompts.md`), and builds the
full production pack (lockups, app icon, monochrome, reverse, favicon, transparent
PNGs) with a byte-identical mark. Vibe-level prompts like "geometric leaf symbol
suggesting growth" are exactly what it exists to replace.

## Edit

Edit prompts name each input image's role inline, state the single change, and
enumerate what must not change (see the invariant language in
`references/consistent-assets.md`).

### text-localization

> Image 1 is the original infographic. Replace only the label text: "Bean Hopper"
> with "Tolva", "Grinder" with "Molino", "Brew Group" with "Grupo de infusion",
> "Boiler" with "Caldera", "Water Tank" with "Deposito de agua", and "Drip Tray"
> with "Bandeja de goteo". Change only the text; preserve the layout, typography,
> spacing, and hierarchy exactly; no extra words; do not alter logos or imagery.

### identity-preserve

> Image 1 is a photo of a person; Images 2 onward are clothing references.
> Replace only the clothing with the provided garments. Preserve the face, body
> shape, pose, hair, expression, and identity completely; match the original
> lighting and shadows; keep the background unchanged; add no accessories and no
> text.

### precise-object-edit

> Image 1 is a room photo. Replace only the white chairs with wooden chairs.
> Preserve the camera angle, room lighting, floor shadows, and every surrounding
> object; keep all other aspects unchanged.

### lighting-weather

> Image 1 is the original photo. Make the scene look like a winter evening with
> gentle snowfall. Preserve the subject's identity, the geometry, the camera
> angle, and the composition; change only the lighting, atmosphere, and weather.

### background-extraction

> Image 1 is a product photo. Isolate the product on a clean transparent
> background — if true transparency is not available, use a perfectly flat
> high-contrast solid background color that does not appear in the subject.
> Crisp silhouette with generous padding; no shadows, gradients, texture,
> reflections, floor plane, or lighting variation in the background; no halos or
> fringing; preserve any label text exactly; no restyling.

### style-transfer

> Image 1 is the style reference. Apply Image 1's visual style — its palette,
> texture, and brushwork — to a man riding a motorcycle on a plain white
> backdrop. Add no extra elements.

### compositing

> Image 1 is the base scene; Image 2 is the subject to insert. Place the subject
> from Image 2 next to the person in Image 1, matching lighting, perspective,
> and scale, with a soft contact shadow. Keep the base framing unchanged; add no
> extra elements.

### character consistency workflow

> Image 1 is the previous character anchor illustration. Continue the story with
> the same character in the same children's book watercolor style as Image 1:
> the same young forest hero gently helping a frightened squirrel out of a
> fallen tree in a snowy forest after a winter storm. Do not redesign the
> character; preserve facial features, proportions, outfit, color palette, and
> personality. No text, no watermark.

### sketch-to-render

> Image 1 is a drawing. Turn it into a photorealistic image, preserving the
> layout, proportions, and perspective exactly and choosing realistic materials
> and lighting. Do not add new elements or text.
