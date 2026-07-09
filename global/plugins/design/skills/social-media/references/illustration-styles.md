# Illustration styles — the catalog, the prompts, and the brand fit

Load this file after `references/styles.md` points you at the illustration-driven direction (or the soft-3D-object direction needs style depth). It documents the illustration genres that generate reliably at high quality, how to prompt each one, and how to match style to brand and industry. Every genre here was tested; fidelity notes and failure modes come from those runs, not theory.

Two rules frame everything below:

- **Style keywords execute with high fidelity.** "Risograph print", "paper-cut collage", "line art with spot color", "flat vector", "soft 3D render" are the highest-leverage words in the prompt — the model commits to them fully. Vague style language ("beautiful illustration", "modern art style") is what produces generic clip-art.
- **Composition is never free.** Each genre has a gravity (floating vignette, centered diorama, tilt-shift blur) and a counter. The counter is part of the recipe, not optional.

## The catalog

### 1. Flat vector (full-bleed scene)

**Look:** clean geometric shapes, no gradients, 4–5 flat colors, modern SaaS landing-page art.
**Fidelity:** high — WITH the composition fix. Without it, guaranteed failure (a tiny centered vignette floating in empty background).
**Brand fit:** friendly SaaS, HR/people tools, education, health-lite, productivity, fintech that wants approachable. The "default nice" illustration style — pick it when the brand is friendly but nothing about it demands texture or drama.
**Social composition:** the quiet zone is a flat field *inside* the scene — a sky band, a wall, a floor plane — placed where the recipe's layout puts type. In portrait (3:4), stack the scene: type band top, scene builds upward from the bottom edge.

Template (adapt subject and palette):

> Flat vector illustration of [scene] that fills the ENTIRE canvas edge to edge with no empty margins and no background border — a full-bleed composition. [Scene elements placed across the frame, each anchored to a region or edge]. Every part of the frame contains scenery: [what fills the top / the quiet zone — e.g. "a smooth cream sky fills the upper third"], [what fills the middle and bottom]. Palette: [4–5 named brand-adjacent colors]. Clean flat shapes, no gradients, modern SaaS landing page art style. No text, no letters, no logos, no watermark.

**Pitfalls:** dropping the full-bleed language (the one proven fatal omission); more than ~5 colors muddies the flatness; "flat" elements can acquire physical depth (dots become holes) — add "perfectly flat 2D, no depth, no shadows" when the scene includes patterns.

### 2. Isometric scene

**Look:** precise 30° diorama, tiny workers/objects, technical charm.
**Fidelity:** high for the style itself; the composition floats — isometric inherently builds a platform with plain background showing at the corners. Slightly "clip-art" energy; premium brands should prefer #3 or #5.
**Brand fit:** logistics, infrastructure, devops/cloud, manufacturing, supply chain, smart-city — anything whose story is *systems with moving parts*. Explains processes well, so it suits "how it works" carousel beats.
**Social composition:** don't fight the gravity — *use* it. Accept the centered diorama as a `subject`-role object: solid brand-color ground, diorama in the lower two thirds, type in the clean band above. For full-bleed instead, demand "the scene spans corner to corner, cropped by the frame on all four sides" and expect to retry once.

Template:

> Detailed isometric vector illustration of [system/scene] viewed at a 30-degree isometric angle, sitting on a smooth [brand color] background. [3–6 concrete elements: vehicles, tiny workers, stacked goods, buildings with exposed floors]. Soft shadows, clean precise linework, rich but harmonious palette of [4 colors]. The upper [third] of the frame is smooth, empty [color] background above the scene. No text, no letters, no logos, no watermark.

**Pitfalls:** asking for full-bleed AND getting corner gaps (decide up front which composition you want); overcrowding — 3–6 elements read, 12 blur.

### 3. Soft 3D render (premium gradient style)

