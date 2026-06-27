---
name: logo-design
description: Use when designing, redesigning, refining, critiquing, or exploring logos, wordmarks, monograms, brand marks, lockups, favicon marks, app marks, or logo variants. This is a design-direction skill for logo taste and judgment: mark architecture, typographic register, symbol approach, restraint, small-size discipline, application context, critique, and decision-ready presentation. In Hivy, use this skill with the canvas skill: think with logo-design, then create and verify the actual logo exploration artifact in Hivy Canvas.
---

# Logo Design For Hivy Canvas

Use this skill to make logo work more tasteful, disciplined, and decision-ready. It tells Kara how to think, critique, narrow, and present logo directions, then routes execution through a Canvas artifact.

The Canvas artifact is the source of truth. Markdown notes are supporting documentation only.

## Core Model

A logo is a system of marks, not a single drawing. Good logo work explores architectures, tests application contexts, removes weak ideas, and presents a small number of meaningfully different directions with clear rationale.

Work in this order:

1. Clarify the brand and application context.
2. Select the mark architectures worth exploring.
3. Establish typographic and symbol direction.
4. Create distinct Canvas artifact variants, not minor style tweaks.
5. Test each variant in real logo contexts.
6. Present the strongest directions with rationale, rejects, and next-step recommendations.

## Required Canvas Coupling

When the user asks to create, draw, mock up, iterate, export, or present logo work:

1. Load the `canvas` skill before making or editing the logo artifact.
2. Run `canvas doctor`, then list or create the target project.
3. Run `canvas brands list` before choosing colors, typography, logo constraints, or voice. Use the default org brand when present; otherwise inspect the relevant brand with `canvas brands view <brand-id>`. If the list is empty and color direction is needed, choose a defensible starting palette from the brief or product context, create it with `canvas brands create`, and tell the user what was saved.
4. Create or open a `web_page` artifact for the logo exploration.
5. Build logo exploration sections in semantic HTML: brief, territories, variants, application tests, shortlist, and export-ready direction.
6. Use `article` elements for individual variants and `figure` elements for marks, lockups, and mockups. Add stable `data-canvas-id` anchors to every major section and variant.
7. Run `canvas artifact validate` after each structural batch and fix error-level issues.
8. Use Agent Browser to inspect desktop and mobile renderings, test any variant filters or tabs, and capture screenshots.
9. Run `canvas artifact verify`, then `canvas artifact sync` when the exploration is ready.

Use Canvas for the actual work: variant sections, lockups, favicon tests, monochrome tests, reverse-on-dark tests, application mockups, and final selected direction.

Do not produce final logo work only as prose, SVG snippets, or local image files unless the user explicitly asks for non-Canvas output. If a local SVG or PNG is useful, use it as a temporary construction aid or generated asset, then include it in the Canvas artifact.

## Brand Source Rule

Org brands are the source of truth for existing brand values. Always run `canvas brands list` before selecting colors, typography, logo usage, or brand voice. Use the default brand when one exists. If there are multiple brands and no default, use `request_user_input` to choose which brand to design against.

If no org brand exists, choose a starter palette only when the brief, audience, category, or desired tone gives enough direction; then create a brand record with `canvas brands create` before treating that palette as the brand. Tell the user what brand and palette you saved. If context is too thin, use `request_user_input` for one focused direction question.

## When To Use

Use this skill when:

- The user asks for a logo, wordmark, brand mark, app icon, favicon mark, monogram, lockup, or logo refresh.
- The user wants tasteful logo directions rather than a quick icon.
- A brand identity task has a logo-specific phase.
- The user asks for logo critique, logo variants, or production refinement.
- The user wants a mark to work across web header, social avatar, favicon, app icon, merch, print, signage, or motion.

Do not use it for one-off illustration, generic UI design, full brand strategy, or broad product creative direction unless the task includes concrete logo work.

## Inputs To Gather

Ask only for missing inputs that would materially change the design. When input is needed, use the `request_user_input` tool instead of asking a plain-text questionnaire. Keep it to one to three focused questions, with concrete options when selection is useful. If the user gives enough context, proceed and state assumptions in the Canvas presentation notes.

Collect:

- Brand name: exact spelling, casing, punctuation, and whether a descriptor travels with it.
- Category: SaaS, finance, legal, education, creator tools, hospitality, consumer goods, etc.
- Audience: who sees the logo and what they need to feel or understand quickly.
- Tone: formal/casual, restrained/expressive, heritage/modern, premium/accessible.
- Org brand: selected from `canvas brands list` when available, including existing colors, typography, logos, and voice; if none exists, the newly created Canvas brand after choosing a starter palette.
- Applications: web header, favicon, app icon, social avatar, business card, signage, print, merch, motion.
- References liked: what specifically resonates, not just the brand name.
- References disliked: what to avoid.
- Hard constraints: org brand values, colors, symbols, legal restrictions, existing type, one-color requirement, embroidery, small-size requirement.

## Logo Judgment Framework

### 1. Mark Architecture

Choose the structure before drawing details:

- Wordmark: the name is the logo. Best when the name is distinctive and typography can carry the brand.
- Lockup: wordmark plus symbol. Best default for new brands because it supports recognition and fallback marks.
- Symbol-only: rarely primary for new brands; useful as a secondary mark after recognition exists.
- Letterform-as-symbol: a letter becomes the mark. Strong for favicons, app icons, and constrained square contexts.
- Monogram: multiple letters become a mark. Strong for longer names, institutional categories, hospitality, legal, finance, and premium services.

