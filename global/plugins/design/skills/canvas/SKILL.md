---
name: canvas
description: Use when a design agent needs to create, edit, validate, preview, browser-test, sync, or explain Hivy Canvas projects and artifacts. This is the execution skill for Canvas artifact work: project creation, web_page and presentation artifacts, semantic HTML, data-canvas-id anchors, artifact validation, Agent Browser QA, image-generation assets, and final sync.
---

# Canvas Artifacts

Use this skill when design work should become an interactive Canvas artifact. Canvas artifacts are first-party HTML deliverables stored in the sandbox at `/workspace/canvas`, validated locally, previewed in the browser, and synced to the control plane.

Canvas is not a prose-only design process. When the user asks for a mockup, prototype, page, app screen, deck, logo exploration, or visual direction that should be seen and clicked, create or edit an artifact.

## Artifact Types

Use the simplest supported type that matches the requested work:

- `web_page`: responsive page-like artifacts. Use for landing pages, dashboards, settings pages, onboarding flows, web app screens, product pages, portfolio pages, logo exploration pages, social/cover image previews, and single-screen prototypes.
- `presentation`: slide decks. Use for pitch decks, sales decks, board updates, executive briefings, webinars, proposals, and narrative slide sequences. Each slide is a standalone HTML file listed in `artifact.json`.

Keep artifacts self-contained where practical: HTML, CSS, and JavaScript in the artifact folder. Remote image/font URLs are allowed when they are intentional and stable.

## Core Workflow

Run this loop for Canvas work:

```bash
canvas doctor
canvas project list
canvas project create --name "Project name"
canvas artifact create --project <project-slug-or-id> --type web_page --name "Artifact name"
canvas artifact create --project <project-slug-or-id> --type presentation --name "Deck name"
canvas artifact validate /workspace/canvas/projects/<project>/artifacts/<artifact>
canvas artifact verify /workspace/canvas/projects/<project>/artifacts/<artifact>
canvas artifact sync /workspace/canvas/projects/<project>/artifacts/<artifact>
```

Use an existing project from `canvas project list` when the work clearly belongs there. Create a new project when the user is starting a new initiative, redesign, deck, campaign, or artifact family.

`canvas artifact create` scaffolds the artifact locally. Edit the generated HTML files directly. `canvas artifact validate` and `canvas artifact verify` return structured JSON; read the `issues` array, apply the `fix` guidance, and rerun until errors are gone. Treat warnings as quality feedback unless the user explicitly asks for a rough sketch.

Run `canvas artifact sync` after validation and browser QA. The runtime watcher may sync changes automatically, but an explicit final sync gives the agent a clear result to report.

## File Layout

Expected layout:

```text
/workspace/canvas/
  projects/
    project-slug/
      project.json
      artifacts/
        artifact-slug/
          artifact.json
          index.html
        deck-slug/
          artifact.json
          slides/
            slide-001.html
            slide-002.html
```

For `web_page`, the primary file is usually `index.html`.

For `presentation`, create one HTML file per slide and keep the manifest `slides` array in the same order the user should review the deck.

## HTML Structure

Write semantic HTML first. The validator expects Canvas artifacts to have a meaningful document structure that humans and tools can inspect.

Minimum shape:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Artifact name</title>
    <style>
      /* artifact styles */
    </style>
  </head>
  <body>
    <main data-canvas-id="page">
      <section data-canvas-id="hero">
        <h1>Clear artifact title</h1>
      </section>
    </main>
    <script>
      // artifact interactions
    </script>
  </body>
