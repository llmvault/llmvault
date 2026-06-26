---
name: design-taste
description: Use when designing, redesigning, critiquing, polishing, or improving the taste of websites, web apps, landing pages, homepages, portfolios, product pages, pricing pages, onboarding flows, settings surfaces, dashboards, or high-polish web interfaces. This is a design-direction skill for avoiding generic generated design: brief inference, visual dials, typography, color, layout, motion, asset, interaction-state, and preflight review discipline. In Hivy, use this skill with the canvas skill whenever the design artifact should be created, reviewed, or edited in Canvas.
---

# Design Taste For Hivy Canvas

Use this skill to make website and web app designs feel specific, intentional, and appropriate to their audience. It is not a code framework. It is a taste and review framework for design work that should usually become a Canvas artifact.

The Canvas file is the source of truth when the user asks for a design, redesign, mockup, critique, or polish pass. Notes are useful, but the design decisions should be visible in Canvas.

## Core Rule

Read the brief before choosing an aesthetic. Generic design usually happens when the agent starts from a default pattern instead of the product, audience, category, and constraints.

Before making or changing a design, state one concise design read:

```text
Reading this as: [surface type] for [audience], with a [tone] visual language, leaning toward [design family].
```

Examples:

- `Reading this as: B2B SaaS homepage for technical buyers, with a restrained product-led language, leaning toward crisp typography, quiet surfaces, and precise product visuals.`
- `Reading this as: consumer wellness landing page for design-conscious buyers, with a premium but warm language, leaning toward editorial spacing, tactile product imagery, and low-saturation accents.`
- `Reading this as: web app onboarding for busy operators, with a utilitarian trust-first language, leaning toward dense but calm forms, clear progress, and explicit states.`

If the design read could diverge in materially different directions, use `request_user_input` with one focused question. Do not ask a long questionnaire.

## Required Canvas Coupling

When creating, redesigning, critiquing, or polishing a website or web app design:

1. Load the `canvas` skill before editing the artifact.
2. Run `canvas doctor`, then `canvas init`.
3. Open or create the target Canvas file and page.
4. Inspect the current file, page structure, existing colors, typography, components, and layout constraints.
5. Run `canvas brands list` before choosing palette, typography, logo treatment, or voice. Use the default org brand when present; otherwise inspect the relevant brand with `canvas brands view <brand-id>`. If the list is empty and a palette is needed, choose a defensible starting palette from the brief or product context, create it with `canvas brands create`, and tell the user what was saved.
6. Create or update boards in Canvas using `canvas mcp execute_code`.
7. Verify structurally after write batches.
8. Export the affected board with `canvas mcp export_shape`, decode it to `/tmp`, and inspect it with `read_file` before claiming the design is visually correct.

For pure critique, inspect the Canvas file or supplied screenshot first. If the user wants changes, apply them in Canvas.

## Brand Source Rule

Org brands are the source of truth for brand-sensitive design decisions. Always run `canvas brands list` before selecting or changing colors, typography, logo usage, or voice.

Use the default brand when one exists. If there are multiple brands and no default, use `request_user_input` to choose the brand. If no brand exists, choose a tasteful starter palette only when the brief, audience, product category, or current Canvas file gives enough direction; then persist it with `canvas brands create` and tell the user. If the context is too thin to choose responsibly, use `request_user_input` for one focused direction question.

## Scope

Use this skill for:

- Websites, landing pages, marketing pages, launch pages, portfolios, and editorial pages.
- Web app surfaces that need strong product design: onboarding, empty states, dashboards, settings, forms, pricing, account, analytics, workflow screens.
- Redesign audits and polish passes.
- Design critique focused on why something feels generic, crowded, flat, mismatched, or untrustworthy.
- Canvas mockups where visual direction, hierarchy, typography, color, or layout quality matter.

Do not use it as the primary skill for logo-specific judgment, landing page copy, or Canvas operations. Use `logo-design`, `landing-page-copy`, and `canvas` alongside it when those concerns are present.

## Design Dials

After the design read, set three dials and use them to guide layout, density, and motion:

