# The backdrop system — plan roles, generate few, reuse everywhere

Backdrops are the only generated pixels in a social asset. They are pure atmosphere: no text, no layout, no logos. This file is the method for generating them consistently and cheaply. The prompting fundamentals (clause order, style keywords, negative-space rules) come from the `imagegen` skill; this file applies them to the social case.

## The image gate — run this before planning any generation

The default number of generated images for any asset is **zero**. Type, brand color, and CSS carry most social design; an image is a deliberate escalation, not a starting ingredient. Gate every asset (and every individual slide) with three questions:

1. **Does the image add emotion or information the type cannot?** A birthday card gains real emotion from balloons; a "we're hiring" post gains nothing from an abstract swoosh behind the headline.
2. **Would a designer reach for a photo/render here, or for type?** If the message is a sentence, a number, a date, or a list — type. If it's an occasion, a mood, a place, a product, a person — image.
3. **Can CSS or SVG produce it?** Gradients (including mesh-like multi-stop radials), glows, grain/noise, hairlines, geometric patterns, duotone overlays on *existing* photos, confetti dots — all code. Generate only what code can't make: photographic light, organic material, illustrated scenes, believable objects. (Same gate as `imagegen` → `references/backgrounds.md`.)

If the honest answer is "the slide would look fine without it," it will look *better* without it — commit to the editorial-typographic direction (`references/styles.md` #2) and spend the effort on type scale and spacing instead.

**Ship with ZERO images — practical examples:**

- Quote card: the quote at 90px on brand ground IS the design; an atmospheric backdrop behind a quote reads as a motivational-poster cliché.
- Stat post ("+327% this quarter"): a 200px tabular number with a gradient span beats any backdrop; decoration dilutes the number's authority.
- Text-led tip/lesson carousel ("5 lessons from…"): dark brand ground, hairline rows, mono eyebrows — all 10 slides pure CSS. The hook works as display type alone.
- Changelog / feature-list post: the list is the content; structure it with hairlines, not imagery.
- Hiring post ("We're hiring — Senior Go engineer"): role, location, and a CTA pill; an image here is noise.
- Event save-the-date where the details table is the message and the brand is typographic.
- X/LinkedIn banner for a typographic brand: a CSS gradient band + tagline is cleaner than a cropped generation.
- Any asset for a brand whose identity is already type-first (think Vercel-like minimalism) — imagery off-brand by definition.

**Images EARN their place — practical examples:**

- Carousel hook slide that must stop the scroll (one dramatic backdrop; the 9 slides behind it can still be pure CSS).
- Celebration cards: balloons/confetti/gift render creates the occasion's warmth — this is the emotion type can't fake.
- Premium launch moment: a cinematic glow gives a product announcement weight on a dark brand.
- Photography-voiced brands (hospitality, wellness, food, travel): the photo IS the brand voice; a type-only post would be off-brand.
- Illustration-driven storytelling where scenes carry the narrative.
- A physical subject — product shot, venue, dish — where seeing it is the point.
- A real product screenshot exists → use it (framed in CSS device chrome); that's an image slot filled *without* generation.

Mixed sets are the norm, and the gate applies per role: a 10-slide carousel often ships as image `hook` + CSS `content` slides + image `closer` — 2 generations, not 4. State the gate's outcome in the role map ("content: CSS-only, no generation").

## Backdrop roles

Never think "an image per slide." Think in roles — a small cast of backdrops the whole set draws from:

| Role | Job | Energy | Used by |
|---|---|---|---|
| `hook` | Stop the scroll on slide 1 | High — the set's one dramatic image | Slide 1 only |
| `content` | Recede behind lists, steps, body copy | Near-invisible — texture or tone | Every middle slide (2…N-1); this is where reuse happens |
| `accent` | Optional mid-set beat: a section divider, a big stat, a mood shift | Medium | 1–2 slides at most |
| `closer` | Frame the CTA / sign-off | Medium-high, composed around one focal bloom or object | Last slide only |
| `subject` | A celebration or announcement's object art (balloons, product, trophy) | High but zone-disciplined | Single-asset genres |

Budget by set size — ceilings for the roles that passed the image gate, not targets (exceeding needs an explicit user reason):

- Single asset: 0–1 image (`subject` or `hook`; 0 whenever the gate says type carries it).
- 3–5 slides: 0–3 images (hook, content, optional closer).
- 6–10 slides: 0–5 images (hook, 1–2 content variants, optional accent, closer).
- Multi-format campaign: same budget as its largest set — formats reuse, they don't add.

A second `content` variant is justified only when the set has two distinct content *chapters* (e.g. "problems" vs "solutions") that benefit from a tonal shift.

## The planning worksheet

Before writing any prompt, write this table for the set (it also goes in your final report):

```text
role      | quiet zone                  | energy zone            | consumed by
hook      | upper 2/3                   | bottom edge glow       | slide 1
content   | everywhere (tone-on-tone)   | faint top-corner sheen | slides 2–8
closer    | upper half + left           | lower-third bloom      | slide 9 (CTA)
```

The quiet zone is wherever the recipe's layout puts text. Decide it here, from the layout — not after generation.

## Prompt construction

Write ONE shared style sentence for the set — medium + palette intent + lighting — and reuse it verbatim in every role's prompt. This is the single strongest set-consistency lever. Example:

> Matte, cinematic, premium; palette: near-black #0A0B0D background, electric blue #0171FF, bright cyan #00DFFC.

Then compose each role's prompt in the `imagegen` clause order, as one flowing paragraph:

1. **Medium/style** — from the chosen direction in `references/styles.md` ("long-exposure photograph of abstract light", "soft 3D render with matte materials", "flat vector illustration…").
2. **Scene** — what the energy zone contains, anchored to a frame edge ("a soft electric-blue light bloom rising from the bottom edge of the frame").
3. **Quiet zone as scenery** — name what fills it and where it sits ("the upper two thirds of the frame is smooth, uniform near-black darkness, empty corner to corner with no objects and no focal point"). Never say what the space is *for*.
4. **Palette** — the shared style sentence's colors; name the background color TWICE (start and end) to fight drift.
5. **Mood** — two or three words ("cinematic, quiet, premium").
6. **Negatives** — always: "No text, no letters, no writing, no logos, no watermark." Add style-specific ones from the styles file.

Role-specific pressure:

- `content` backdrops obey the background prime directive (see `imagegen` → `references/backgrounds.md`): they must *recede*. Tone-on-tone, "the texture barely visible", "no focal point, no objects, uniform corner to corner". If a content backdrop is beautiful enough to notice, it failed. Consider CSS-only (no image) for content slides — a solid `--bg` with a faint CSS gradient is often better and free.
- `hook`/`closer` carry the drama but still surrender their quiet zone completely.
- `subject` art must be **cropped by the frame** on the edges opposite the quiet zone ("a cluster of balloons rising along the bottom and right edges of the frame, cropped by the frame on the bottom and right sides") — this defeats the centered-diorama gravity and composes the object against the text zone.

## Tested worked prompts

These four shipped a real 4-slide carousel + celebration card and are safe starting points. Adapt palette hexes to the actual brand; keep the structure.

`hook` (dark gradient-glow direction, 3:4):

> Long-exposure photograph of abstract light in deep darkness: a soft electric-blue and cyan light bloom rising from the bottom edge of the frame, like light glowing up through dark water, thin drifting wisps of cyan fading as they rise. The upper two thirds of the frame is smooth, uniform near-black darkness, empty corner to corner with no objects and no focal point. Palette: near-black background, electric blue #0171FF, bright cyan #00DFFC. The background is near-black, matte and smooth. Cinematic, quiet, premium. No text, no letters, no logos, no watermark.

`content` (same set, 3:4):

> Macro photograph of dark matte paper texture in near-black, lit by a very faint cool blue sheen from the top edge, extremely subtle, the surface fading to uniform darkness across the frame with no focal point and no objects. Palette: near-black #0A0B0D with the faintest hint of cool electric blue. Quiet and uniform corner to corner, the texture barely visible. No text, no letters, no logos, no watermark.

`closer` (same set, 3:4):

> Smooth abstract gradient backdrop: a deep near-black field with a single soft radial bloom of electric blue melting into bright cyan, sitting in the lower third of the frame and fading to darkness at every edge. No shapes, no objects, no added texture, no vignetting beyond the described bloom, uniform matte darkness corner to corner everywhere else. Palette: near-black #0A0B0D, electric blue #0171FF, bright cyan #00DFFC; the field stays near-black. No text, no letters, no logos, no watermark.

`subject` (celebration card, 3:4):

> Soft 3D render with matte materials: a festive cluster of balloons in electric blue, bright cyan, and warm off-white rising along the bottom and right edges of the frame, cropped by the frame on the bottom and right sides, with small matte confetti discs drifting between them. The upper left two thirds of the frame is a smooth, empty near-black backdrop, dark corner to corner. Soft studio lighting from the upper left, a gentle cyan rim light on the balloons. Palette: near-black background, electric blue #0171FF, bright cyan #00DFFC, off-white #FCFCFB; the background stays near-black. No text, no letters, no logos, no watermark.

## Generating and the role map

Call `generate_image` once per role at the mapped aspect ratio (`references/formats.md`). Use `count: 2` on `hook`, `closer`, and `subject` (pick the better take); `count: 1` is fine for `content`. As results return, build the role map and keep it in your notes through the whole task:

```text
role     | drive_asset_id | public_url            | verdict
hook     | ast_…          | https://…/hook.png    | accepted — glow landed bottom-center
content  | ast_…          | https://…/texture.png | accepted
closer   | ast_…          | https://…/bloom.png   | bloom landed lower-LEFT → left-align CTA layout
```

The `public_url` is what goes into the HTML. The `drive_asset_id` is what future sets remix from — report it so series work stays consistent.

## Review and adaptation

Inspect every backdrop before writing HTML:

- Quiet zone actually quiet? (Uniform tone, no objects, no letter-like artifacts — zoom in.)
- Background color close enough that a CSS scrim can blend the seams? (Exact match is CSS's job, not the model's.)
- Where did the energy *actually* land? Update the worksheet — **layout follows the image**. A bloom that landed left means a left-aligned slide, not a regeneration.

Regenerate only for: drift that a scrim can't hide, objects/artifacts in the quiet zone, text-like marks, or a broken subject. Change ONE prompt dimension per retry (per `imagegen` iteration method). Two failed retries on the same dimension → switch technique or style direction.

## Reuse mechanics

- Within a set: the same `public_url` appears in every slide file that consumes the role. Zero extra credits, guaranteed consistency.
- Across formats: same URL, recomposed with `background-position`/`background-size` per geometry (`references/formats.md` aspect rules).
- Across time (a series — "Tip Tuesday #7", next month's episode): pass the accepted backdrop's `drive_asset_id` as `reference_asset_ids` to `remix_image` with invariant language ("same palette, same lighting, same material treatment — do not redesign"), changing only the scene element that must change. Read `imagegen` → `references/consistent-assets.md` before any remix chain.
- Recolor/reskin for a sub-brand: prefer regenerating with the new palette in the shared style sentence over remixing — atmosphere is cheap; identity is what remix protects.
