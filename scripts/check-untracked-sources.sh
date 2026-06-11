#!/usr/bin/env bash
# check-untracked-sources — fail when build-required source files are untracked.
#
# Rationale: Go compiles every *.go file in a package directory regardless of
# whether git tracks it, so a local working tree can build perfectly while the
# committed HEAD does not stand alone. A fresh clone of such a HEAD fails to
# compile because the symbols defined in the untracked files are missing. The
# same hazard applies to migration SQL files referenced by contracts.
#
# This guard makes that failure mode loud and local: it lists files that the
# build (or migrations) require but git does not track, and exits non-zero so
# they get committed before the tree is pushed.
#
# Scope (build-required, non-optional):
#   * **/*.go            — excluding *_test.go (tests are required for `go test`
#                          but a missing test file does not break a fresh build;
#                          we still report them as a warning, see below).
#   * internal/migrations/sql/*.sql — migrations are load-bearing for runtime
#                          correctness and are referenced by contract docs.
#
# Untracked test files are reported as a non-fatal warning: a fresh clone still
# builds, but `go test ./...` would miss them. Set STRICT_TEST_SOURCES=1 to make
# untracked test files fatal too.
#
# Honors .gitignore: an intentionally-ignored file is never reported (it is not
# "untracked" in the sense git would surface with `status`).
#
# Portable bash 3.2+ (macOS default).

set -euo pipefail

if ! command -v git >/dev/null 2>&1; then
  echo "  (skipping untracked-source check: git not in PATH)"
  exit 0
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "  (skipping untracked-source check: not inside a git work tree)"
  exit 0
fi

# Untracked, non-ignored files (NUL-delimited, paths relative to repo root).
untracked=()
while IFS= read -r -d '' f; do
  untracked+=("$f")
done < <(git ls-files --others --exclude-standard -z)

fatal_tmp="$(mktemp)"
warn_tmp="$(mktemp)"
trap 'rm -f "$fatal_tmp" "$warn_tmp"' EXIT

for f in "${untracked[@]}"; do
  case "$f" in
    vendor/*|.ignored/*)
      continue ;;
    *_test.go)
      echo "  $f" >> "$warn_tmp" ;;
    *.go)
      echo "  $f" >> "$fatal_tmp" ;;
    internal/migrations/sql/*.sql)
      echo "  $f" >> "$fatal_tmp" ;;
  esac
done

strict_test="${STRICT_TEST_SOURCES:-0}"

if [[ "$strict_test" == "1" && -s "$warn_tmp" ]]; then
  cat "$warn_tmp" >> "$fatal_tmp"
  : > "$warn_tmp"
fi

if [[ -s "$warn_tmp" ]]; then
  echo "::warning::Untracked test source files (a fresh clone builds, but 'go test ./...' would skip these):"
  cat "$warn_tmp"
  echo
fi

if [[ -s "$fatal_tmp" ]]; then
  echo "::error::Build-required source files are untracked. The committed HEAD will not"
  echo "compile from a fresh clone because these files define symbols the tree references:"
  cat "$fatal_tmp"
  echo
  echo "Fix: 'git add' the files above (and commit them) so the HEAD stands alone."
  echo "If a file is intentionally excluded, add it to .gitignore so it is not 'untracked'."
  exit 1
fi

echo "✓ No build-required source files are untracked."