- `DESIGN_VARIANCE`: 1 means conventional and symmetrical; 10 means highly expressive.
- `MOTION_INTENSITY`: 1 means static; 10 means cinematic and motion-led.
- `VISUAL_DENSITY`: 1 means airy and sparse; 10 means dense and operational.

Useful defaults:

- B2B SaaS landing: variance 6, motion 4, density 4.
- Creative agency or portfolio: variance 8, motion 7, density 3.
- Premium consumer site: variance 7, motion 5, density 3.
- Web app dashboard: variance 3, motion 2, density 7.
- Settings, forms, admin, or workflow UI: variance 3, motion 2, density 6.
- Public-sector or regulated service: variance 2, motion 1, density 5.
- Redesign preserve: match existing dials, then improve typography, spacing, and rhythm first.
- Redesign overhaul: increase variance only after preserving information architecture and brand-critical cues.

Do not ask the user to tune these numbers. Infer them from the brief and state them in the Canvas notes or critique summary when useful.

## Website And Web App Distinction

Websites sell, explain, or build confidence. They need clear first impression, memorable visual direction, proof, and a strong content rhythm.

Web apps help users do repeated work. They need scanability, stable controls, explicit states, accessible forms, predictable navigation, and density matched to task complexity.

Do not force website drama onto operational UI. Do not make web apps look like landing pages unless the screen is actually an onboarding, upsell, or empty-state moment.

## Anti-Default Discipline

Avoid these default generated-design patterns unless the brief explicitly justifies them:

- Purple-blue gradient glow as the main identity unless the selected org brand requires it.
- Centered hero with vague headline, subheadline, and three cards.
- Three identical feature cards as the primary section.
- Decorative blobs, random glass panels, and fake depth.
- Div-based fake screenshots that do not show a real product state.
- Text-only "minimal" pages with no meaningful visual asset.
- Overused beige, brass, clay, oxblood, and espresso palettes for premium consumer brands unless those values come from the selected org brand.
- Inter plus slate palette as the unexamined default.
- Excessive section eyebrows, section numbers, scroll cues, version labels, and decorative status dots.
- Generic copy like "elevate", "seamless", "next-gen", "unlock", or "revolutionize."
- Fake-perfect metrics and fake customer names.

The point is not to be weird. The point is to choose a visual language because it fits the surface and audience.

## Typography

Typography should carry hierarchy before decoration does.

- Use hero-scale type only for real heroes or editorial moments.
- Keep app-panel headings smaller and tighter than marketing-page headings.
- Prefer strong sans-serif display choices for modern web work unless the brand clearly calls for serif.
- Use serif only when the brief is genuinely editorial, luxury, heritage, publication, or explicitly brand-led.
- Do not mix a random serif word into a sans headline just for visual interest. Use weight, italic, or color within the same type family first.
- Watch descenders in italic display text. Leave enough line height and padding so letters like `g`, `j`, `p`, `q`, and `y` do not clip.
- Keep body copy readable and line lengths controlled.

In Canvas, test type at the actual board size and at the likely mobile width. Rewrite copy or adjust scale when text breaks awkwardly.

## Color

Pick a palette from the selected org brand and then lock it. If no org brand exists, choose a restrained, context-specific starter palette, persist it with `canvas brands create`, and then lock it. Do not randomly choose a brand palette.

- Use one dominant accent color unless the brand system already requires more.
- Keep saturation restrained by default.
- Keep neutral temperature consistent. Do not mix warm and cool grays by accident.
- Avoid purple-blue gradients unless they come from the selected org brand or an explicit user brief.
- If no org brand defines the palette, avoid premium-consumer autopilot palettes: warm cream plus brass, clay, oxblood, and espresso.
- Ensure text, buttons, forms, and ghost controls meet accessible contrast.
- Test reverse or dark-mode surfaces as a system, not section by section.

In Canvas, create or reuse color tokens when possible. Do not invent one-off colors across sections.

## Layout

Layout should create rhythm without repeating the same trick.

