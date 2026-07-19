#!/usr/bin/env bash
# check-bare-goroutines — forbid bare 'go ' statements in production Go code.
#
# Rationale: bare goroutines spawned without panic recovery can silently crash
# workers. The repo provides internal/goroutine.Go for singleton background
# goroutines and sourcegraph/conc pools for fan-out work. Past production bugs
# traced directly to unrecovered panics in bare 'go func' calls.
#
# Exclusions:
#   * internal/goroutine/ — the implementation itself.
#   * *_test.go files — tests may use bare goroutines freely.
#   * scripts/goroutine-allowlist.txt — legitimate exceptions:
#       - channel-producer goroutines whose lifecycle is bounded by channel
#         close and whose callers propagate cancellation via ctx.Done();
#       - WaitGroup fan-out with inline panic recovery;
#       - drain goroutines that include their own inline recovery + wg.Done.
#     Each entry must have a comment explaining the justification.
#
# Portable bash 3.2+ (macOS default).

set -euo pipefail

ALLOWLIST="scripts/goroutine-allowlist.txt"

allowlist_tmp="$(mktemp)"
trap 'rm -f "$allowlist_tmp"' EXIT

if [[ -f "$ALLOWLIST" ]]; then
  # Strip comments and blank lines; keep "file:line" entries.
  sed -e 's/#.*//' -e 's/[[:space:]]*$//' -e '/^$/d' "$ALLOWLIST" > "$allowlist_tmp"
fi

violations_tmp="$(mktemp)"
trap 'rm -f "$allowlist_tmp" "$violations_tmp"' EXIT

while IFS= read -r -d '' file; do
  # Skip generated/vendored/test files and the goroutine package itself.
  case "$file" in
    ./vendor/*|./.ignored/*|./ansible/.secrets/*|./internal/goroutine/*)
      continue ;;
    *_test.go)
      continue ;;
  esac

  # Find lines that contain a bare 'go ' statement.
  # Match: optional whitespace, then 'go ' followed by a non-blank character
  # (function call or func literal start). This mirrors what the Go parser
  # considers a GoStmt and is conservative — no false positives from comments
  # because we only scan non-comment lines via grep.
  while IFS= read -r match; do
    lineno="${match%%:*}"
    entry="${file#./}:${lineno}"

    if grep -Fxq -- "$entry" "$allowlist_tmp" 2>/dev/null; then
      continue
    fi

    # Also accept a prefix match on the file alone (allowlist whole files).
    relfile="${file#./}"
    if grep -Fxq -- "$relfile" "$allowlist_tmp" 2>/dev/null; then
      continue
    fi

    code="${match#*:}"
    echo "  ${file#./}:${lineno}: bare goroutine: ${code}" >> "$violations_tmp"
  done < <(grep -n $'^\t* *go [a-zA-Z(]' "$file" 2>/dev/null || true)

done < <(find . -name "*.go" -type f -print0)

if [[ -s "$violations_tmp" ]]; then
  echo "::error::Bare 'go' statements found outside the allowlist:"
  cat "$violations_tmp"
  echo
  echo "Fix options:"
  echo "  1. Replace with goroutine.Go(ctx, func(ctx context.Context) { ... })"
  echo "     for singleton background goroutines (preferred)."
  echo "  2. Use sourcegraph/conc for fan-out worker pools."
  echo "  3. If the goroutine is a bounded channel producer or has its own"
  echo "     inline panic recovery, add 'file:line' to scripts/goroutine-allowlist.txt"
  echo "     with a comment justifying the exception."
  exit 1
fi

echo "✓ No bare goroutines outside the allowlist."
