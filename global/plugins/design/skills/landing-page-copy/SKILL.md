---
name: landing-page-copy
description: Use when writing, revising, critiquing, or placing landing page copy, hero copy, value propositions, CTA labels, signup/demo page copy, pricing page copy, sales page sections, opt-in page copy, or conversion-focused website sections. This is a design-support skill for copy that must fit the layout, audience, offer, brand voice, and conversion goal. In Hivy, use this skill with the canvas skill whenever the copy belongs in a Canvas landing page design or mockup.
---

# Landing Page Copy For Hivy Design

Use this skill to write conversion-focused landing page copy that can live inside a real design. The copy should make the page easier to understand, easier to scan, and more persuasive without becoming generic marketing language.

This skill can produce standalone copy when asked, but when the user is designing or editing a landing page, the Canvas file is the source of truth. Put the copy into the actual Canvas layout and verify that it fits.

## Core Model

A landing page has one conversion goal. Every section should reduce uncertainty and move the visitor toward one action.

Good landing page copy:

1. Names the audience and their situation clearly.
2. Makes a specific promise.
3. Explains the mechanism behind that promise.
4. Shows proof early enough to earn trust.
5. Handles objections before the CTA asks for commitment.
6. Uses CTAs that describe the action and outcome.
7. Fits the visual hierarchy and available space.

## Required Canvas Coupling

When the user asks to create, edit, mock up, or place landing page copy in a design:

1. Load the `canvas` skill before editing the artifact.
2. Run `canvas doctor`, then `canvas init`.
3. Run `canvas brands list` before choosing brand voice, naming, typography-sensitive copy length, or brand-sensitive proof language. Use the default org brand when present; otherwise inspect the relevant brand with `canvas brands view <brand-id>`. If the list is empty and brand voice or palette is needed, choose from the brief or product context, create a brand with `canvas brands create`, and tell the user what was saved.
4. Create or open the target Canvas file/page.
5. Inspect the current layout, text boxes, section order, and visible constraints before writing.
6. Use `canvas mcp execute_code` to place or update the copy in the Canvas design.
7. Verify structurally after each write batch.
8. Export the affected board with `canvas mcp export_shape`, decode it to `/tmp`, and inspect it with `read_file` before claiming the copy fits visually.

Do not treat copy as separate from layout. Shorten, split, or restructure copy when the design needs scannability, mobile fit, or stronger hierarchy.

## Brand Source Rule

Org brands are the source of truth for brand voice, terminology, logo references, color-sensitive copy placement, and typography constraints. Always run `canvas brands list` for Canvas-backed landing page work before writing or placing copy.

Use the default brand when one exists. If there are multiple brands and no default, use `request_user_input` to choose one. If no brand exists, infer a starter voice or palette only when the offer, audience, product category, or current Canvas file gives enough direction; then persist it with `canvas brands create` and tell the user. If context is too thin, use `request_user_input` for one focused direction question.

## When To Use

Use this skill for:

- Landing page hero sections.
- Product, homepage, signup, demo, pricing, waitlist, launch, or campaign pages.
- Value proposition and positioning copy for a page.
- CTA button labels and supporting microcopy.
- Feature sections written as outcomes, not specs.
- Social proof, testimonials, proof blocks, objections, FAQs, and final CTA sections.
- Copy critique for a landing page design.

Do not use it for long-form blog posts, emails, legal copy, generic brand voice definition, or product UI microcopy outside a conversion page.

## Inputs To Gather

Ask only for missing inputs that would materially change the copy. When input is needed, use the `request_user_input` tool instead of asking a plain-text questionnaire. Keep it to one to three focused questions with concrete choices when possible.

Collect:

- Offer: what exactly the visitor gets.
- Audience: who this page is for, and what situation they are in.
- Conversion goal: one primary action, such as sign up, book demo, join waitlist, buy, download, or contact sales.
- Main objection: the most likely reason the visitor will hesitate.
- Brand voice: read from the selected org brand when available; if no brand exists, infer it from strong offer/audience context only when you will persist it as a new Canvas brand, otherwise request it.
- Proof: customer logos, testimonial snippets, metrics, case studies, funding, security, compliance, press, or founder credibility.
- Constraints: page length, regulatory restrictions, SEO requirement, existing layout, existing brand language.

