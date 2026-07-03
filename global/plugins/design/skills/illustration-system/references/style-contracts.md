# Decoding a style into a contract

A style contract is one verbatim paragraph that captures an illustration system's
rules precisely enough that a vector model can generate *new* subjects in it. This
method was validated empirically: four real samples from a well-known meditation
brand were decoded with the six axes below, four unseen subjects were generated,
and six of eight first-take outputs were on-system (three shippable).

## The six axes

Study the sample illustrations and write one or two concrete sentences per axis.
Vague observations produce vague contracts — every sentence must be checkable
against an output image.

1. **Scale & composition.** How do shapes relate to the frame? Cropped-giant
   (enormous forms bleeding off every edge) vs contained spot illustration
   (subject floats with padding)? Full-bleed color ground vs white page?
2. **Fill & rendering discipline.** Flat solid fills? Gradients? Texture/grain?
   Outlines or strokeless shapes? This axis is the most drift-prone — state it
   as an absolute ("flat solid fills only — no gradients, no texture, no
   outlines").
3. **Light & shading logic.** How are shadow and light *drawn*? A single
   hard-edged one-step-darker shape? Soft blended shade? A light beam that shifts
   every color it crosses one step lighter? None at all? This is usually the
   system's signature mechanic — describe it as a rule, not a mood.
4. **Palette.** Exact hexes, grouped into families, with a per-image budget
   ("3–5 tones per image, one of two families: …"). Never color names alone.
5. **Figure & character language.** How are people/faces/objects stylized, and at
   what detail ceiling? ("Faces are exactly three rounded strokes: two closed-eye
   arcs and one smile — nothing else." "Bodies are round-headed single-color
   blobs.") Without this axis the model draws generic detailed
   illustration-humans — the most common failure observed.
6. **Geometry vocabulary.** Which primitives the system permits: circles and
   wedges only? Rounded rectangles? Are there corners anywhere? Stroke terminals
   rounded or cut?

## The validated worked example

The contract that produced the successful experiment, verbatim — use it as the
quality bar for specificity:

> Style contract: flat vector illustration in a warm minimalist meditation-brand
> style. Enormous overlapping circles and soft wedges, cropped by the frame so
> the big shapes bleed off the edges. Flat solid fills only — no gradients, no
> texture, no outlines. Shading is a single hard-edged shape exactly one step
> darker; where a wedge-shaped beam of light crosses the scene, every color
> inside it shifts exactly one step lighter. Palette strictly limited to one of
> two families: golden yellow #FFCE00 with warm orange #FF7E1D and deep
> orange-red #F04C24, or royal blue #0061EF with sky blue #61A0FF and golden
> yellow #FFCE00. If a face appears it is exactly three rounded strokes in deep
> navy #26254A: two closed-eye arcs and one long serene smile — nothing else.
> Calm, warm, sunrise mood. No text, no letters, no watermark.

Subject clauses that were prepended to it (one to three sentences; the subject
plus how it *uses the system's compositional moves*):

> Two enormous round faces in profile facing each other with serene closed-eye
> expressions, their circle heads overlapping and cropped by the edges of the frame.

> A sunrise over calm water: a giant half-circle sun on the horizon, flat bands
> of water below, a wide wedge-shaped beam of light crossing the water and
> lightening everything it touches.

## What the experiment established

- **The contract carries the system.** Composition, light mechanics, flat fills,
  and palette discipline all came from the written contract. Contract-only
  generations executed the hardest rule (beam tone-shifts) correctly in 3 of 4
  images.
- **References carry the character language.** The one thing the contract-only
  condition got wrong every time was the figure: it drew detailed generic
  illustration-humans. Passing the sample images as references taught the model
  the blob-figure, the stroke face, even a floating accent-dot motif — things
  prose can't fully verbalize.
- **References also import noise.** With references attached, gradients crept
  into fills, one beam washed out to an off-palette tone, and white voids
  appeared where the system wants saturated full-bleed grounds.
- **Therefore: always use both, and harden the contract against the noise**
  (next section).

## Anti-drift hardening rules

1. **Restate the fill discipline even though references are attached** — "flat
   solid fills only, no gradients" must survive in every prompt. References make
   the model *more* likely to add gradients (rendering artifacts in the samples),
   not less.
2. **Name the ground treatment.** If the system is full-bleed, say "a saturated
   ground color fills the entire frame — no white or empty background regions."
   White voids are the second most common reference-induced drift.
3. **Exact hexes in every prompt.** The model approximates color; small drifts
   compound across a set. (Delivered SVGs can also be recolored in post — fills
   are inline text.)
4. **One subject clause + the verbatim contract.** Never paraphrase the contract
   per image; verbatim reuse is what makes a set read as siblings.
5. **Approved outputs join the reference pool.** When the user approves a
   generated illustration, add its drive asset to `reference_asset_ids` for
   subsequent subjects — the system self-reinforces and drift shrinks over the
   life of the set.
6. **Reject-and-restate.** When an output violates a rule, regenerate with the
   violated clause moved to the FRONT of the subject clause ("Flat solid fills
   only, absolutely no gradients: <subject>…"). Change one thing per retry.

## Trade dress

Decoding a famous brand's system is for learning and for internal experiments.
A real client's illustration system must get its own palette, motifs, and
character language — built with the same six axes, not copied from someone
else's brand. Never deliver work that imitates an identifiable company's
illustration style.
