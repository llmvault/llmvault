---
name: illustration-system
description: Use when a user wants website or product illustrations in one consistent visual style — an illustration system: spot and section illustrations, empty states, onboarding art, 404 pages, feature graphics, or "more images like these" matching existing samples. Generates native SVG illustrations that hold a style across many subjects using a written style contract plus reference conditioning. Use for graphic-heavy colorful sites and multi-illustration sets; for logos use logo-design, and for raster photos/heroes use imagegen.
---

# Illustration systems: one style, any subject

This skill produces **sets of vector illustrations that read as one system** — the
thing that makes graphic-heavy sites (meditation apps, fintech, consumer SaaS)
look designed rather than assembled. The method is empirically validated: a
written **style contract** carries the system's mechanics, attached **reference
images** carry the character language, and hardening rules suppress the drift
each one causes alone.

## The principle

One generation call is creative once; a *system* needs every call disciplined the
same way. So:

- **The contract** (one verbatim paragraph, reused in every prompt) enforces
  composition, fill discipline, light logic, palette, and geometry.
- **The references** (sample images passed via `reference_asset_ids`) teach what
  prose can't verbalize — how figures and faces are stylized, recurring motifs.
- **Neither is optional.** Contract-only draws off-system characters;
  references-only (or a soft contract) lets gradients, palette drift, and empty
  white voids creep in.

## Procedure

### 1. Establish the style source

Three modes — pick by what the user has:

- **Samples provided** (uploads or drive assets): the ideal case. Use 3–6 images
  from ONE system. These become both your decoding source and your references.
- **Brand but no samples**: build a new system. Pull the brand's colors (ask, or
  `canvas brands list` if available), choose system mechanics that fit the
  brand's tone, and write the contract from scratch. The first approved
  generations become the reference pool.
- **Nothing yet**: ask one `request_user_input` with 2–3 named directions (e.g.
  "flat geometric with giant cropped shapes", "hand-drawn line + one accent
  color", "chunky characters, saturated palette") before writing anything.

### 2. Decode the style into a contract

Read `references/style-contracts.md` (required) and write the contract using its
six axes: scale & composition, fill discipline, light & shading logic, palette
(exact hexes), figure & character language, geometry vocabulary. One paragraph,
every sentence checkable against an image. Include the anti-drift clauses even
in the first draft: "flat solid fills only — no gradients" (if true of the
system), the ground treatment ("a saturated ground fills the entire frame — no
white voids"), and "no text, no letters, no watermark".

If the user provided samples and they're not in the drive yet, save them there
first (they must be drive assets to be used as references).

### 3. Seed check — two test subjects

Generate two simple subjects before committing to the full set. Each call to
`hivy_generate_vector_image`:

- `description`: one subject clause (1–3 sentences: the subject **and how it
  uses the system's compositional moves**) + the verbatim contract.
- `reference_asset_ids`: the sample assets (3–6).
- `type`: `illustration`.

Post the results' `public_url`s and let the user judge. Tune the contract — one
clause per revision — until a seed passes. **Do not proceed to the set until the
user approves a seed.**

### 4. Produce the set

One subject per call, same recipe. Two rules that keep the set coherent:

- The contract is **verbatim identical** in every call. Only the subject clause
  changes.
- **Approved outputs join the reference pool**: add each user-approved
  illustration's `drive_asset_id` to `reference_asset_ids` for later subjects.
  The system self-reinforces.

When an output drifts (gradient, white void, off-palette tone, off-system
character), regenerate with the violated clause moved to the front of the
subject clause. One change per retry.

### 5. Deliver — link EVERY asset

Each generation is already stored in the drive; you also deliver organized
copies and optional PNGs:

```bash
mkdir -p /workspace/illos
# download each approved SVG (the public_url redirects — -L is required)
curl -fL --retry 3 -o /workspace/illos/<subject>.svg "<PUBLIC_URL>"

# optional PNG export (transparent by default)
node .skills/illustration-system/scripts/svg_to_png.mjs /workspace/illos/<subject>.svg \
  --out /workspace/illos/png --sizes 1600,800

# upload everything and print one URL per file
bash .skills/illustration-system/scripts/deliver.sh /workspace/illos illustrations/<system-slug>
```

**Every file's `asset_url` goes in your reply — every single one, grouped under
short headings.** A file you don't link is a file the user can't see. State that
SVGs are the masters (recolorable — fills are inline text) and PNGs are
transparent exports.

## Critical nuances

- **Do NOT recrop viewBoxes on illustrations.** In full-bleed systems the crop
  IS the composition — giant shapes are *supposed* to bleed off the canvas.
  (This is the opposite of the logo pipeline, which recrops marks to content.)
  If an illustration arrives visibly mis-framed, regenerate instead.
- **Aspect ratio**: shape it through the subject clause ("wide landscape
  composition", "tall vertical composition") — the SVG canvas comes back as
  generated.
- **Recoloring a delivered set** (e.g. brand hex tweak): SVG fills are plain
  text — a careful find/replace of exact hexes across the set is deterministic
  and drift-free. Prefer it over regenerating.
- **Trade dress**: emulating a famous brand's exact system is for learning only.
  Client systems get their own palette, motifs, and character language via the
  same six axes. See the note in `references/style-contracts.md`.
- **Iterating a near-miss**: as with logos, you can pass a liked illustration
  back as a reference with a revised clause — conditioning, not editing; the
  user judges each round.

## Scripts

- `scripts/svg_to_png.mjs` — SVG → transparent PNG at a size ladder
  (`rsvg-convert` when present, else auto-installs `@resvg/resvg-js`).
- `scripts/deliver.sh` — uploads a directory to the org drive, prints
  `<file>  <asset_url>` per file.

## Reference

- `references/style-contracts.md` — REQUIRED before writing any contract: the
  six decoding axes, the validated worked example with its exact contract and
  subject clauses, the experiment findings, the anti-drift hardening rules, and
  the trade-dress rule.
