#!/usr/bin/env bash
# deliver.sh — Upload every file in a directory to the org drive and print each
# file's public asset URL. This is how logo-pack deliverables reach the user.
#
# Usage:
#   bash deliver.sh <local_dir> <drive_subpath>
# Example:
#   bash deliver.sh /workspace/logo-pack logos/meadowlark
#
# Requires the sandbox-injected env vars HIVY_DRIVE_UPLOAD_URL and
# HIVY_DRIVE_UPLOAD_BEARER (present automatically in the agent sandbox).
#
# Output: one line per uploaded file — "<relative-path>  <asset_url>" — plus a
# final "delivered N file(s)" summary on stderr. Put the printed asset URLs in
# your reply so the user can see and download them.
set -euo pipefail

DIR="${1:?usage: deliver.sh <local_dir> <drive_subpath>}"
SUB="${2:?usage: deliver.sh <local_dir> <drive_subpath>}"
: "${HIVY_DRIVE_UPLOAD_URL:?missing HIVY_DRIVE_UPLOAD_URL (run inside the agent sandbox)}"
: "${HIVY_DRIVE_UPLOAD_BEARER:?missing HIVY_DRIVE_UPLOAD_BEARER (run inside the agent sandbox)}"

[ -d "$DIR" ] || { echo "not a directory: $DIR" >&2; exit 1; }
SUB="${SUB#/}"; SUB="${SUB%/}"
ROOT="${HIVY_DRIVE_UPLOAD_URL%/}"

count=0
while IFS= read -r -d '' f; do
  rel="${f#"$DIR"/}"
  if [ ! -s "$f" ]; then echo "skip empty: $rel" >&2; continue; fi
  ct="$(file -b --mime-type "$f" 2>/dev/null || echo application/octet-stream)"
  resp="$(curl -fsS --retry 3 --retry-all-errors --connect-timeout 10 --max-time 300 -X PUT \
    -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
    -H "Content-Type: $ct" \
    --upload-file "$f" \
    "$ROOT/$SUB/$rel")"
  # Prefer jq; fall back to sed if jq is unavailable.
  asset="$(printf '%s' "$resp" | jq -r '.asset_url // empty' 2>/dev/null || true)"
  [ -n "$asset" ] || asset="$(printf '%s' "$resp" | sed -n 's/.*"asset_url":"\([^"]*\)".*/\1/p')"
  if [ -z "$asset" ]; then echo "upload failed for $rel: $resp" >&2; exit 1; fi
  echo "$rel  $asset"
  count=$((count + 1))
done < <(find "$DIR" -type f -print0 | sort -z)

echo "delivered $count file(s)" >&2
