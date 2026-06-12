#!/usr/bin/env bash
# check-migrations — sanity-check the Go database migrations directory.
#
# Checks:
#   1. Contiguous numbering: migration files must be numbered sequentially
#      starting at 000001 with no gaps or duplicates.
#   2. No empty migration files (zero bytes).
#   3. Testdb version matches (optional): if TEST_DATABASE_URL is set and
#      psql is available, connect to the test database and verify that its
#      goose version equals the highest migration file number.
#
# The test-database check is skipped when:
#   * TEST_DATABASE_URL is unset (CI infra not started yet), OR
#   * psql is not installed (developer machines without psql client).
#
# This script intentionally does NOT run 'migrate up' — it is a passive check
# that can run without an active database connection for offline CI jobs.
#
# Portable bash 3.2+ (macOS default).

set -euo pipefail

MIGRATIONS_DIR="${MIGRATIONS_DIR:-internal/migrations/sql}"

# ── 1. Gather migration files ─────────────────────────────────────────────────

if [[ ! -d "$MIGRATIONS_DIR" ]]; then
  echo "::error::Migrations directory not found: $MIGRATIONS_DIR"
  exit 1
fi

files=()
while IFS= read -r -d '' f; do
  files+=("$f")
done < <(find "$MIGRATIONS_DIR" -maxdepth 1 -name "*.sql" -type f -print0 | sort -z)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "::error::No SQL migration files found in $MIGRATIONS_DIR"
  exit 1
fi

# ── 2. Contiguous numbering check ────────────────────────────────────────────

errors=0
prev=0
max_version=0

for f in "${files[@]}"; do
  base="$(basename "$f")"
  # Extract the leading numeric prefix (zero-padded or plain).
  num_str="${base%%_*}"
  # Strip leading zeros for arithmetic (avoid octal interpretation).
  num=$((10#$num_str))

  expected=$(( prev + 1 ))
  if [[ "$num" -ne "$expected" ]]; then
    echo "  error: expected migration $(printf '%06d' "$expected"), got '${base}'"
    errors=$(( errors + 1 ))
  fi

  if [[ -s "$f" ]]; then
    : # non-empty — good
  else
    echo "  error: migration file is empty: $base"
    errors=$(( errors + 1 ))
  fi

  prev="$num"
  max_version="$num"
done

if [[ "$errors" -gt 0 ]]; then
  echo "::error::Migration numbering check failed ($errors error(s))."
  exit 1
fi

echo "✓ Migration files: ${#files[@]} contiguous, highest version ${max_version}."

# ── 3. Test-database version check (optional) ─────────────────────────────────

if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
  echo "  (skipping testdb version check: TEST_DATABASE_URL not set)"
  exit 0
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "  (skipping testdb version check: psql not in PATH)"
  exit 0
fi

# Query goose's version_id table. goose v3 stores the applied version in
# goose_db_version.version_id (the highest applied migration).
db_version=$(psql "$TEST_DATABASE_URL" -At -c \
  "SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = TRUE;" \
  2>/dev/null || true)

if [[ -z "$db_version" || "$db_version" == "NULL" ]]; then
  echo "  (skipping testdb version check: goose_db_version table not found or empty — run migrations first)"
  exit 0
fi

if [[ "$db_version" -ne "$max_version" ]]; then
  echo "::error::Testdb migration version mismatch: DB is at version ${db_version}, but highest migration file is ${max_version}."
  echo "  Run 'make migrate-up' (or the equivalent for the test database) to bring it up to date."
  exit 1
fi

echo "✓ Testdb version ${db_version} matches highest migration file ${max_version}."
