---
name: "social-media"
description: "Use when a design agent needs to create social media graphics: Instagram carousels and single posts, story/reel covers, birthday, anniversary, welcome, or milestone celebration cards, quote cards, product announcements, event invites, stat posts, X/LinkedIn/Facebook post images, YouTube thumbnails, profile banners, or OG share cards. Teaches the backdrop-first method — generate a small set of reusable image backdrops, then compose every asset as a pixel-exact Canvas artifact with all text in HTML — plus exact platform dimensions, artifact planning (one presentation artifact per slide set, one web_page per single asset), and per-genre recipes. Do not use for logos (logo-design skill) or landing pages (imagegen hero patterns)."
---

# Social Media Assets

This skill turns a social-graphics request into on-brand, pixel-exact Canvas artifacts. The method is **backdrop-first**: image generation paints atmosphere only; every word, logo, and layout decision lives in HTML/CSS where it is pixel-perfect and editable forever. A 10-slide carousel is one Canvas `presentation` artifact with 10 slide files — but typically only 3–5 generated images, because backdrops are planned as reusable *roles*, not per-slide art.

Work through the numbered workflow below. Load only the reference files the current step names.

## Scope

Use this skill for feed posts, carousels, stories, celebration cards, quote cards, announcements, invites, stat posts, thumbnails, banners, and share cards — any fixed-dimension graphic destined for a social platform or internal celebration.

Do not use it as the primary skill for:

- Logos and brand marks — load `logo-design`.
- Landing pages and website heroes — load `imagegen` (hero patterns) with `design-taste`.
- Slide decks meant to be presented — load `presentation-creator`.
- General design direction questions — load `design-taste`.

This skill depends on the `canvas` skill for all artifact mechanics. Load it before creating or editing artifacts and follow its workflow exactly; nothing here overrides it.

## Hard rules

1. **Images carry zero text.** Every headline, name, date, handle, and CTA is HTML. Every backdrop prompt ends with "No text, no letters, no writing, no logos, no watermark." If a request seems to need rendered text inside the image, it doesn't — the HTML layer is the text engine.
2. **Not every design needs an image — default to zero.** Run the image gate in `references/backdrop-system.md` before planning any generation. Quote cards, stat posts, text-led carousels, changelog/hiring announcements, and typographic brands are usually stronger as pure type + CSS; gradients, grain, and geometric pattern are CSS/SVG jobs, never generations. An image must add emotion or information the type cannot — if removing it would lose nothing but decoration, don't generate it.
3. **N slides ≠ N images.** When images do earn their place, plan backdrop *roles* (hook, content, accent, closer) before generating anything. Content slides share one backdrop. A set that generates more than 5 images without an explicit user reason is a planning failure.
4. **Quiet zones are photographic, never instructional.** Decide where text will sit *before* writing the prompt, then describe what fills that region as scenery ("the upper two thirds of the frame is smooth, uniform near-black darkness"). Never write "leave space for text."
5. **Dimensions are exact.** Each asset is built at its export pixel size (`references/formats.md`) with a fixed-size body. Never design at one size and imagine it scaling.
6. **One dimension per artifact.** A carousel (all slides same size) is one `presentation` artifact. The same campaign at 4:5, 1:1, and 9:16 is three artifacts in one project, recomposed per format — never scaled.
7. **Brand truth lives in CSS.** Run `canvas brands list` first; brand hexes go into prompts only as palette *intent* (models drift), and into the stylesheet as exact tokens. Blend image edges into the true background with a CSS scrim, not by hoping the model matched the hex.
8. **Adapt layout to the image before regenerating.** After generation, look at where the energy actually landed and move the composition to meet it. Regenerate only for drift, artifacts, or a violated quiet zone — and change one prompt dimension per retry.

## Core workflow

1. **Read the brief.** Identify: genre (carousel, celebration card, quote card, …), platform(s), the single message per asset, and the brand voice. State a one-line design read (per `design-taste`): `Reading this as: [genre] for [audience], with a [tone] visual language, leaning toward [style direction].` If the user's ask could go two materially different directions, ask ONE focused question.
2. **Resolve the brand.** `canvas brands list`, then `canvas brands view <brand-id>` for the default or user-chosen brand. Extract: background/foreground/muted/border colors, accent(s) or gradient, font families, logo asset. No brand? Follow the `design-taste` Brand Source Rule (choose a defensible palette, persist with `canvas brands create`, tell the user).
3. **Pick format and dimensions.** Open `references/formats.md`. It maps every asset type to export pixels, the Canvas artifact type, the `default_viewport`, the generation aspect ratio, and platform safe zones.
4. **Plan the artifacts.** One Canvas project per campaign or asset family (`canvas project create --name "Social — <campaign>"` or reuse via `canvas project list`). Then:
   - Slide set (carousel, story sequence): ONE `presentation` artifact; one HTML file per slide under `slides/`; manifest `slides` array in swipe order; `default_viewport` = the slide's exact pixels.
   - Single asset (celebration card, quote card, announcement, thumbnail, banner): ONE `web_page` artifact with `index.html` sized exactly; `default_viewport` = the asset's pixels.
   - Multi-platform campaign: one artifact per dimension, sharing the same generated backdrops by URL.
