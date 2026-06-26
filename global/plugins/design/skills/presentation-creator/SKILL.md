---
name: presentation-creator
description: Use this skill when the user asks a design agent to create, design, outline, critique, rewrite, or improve slides, slide decks, presentations, pitch decks, investor decks, sales decks, proposal decks, executive briefings, webinar decks, or business/conversion presentations. It teaches narrative structure, persuasive slide language, one-message-per-slide discipline, business proof, conversion asks, speaker notes, and slide QA, then directs the agent to create and verify the actual deck inside Hivy Canvas with the canvas skill.
---

# Presentation Creator

Use this skill to shape slide decks that persuade clearly, then build the actual artifact in Hivy Canvas.

- **Use for:** narrative arcs, slide sequences, slide copy, business proof, conversion asks, pitch decks, sales decks, executive decks, stakeholder updates, speaker notes, and slide QA.
- **Do not use for:** long-form prose, generic product UI design, logo work, or final decks built only as prose. Use `design-taste`, `landing-page-copy`, `logo-design`, and `canvas` alongside this skill when those concerns are present.

## Required Canvas Coupling

When creating, editing, redesigning, or presenting a deck artifact:

1. Load the `canvas` skill before editing the artifact.
2. Run `canvas doctor`, then `canvas init`.
3. Run `canvas brands list` before choosing palette, typography, logo treatment, or voice. Use the default org brand when present; otherwise inspect the relevant brand with `canvas brands view <brand-id>`. If the list is empty and the deck needs a palette, choose a defensible starter palette from the brief or business context, create it with `canvas brands create`, and tell the user what was saved.
4. Create or open the target Canvas file and page.
5. Build each slide as a 16:9 Canvas board. Name boards with slide number and purpose, such as `03 - Problem cost`.
6. Use `canvas mcp execute_code` for slide boards, text, shapes, charts, callouts, speaker-note areas, and proof elements.
7. Verify structurally after write batches.
8. Export representative slides or the affected deck boards with `canvas mcp export_shape`, decode them to `/tmp`, and inspect with `read_file` before claiming the deck is visually correct.

Use Canvas for the actual slide work: board sequence, visual hierarchy, copy fit, business proof, speaker notes, variants, and final export boards.

Do not produce final deck work only as prose, Markdown, SVG snippets, local PPTX files, or local images unless the user explicitly asks for non-Canvas output. If a local file is useful, create it as an export or temporary aid and bring the result back into Canvas.

## Brand Source Rule

Org brands are the source of truth for deck palette, typography, logo usage, voice, and tone. Always run `canvas brands list` before brand-sensitive slide work.

Use the default brand when one exists. If multiple brands exist and no default is present, use `request_user_input` to choose which brand to design against. If no org brand exists, choose a starter palette or voice only when the brief, audience, category, or desired outcome gives enough direction; then persist it with `canvas brands create` before treating it as the deck system. Tell the user what brand and palette you saved. If context is too thin, use `request_user_input` for one focused direction question.

## Workflow

Track this checklist:

```text
Presentation progress:
- [ ] Step 1: Gather context and conversion goal
- [ ] Step 2: Pick deck mode and narrative arc
- [ ] Step 3: Outline slide sequence
- [ ] Step 4: Write slide copy and proof
- [ ] Step 5: Design slides in Canvas
- [ ] Step 6: Add speaker notes when useful
- [ ] Step 7: QA slides and export verified boards
```

### Step 1: Gather Context

Ask only for missing inputs that materially change the deck. Use `request_user_input` instead of a plain-text questionnaire when input is needed.

Collect:

- Audience: executives, buyers, investors, team, customers, board, conference attendees, or mixed.
- Setting: live presentation, async/forwarded deck, sales call, demo, proposal, board update, or investor pitch.
- Desired action: approve, buy, book a call, fund, align, decide, adopt, share, or continue evaluation.
- Three messages: what the audience must remember when the deck is over.
- Proof: metrics, customer quotes, screenshots, research, case studies, logos, security/compliance, or financial model.
- Objection: the main reason the audience may hesitate.
- Constraints: deck length, brand, deadline, required sections, existing material, export format, and presenter notes.

### Step 2: Route by Deck Mode

