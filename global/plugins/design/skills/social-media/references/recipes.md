# Genre recipes

Slide-by-slide plans per genre: structure, image budget, copy patterns. Dimensions from `references/formats.md`; backdrop mechanics from `references/backdrop-system.md`; construction from `references/slide-kit.md`. Every recipe assumes the brand is already resolved.

## Carousel (Instagram / LinkedIn) — the flagship genre

One `presentation` artifact, 1080×1350 (or 1200×1350 LinkedIn), one file per slide.

**Narrative spine** — a carousel is an argument, not a photo album:

1. Hook (slide 1): the scroll-stopper. One claim, 3–6 words, display type, `hook` backdrop, swipe cue.
2. Body (slides 2…N-1): one message per slide on `content` backdrops. Rotate layout systems so the middle doesn't flatline: editorial rows → big-word steps → a stat beat (`accent` backdrop) → a quote beat.
3. Closer (slide N): the ask. CTA pill, `closer` backdrop, handle/domain.

**Worked example — 10-slide carousel, 4 images** ("5 things your AI employee did while you slept"):

| Slide | Job | Layout system | Backdrop role |
|---|---|---|---|
| 001 | Hook: "It worked the night shift." | Display headline + swipe cue | `hook` |
| 002 | Setup: what happened at 2am | Section headline + short body | `content` |
| 003–007 | The 5 things, one per slide | Numbered row: mono `01`, item title, 1-line body | `content` (same URL ×6) |
| 008 | Stat beat: "14 tasks. 0 meetings." | Oversized stat, gradient span | `accent` |
| 009 | Proof/objection: how it stays safe | Two-column rows | `content` |
| 010 | CTA: "Hire yours." | Display + CTA pill + handle | `closer` |

Budget: `hook` ×1, `content` ×1 (consumed by six slides), `accent` ×1, `closer` ×1 = **4 generations for 10 slides**. This ratio is the ceiling, not the target: run the image gate per role (`references/backdrop-system.md`). A text-led version of the same carousel — "5 lessons from shipping our v2" — ships all 10 slides pure CSS (dark ground, hairline rows, display-type hook) with **0 generations**, and a common middle path is image hook + CSS everything else (1 generation).

Copy pressure: the hook must work with zero context; each body slide must survive being screenshot alone; numbered markers only when order is real (steps, ranking) — variety beats a forced sequence.

## Celebration cards (birthday, work anniversary, welcome, milestone)

One `web_page` artifact per card, 1080×1350 or 1080×1080. Style direction 4 (soft-3D-object) + optionally 7 (vector confetti). Budget: 1 `subject` image.

Structure (all HTML): brand lockup top-left → mono eyebrow (`FROM ALL OF US AT <BRAND>`) → display headline with the name as the gradient span (`Happy birthday, <span class="grad-text">Amara.</span>`) → 2–3 lines of personal, specific copy → mono sign-off footer.

- The `subject` backdrop's quiet zone carries ALL the type — plan it upper-left unless the layout says otherwise.
- Personalization is a copy job, not an image job: reusing the same balloon backdrop for every teammate's birthday is correct and instant; only the HTML changes. Save the `drive_asset_id` and tell the user the card is now a reusable template.
- Variants (anniversary: number as oversized stat; welcome: "Say hi to <name>" + role line; milestone: the metric as the display element) reuse the same structure and often the same backdrop family, re-generated once per occasion type.

## Quote card

One `web_page`, 1080×1350 (portrait reads best for text). Style direction 2; budget 0–1 images.

Structure: oversized opening quote mark (display font, accent color, ~200px) → the quote at display scale (60–90px depending on length; break lines by sense) → attribution as mono label (name + role) → brand chrome. If the quote exceeds ~140 characters, drop to 48–56px and tighten the measure rather than shrinking margins. Optional texture backdrop only.

## Product announcement / launch post

Single `web_page` (1080×1350) or a 4–6 slide carousel for feature depth. Directions 1, 4, or 6. Budget: 1–3.

Single-post structure: mono eyebrow (`NEW` / `NOW IN BETA`) → product name at display scale → one-line value claim → optional product visual → CTA pill. If a real product screenshot exists in drive or the repo, USE IT (framed in a CSS browser/device chrome) instead of generating a fake one — per `design-taste`, real product beats invented product. Generate only atmosphere behind it.

## Stat / metric post

One `web_page`, 1080×1080 or 1080×1350. Direction 2 (+1 for dark brands); budget 0–1.

The number IS the composition: 160–260px, tabular-nums, gradient on the number OR its unit, one line of context above (mono eyebrow) and one below (source/period — real numbers only, never invented). For 2–4 related stats, a hairline-separated stack; more than 4 belongs in a carousel with a stat beat per slide.

## Event invite (webinar, meetup, launch event)

One `web_page` 1080×1350 + usually a 1080×1920 story variant (second artifact, same backdrops). Directions 1, 3, or 6. Budget 1–2.

Structure: mono eyebrow (event type) → event title display → the details block as a hairline table (DATE / TIME + zone / WHERE) in mono labels with sans values → speaker row if any (name + role; real photos only if provided — never generate a speaker's face) → CTA pill (`Save your seat →`).

## Story / reel cover (1080×1920)

One `web_page` per frame or a `presentation` for a sequence. Any direction; budget 1 (sequences reuse it).

All type inside the safe-zone inner section (top 250px, bottom 310px — see `references/formats.md`). Vertical rhythm: backdrop energy can run full-bleed top-to-bottom, but compose ONE focal message in the middle band; stories are glanced at, not read. For covers, remember the platform overlays play chrome dead-center — keep the center-third clear of critical type.

## Profile banners (X 1500×500, LinkedIn 1584×396)

One `web_page` each. Directions 1, 2, or 6 only — object/scene compositions die in the extreme crop. Budget 0–1 (16:9 generation, center band survives — see the aspect rules).

Structure: tagline left or right of center at 60–80px, clear of the avatar zone (bottom-left ~400×400px on X, left ~360px square on LinkedIn — keep that corner quiet), domain as mono label. Test: does the banner still work if 60px is shaved off every edge? (Platforms crop unpredictably across devices.)

## Multi-platform adaptation

When one creative ships to several platforms: build the primary format first (usually IG portrait), get it approved, then create one artifact per additional dimension in the same project. Reuse the same backdrop `public_url`s with re-anchored `background-position`; recompose layout per geometry (a 16:9 X post is a HORIZONTAL composition — copy left, energy right — not a squashed portrait). Type scale multipliers in `references/slide-kit.md`. Chrome, tokens, and copy stay identical; only geometry moves.

## Series and recurring formats

For a recurring series (weekly tips, monthly changelog, every teammate's birthday):

1. Design episode 1 as above and get it approved.
2. Record in the artifact notes (or a `templates/` file in the artifact): the role map with `drive_asset_id`s, the style sentence, and the layout systems used.
3. Next episode: copy the artifact's HTML, change only copy — same backdrops when the mood repeats, or `hivy_remix_image` from the recorded asset ids when the scene must evolve but stay on-model.
4. Never restyle mid-series without the user asking — recognizability is the point of a series.
