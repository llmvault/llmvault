# Card hero

The image lives inside a rounded card (or stacked cards) floating over a plain, gradient, or subtly textured section background. Copy sits beside or above the card. Common in modern SaaS, fintech, and portfolio sites; the card gives the image a finished, "product shot" quality.

## What the layout does to the image

- The card crops the image to its own aspect with rounded corners — corner content gets clipped.
- The card usually has a drop shadow and sits on a quiet background, so the image provides ALL the visual richness.
- Cards are often smaller than half the viewport: composition must read at modest size.
- Stacked/overlapping card layouts may show only a strip of secondary images.

## Image requirements

- A contained, deliberate composition — this is the one overlay-free pattern where **centered subjects and diorama-style images work beautifully** (the centered-diorama gravity of isometric/soft-3D styles is an asset here, not a bug).
- Nothing critical in the extreme corners (rounded-corner clipping).
- Slightly elevated contrast and saturation — cards compete with page chrome.
- A coherent single idea; multi-focal busy scenes fail at card size.

## Aspect ratio

Match the card: `4:3` and `1:1` are most common; `3:4` for tall cards.

## Example prompts

Fintech / analytics product (soft 3D — plays to the style's centered gravity):

> Premium soft 3D render, square composition. A stylized abstract composition of smooth rounded shapes: floating matte glass panels arranged like ascending steps, a glowing amber sphere resting on the highest step, soft indigo and peach gradient shapes rising behind them like abstract towers. Gentle studio lighting, subtle ambient occlusion, dreamy depth of field. Palette of deep indigo, peach, amber, off-white. Futuristic, calm, high-end. Scene fills the frame with soft gradient atmosphere reaching every edge. No text, no letters, no logos.

Travel platform (photo):

> Vibrant travel photograph, 4:3 composition filled edge to edge. A turquoise cove seen from a cliffside terrace: a striped parasol and two empty loungers in the lower right, impossibly clear water with a small white boat centered in the middle distance, sun-bleached rocks framing the left edge. Bright Mediterranean light, punchy saturated grade with deep blue water and warm stone tones, crisp detail. Nothing important in the corners. No people's faces, no text, no letters, no logos.

## Pitfalls

- Don't use negative-space compositions — a half-empty image in a small card reads as a mistake.
- Check the corners of generated results; a subject touching a corner will be visibly amputated by the border radius.
- If the section background is a gradient, keep the image's palette adjacent to it (analogous hues) or the card will look pasted on.
