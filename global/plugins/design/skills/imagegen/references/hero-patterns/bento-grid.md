# Bento grid hero

Headline plus a mosaic of 3–6 tiles in mixed sizes (the "bento box"), each tile holding an image, stat, or mini-feature. The hero's impact comes from the *set*: tiles must feel like siblings from one family. Popular with product-led SaaS, agencies, and portfolio sites.

## What the layout does to the images

- Each tile crops its image to a different aspect (squares, wide strips, tall cells) with rounded corners.
- Tiles sit side by side, so stylistic mismatch between images is instantly visible — consistency matters more than any single image's quality.
- Some tiles hold non-image content (stats, logos, UI snippets); generated images typically fill 2–4 tiles.

## Image requirements

- **One shared style contract across all tiles**: same medium, same palette, same lighting logic, same level of detail. Write the contract once and prefix every prompt with it verbatim.
- Each tile gets ONE simple subject — bento tiles are small; one idea per tile.
- Centered or near-centered subjects (tiles crop from all sides at different ratios).
- Slight variation in subject scale across tiles (one close-up, one medium, one wide) makes the grid feel curated rather than repetitive.

## Aspect ratio

Generate each image at its tile's ratio: `1:1` for squares, `16:9` for wide strips, `3:4` for tall cells. Don't generate one image and crop it into all tiles.

## Consistency techniques

1. **Shared prompt prefix** (strongest lever): identical style/palette/lighting sentence starting every tile prompt.
2. **Anchor + remix**: generate the first (largest) tile with `hivy_generate_image`, save it, then produce the remaining tiles with `hivy_remix_image` referencing the anchor (see `references/consistent-assets.md`).
3. Generate all tiles in one session, and 2 variants per tile — mismatched tiles get swapped from variants.

## Example prompt set

Shared prefix (use verbatim on every tile):

> Soft 3D render, matte clay-like material, gentle studio lighting, palette of sage green, cream, terracotta and charcoal, minimal and premium, subject centered, soft gradient backdrop filling every edge of the frame. No text, no letters, no logos.

Tile 1 (1:1, close-up): shared prefix + "A single stylized potted olive tree, medium close-up."
Tile 2 (16:9, wide): shared prefix + "A row of five small stylized houses on a gentle hill, wide shot."
Tile 3 (3:4, tall): shared prefix + "A tall stylized watering can pouring a thin stream of water, full-height composition."

## Pitfalls

- The #1 failure is stylistic drift between tiles — if one result drifts (different lighting, saturation, render style), regenerate that tile with the reference-chaining technique rather than accepting it.
- Don't put a panorama in a small tile or a close-up in the biggest tile; match subject scale to tile size.
- Keep palettes tight: four named colors max in the contract, or the grid turns kaleidoscopic.
