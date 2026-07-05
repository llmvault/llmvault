# Hivy email templates

React Email sources for Hivy transactional emails. They render to **static HTML
and plaintext files containing `{{{placeholder}}}` tokens** which the Go backend
(`internal/email`) embeds and substitutes at send time. Nothing here talks to
Resend or needs an API key.

## Layout

- `emails/*.tsx` — the 7 email templates.
- `emails/static/hivy-logo.png` — logo asset (served by the backend under `{{{assetBaseUrl}}}`).
- `lib/hivy-email.tsx` — shared shell (logo, footer, brand styles). `siteUrl` and
  the asset base URL are the placeholder strings `{{{siteUrl}}}` / `{{{assetBaseUrl}}}`.
- `registry.ts` — maps each template to placeholder props (`{{{key}}}`).
- `templates.ts` — alias / subject / variables metadata.
- `scripts/build-templates.ts` — renders every template to `dist/`.
- `dist/` — generated output (committed; embedded by Go):
  - `<alias>.html` and `<alias>.txt` for each of the 7 templates.
  - `manifest.json` — `[{ alias, subject, variables[] }]`.

## Build

```sh
pnpm install   # first time only
pnpm run build # regenerates everything under dist/
```

`pnpm run build` runs `tsx scripts/build-templates.ts`, which calls
`render(createElement(component, placeholderProps))` (and `render(..., { plainText: true })`)
for each entry in `registry.ts`, wiping and rewriting `dist/`.

## Editing / preview

```sh
pnpm run dev   # React Email dev server on http://localhost:30113
```

Edit the `.tsx` sources, then re-run `pnpm run build` to regenerate `dist/`.

## Placeholders

Every template's HTML/text contains its own variables (e.g. `{{{code}}}`,
`{{{firstName}}}`) plus `{{{siteUrl}}}` and `{{{assetBaseUrl}}}` from the shell.
The exact variable set per template lives in `dist/manifest.json`.
