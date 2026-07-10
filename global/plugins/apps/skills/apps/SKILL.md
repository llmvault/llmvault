---
name: apps
description: Use to build, preview, publish, debug, or roll back a Hivy app: a template-based Go API and React SPA bound to one sheet.
---

# Hivy Apps

An app is bound permanently to one sheet. It reads that sheet's structure and CRUDs rows; schema changes remain Sheets work. The app template owns authentication, sessions, platform configuration, logging, and static serving.

## Non-negotiable boundaries

- Create the app with its sheet before previewing. Describe the sheet and use its stable `fld_…` IDs; never invent field IDs.
- Edit `api/` and `web/` only. Never alter `hivycore/` or `template_version` in `app.json`.
- Handlers access data only through `app.Sheets()` with request context. The SPA calls same-origin `/api/*` only. Do not place secrets, login/session logic, or third-party browser calls in `web/`.
- `app_publish` is a production deployment. It requires explicit authorization for every publish or republish.
- Never read, print, or paste the preview environment or channel-secret values.

## Build workflow

1. Use Sheets tools to find or create the required sheet, then describe its pages, fields, and representative rows.
2. Call `app_create` early and retain `app_id` and `slug`.
3. Fetch the template, unpack it at `/workspace/apps/<slug>/`, and read its `README.md`; it is the implementation reference.

```bash
template_url="${HIVY_DRIVE_UPLOAD_URL%/drive}/apps-template.zip"
curl -fsS --retry 3 -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -o /tmp/apps-template.zip "$template_url"
mkdir -p "/workspace/apps/$SLUG"
unzip -q /tmp/apps-template.zip -d "/workspace/apps/$SLUG"
```

4. Implement the smallest complete API and UI in the template's patterns. Preserve its error handling, routing, query infrastructure, and authentication behavior.
5. Build before previewing. The template README and Makefile are authoritative for dependencies, build, test, and artifact commands.
6. Preview with:

```bash
make preview APP_ID=<app_id> PORT=3000
```

Read and share only the final hinted URL printed by the command, including `?app=<app_id>`. Verify its `/healthz` and `/` endpoints. The preview expires with the builder session; a direct browser visit can correctly show the sign-in wall.

7. After explicit deployment approval, build the source and bundle archives, calculate both SHA-256 hashes, upload both through the Drive endpoint, and call `app_publish` with fresh keys, hashes, and version notes.
8. Verify every publish with `app_status`, `<url>/healthz`, and a small `app_logs` read. Share the returned hinted live URL only after those pass. Roll back to the last good `version_id` first when a deployment regresses.

## Handoff

For a preview, give the hinted URL, what to test, and its temporary lifetime. For a publish, give the live URL, bound sheet/pages, version notes, verification result, and assumptions the user may want changed.
