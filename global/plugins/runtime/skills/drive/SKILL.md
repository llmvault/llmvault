---
name: drive
description: Use when saving, sharing, attaching, uploading, or linking generated files, images, videos, screenshots, charts, exports, PDFs, archives, reports, or demo artifacts.
---

# Drive

Write files to the agent drive using `HIVY_DRIVE_UPLOAD_URL` and `HIVY_DRIVE_UPLOAD_BEARER`.

Do not invent an upload endpoint or expose local sandbox paths to teammates. Use the provided drive endpoint and environment variables exactly as provided. Append a descriptive filename or relative path so humans can find the file later in the agent drive.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `HIVY_DRIVE_UPLOAD_URL` | Agent drive upload root |
| `HIVY_DRIVE_UPLOAD_BEARER` | Bearer token for writing to the agent drive |

```bash
test -n "$HIVY_DRIVE_UPLOAD_URL" || { echo "HIVY_DRIVE_UPLOAD_URL is not set" >&2; exit 1; }
test -n "$HIVY_DRIVE_UPLOAD_BEARER" || { echo "HIVY_DRIVE_UPLOAD_BEARER is not set" >&2; exit 1; }
```

## Upload command

Validate the file before uploading. Never upload an empty file.

```bash
file_path="./output.png"
test -s "$file_path" || { echo "Refusing to upload empty or missing file: $file_path" >&2; exit 1; }
content_type="$(file -b --mime-type "$file_path")"
bytes="$(wc -c < "$file_path" | tr -d ' ')"
printf 'Uploading %s (%s bytes, %s)\n' "$file_path" "$bytes" "$content_type" >&2
```

```bash
url=$(
  curl -fsS --retry 3 --retry-all-errors --connect-timeout 10 --max-time 300 -X PUT \
    -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
    -H "Content-Type: $content_type" \
    --upload-file "$file_path" \
    "$HIVY_DRIVE_UPLOAD_URL/output.png" \
  | jq -r 'if (.bytes // 0) <= 0 then error("drive upload returned zero bytes") else .asset_url end'
)
printf '%s\n' "$url"
```

For organized files, append a relative path below the agent drive root:

```bash
curl -fsS --retry 3 --retry-all-errors --connect-timeout 10 --max-time 300 -X PUT \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -H "Content-Type: text/csv" \
  --upload-file ./metrics.csv \
  "$HIVY_DRIVE_UPLOAD_URL/artifacts/metrics.csv"
```

## Download then upload

When downloading a file from the web before uploading it, follow redirects and verify the downloaded file before uploading. Many image hosts redirect to the actual image.

```bash
curl -fL --retry 3 --retry-all-errors --connect-timeout 10 --max-time 120 \
  -o ./portrait-900x1200.jpg \
  "https://picsum.photos/900/1200"

test -s ./portrait-900x1200.jpg || { echo "Downloaded file is empty" >&2; exit 1; }
file -b --mime-type ./portrait-900x1200.jpg | grep -q '^image/' || {
  echo "Downloaded file is not an image" >&2
  file ./portrait-900x1200.jpg >&2
  exit 1
}

curl -fsS --retry 3 --retry-all-errors --connect-timeout 10 --max-time 300 -X PUT \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -H "Content-Type: $(file -b --mime-type ./portrait-900x1200.jpg)" \
  --upload-file ./portrait-900x1200.jpg \
  "$HIVY_DRIVE_UPLOAD_URL/drive-test/large/portrait-900x1200.jpg" \
  | jq -r 'if (.bytes // 0) <= 0 then error("drive upload returned zero bytes") else .asset_url end'
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
- Use `curl -fL` when downloading files from URLs before upload.
- Check `test -s <file>` and MIME type before uploading.
- Check the upload response has `.bytes > 0` before reporting success.
- Use descriptive paths such as `reports/2026-06/revenue-summary.pdf` or `screenshots/linear/eng-123-before.png`.
- Keep path components URL-safe: lowercase letters, numbers, `-`, `_`, `.`, and `/`.
- Do not upload secrets.
