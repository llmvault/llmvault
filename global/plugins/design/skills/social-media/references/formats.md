# Formats — dimensions, artifact plans, and aspect mapping

This file is the canonical dimension table for social assets. Every asset is built at its export pixel size with a fixed-size body (`references/slide-kit.md`) — that fixed body is what makes the Canvas preview show the asset 1:1, and it is where the export dimensions live.

## Dimension table

| Asset | Export px | Ratio | Canvas type | Generation ratio | Notes |
|---|---|---|---|---|---|
| Instagram feed portrait (default for posts & carousels) | 1080×1350 | 4:5 | `presentation` (set) / `web_page` (single) | `3:4` | The highest-real-estate feed format; prefer it over square unless the user asks. |
| Instagram square | 1080×1080 | 1:1 | same | `1:1` | Grid-perfect; use for series that must tile. |
| Instagram/Facebook story, Reel cover, TikTok | 1080×1920 | 9:16 | `web_page` / `presentation` (sequence) | `9:16` | Safe zones below. |
| X (Twitter) post image | 1600×900 | 16:9 | `web_page` | `16:9` | 1200×675 also acceptable; 1600×900 stays sharp on retina. |
| X header | 1500×500 | 3:1 | `web_page` | `16:9` | Extreme crop — see banner rules. |
| LinkedIn feed portrait | 1200×1350 | ~8:9 | `presentation` / `web_page` | `3:4` | LinkedIn also accepts 4:5 at 1080×1350; either works. |
| LinkedIn/OG link share card | 1200×627 | ~1.91:1 | `web_page` | `16:9` | Also the standard OG image size for blog/social share. |
| Facebook feed | 1080×1350 | 4:5 | same as IG | `3:4` | Reuse the IG artifact when the creative is identical. |
| YouTube thumbnail | 1280×720 | 16:9 | `web_page` | `16:9` | Type must survive ~120px-wide rendering; test small. |
| Pinterest pin | 1000×1500 | 2:3 | `web_page` | `2:3` | Native generation ratio exists — no crop loss. |
| LinkedIn banner | 1584×396 | 4:1 | `web_page` | `16:9` | Extreme crop — see banner rules. |

Supported generation ratios (`hivy_generate_image`): `16:9`, `9:16`, `3:2`, `2:3`, `4:3`, `3:4`, `1:1`.

## Aspect mapping rules

Backdrops are placed with `background-size: cover`, so the generated ratio only needs to be *close*; cover-cropping absorbs the difference. Rules:

1. Generate at the supported ratio nearest the export ratio (table above). 4:5 exports use `3:4` — cover-crop loses ~6% top/bottom.
2. Keep load-bearing image detail center-weighted or anchored to the edge the layout keeps. State the anchor in the prompt ("rising from the bottom edge of the frame") and set `background-position` to protect it (`center bottom` keeps a bottom anchor through any crop).
3. Extreme ratios (3:1 header, 4:1 banner) crop most of a 16:9 image away. Only atmosphere survives: use gradient/texture/glow styles whose horizontal center band carries the interest, and set `background-position: center`. Never put an object composition behind a banner — compose objects in HTML/SVG instead.
4. Never generate one image per export size for the same creative. Generate once at the ratio closest to the *largest* consumer, reuse the `public_url` everywhere, and let cover + position recompose it.

## Story/vertical safe zones (1080×1920)

Platform UI covers the edges. Keep all text and chrome inside:

- Top: 250px reserved (username, camera icons).
- Bottom: 310px reserved (reply bar, CTA chrome).
- Sides: 60px minimum margin.

Backdrop energy may fill the whole frame; *type may not*. In the slide kit this is a padded inner `section` (top: 250px, bottom: 310px) that all text lives inside.

## Artifact planning

- **Project**: one per campaign or asset family. Name it `Social — <brand or campaign>` (e.g. `Social — Q3 launch`, `Social — team celebrations`). Reuse an existing project when the request extends a family (`canvas project list` first).
- **Slide sets** (carousel, story sequence): one `presentation` artifact. One HTML file per slide at `slides/slide-001.html`, `slide-002.html`, …; the manifest `slides` array is the swipe order. All slides in one artifact share one dimension.
- **Single assets**: one `web_page` artifact, `index.html` as entrypoint.
- **Multi-format campaigns**: one artifact per dimension in the same project, e.g. `ig-carousel-launch` (presentation, 1080×1350), `story-launch` (web_page, 1080×1920), `x-post-launch` (web_page, 1600×900). They share generated backdrops by `public_url` but each recomposes layout for its geometry.
- **Naming**: artifact names describe platform + genre + subject: `IG carousel — launch`, `Birthday card — Amara`, `X header — brand refresh`. Slugs derive automatically.

After `canvas artifact create`, edit `artifact.json` to set the name and slide list, e.g. for an IG portrait carousel:

```json
{
  "schema_version": 1,
  "kind": "hivy.canvas.artifact",
  "type": "presentation",
  "name": "IG carousel — launch",
  "slides": [
    "slides/slide-001.html",
    "slides/slide-002.html"
  ]
}
```

Keep every existing manifest field the scaffold created; only change `name` and the `slides`/`files` lists. When you add a slide file, add it to the manifest in the same edit, then re-run `canvas artifact validate`. The export pixels are carried by the fixed-size body (`references/slide-kit.md`), not the manifest; QA at that size with `browser set viewport <w> <h>`.

## Export note

Rendering/exporting final PNGs is not part of this skill. The synced Canvas artifact IS the deliverable: the preview shows each asset at exact pixels, and users comment via `data-canvas-id` anchors. If the user asks for downloadable files, tell them export happens from the Canvas preview, and make sure the artifact is synced.
