#!/usr/bin/env bash
# check-ts-file-length — enforce a soft line-count ceiling on TypeScript/TSX files
# in the web application.
#
# Rationale: mirrors the Go check (scripts/check-go-file-length.sh). Files longer
# than ~600 lines are hard to review and usually signal a missing seam.
#
# Exclusions live in two places:
#   * EXCLUDE_REGEX — generated, vendored, or external code. Skipped entirely.
#   * scripts/ts-file-length-allowlist.txt — hand-written files already over the
#     limit when the rule was introduced. Grandfathered in so CI isn't red from
#     day one. Shrink these over time; removing an entry is always OK.
#
# New TypeScript files are held to the limit. If you genuinely need to exceed it:
#   1. Split the file into smaller, focused ones (preferred).
#   2. If the file is generated, extend EXCLUDE_REGEX in this script.
#   3. If it's a legitimate grandfathered case, add the path to
#      scripts/ts-file-length-allowlist.txt with a comment explaining why.
#
# Portable bash 3.2+ (macOS default).

set -euo pipefail

MAX_LINES="${MAX_TS_FILE_LINES:-600}"
ALLOWLIST="scripts/ts-file-length-allowlist.txt"
SEARCH_ROOT="${TS_FILE_LENGTH_ROOT:-apps/web}"

# Generated / vendored / external code — skipped entirely.
EXCLUDE_REGEX='(/node_modules/|/\.next/|/dist/|/build/|\.d\.ts$|_generated\.|\.generated\.)'

# Extract just the path entries from the allowlist (strip comments & blanks).
allowlist_tmp="$(mktemp)"
trap 'rm -f "$allowlist_tmp"' EXIT

if [[ -f "$ALLOWLIST" ]]; then
  sed -e 's/#.*//' -e 's/[[:space:]]*$//' -e '/^$/d' "$ALLOWLIST" > "$allowlist_tmp"
fi

violations_tmp="$(mktemp)"
seen_tmp="$(mktemp)"
trap 'rm -f "$allowlist_tmp" "$violations_tmp" "$seen_tmp"' EXIT

while IFS= read -r -d '' file; do
  if [[ "$file" =~ $EXCLUDE_REGEX ]]; then
    continue
  fi

  lines=$(wc -l < "$file" | tr -d ' ')
  (( lines > MAX_LINES )) || continue

  # Strip leading ./ for allowlist comparison.
  rel="${file#./}"

  if grep -Fxq -- "$rel" "$allowlist_tmp"; then
    echo "$rel" >> "$seen_tmp"
    continue
  fi
  echo "$rel: $lines lines (max $MAX_LINES)" >> "$violations_tmp"
done < <(find "./$SEARCH_ROOT" \( -name "*.ts" -o -name "*.tsx" \) -type f -print0)

if [[ -s "$violations_tmp" ]]; then
  echo "::error::TypeScript files exceeding $MAX_LINES lines (not on allowlist):"
  sed 's/^/  /' "$violations_tmp"
  echo
  echo "Fix options:"
  echo "  1. Split the file into smaller, focused ones (preferred)."
  echo "  2. If the file is generated, extend EXCLUDE_REGEX in this script."
  echo "  3. If it's a legitimate grandfathered case, add the path to"
  echo "     $ALLOWLIST with a comment explaining why."
  exit 1
fi

# Warn about allowlist entries that are stale (file shrunk or was deleted).
# Informational only — doesn't fail CI.
if [[ -s "$allowlist_tmp" ]]; then
  stale="$(comm -23 <(sort -u "$allowlist_tmp") <(sort -u "$seen_tmp" 2>/dev/null || true) || true)"
  if [[ -n "$stale" ]]; then
    echo "Note: $ALLOWLIST has entries no longer over the limit — please remove:"
    echo "$stale" | sed 's/^/  /'
  fi
fi

echo "✓ All TypeScript files under $MAX_LINES lines (or explicitly allowlisted)."