If the user gives enough context, proceed and state assumptions in the Canvas notes or copy summary.

## Page Framework

Use these sections as a decision model. The page can be short or long, but these jobs still need to be covered.

### 1. Hero

The hero decides whether the visitor stays.

Include:

- Headline: the promise, specific and outcome-focused.
- Subheadline: the mechanism or reason to believe.
- Primary CTA: one clear action.
- Supporting cue: optional risk reducer such as "No credit card required" or "Takes 2 minutes."

Avoid:

- Generic adjective stacking.
- "Welcome to" headlines.
- Brand-name-only headlines.
- Vague claims like "streamline your workflow."

### 2. Early Proof

Prove quickly that someone credible trusts the offer.

Use whichever proof exists:

- Customer logo bar.
- Trust statistic.
- One specific testimonial.
- Press or partner mentions.
- Security/compliance cue when trust is the main barrier.

### 3. Problem And Promise

Show that the page understands the visitor's situation before selling.

Use the visitor's language. Keep it short. If the section sounds like internal marketing language, rewrite it.

### 4. Solution And Mechanism

Explain how the offer works.

Frame features as outcomes:

- Weak: "Real-time collaboration."
- Strong: "Edit together without copying screenshots into Slack."

Use 3-5 feature/outcome blocks for most pages.

### 5. Deeper Proof

For committed visitors, add proof with specifics:

- Named case study.
- Before/after metric.
- Testimonial with role and company.
- Product screenshots or demo clips.
- Data points that support the headline.

### 6. Objection Handling

Name the reasons visitors hesitate:

- Price.
- Time to set up.
- Trust.
- Risk.
- Switching cost.
- Comparison to alternatives.
- Team adoption.

Handle objections through FAQ, comparison, guarantee, trial, setup note, security note, or proof.

### 7. Final CTA

Close with the same primary action from the hero. Do not introduce a new offer at the bottom.

Use CTA labels with action and outcome:

- "Start your free trial."
- "Book a 15-minute demo."
- "Join the beta."
- "Get the migration checklist."

Avoid "Submit", "Click here", and vague "Learn more" CTAs unless the page truly is informational.

## Canvas Copy Fit Rules

When writing directly into a Canvas layout:

- Keep hero headlines short enough to scan in one breath.
- Prefer one strong sentence over stacked marketing phrases.
- Make CTA text fit the button without shrinking type below the design system's readable size.
- Do not use copy to explain UI controls that should be visually self-evident.
- Maintain hierarchy: headline, subheadline, proof, CTA, then details.
- Watch mobile widths. If a line breaks awkwardly, rewrite the copy rather than forcing the layout.
- Do not add new sections that the layout cannot support unless the user asked for a broader redesign.
- Preserve existing brand terminology unless it weakens clarity.

## Critique Standard

When critiquing existing landing page copy:

1. Identify the page's primary conversion goal.
2. Check whether the hero makes a specific promise to a specific audience.
3. Check whether proof appears early enough.
4. Check whether features translate into outcomes.
5. Check whether the main objection is handled before the CTA.
6. Check whether CTA labels are clear and low-friction.
7. Check whether the copy fits the visual hierarchy and mobile constraints.

Separate copy problems from design/layout problems. If the issue is layout, say so and use Canvas when editing the design.

## Output Standard

When working in Canvas:

1. State the Canvas file/page updated.
2. Summarize the section-level copy changes.
3. Note the conversion goal and main objection addressed.
4. Mention any copy that was shortened to fit the layout.
5. Report verification: structural check plus exported board inspection.

When standalone copy is requested, provide a structured page draft:

```markdown
# [Page Title]

## SECTION: Hero
- Headline:
- Subheadline:
- Primary CTA:
- Supporting cue:

## SECTION: Early Proof

## SECTION: Problem / Promise

## SECTION: Solution

## SECTION: Deeper Proof

## SECTION: Objection Handling

## SECTION: Final CTA

## Variants To Test
```

## Reference Files

Load only what the task needs:

- `references/hero-formulas.md`: use when generating or improving hero headline directions.
- `references/objection-library.md`: use when identifying and handling conversion objections.

Treat the reference files as copy judgment references; translate any design-related output into Canvas sections and verified Canvas edits when the user is building a landing page artifact.