</html>
```

Use semantic sections:

- `main` for the primary artifact surface.
- `section` for major page or slide regions.
- `article` for repeated cards, variants, posts, testimonials, pricing tiers, or slide content blocks.
- `nav` for navigation, tabs, slide controls, or page menus.
- `aside` for secondary panels, notes, filters, or supporting proof.
- `form` for settings, signup, search, checkout, feedback, or data-entry surfaces.
- `figure` for images, charts, screenshots, diagrams, or mockups with captions.
- `table` for real tabular data.
- `header` and `footer` for page-level or main-level framing.

Use `div` and `span` for styling only after the semantic structure is clear.

## Canvas IDs

Use `data-canvas-id` to create stable anchors for preview selection, comments, and agent edits. IDs should describe the role of the element, not its current color or position.

Required by validation:

- Every `main`.
- Every `section`, `article`, `nav`, `aside`, `form`, `figure`, and `table`.
- Page-level or main-level `header` and `footer`.
- Rich repeated `li` items, such as pricing cards, testimonial cards, feature cards, or variant list items.

Recommended:

- Important interactive controls such as primary CTA buttons, tabs, accordions, carousel controls, settings toggles, and slide navigation.
- Major generated or external assets that the user may comment on.

Format:

- Lowercase kebab-case.
- Start with a letter.
- Keep under 80 characters.
- Make each ID unique within the HTML file.

Good examples:

```html
<section data-canvas-id="customer-proof">
<article data-canvas-id="pricing-card-pro">
<button data-canvas-id="hero-primary-cta">
<figure data-canvas-id="product-dashboard-preview">
<li data-canvas-id="testimonial-acme-ops">
```

Avoid random IDs, numeric-only IDs, styling names, and implementation names such as `blue-box`, `div-17`, or `left-column`.

## Validation Rules

`canvas artifact validate <path>` checks:

- `artifact.json` is valid JSON.
- `schema_version`, `name`, `project`, `type`, and `files` are present.
- `type` is `web_page` or `presentation`.
- `web_page` artifacts define an `entrypoint`.
- `presentation` artifacts define `slides`.
- Every manifest file path is relative and stays inside the artifact directory.
- Every listed file exists and at least one listed file is HTML.
- HTML includes explicit `<html>` and `<body>` elements.
- HTML has one primary `<main>` element that is a direct child of `<body>`.
- `<main>` contains semantic content sections.
- Required semantic elements have valid, unique `data-canvas-id` values.
- Heading issues are warnings: missing headings, missing primary `h1` on web pages, and skipped heading levels.

Use `canvas artifact verify <path>` before final sync. It wraps validation output in a `verified` result so the agent can report whether the artifact is structurally ready.

## Browser QA

For visual or interactive work, validate is not enough. Use Agent Browser to open the artifact, inspect the rendered page, click through important controls, and capture screenshots.

For a `web_page` artifact:

```bash
cd /workspace/canvas/projects/<project>/artifacts/<artifact>
python3 -m http.server 4173 --bind 127.0.0.1 >/tmp/canvas-artifact.log 2>&1 &
browser open http://127.0.0.1:4173/index.html
browser set viewport 1440 1000
browser snapshot -i
browser screenshot --full /tmp/<artifact>-desktop.png
browser set viewport 390 844
browser screenshot --full /tmp/<artifact>-mobile.png
```

For a `presentation` artifact:

```bash
cd /workspace/canvas/projects/<project>/artifacts/<artifact>
python3 -m http.server 4174 --bind 127.0.0.1 >/tmp/canvas-deck.log 2>&1 &
browser open http://127.0.0.1:4174/slides/slide-001.html
browser set viewport 1440 900
browser snapshot -i
browser screenshot /tmp/<artifact>-slide-001.png
```

If a port is busy, choose another local port. Local URLs are only for agent QA inside the sandbox; do not send `localhost` or `127.0.0.1` URLs to the user.

During QA:

- Re-run `browser snapshot -i` after every click, route change, modal, dropdown, form submit, or slide navigation.
- Click primary CTAs, tabs, accordions, nav items, carousel controls, and forms.
- Check hover/focus-sensitive surfaces when they are part of the design.
- Use desktop and mobile viewport sizes for responsive artifacts.
- For decks, inspect the first slide, last slide, and any slide with dense proof, charts, product screenshots, or motion.
- Fix visual breakage in HTML/CSS/JS, rerun `canvas artifact validate`, and repeat browser QA before syncing.

## Image Generation

Use image-generation tools when a prototype needs real visual material and no suitable asset exists.

Use `generate_image` for:

- Hero photos or illustrations.
- Product scenes, editorial imagery, lifestyle scenes, backgrounds with real subject matter.
- App mockup imagery, feature visuals, campaign graphics, and social/cover previews.

Use `generate_vector_image` for:

- Icons, simple illustrations, decorative vector systems, pattern assets, diagram-like visuals, and exploratory logo marks.

Prompt image tools with concrete design direction:

```json
{
  "prompt": "Create a clean editorial hero image for a B2B operations dashboard: laptop showing abstract workflow cards, confident daylight studio, restrained teal and graphite palette, no legible text, generous negative space on the left.",
  "aspect_ratio": "16:9",
  "type": "hero image",
  "count": 1
}
```

Use `reference_asset_ids` when the user provided brand assets, product screenshots, or prior generated images. Keep `count` between 1 and 4.

After generation, use the returned `public_url` in the artifact HTML and preserve the `drive_asset_id` in notes or manifest metadata when useful. Reopen the artifact in Agent Browser and verify that the image loads, crops correctly, and does not hide text or controls.

Do not generate filler images. If an asset does not make the artifact more concrete, credible, or testable, use typography, layout, and product UI instead.

## Artifact-Specific Guidance

For landing pages and websites:

- Use `web_page`.
- Build full responsive sections, not a cropped hero-only mockup unless the user asked for only a hero.
- Include real CTAs, proof, product visuals, objections, and final CTA when appropriate.

For web app screens:

- Use `web_page`.
- Include realistic repeated-use states: loading, empty, error, success, hover/focus, disabled, validation, and responsive behavior when relevant.
- Keep controls stable and scannable.

For presentations:

- Use `presentation`.
- One slide, one message.
- Keep each slide self-contained and sized for a 16:9 default viewport.
- Use slide filenames and IDs like `slide-001`, `slide-002`, and names that describe the slide job.

For logo or brand explorations:

- Use `web_page` to create a decision-ready exploration artifact.
- Structure it with sections for brief, territories, variants, application tests, shortlist, and export-ready direction.
- Use semantic `article` elements for individual variants and `figure` elements for application mockups.

## Final Response Checklist

Before reporting completion, make sure you can state:

- Project name or slug.
- Artifact name, slug, and type.
- Artifact path under `/workspace/canvas`.
- Validation or verify result.
- Browser QA performed, including viewport sizes and interactions tested.
- Sync result, or why sync was not run.
- Any assumptions, missing assets, or follow-up decisions the user should make.