Default for most new brands: primary lockup, plus a square symbol or letterform fallback, plus monochrome and reverse versions.

### 2. Typographic Register

Pick type for category, longevity, and signal:

- Geometric sans: modern, optimistic, clean; risks generic startup polish.
- Humanist sans: professional, warm, readable; good default when trust and approachability both matter.
- Neo-grotesque sans: competent and current; needs a strong mark or custom detail to avoid anonymity.
- Transitional serif: editorial, considered, intellectual; useful for premium, legal, publishing, finance.
- Old-style serif: heritage, institutional, warm; can feel conservative if unchallenged.
- Slab serif: sturdy, declarative, journalistic; useful for strong editorial or maker brands.
- Custom display: distinctive but easy to overdo; justify each custom letterform.

Do not choose a trendy typeface just because it feels current. Choose the register that matches the audience-side perception.

### 3. Symbol Approach

Choose one primary symbolic idea:

- Literal: recognizable but prone to cliche unless drawn with specificity.
- Abstract gesture: expressive but must have clear formal logic.
- Geometric reduction: confident and scalable but often generic.
- Letterform-derived: coherent because it comes from the name.
- Monogram: compact and institutional; best when initials matter.

A logo can hold one idea well, sometimes two. Three ideas usually become committee work.

### 4. Application Discipline

Every serious candidate must be tested in Canvas:

- Primary lockup on a web header.
- Square avatar/app icon.
- Favicon at 32px and 16px.
- Single-color black on white.
- Reverse white on dark.
- Business card or small print context.
- Optional based on brief: embroidery patch, signage, motion frame, merch.

Eliminate candidates that fail small-size legibility, single-color reproduction, or silhouette recognition unless they are explicitly display-only marks.

### 5. Restraint

Most logo failure is over-design. Use these tests:

- Silhouette: the mark still reads when reduced to shape.
- Sketchability: someone can redraw the core idea from memory.
- Single-color: color is not doing the recognition work.
- Small-size: details survive at 16px and 32px.
- Distinctiveness: it does not look like three category competitors.
- Two-second memory: people remember one clear feature, not a bundle of decorative details.

If an element does not help at the smallest important size, remove it or reserve it for a larger application.

## Canvas Artifact Structure

Create a dedicated `web_page` artifact named `Logo Exploration` unless the existing project already has a suitable artifact.

Use these semantic sections:

- `logo-brief`: brand name, audience, category, constraints, references, rejection list.
- `logo-territories`: 3-5 concept territories, each with a short rationale and visual rules.
- `logo-variants`: 6-12 distinct variants across architectures. Fewer is better if each is strong.
- `logo-application-tests`: favicon, avatar, monochrome, reverse, web header, business card, and any brief-specific contexts.
- `logo-shortlist`: top 2-4 candidates with recommendation, risks, and next iteration notes.
- `logo-export`: final selected direction and export-ready arrangements, only after the user chooses a direction.

Use semantic `data-canvas-id` names such as `variant-03-symbol`, `variant-03-wordmark`, `test-favicon-16`, and `lockup-horizontal`. Avoid appearance-only names like `blue-shape`.

## Variant Standard

Each variant in the Canvas artifact should include:

- Variant name and index.
- Architecture: wordmark, lockup, symbol, letterform, or monogram.
- Typographic register and any custom letterform notes.
- Symbol approach and what it signals.
- What it rejects.
- Application notes: where it works and where it needs fallback.
- Small-size test strip: 64px, 32px, 16px.
- Single-color and reverse versions.

Make variants meaningfully different. Do not present six near-identical geometric sans wordmarks.

## Presentation Standard

When presenting back to the user:

1. State the Canvas project, artifact, and artifact path.
2. Summarize the strongest directions, not every operation performed.
3. For each shortlisted direction, explain:
   - what it communicates,
   - where it works best,
   - what it intentionally avoids,
   - what risk remains.
4. Recommend one direction when the evidence supports it.
5. Use `request_user_input` to ask for selection or targeted feedback, not broad open-ended taste reactions.

Ask "Which direction best fits the brand's audience and use cases?" rather than "Which one do you like?"

## Critique Standard

When critiquing an existing logo:

1. Inspect the actual asset, Canvas artifact, or supplied screenshot when available.
2. Score it against architecture, type, symbol logic, small-size legibility, single-color reproduction, distinctiveness, and category fit.
3. Separate fixable production issues from deeper strategic issues.
4. Recommend whether to refine, refresh, or redesign.
5. If redesigning, create the exploration in Canvas.

## Reference Files

Load only what the task needs:

- `references/architectures-explained.md`: use when choosing or explaining mark architecture.
- `references/typographic-registers.md`: use for wordmarks, typography, and category-fit decisions.
- `references/symbol-approaches.md`: use when exploring symbol, monogram, or letterform concepts.
- `references/application-contexts.md`: use for favicon, app icon, print, embroidery, signage, motion, and production tests.
- `references/category-conventions.md`: use when the category has strong logo conventions or cliches.
- `references/example-variant-spec.md`: use when writing detailed rationale for variants.
Treat the reference files as design judgment references; translate any non-Hivy output expectations into Canvas artifact sections and verified previews.
