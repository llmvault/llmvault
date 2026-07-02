# Product UI vignette

The stylized fake-screenshot style SaaS marketing sites use everywhere: simplified UI fragments — a stat card, a workflow diagram, a chat bubble, a listing card — redrawn with oversized typography and floating white cards on a flat pastel background. Popularized by Stripe, Intercom, Mailchimp, and Kit. It communicates *what the product does* without the noise of a real screenshot, and it stays legible at any size.

Use it for: feature-section visuals, split-hero image columns, `card-hero` and `bento-grid` tiles, blog/social imagery about product capabilities. It pairs naturally with the `device-frame` hero pattern (a vignette can be the frame's content).

**When NOT to generate:** if a real product screenshot exists and accuracy matters (docs, changelogs, product pages proper), screenshot the real UI or build the vignette in HTML/Figma from real components. Generated vignettes are for marketing surfaces where the UI shown is intentionally idealized and fictional.

## The visual system contract

Every vignette in a set must follow one system. Write this contract once and reuse it verbatim across the set:

- **Flat solid pastel background**, one hue per image, all hues from one brand palette (e.g. light blue, lavender, peach, mint — or a dark charcoal family for dark-mode brands).
- **White rounded-rectangle cards with soft drop shadows** floating on that background (near-black cards with subtle glow for dark mode).
- **One concept per image.** A vignette shows exactly one interaction, metric, or flow. Two ideas = two images.
- **Radical simplification**: no browser chrome, no nav, no toolbars — just the fragment that tells the story, drawn 2–3× larger than real UI.
- **Curated realistic data**: believable names and numbers, not lorem ipsum, not real customer data.
- Square-ish aspect (`1:1` default, `4:3` for wide chart cards, `3:4` for tall lists).

## Text rules — this type is the exception

Unlike most generated imagery, short verbatim UI text renders reliably in this flat style. The rules, learned from repeated testing:

1. **Maximum ~6 short strings per image.** Labels of 1–3 words and numbers. Every string quoted verbatim in the prompt.
2. **Safe characters:** letters, digits, and `$ % + : , . ?`. **Avoid** midpoint `·`, em dashes, ampersands, and other typographic symbols — they get swapped for lookalikes (verified: `·` became `~`).
3. Ask for "exact spelling of every string, crisp modern sans-serif".
4. **Inspect every string glyph-by-glyph at full size** before accepting. Spelling is usually right; when it fails it fails subtly.
5. Everything that isn't a quoted string: "No other text, no logos, no watermark."
6. **The no-text fallback always works:** replace strings with "grey placeholder text bars" and "a large dark rounded block where a number would sit", then overlay real text in HTML. Use this when text keeps failing, when the copy isn't final, or when the asset must localize.

## Geometry rules — the actual failure surface

Spelling is reliable; *geometry* is where these images fail. Counter it in the prompt:

- Demand precision explicitly: "perfectly straight parallel edges", "crisp corners", "all rows exactly the same height, evenly spaced", "aligned to a common left edge". This measurably improves regularity.
- **Do not ask for overlapping badges/toasts.** An element told to "overlap the card's edge" renders as a tilted physical sticker, torn or folded, detached from its parent. Either keep every element separate on the background, or accept the sticker look, or composite the badge in post. This is the most consistent failure of the type.
- Expect ~1 in 4 images to drift the background color (cream can come back charcoal; teal can collapse to black corners). State the background color early in the prompt AND restate it at the end; regenerate on drift — it's a dice roll, not a prompt defect.
- Small icon glyphs (inside badges/nodes) come out slightly muddled; keep icons big and simple (checkmark, envelope, heart, plus).
- Avatars: say "circular avatar photo" for photographic faces or "illustrated avatar" for flat faces — unspecified, the model picks either.

## Archetype menu — tested recipes

All examples share the light-mode contract: *"Clean product interface vignette illustration: white rounded rectangle card(s) with crisp corners and soft drop shadows floating on a flat solid [color] background … Exact spelling of every string, crisp modern sans-serif, generous whitespace, flat modern SaaS marketing style. No other text, no logos, no watermark."* Swap the middle for the archetype:

- **Stat cards**: "a vertical stack of three metric cards, each with a small grey label above a huge bold black number: \"Views\" with \"15,529\" …" — heavy-text archetype, works but watch card outlines.
- **Line/area chart + stat**: "in the upper left, the small grey label \"Revenue\" and below it the huge bold number \"$48,290\" with a small green pill badge \"+12%\"; the lower two thirds contain a single smooth upward-trending line chart with a soft gradient fill over thin parallel grid lines."
- **Flow / node diagram** (highest success rate): "two white cards connected vertically by a thin line with a small circular plus button at its midpoint; the top card has a round green icon badge with a white envelope symbol and the bold text \"Welcome Series\" …"
- **Funnel**: "three descending flat orange bars, each in a deeper shade; above them three labels with bold numbers: \"Received\" \"88,531\" …" — expect the funnel to center rather than align under labels.
- **List rows** (best no-text archetype): "four identical horizontal list rows, evenly spaced: each with a circular avatar, a dark grey placeholder bar and a lighter shorter bar below; the second row highlighted with a soft green tint."
- **Kanban / board**: "three identical white columns with equal gaps, titled \"To do\", \"Doing\", \"Done\"; the middle column's top card lifted slightly with a stronger shadow as if dragged."
- **Chat / quick replies**: "a chat bubble card with a circular avatar and the bold text \"How can we help?\"; below, three white pill buttons in a neat vertical stack: \"Track my order\", \"Billing\", \"Talk to a human\"."
- **Schedule / calendar**: "three horizontal appointment blocks stacked with even spacing — a soft teal block with \"Dr. Osei\" and \"9:00 AM\", a lavender block with \"Check-up\" and \"10:30 AM\", and an empty slot drawn as a dashed outline."
- **Media listing card**: "a listing card whose upper two thirds is a photograph of [subject]; the lower third holds the bold price \"$425,000\" above smaller grey details, with a heart icon" — mixes a photo INTO the vignette; works well.
- **Dark mode variant** (reliable): "one near-black rounded card with a subtle border and soft glow, floating on a flat deep charcoal background in a slightly lighter shade; [archetype] with neon green and violet accents; labels in crisp light-grey monospace-style type."

## Industry transfer

The recipe transfers across SaaS verticals unchanged — swap the archetype's content and the palette: healthcare scheduling (clinical teal, appointment blocks), proptech (warm cream/terracotta, listing cards), dev tools (dark mode, pipelines), e-commerce (funnels, order chats), creator tools (subscriber stats). Choose the palette from the brand, the archetype from the feature being sold.

## Sets and consistency

Marketing pages need 3–6 vignettes that read as siblings:

1. One shared contract sentence (background family, card style, palette, typography) pasted verbatim into every prompt.
2. Rotate background hues across the set from one palette; keep card treatment identical.
3. Generate the anchor image first; produce the rest with `remix_image` referencing the anchor (see `references/consistent-assets.md`).
4. Generate `count: 2–4` per vignette and pick the most consistent sibling, not the prettiest loner.

## Generation parameters

- `aspect_ratio`: `1:1` default; `4:3` wide charts; `3:4` tall lists/chats.
- `type`: `illustration`.
- `count`: 2–4.

## Review checklist (reject → regenerate)

- Every quoted string correct, glyph by glyph, at full zoom.
- No stray pseudo-text anywhere else.
- Card edges straight, corners even, rows/pills genuinely aligned.
- Background is the named color, no dark corner collapse.
- No unintended sticker-tilt on badges.
- Icons legible; avatars match the requested treatment.
- Siblings in a set: same shadows, same radii, same palette temperature.
