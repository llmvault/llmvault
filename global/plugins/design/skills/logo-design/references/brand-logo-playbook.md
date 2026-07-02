# Brand → logo playbook

How to choose the *right* mark for the brand in front of you. Work through the
three decisions in order, check the exemplar principles, then write the prompt
(see `references/mark-prompts.md` for symbols, `references/type-and-color-prompts.md`
for letterforms).

**Exemplars are for principles, never for imitation.** Analyze *why* a famous mark
works and apply that mechanism to a fresh idea. Never generate anything resembling a
real company's trademark. (If the user needs a real company's actual logo — say, for
an integrations page — that's the `thesvg` skill, not generation.)

## Decision 1 — Mine the name before the category

The single best source of a distinctive mark is the **brand name itself**, because a
name-derived mark is ownable — no competitor can use it. A category-derived mark
(wheat for a farm, a tooth for a dentist, a house for real estate) is what every
competitor already has.

Interrogate the name in this order:

1. **Does the name contain an image?** Meadowlark → a lark. Northgrid → a compass
   needle in a grid. Stripe → a stripe. Draw *that*, reduced to geometry.
2. **Does the name contain a metaphor or action?** "blip" → a radar pulse, a dot
   appearing. Amazon famously draws its a→z arrow as a smile — the name's promise
   ("everything from a to z") made literal.
3. **Does a letter want to become the mark?** A distinctive initial can carry an
   app icon and favicon better than a busy symbol (see the lettermark templates).
4. **Only if the name yields nothing**, fall back to the category's world — and then
   draw the *most specific* thing in it, not the most generic.

Present name-derived and category-derived candidates side by side when both exist;
users almost always prefer the name-derived one once they see it.

## Decision 2 — Architecture, by two questions

**Q1: Is the name short and distinctive enough to BE the logo?**
**Q2: Will the mark live mostly in tiny squares (app icon, avatar, favicon)?**

| | Lives in squares | Lives on headers/print |
|---|---|---|
| **Name can carry it** | Lettermark or custom wordmark + letter fallback | Wordmark (type is the logo) |
| **Name can't carry it** | Symbol-first; wordmark secondary | Symbol + wordmark lockup |

Default for most new brands: **lockup (symbol + wordmark) with the symbol designed
square-first**, because the pack needs an app icon and favicon regardless.

## Decision 3 — Register, by audience seriousness

Pick the typographic and formal register from what the *audience* must feel, not
what's trendy:

| Audience / category | Forms | Type register | Color stance |
|---|---|---|---|
| Enterprise, dev tools, B2B | precise geometry, balanced negative space | geometric / neo-grotesque sans | one confident hue (indigo, blue, teal); no gradients |
| Finance, legal, insurance | symmetry, stability, monograms | grotesque or transitional serif | deep blue, forest, ink; muted > bright |
| Consumer social, chat, Gen-Z | one bold rounded silhouette | heavy rounded sans, often lowercase | vivid, saturated; gradients acceptable here |
| Food, farm, hospitality | specific natural forms, smooth organic curves | humanist sans or old-style serif | earthy greens, ochre, terracotta, cream |
| Luxury, fashion, beauty | type IS the logo; monogram at most | high-contrast serif or wide-tracked caps sans | monochrome; one color, lots of space |
| Health, education, kids | soft rounded geometry, open forms | humanist / rounded sans | teal, warm blue, green; bright multi-color for kids |

## Mechanisms that make famous marks work

Each of these is a *mechanism you can reuse*, stated accurately:

- **Negative space carries a second idea.** FedEx hides an arrow between the E and x;
  the NBC peacock's body is negative space; WWF's panda lets the white page be the
  white fur. Mechanism: design the gaps, not just the fills. This is the highest-value
  trick in logo design and SVG generation can do it if you *describe the negative
  space explicitly* in the construction.
- **One geometric element, owned completely.** Vercel is a single triangle; Target is
  two concentric circles; Chase is four shapes around a square void. Mechanism: if the
  form is this reducible, it survives any size and anyone can sketch it — instant
  memorability. The risk is genericness; buy distinctiveness with proportion or angle,
  not added detail.
- **A disambiguating detail earns its place.** Apple's bite exists so the silhouette
  reads as an apple, never a cherry, at any size. Mechanism: every detail must do a
  *job* (scale disambiguation, direction, recognition). If a detail has no job, cut it.
- **Constructed from circles and arcs.** The classic Twitter bird was famously built
  from overlapping circles — that's why its curves feel inevitable rather than drawn.
  Mechanism: specify arc-and-circle construction in prompts to get smooth,
  professional curvature instead of wobbly freehand.
- **Interlock survives one color.** Mastercard's two circles overlap; the mark works
  in pure black because the *relationship* is the idea, not the colors. Mechanism:
  test every candidate in single-ink; if the idea disappears, the color was doing the
  work.
- **Rhythm as identity.** IBM's eight-bar wordmark (Paul Rand) turns plain letters
  into a proprietary texture. Mechanism: a single systematic intervention across a
  wordmark (bars, a repeated cut, one joined pair) is ownable; five interventions are
  noise.
- **Silhouette is the brand.** Snapchat's ghost reads at any size with zero interior
  detail. Mechanism: for consumer/social brands, design the outline first and let the
  interior stay almost empty.

## Cliché blacklist

Reject these on sight unless deliberately subverted: globes and swooshes (generic
tech), three overlapping circles (community), lightbulbs (ideas), rockets (startups),
shields with checkmarks (security), handshakes, generic leaves on anything "eco",
sparkles for "AI", gradients used to hide weak geometry.

## The quality gate

A candidate must pass **all five** before you build its pack:

1. **Silhouette** — reads as a distinct shape with no color.
2. **Sketchability** — a person could redraw the core idea from memory.
3. **Single-ink** — works solid black on white and solid white on dark.
4. **16px** — still legible at favicon size (render it small and look).
5. **Ownability** — wouldn't be mistaken for three competitors' marks.

If it fails, simplify the geometry or move the idea into negative space — don't add
detail.