5. **Choose a style direction.** Open `references/styles.md` and pick exactly one direction for the whole set (gradient-glow, editorial-typographic, illustration-driven, soft-3D-object, photographic, duotone-wash, vector-accent). The direction fixes the shared style sentence used in every prompt of the set.
6. **Run the image gate, then plan and generate backdrops.** Open `references/backdrop-system.md`. First decide whether the asset needs generated images at all (the gate at the top of that file — many assets ship stronger with zero). For the slides/assets that pass the gate: assign backdrop roles, write one prompt per role (not per slide), generate with `generate_image`, review against the checklist, and record each result's `public_url` + `drive_asset_id` in a role map. This is the only step that spends image credits.
7. **Build the HTML.** Open `references/slide-kit.md`. Compose each slide/asset file with the fixed-size boilerplate, brand tokens as CSS custom properties, the backdrop's `public_url` as a layered `background-image`, and the brand chrome (logo lockup, slide index, footer). Semantic structure and `data-canvas-id` anchors per the `canvas` skill.
8. **Follow the genre recipe.** Open `references/recipes.md` for the genre's slide-by-slide plan, image budget, and copy patterns — including a fully worked 10-slide carousel.
9. **Validate, QA, sync.** Per the `canvas` skill: `canvas artifact validate`, Agent Browser QA at the exact viewport (`browser set viewport 1080 1350` for a 4:5 post), inspect every slide at full size — text collisions with image energy, contrast in text zones, glyph clipping on display type — then `canvas artifact verify` and `canvas artifact sync`.

## Artifact planning quick map

Budgets below are ceilings for sets that pass the image gate — a text-led version of any row can be 0.

| Request | Canvas artifacts | Files | Typical image budget |
|---|---|---|---|
| 10-slide IG carousel | 1 `presentation` | `slides/slide-001.html` … `slide-010.html` | 0–5 (hook, 1–2 content, accent, closer) |
| 4-slide IG carousel | 1 `presentation` | 4 slide files | 0–4 |
| Single post / celebration card / quote card | 1 `web_page` | `index.html` | 0–2 |
| Story or reel cover | 1 `web_page` | `index.html` | 0–1 |
| Story sequence (3 frames) | 1 `presentation` | 3 slide files | 1–2 |
| Campaign: 4:5 post + 1:1 + 9:16 story | 3 artifacts, 1 project | one per dimension | 2–4 total, shared across all three |
| Profile banner + avatar-area check | 1 `web_page` | `index.html` | 0–1 |

## Image tools in this skill

- `generate_image` — backdrops and scene art. Returns `{drive_asset_id, public_url, …}`; the `public_url` goes straight into the HTML. Use `count: 2` for load-bearing backdrops (hook, closer) and pick the better.
- `remix_image` — when a new backdrop must contain something that already exists (a recurring mascot, a product, next month's episode of a series). Requires `reference_asset_ids`; read `imagegen` → `references/consistent-assets.md` first.
- `generate_vector_image` — crisp vector decoration (confetti, sparkles, badges, dividers) for the vector-accent style; SVG scales to any export size.

All prompting depth — hard rules, clause order, style keywords, iteration method — is the `imagegen` skill's domain. This skill's `references/backdrop-system.md` applies those rules to the social-backdrop case; for anything beyond it, load `imagegen`.

## Reference map

Load only what the current step needs.

- `references/formats.md` — every platform/asset type: export pixels, ratio, Canvas artifact type, `default_viewport`, generation aspect mapping, safe zones. Load at step 3.
- `references/backdrop-system.md` — backdrop roles, image budgets, the set-consistency method, prompt skeletons, tested worked prompts, the reuse ledger, review checklist. Load at step 6.
- `references/slide-kit.md` — the fixed-size HTML boilerplate, brand token block, chrome components, type scale, scrims and text-safety, fonts, `data-canvas-id` conventions. Load at step 7.
- `references/styles.md` — the seven style directions with prompt templates and CSS notes. Load at step 5.
- `references/illustration-styles.md` — the illustration genre catalog: eight tested genres (flat vector, isometric, soft 3D, risograph, line + spot color, paper collage, gouache, clay miniature) with fidelity notes, prompt templates, composition counters, and the brand/industry → genre map. Load when step 5 lands on the illustration-driven (or soft-3D) direction.
- `references/recipes.md` — per-genre plans (carousel, celebration, quote, announcement, stat, invite, story, banner, multi-platform), each with slide structure, image budget, and copy guidance. Load at step 8.

## Final response checklist

Beyond the `canvas` skill's checklist, be able to state:

- Genre, platform(s), and export dimensions per asset.
- Style direction and the design read.
- The image-gate outcome: which assets/slides shipped CSS-only and why, plus the backdrop role map for the rest — each generated image's role, which slides consume it, `drive_asset_id`, and credits spent vs. slide count (e.g. "10 slides, 2 images").
- Brand used (`canvas brands` id) and which tokens went into CSS.
- Where all text lives (must be: HTML, all of it).
- Validation, viewport QA at exact export size, and sync results.