**Look:** smooth rounded matte 3D forms, subtle gradients, soft studio light, gentle ambient occlusion — Stripe/Ramp-era marketing art.
**Fidelity:** high and genuinely premium; strong centered-podium gravity (it will build you a pedestal scene with dead edges).
**Brand fit:** fintech, AI/ML platforms, developer infrastructure, B2B platforms with money in the room. The style says "we are calm and well-funded."
**Social composition:** two working modes. (a) `subject` mode — a rounded 3D object cluster **cropped by the frame** along the edges opposite the text zone (the celebration-card balloon recipe is this genre). (b) Centered-hero mode — accept the podium: object centered in the lower half, type above. Say "soft 3D render, matte materials" — never "claymorphism" (see #8).

Template (subject mode):

> Soft 3D render with matte materials: [object or abstract cluster — stacked blocks, rounded product forms, a stylized machine] rising along the [bottom and right] edges of the frame, cropped by the frame on those sides. The [upper left two thirds] of the frame is a smooth, empty [brand color] backdrop. Soft studio lighting from the upper left, gentle [accent color] rim light. Palette: [bg color named twice], [2–3 accents]. Premium, futuristic, calm. No text, no letters, no logos, no watermark.

**Pitfalls:** background color drift (name it twice, blend with scrim); glossy plastic when you wanted matte — say "matte" with the materials, not as an afterthought.

### 4. Risograph / textured editorial — the proven standout

**Look:** bold flat shapes, visible grain, ink misregistration, 2-color print on paper — magazine-cover quality, zero stock energy. The highest-rated genre tested.
**Fidelity:** excellent — grain texture, riso registration, and dynamic diagonal composition all land.
**Brand fit:** human/anti-corporate brands — communities, newsletters, events and conferences, climate, coffee/food craft, culture, labor and trades, indie dev tools with personality. The best answer whenever the brief says "we don't want to look corporate." Bonus: a 2-color brand palette maps 1:1 onto the 2-ink riso conceit.
**Social composition:** riso loves a diagonal — one bold element cutting the frame, silhouettes doing real work, textured sun/sky filling the quiet zone. Portrait adaptation: run the diagonal from bottom-left to mid-right and keep the top third as textured flat sky for type.

Template:

> Editorial illustration in a textured risograph print style, the artwork covering the entire frame with no borders. [Scene: people doing one concrete thing], rendered in bold flat shapes with visible grain texture and slight ink misregistration, like a two-color riso print in [brand color 1] and [brand color 2] with [paper color] showing through. Dynamic diagonal composition: [the main element] cuts across the frame, [supporting elements anchored to edges], textured [sun rays / flat sky] fills the [top third]. Hand-crafted, warm, human, magazine-cover quality. No text, no letters, no logos, no watermark.

**Pitfalls:** almost none observed — this genre is the safe *distinctive* choice. Keep it to two inks + paper; a third color starts eroding the riso credibility.

### 5. Line art + spot color

**Look:** confident thin contour lines on a quiet ground, with 2–3 elements filled in flat spot color that pop exactly where placed.
**Fidelity:** high for linework and spot-color placement. Known leak: stray letter-like marks appear in dense linework even with no layout words in the prompt.
**Brand fit:** premium studios, architecture, legal, consulting, finance-editorial, luxury services — brands that whisper. The most typographic-friendly illustrated style: line density can thin out wherever type needs room, and it sits beautifully next to display type.
**Social composition:** let linework fill the frame but thin it toward the text zone ("the linework grows sparse in the upper third, only single-stroke clouds"); the spot colors are your brand accents doing focal work.

Template:

> Sophisticated line art illustration spanning the full frame: [scene] drawn in confident thin [charcoal-black] contour lines on a [warm cream / brand ground] background, with only [two or three] elements filled in flat spot color — [element: accent color, element: accent color]. The linework is dense enough to fill the canvas — [overlapping outlines, lattice, single-stroke birds and clouds] reaching every edge — but grows sparse in the [text zone region]. Elegant, architectural, premium studio brand illustration. No text anywhere, no letters, no writing, no stray marks that resemble letters, no logos, no watermark.

**Pitfalls:** the letter-leak — use the strengthened negative above and inspect linework at full zoom before accepting; spot colors multiplying beyond the asked count (re-state "only three elements are colored").

### 6. Paper-cut collage

**Look:** layered colored card with real drop shadows and visible cut edges — a shadowbox photographed straight on. Tactile, dimensional, warm.
**Fidelity:** excellent — layering, texture, and shadows all land; naturally produces a solid foreground band that is perfect headline space (proven in testing: a solid dark foreground layer carried white display type).
**Brand fit:** education, children/family, nonprofits, community orgs, crafts and makers, celebrations and internal-culture posts, HR announcements. Warmth without babyishness when the palette stays sophisticated.
**Social composition:** the layers ARE the composition — order them so the nearest layer is a solid band (bottom or top) in a dark brand color; that band is the text zone, no scrim needed.

Template:

> Layered paper-cut collage illustration covering the entire frame edge to edge: [scene] built from overlapping sheets of colored card — [4–5 named colors] — each layer casting a soft real drop shadow. Foreground layer: a solid [dark brand color] band across the [bottom quarter] with silhouetted paper [subjects] rising from it. Middle layers: [elements] cut from card. Background layers: paper [sky elements] reaching the top corners. Visible paper texture and subtle cut edges, dimensional depth like a shadowbox photographed straight on. No text, no letters, no logos, no watermark.

**Pitfalls:** few; keep the layer count implied at 3–4 depths or the collage flattens.

### 7. Painterly gouache / impasto

**Look:** loose confident brushstrokes, thick paint texture, atmospheric light — WPA poster crossed with concept art. The highest *artistic* quality of the tested set.
**Fidelity:** excellent; tends to read "oil painting" even when asked for gouache — fine in practice.
**Brand fit:** food and restaurants, wine and coffee, travel and hospitality, heritage and craft brands, editorial storytelling, anything selling atmosphere over features. Wrong for dense B2B (reads decorative next to data).
**Social composition:** paint the quiet zone as luminous sky/atmosphere ("a luminous apricot sky occupies the upper half") — painterly gradients ARE text-safe when tonally uniform; add a light scrim only if brushwork gets busy.

Template:

> Atmospheric digital gouache painting filling the whole canvas: [scene] at [golden hour / dawn], painted with loose confident brushstrokes and thick visible paint texture. [Light description] floods the scene; [subjects] painted in [deep tone] silhouette against a luminous [2 colors] sky that occupies the [upper half]. Edges of the frame fully painted, no white borders. Evokes [reference mood — e.g. classic WPA-era posters, mid-century travel posters]. Rich, warm, hand-painted feel. No text, no letters, no logos, no watermark.

**Pitfalls:** busy brushwork drifting into the text zone — "tonally uniform sky, no clouds or strokes breaking the gradient" when type must sit there.

### 8. Clay miniature (know what you're actually ordering)

"Claymorphism" does NOT produce a 3D illustration — it produces **tilt-shift photography of a physical clay diorama**: shallow depth of field, heavy blur at the frame edges, miniature charm. Two honest uses:

- You want the render look → order genre #3 with "soft 3D render, matte clay material" — never the word claymorphism.
- You want the miniature-photo look (toys, kids' products, playful consumer, quirky recruiting posts) → order it knowingly and keep type OUT of the blurred edges: "photographed with tilt-shift lens, the [upper third] falls into soft uniform bokeh" — the bokeh region is your text zone.

## Picking the style — brand and industry map

First read the brand (`canvas brands view`): palette temperature, existing marketing art, how much personality the voice carries. Then map:

| Brand / industry | First pick | Second | Why |
|---|---|---|---|
| Friendly SaaS, HR, productivity | 1 flat vector | 6 paper collage | Approachable without childish; fast set consistency |
| Fintech, AI platform, dev infrastructure | 3 soft 3D | 5 line + spot | Premium calm; matte materials read expensive |
| Logistics, cloud, manufacturing, IoT | 2 isometric | 1 flat vector | Systems-with-parts storytelling |
| Community, events, newsletters, climate, craft | 4 risograph | 7 gouache | Human, anti-corporate, distinctive |
| Architecture, legal, consulting, luxury services | 5 line + spot | 7 gouache | Restraint; typographic-friendly |
| Education, nonprofit, family, internal culture | 6 paper collage | 1 flat vector | Tactile warmth |
| Food, travel, hospitality, wine, heritage | 7 gouache | 4 risograph | Atmosphere is the product |
| Kids, toys, playful consumer | 8 clay miniature | 6 paper collage | Charm is on-brand |
| Celebrations (any industry) | 3 soft 3D (subject mode) | 6 paper collage | Objects carry the emotion |

Tie-breakers when the map gives no clear answer:

- **Palette count:** brand has exactly two strong colors → risograph is almost free-won (2-ink conceit). Four-plus pastel-ish colors → flat vector or paper collage absorb them best. One accent on neutral → line + spot color showcases it.
- **Ground tone:** these genres were proven on light/warm grounds. For dark-first brands, soft 3D (#3) on a near-black backdrop is the tested dark option (the celebration-card recipe); riso and gouache can go dark ("deep navy paper", "night scene") but expect one extra retry for background drift. If the brand is dark AND minimal, reconsider gradient-glow or editorial-typographic instead of illustration.
- **Tone axis:** corporate ← 3 · 5 · 2 · 1 · 6 · 7 · 4 · 8 → playful. Place the brand's voice on that axis and don't jump more than one slot for effect.
- **Density:** carousels that repeat a style across 10 slides need styles that simplify gracefully — flat vector, line + spot, and riso reduce to a corner-of-the-world for `content` slides; gouache and clay don't (their content slides should be CSS-only).

## Set consistency in a multi-slide illustration carousel

1. One genre for the whole set — never mix genres across slides.
2. The shared style sentence (see `references/backdrop-system.md`) is the genre's style keywords + palette + texture words, repeated **verbatim** in every prompt.
3. Reuse a scene *vocabulary*: the same named elements (the crane, the courier, the same little workers) appear across roles so slides feel like one world. Describe them identically each time.
4. A recurring *specific* character (the same mascot in different poses) is identity work: generate the master once, then `hivy_remix_image` with `reference_asset_ids` + invariants ("do not redesign; preserve proportions, palette, and defining features") per `imagegen` → `references/consistent-assets.md`.
5. `content`-slide illustrations should be the genre's *quietest* reduction (a sparse corner of the scene, linework only, a single paper layer) — or pure CSS in the genre's palette. Full scenes on every slide exhaust the eye and the budget.
6. The image gate still applies per slide: an illustration carousel with a full scene on the hook, reduced texture on two beats, and CSS elsewhere is the shape to aim for.

## Review checklist (illustration-specific, on top of the backdrop checklist)

- Composition matches the ordered mode: full-bleed genres reach every edge; diorama/subject modes are cropped by the frame or deliberately banded — no accidental floating vignette.
- Style committed: grain visible (riso), cut edges visible (collage), brushwork visible (gouache), lines confident (line art). Half-committed style = regenerate, it reads as clip-art.
- Zoom the linework and dense areas for letter-like artifacts (the known line-art leak).
- Quiet zone honored *within the scene* (flat sky/wall/band exactly where the layout needs it).
- Palette within the named count; spot colors only where asked.
- Across a set: same genre, same palette, same texture level, same scene vocabulary — a stranger should sort the slides into one deck without hints.
