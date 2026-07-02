# Writing mark prompts that actually work

The mark prompt is the highest-leverage input in the whole pipeline. These rules
were validated against the live generation API — every claim below was tested, and
the example prompts are reproduced verbatim with what they produced.

## The core discovery: describe construction, not vibes

A vibe prompt ("a playful speech bubble with energy") makes the model invent every
geometric decision, and invented decisions default to generic. A **constructive
prompt** — written the way a draftsman would build the mark — gets followed almost
literally. Specify:

- **What strokes/shapes compose it** ("one solid filled circle... two concentric
  arc pulses radiate from it")
- **Relative dimensions** ("each stroke as thick as the circle radius; the spacing
  between the arcs equals the stroke thickness")
- **Terminals and corners** ("fully rounded terminals", "flat cut terminals",
  "small consistent corner radii")
- **Negative space, explicitly** ("the eye is a small circle of negative space")

## Prompt recipe

Every mark prompt has four blocks, in one flowing paragraph:

```
1 SUBJECT   A minimalist logomark of <one name-derived idea>, symbol only,
            NO text, NO letters, NO words.
2 CONSTRUCT Construction: <the geometric recipe — shapes, strokes, relative
            sizes, terminals, negative space>.
3 FINISH    Flat vector, one <color name> <#hex> on a transparent background,
            centered with generous padding.
4 (instructions field) Silhouette-first: the shape must read clearly at 16
            pixels. Single subject, single flat solid color, no gradients, no
            shadows, no 3D, no outlines around fills, no background shapes,
            production-ready SVG.
```

Derive the SUBJECT from the **name first** (see `references/brand-logo-playbook.md`,
Decision 1) — a name-derived mark is ownable; a category-derived one is what every
competitor has.

## Validated prompts (verbatim) and what came back

**Meadowlark (farm) — bird mark.** Produced a beautiful lark profile with a
negative-space wing sweep and circle eye, followed nearly clause-for-clause:

> A minimalist logomark of a meadowlark songbird in profile, symbol only, NO text,
> NO letters, NO words. Construction: the bird is reduced to smooth nested arcs —
> one continuous curve forms the back and head, a second arc forms the breast and
> belly, a short cut wedge forms the tail; the eye is a small circle of negative
> space; the beak is a single small sharp triangle. Flat vector, one warm meadow
> green #3F7D4E on a transparent background, smooth confident curves, centered with
> generous padding.

**blip (Gen-Z chat) — radar pulse.** Produced exactly the constructed geometry:
solid circle lower-left, concentric rounded-terminal pulses:

> A minimalist logomark of a radar blip, symbol only, NO text, NO letters, NO
> words. Construction: one solid filled circle sits at the lower left; two
> concentric arc pulses radiate from it toward the upper right, each stroke as
> thick as the circle radius, with fully rounded terminals; the spacing between the
> arcs equals the stroke thickness. Flat vector, one vivid purple #7C3AED on a
> transparent background, bold monolinear geometry, centered with generous padding.

## What the model does well and badly (all observed)

- **Follows construction clauses** — arcs, stroke ratios, terminals, single-body
  negative space all landed as written.
- **Negative space works INSIDE one body, not across bodies.** "The eye is a circle
  of negative space" ✔ worked. "The gap between four separate blocks forms an
  arrow" ✘ failed — multi-body negative-space constructions collapse into generic
  shapes. If the idea lives in a gap, put the gap inside a single silhouette.
- **Color is approximate.** Asked for `#7C3AED`, received `#6F2BF5`; two calls in
  the same session returned two different purples. Never rely on the model for
  brand-exact color — pass `--recolor true` to `logo_pack.mjs`, which forces every
  non-white fill to the exact brand hex deterministically.
- **The viewBox is sometimes broken** (content cropped or drifted off-canvas). Do
  not judge — and above all do not discard — a mark from a raw render alone;
  `logo_pack.mjs` automatically recrops the viewBox to the content bounding box.
  Two of our best validated marks looked "broken" before normalization.
- **Long thin strokes can wobble slightly at the ends.** Generating 2–3 candidates
  and letting the user pick absorbs this.

## Iteration protocol

Generate 2–3 candidates per round. **Change exactly one clause between candidates**
(the subject, one construction decision, or the style register) so the user's
choice tells you which decision mattered. When the user asks for a tweak, keep
every other clause frozen and modify only the named one — the prompt is your source
file; treat it like code.

**Refining a near-miss: pass the previous mark back as a reference.** When the user
likes a candidate but wants a change ("this one, but thicker strokes"), call
`hivy_generate_vector_image` again with `reference_asset_ids: ["<drive_asset_id of
the liked mark>"]` plus the revised prompt — name what stays ("same lark silhouette
and negative-space wing") and what changes ("thicken all strokes ~30%"). The
generator redraws *guided by* the reference, so the concept carries over but
geometry still drifts — this is conditioning, not editing. Use it only **before**
the mark is chosen. Once the user approves a mark it is frozen: recoloring, sizing,
lockups, and variants are deterministic `logo_pack.mjs` work, never regeneration.

## The five hard rules (unchanged, still mandatory)

1. Symbol only — NO text, NO letters, NO words (state it twice).
2. One subject; one idea.
3. Flat vector, clean geometry.
4. One color with an exact hex, transparent background.
5. Centered, generous padding, no background shapes or frames.
