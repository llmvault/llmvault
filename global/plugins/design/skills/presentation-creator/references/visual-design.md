# Visual Design

Design slides as branded decision surfaces. The visual system should make the argument easier to follow.

## Core Style

Use the selected Canvas org brand first:

- Palette, typography, logo usage, and voice come from `canvas brands list` and `canvas brands view`.
- If no brand exists, use the Canvas skill's fallback: choose a restrained starter palette from the brief, persist it with `canvas brands create`, and tell the user.
- Do not default to dark-first, gradients, or arbitrary section colors unless the brand or brief supports them.

## Canvas Board Setup

- Use 16:9 boards, typically 1920x1080.
- Name boards with slide number and job: `07 - Objection risk`.
- Use consistent margins, usually 80-120px on desktop-scale boards.
- Keep reusable header, footer, slide number, section label, and source-note components consistent.
- Put speaker notes or internal rationale on separate notes boards or clearly labeled off-slide areas, not inside the visible slide.

## Typography Hierarchy

Use scale and placement before adding decoration.

| Level | Role | Guidance |
|-------|------|----------|
| Section label | Orientation | Small, consistent, brand accent or muted color |
| Headline | Main idea | Largest text; complete claim; 1-4 lines |
| Subtitle | Context | One or two lines, lower contrast |
| Body/bullets | Support | 2-4 bullets, strong lead-ins |
| Metric | Proof | Large number, clear label, source nearby |
| Caption | Source | Small but legible |

For live presentations, text can be sparser because the presenter adds context. For async decks, headlines and body copy must carry more meaning.

## Layout Patterns

### Full Statement

Use for a key point, section thesis, or decisive recommendation.

```text
[section label]

Large claim
supporting line or metric
```

### Split Proof

Use when a slide needs text plus screenshot, chart, matrix, or workflow.

```text
[claim]                       [visual proof]
[2-3 support points]          [callouts]
```

### Metrics Row

Use for traction, impact, or business case.

```text
[headline claim]

[metric 1] [metric 2] [metric 3]
[source or interpretation]
```

### Before / After

Use when the value is a visible change.

```text
Before: current pain          After: better workflow
```

### Objection Slide

Use when risk needs explicit handling.

```text
[objection as headline]
[response] [proof] [next step]
```

## Visual Elements

- **Charts:** simplify to the one comparison or trend the headline claims.
- **Screenshots:** annotate with two or three callouts max.
- **Logos:** use only when they prove credibility; respect brand usage.
- **Icons:** simple, consistent, and sparse.
- **Color:** use color for meaning, hierarchy, or wayfinding. Avoid decorative accents that imply false section changes.
- **Whitespace:** create focus; do not fill the slide just because space exists.

## Slide Type to Layout Mapping

| Slide type | Best layout |
|------------|-------------|
| statement | Full statement |
| big-statement | Centered or left-weighted claim |
| question | Full statement with space to pause |
| data | Metrics row or annotated chart |
| product-proof | Split proof or annotated screenshot |
| framework | Matrix, flow, or three-part model |
| quote | Large quote with attribution and context |
| case-study | Customer, challenge, result |
| objection | Objection slide |
| next-steps | Owner/date/action structure |

## Avoid

- Paragraph-heavy slides for live presentation.
- Sparse slogans for async decks.
- More than two consecutive slides with the same layout.
- Multiple competing focal points.
- Tiny chart labels, missing sources, or unlabeled axes.
- Random section colors that are not in the Canvas brand or created starter brand.
- Low-contrast text or decorative imagery that hides the argument.
