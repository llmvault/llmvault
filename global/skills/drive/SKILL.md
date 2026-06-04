---
name: drive
description: Use whenever you need to save a file produced in the sandbox to the employee drive. The Hivy runtime provides HIVY_DRIVE_UPLOAD_URL and HIVY_DRIVE_UPLOAD_BEARER. Write generated images, videos, audio, screenshots, charts, CSV/Excel exports, PDFs, zip bundles, reports, and PR/demo artifacts here, then share the returned file URL.
---

# Drive

Write files to the employee drive using `HIVY_DRIVE_UPLOAD_URL` and `HIVY_DRIVE_UPLOAD_BEARER`.

You are running inside the Hivy runtime. Use this Hivy-provided drive endpoint exactly as provided; it is used for security, access control, and tracking. Append a descriptive filename or relative path so humans can find the file later in the employee drive.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `HIVY_DRIVE_UPLOAD_URL` | Employee drive upload root |
| `HIVY_DRIVE_UPLOAD_BEARER` | Bearer token for writing to the employee drive |

```bash
test -n "$HIVY_DRIVE_UPLOAD_URL" || { echo "HIVY_DRIVE_UPLOAD_URL is not set" >&2; exit 1; }
test -n "$HIVY_DRIVE_UPLOAD_BEARER" || { echo "HIVY_DRIVE_UPLOAD_BEARER is not set" >&2; exit 1; }
```

## Upload command

```bash
url=$(
  curl -fsS -X PUT \
    -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
    -H "Content-Type: $(file -b --mime-type ./output.png)" \
    --upload-file ./output.png \
    "$HIVY_DRIVE_UPLOAD_URL/output.png" \
  | jq -r .asset_url
)
printf '%s\n' "$url"
```

For organized files, append a relative path below the employee drive root:

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -H "Content-Type: text/csv" \
  --upload-file ./metrics.csv \
  "$HIVY_DRIVE_UPLOAD_URL/artifacts/metrics.csv"
```

## Response

Successful uploads return `201 Created` with JSON containing the returned URL field:

```json
{
  "id": "...",
  "asset_url": "https://...",
  "key": "pub/e/.../output.png",
  "path": "...",
  "filename": "output.png",
  "content_type": "image/png",
  "bytes": 41284
}
```

Use the returned file URL in replies, PR descriptions, reports, or handoff notes.

## Rules

- Use `HIVY_DRIVE_UPLOAD_URL` exactly as provided.
- Use one PUT request per file.
- Use `--upload-file` for binary and large files.
- Use descriptive paths such as `reports/2026-06/revenue-summary.pdf` or `screenshots/linear/eng-123-before.png`.
- Keep path components URL-safe: lowercase letters, numbers, `-`, `_`, `.`, and `/`.
- Do not upload secrets.