- Avoid using centered hero composition by default when a split, asymmetric, editorial, or product-led composition would better fit.
- The first viewport should reveal the product, object, workflow, or brand promise clearly.
- Hero content should fit the initial viewport. CTA should not require scrolling.
- Navigation should stay one line on desktop.
- Do not repeat the same section layout over and over.
- Avoid three or more consecutive image/text zigzags.
- Do not put cards inside cards.
- Use cards only when elevation communicates grouping or hierarchy. Otherwise use spacing, borders, bands, and alignment.
- Bento grids need real rhythm and exact content count. No empty cells and no all-text white tiles.
- Long lists need better structure: grouped chunks, tabs, accordion, highlight cards, or a separate details page.

For web apps, prioritize stable dimensions, clear grouping, visible states, and repeated-use ergonomics.

## Visual Assets

Websites and web apps need real visual evidence:

- Use product screenshots, actual UI states, generated images, brand assets, or clear Canvas-drawn interface states.
- Do not use decorative SVGs as filler.
- Do not represent a product with random rectangles unless it is explicitly a wireframe.
- Social proof should use real logo marks or credible generated marks, not plain text labels pretending to be logos.
- Image labels, captions, and overlays should be functional, not decorative.

If required assets are missing, use `request_user_input` when the choice matters. Otherwise leave a clear placeholder in Canvas and note the needed asset dimensions.

## Motion And Interaction States

Motion should explain hierarchy, sequence, feedback, or state change. If it cannot be justified in one sentence, skip it.

For Canvas mockups, express motion as annotated states, prototype connections, or motion notes. Do not imply motion exists unless it is represented in the file.

For web app surfaces, include expected states:

- Loading.
- Empty.
- Error.
- Success.
- Hover/focus/active.
- Disabled.
- Validation.

Static happy-path-only app screens are incomplete.

## Redesign Protocol

For redesigns, classify the mode before changing anything:

- Preserve: modernize without breaking brand, content, SEO, or familiar flows.
- Overhaul: new visual language, but still preserve critical content and structure unless approved.
- Greenfield: no existing design constraints or the user explicitly wants a fresh start.

Audit before touching:

- Brand tokens: colors, type, logo, radius, image style.
- Information architecture: nav, routes, sections, conversion paths.
- Content blocks: what is useful, what is filler.
- Patterns to preserve: distinctive interactions, recognizable hero, voice, trusted layout.
- Patterns to retire: generic design tells, broken mobile, clutter, weak hierarchy.
- Accessibility wins: contrast, focus states, labels, keyboard paths.

Never change route structure, primary nav labels, legal/consent copy, analytics-sensitive form fields, or the logo/wordmark without explicit approval.

## Canvas Preflight

Before presenting website or web app design work, run this review against the Canvas export:

- Design read is clear and reflected in the artifact.
- Dials match the surface type and audience.
- Page theme is coherent across sections.
- Color and radius systems are consistent.
- Text contrast is accessible.
- CTA labels fit and do not wrap awkwardly.
- Forms have labels, helper/error placement, focus treatment, and contrast.
- Hero fits the initial viewport for the target board size.
- Hero has no more than one small label element.
- Trust logos or proof sit below the hero unless there is a strong reason.
- Section layouts do not repeat monotonously.
- There are not three identical feature cards as the main design.
- Visual assets are real, generated, or explicitly marked as placeholders.
- No decorative scroll cues, version labels, section numbers, or status dots.
- No fake-perfect metrics or generic names.
- Copy has been reread for clarity and grammar.
- Motion is represented only where justified.
- Mobile or narrow layout is accounted for when relevant.
- Web app screens include loading, empty, error, and interaction states when the flow requires them.
- Exported Canvas board has been inspected with `read_file`.

If a check fails, fix the Canvas design before calling the work done.

## Presentation Standard

When presenting the result:

1. State the design read and the surface type.
2. Name the Canvas file/page/board touched.
3. Summarize the main design decisions.
4. Call out the strongest improvement in hierarchy, trust, scanability, or memorability.
5. Mention any assumptions or asset placeholders.
6. Report verification: structural check plus exported board inspection.

Keep the presentation specific. Do not explain generic design theory unless the user asks for it.