- **Live talk or keynote:** sparse slides, strong pacing, speaker notes. Read `references/outline-structure.md`, `references/writing-slides.md`, `references/visual-design.md`, and `references/speaker-notes.md`.
- **Async or forwarded business deck:** denser headlines, more context, proof visible on-slide, complete section titles. Read `references/business-conversion-decks.md` before writing.
- **Sales, demo, or proposal deck:** buyer problem, current cost, outcome, proof, implementation, investment, next step. Read `references/business-conversion-decks.md`.
- **Investor pitch deck:** standalone pitch structure and explicit ask. Read `references/pitch-decks.md` first, then use the other references as needed.
- **Stakeholder or design review:** hook, context, options, evidence, decision needed. Use this skill with `design-taste` if the deck itself needs visual polish.

### Step 3: Build the Outline

Read `references/outline-structure.md`.

Start with an arc:

```text
Opening -> Context / tension -> Proof / proposal -> Ask / next step
```

For business and conversion decks, every section should answer one of these:

- Why this matters now.
- What changes for the audience.
- Why this recommendation is credible.
- What decision or action is needed next.

### Step 4: Write Slide Copy

Read `references/writing-slides.md`.

Write headlines as claims, not labels:

```text
Weak: Market
Strong: The market is ready because manual work is breaking teams
```

Use one message per slide. Body copy exists only to support that message. In async decks, the headline must still make sense when forwarded with no presenter.

If the deck is conversion-focused, every major section should move the audience closer to a decision: pain, cost, outcome, proof, risk removal, and next step.

### Step 5: Design in Canvas

Read `references/visual-design.md`.

Use the selected Canvas brand first. Do not default to dark slides, gradients, or arbitrary section colors unless the brand or brief supports that direction.

Build slides as Canvas boards:

- 16:9 deck boards, usually 1920x1080 or the local Canvas equivalent.
- One focal idea per board.
- Text sized for the presentation context.
- Proof placed where it can be scanned: charts, metrics, logos, quotes, screenshots, or annotated product moments.
- Reusable components for headers, footers, section labels, slide numbers, and source notes.

### Step 6: Speaker Notes

Read `references/speaker-notes.md` for live talks, demos, sales presentations, and executive briefings.

Skip speaker notes for investor pitch decks or fully async decks unless the user asks. For those, put the missing context on-slide instead.

### Step 7: QA Pass

Review every slide before handing over:

```markdown
| # | Slide | 3-sec test | One message | Business purpose | Brand fit | Canvas verified |
|---|-------|------------|-------------|------------------|-----------|-----------------|
| 1 | Title | pass | pass | pass | pass | pass |
```

- **3-sec test:** can the slide be parsed quickly at presentation size? If not, cut copy or split the slide.
- **One message:** exactly one core idea per slide.
- **Business purpose:** every slide earns its place by creating context, proof, confidence, or action.
- **Brand fit:** colors, type, voice, and logo usage follow the selected Canvas brand or the newly created starter brand.
- **Canvas verified:** structural read plus exported-image inspection completed for the changed boards.

Fix every flagged row before presenting the deck as complete.

## Core Principles

- **The audience is the protagonist:** frame the deck around their problem, decision, risk, opportunity, and desired future.
- **Headlines do the work:** use complete claims, not topic labels.
- **One slide, one job:** if a slide explains two ideas, split it.
- **Proof beats assertion:** business decks need numbers, screenshots, quotes, examples, or credible reasoning.
- **The ask is designed:** the final slide should make the next action obvious, specific, and low-friction.
- **Visual hierarchy carries persuasion:** size, placement, contrast, and sequence should make the intended reading order unavoidable.

## Gotchas

- **Starting in Canvas before the arc is clear:** the deck becomes a decorated list. Lock the audience, desired action, and three messages first.
- **Topic-label headlines:** "Problem", "Solution", and "Traction" are categories, not arguments. Add the claim.
- **Too much text for live delivery:** if the presenter is speaking, the slide should support the talk, not replace it.
- **Too little context for async decks:** if a deck will be forwarded, each slide must explain itself without the presenter.
- **Generic proof:** metrics, screenshots, and case studies must connect directly to the audience's objection or desired outcome.
- **Random visual system:** do not invent slide colors or type when Canvas org brands exist. If no brand exists and you create a starter brand, tell the user.
