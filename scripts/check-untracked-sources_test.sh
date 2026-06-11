#!/usr/bin/env bash
# Regression test for check-untracked-sources.sh.
#
# Builds throwaway git repositories and asserts the guard's exit code and
# output for each case:
#   1. clean tree (all sources tracked)            -> pass (exit 0)
#   2. untracked non-test .go file                 -> fail (exit 1)
#   3. untracked migration .sql                     -> fail (exit 1)
#   4. untracked *_test.go only                     -> pass (exit 0) + warning
#   5. STRICT_TEST_SOURCES=1 with untracked test    -> fail (exit 1)
#   6. .gitignored .go file                         -> pass (exit 0)
#   7. untracked file under vendor/                 -> pass (exit 0)
#
# Portable bash 3.2+ (macOS default). Run: ./scripts/check-untracked-sources_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/check-untracked-sources.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "FAIL: guard script not executable: $GUARD"
  exit 1
fi

fails=0

# new_repo — create an empty git repo with one committed file so the index is
# populated, then echo its path.
new_repo() {
  local dir
  dir="$(mktemp -d)"
  git -C "$dir" init -q
  git -C "$dir" config user.email t@t.t
  git -C "$dir" config user.name t
  echo "module example.com/x" > "$dir/go.mod"
  git -C "$dir" add go.mod
  git -C "$dir" commit -q -m init
  echo "$dir"
}

# run_guard <dir> [env...] — run the guard inside <dir>, capture exit code in
# RC and output in OUT.
run_guard() {
  local dir="$1"; shift
  set +e
  OUT="$(cd "$dir" && env "$@" "$GUARD" 2>&1)"
  RC=$?
  set -e
}

assert_rc() {
  local want="$1" got="$2" name="$3"
  if [[ "$got" -ne "$want" ]]; then
    echo "FAIL [$name]: expected exit $want, got $got"
    echo "----- output -----"; echo "$OUT"; echo "------------------"
    fails=$(( fails + 1 ))
  else
    echo "ok   [$name]"
  fi
}

assert_contains() {
  local needle="$1" name="$2"
  case "$OUT" in
    *"$needle"*) : ;;
    *)
      echo "FAIL [$name]: output missing expected text: $needle"
      echo "----- output -----"; echo "$OUT"; echo "------------------"
      fails=$(( fails + 1 )) ;;
  esac
}

# 1. clean tree
d="$(new_repo)"
mkdir -p "$d/internal/foo"
echo "package foo" > "$d/internal/foo/foo.go"
git -C "$d" add internal/foo/foo.go
git -C "$d" commit -q -m foo
run_guard "$d"
assert_rc 0 "$RC" "clean tree passes"

# 2. untracked non-test .go file
d="$(new_repo)"
mkdir -p "$d/internal/foo"
echo "package foo" > "$d/internal/foo/bar.go"
run_guard "$d"
assert_rc 1 "$RC" "untracked .go fails"
assert_contains "internal/foo/bar.go" "untracked .go reported"

# 3. untracked migration .sql
d="$(new_repo)"
mkdir -p "$d/internal/migrations/sql"
echo "-- up" > "$d/internal/migrations/sql/000034_x.sql"
run_guard "$d"
assert_rc 1 "$RC" "untracked migration fails"
assert_contains "000034_x.sql" "untracked migration reported"

# 4. untracked *_test.go only -> pass + warning
d="$(new_repo)"
mkdir -p "$d/internal/foo"
echo "package foo" > "$d/internal/foo/foo_test.go"
run_guard "$d"
assert_rc 0 "$RC" "untracked test-only passes"
assert_contains "Untracked test source files" "test-only warning emitted"

# 5. STRICT_TEST_SOURCES=1 makes untracked test fatal
d="$(new_repo)"
mkdir -p "$d/internal/foo"
echo "package foo" > "$d/internal/foo/foo_test.go"
run_guard "$d" STRICT_TEST_SOURCES=1
assert_rc 1 "$RC" "strict mode fails on untracked test"

# 6. .gitignored .go file is not "untracked"
d="$(new_repo)"
mkdir -p "$d/internal/foo"
echo "internal/foo/gen.go" > "$d/.gitignore"
git -C "$d" add .gitignore
git -C "$d" commit -q -m ignore
echo "package foo" > "$d/internal/foo/gen.go"
run_guard "$d"
assert_rc 0 "$RC" "gitignored .go passes"

# 7. vendored untracked file is excluded
d="$(new_repo)"
mkdir -p "$d/vendor/x"
echo "package x" > "$d/vendor/x/x.go"
run_guard "$d"
assert_rc 0 "$RC" "vendored untracked passes"

if [[ "$fails" -gt 0 ]]; then
  echo
  echo "$fails check(s) FAILED"
  exit 1
fi
echo
echo "All checks passed."
