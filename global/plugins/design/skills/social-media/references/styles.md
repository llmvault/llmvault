# Style directions

Pick exactly ONE direction per set — it fixes the shared style sentence (see `references/backdrop-system.md`) that opens every backdrop prompt. The direction should come from the brand and the design read, not from habit: a dev-tools brand and a bakery do not share a direction. Palette hexes below are placeholders — always substitute the actual brand tokens.

For each direction: when to pick it, the shared style sentence template, backdrop guidance, and CSS notes.

## 1. gradient-glow (dark cinematic)

**Pick when:** dark-first tech brands (dev tools, AI, fintech), premium/confident tone, white display type.

- Style sentence: `Matte, cinematic, premium; palette: near-black <bg-hex> background, <accent-a> and <accent-b> light.`
- Backdrops: long-exposure light photography language — blooms, wisps, auroras anchored to ONE frame edge; the rest uniform darkness. `content` role: near-invisible dark texture or CSS-only.
- CSS: near-black `--bg`, one gradient accent spent on a single payoff word + the CTA pill; hairline row structure; the tested worked prompts in `backdrop-system.md` are this direction.
- Failure watch: background drift toward blue — name the near-black twice, blend with scrim.

## 2. editorial-typographic (no image / texture only)

**Pick when:** the message IS the visual (quote cards, stat posts, text-led carousels), or brand restraint calls for zero decoration. Cheapest: 0–1 images.

- Backdrops: none, or one barely-there paper/grain texture for warmth (`content`-role rules apply everywhere).
- CSS: type carries everything — oversized display quotes, huge stats, generous margins; solid `--bg` (dark OR light per brand) with at most a faint CSS radial tint. Structure from hairlines and mono eyebrows.
- Failure watch: "minimal" sliding into "empty" — commit to genuinely display-scale type (the quote at 90px, the stat at 160px) and deliberate spacing.

## 3. illustration-driven

**Pick when:** friendly/human brands (HR, education, consumer SaaS, community, events, craft), lighter palettes, storytelling carousels.

- This direction has its own catalog: **load `references/illustration-styles.md`** to choose the specific genre (flat vector, isometric, soft 3D, risograph, line + spot color, paper collage, gouache, clay miniature), get its tested prompt template, and map genre → brand/industry. Do not prompt an illustration from this summary alone.
- Backdrops: full-bleed illustrated scenes — force it: "the scene fills the entire canvas edge to edge, no empty margins." Quiet zone = a flat color field *inside* the scene (a wall, sky band, tabletop) named as scenery. One scene per chapter of the story; content slides reuse a simplified corner of the same world.
- Set consistency: shared style sentence verbatim + same character/prop vocabulary; for a recurring character across slides, generate the master once and `remix_image` the rest (see `imagegen` → `references/consistent-assets.md`).
- CSS: type sits on the scene's flat fields; pick the darkest brand ink for text rather than white-on-busy.
- Failure watch: centered-vignette gravity (hard rule: full-bleed language every prompt); style drift between slides (never paraphrase the style sentence).

## 4. soft-3D-object

**Pick when:** celebrations (birthdays, milestones, welcomes), product-adjacent announcements, playful-premium tone.

- Style sentence: `Soft 3D render with matte materials, studio lighting; palette: <bg-hex> backdrop, objects in <accent hexes + one warm neutral>.`
- Backdrops: object clusters (balloons, confetti, gift boxes, product, trophy) **cropped by the frame** along the edges opposite the text zone; rim light in an accent color ties objects to brand. Say "soft 3D render, matte material" — not "claymorphism" (renders as tilt-shift photos).
- CSS: text in the flat backdrop zone; a warm off-white object among the brand-colored ones keeps the cluster from going monochrome.
- Failure watch: centered-diorama gravity (crop-by-frame language); background drift (name it twice).

## 5. photographic

**Pick when:** human/lifestyle brands (coaching, hospitality, wellness, local services), founder-voice content, recruiting.

- Style sentence: `Photorealistic candid photograph, <lighting: soft window light / golden hour / overcast>, <film grade e.g. Kodak Portra 400 warmth>.`
- Backdrops: real scenes with believable imperfection; quiet zone from photography itself — negative-space sky/wall/bokeh named as scenery, or shallow depth (`f/1.8`) melting the text-zone background. People: gaze direction relative to the frame; never crop faces with the cover-crop (center-anchor faces, check after generation).
- CSS: scrim almost always needed for white type over photography — directional, under 50%; or place type in the photographic negative space and skip the scrim.
- Failure watch: stock-photo genericness — concrete scene nouns and imperfection language, no keyword soup.

## 6. duotone-brand-wash

**Pick when:** strong two-color brands, editorial/music/event energy, photography that must read as *branded texture* rather than literal scene.

- Style sentence: `High-contrast duotone photograph mapped to <dark-hex> shadows and <accent-hex> highlights.`
- Backdrops: one dramatic subject (crowd, skyline, hands, macro object) generated as duotone; the wash makes almost any region text-safe, so quiet-zone pressure is lower — still name one.
- CSS: type in white or the paper color; the duotone IS the accent, so the CTA pill goes neutral.
- Failure watch: model drifting to full color — "strictly two colors" + name both twice.

## 7. vector-accent

**Pick when:** layering celebration/energy onto an otherwise CSS-built asset (confetti on a birthday card, sparkles on a launch post), or when decoration must stay crisp across multiple export sizes. Combines with directions 1, 2, 4.

- Use `generate_vector_image`: `Scattered confetti of small flat circles and thin rectangles in <accent-a>, <accent-b>, and <neutral>, drifting diagonally, generous spacing, on a transparent background. No text, no letters.`
- Place the returned SVG `public_url` via absolutely-positioned `<img>` layers (one behind text, one in front at low density for depth). Request exact brand hexes in the prompt.
- Failure watch: decoration invading the text column — position layers so density falls to zero behind copy.

## Choosing under uncertainty

Start with the image gate (`references/backdrop-system.md`): if the asset ships with zero images, the direction is 2 (editorial-typographic) and the choice is done. Otherwise map from the brand: dark UI product → 1; text-heavy thought leadership → 2; warm human brand → 3 or 5; celebration genre → 4 (+7); two-color identity → 6. If the brand supports two directions equally and the user gave no signal, that is the one focused question worth asking (per the workflow's step 1) — with a named recommendation.
