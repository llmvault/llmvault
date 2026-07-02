# Device-frame hero

The hero's visual is content displayed inside a device or browser frame — a floating laptop, phone, tablet, or browser window mockup beside or below the copy. Standard for SaaS products where the product IS the screen. The generated image is the *content inside the frame*, or occasionally the full framed mockup.

## Two variants — decide first

**A. Real product screenshot in a CSS/SVG frame (preferred).** If an actual product screenshot exists, use it — never generate a fake UI for a real product. The image model's job is then only the *backdrop* behind the floating device (see `abstract-gradient` or `dark-cinematic-glow`).

**B. Generated scene-as-content.** For prototypes, concepts, or non-UI content (a map, a photo feed, a video still), generate the content image to place inside the frame.

## What the layout does to the image (variant B)

- The image is displayed inside a frame at roughly 16:10 (browser/laptop) or 9:19.5 (phone), often at small size — detail must survive shrinking.
- The frame adds its own chrome; the content should NOT include any device edges, bezels, or browser bars.
- Perspective-tilted mockups will skew the image; centered compositions survive skew best.

## Image requirements

- Straight-on, flat perspective — no angle, no vanishing point (the mockup transform adds that).
- Edge-to-edge content, no borders or margins.
- Bold, simple composition readable at thumbnail size.
- If it must look like an app: use the product-UI-vignette recipe (`references/product-ui-vignette.md`) — up to ~6 short verbatim strings are fine in that flat style; anything denser should be abstract dashboard-like *shapes* (tiles, charts as colored blocks).

## Aspect ratio

`16:9` for browser/laptop content (crop to 16:10 in CSS), `9:16` for phone content.

## Example prompts

Map/logistics app content (phone frame):

> Clean stylized map illustration, tall 9:16 composition filling the frame edge to edge with no border. A simplified city street grid in soft off-white and light grey, a bold rounded route line in vivid blue sweeping from the bottom third to a location pin in the upper third, small rounded building shapes in pale pastels, a few tree clusters in soft green. Flat design, generous scale, high contrast between the route and the background. No text, no letters, no labels, no UI elements.

Photography platform content (browser frame):

> Bright editorial photograph, wide composition filling the frame completely, shot straight-on with flat perspective. A warm flat-lay of a creative workspace: camera, printed photographs spread in a loose grid, film rolls, a ceramic coffee cup on pale oak wood, soft even daylight, gentle shadows. Rich but airy palette of cream, tan, and muted teal accents. Crisp detail that stays readable when small. No text, no letters, no logos, no screens.

## Pitfalls

- Never generate the device itself when the page already draws a frame — double bezels look absurd. Decide variant A/B first.
- Dense real-looking UI = garbled text. Either follow the vignette text rules (few short quoted strings, glyph review), abstract the interface into shapes and blocks, or use a real screenshot.
- High-detail scenes turn to noise at mockup scale; err toward bold and simple.
