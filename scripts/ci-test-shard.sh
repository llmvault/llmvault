#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

suite="${1:?suite is required}"
shard_index="${SHARD_INDEX:-${CI_NODE_INDEX:-0}}"
shard_total="${SHARD_TOTAL:-${CI_NODE_TOTAL:-1}}"
timeout="${GO_TEST_TIMEOUT:-5m}"

test_env=(
  "DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "HIVY_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "TEST_DATABASE_URL=postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
  "HIVY_NANGO_DATABASE_URL=postgres://hivy:localdev@localhost:5433/nango?sslmode=disable"
  "HIVY_REDIS_ADDR=localhost:16279"
  "TEST_REDIS_ADDR=localhost:16279"
  "HIVY_NANGO_ENDPOINT=http://localhost:23003"
  "HIVY_QDRANT_HOST=localhost"
  "HIVY_QDRANT_PORT=6334"
  "HIVY_QDRANT_USE_TLS=false"
  "HIVY_AWS_ENDPOINT_URL=http://localhost:9000"
)

shard_lines() {
  awk -v s="$shard_index" -v n="$shard_total" 'NR % n == s'
}

run_packages() {
  local packages="$1"
  shift || true
  if [[ -z "$packages" ]]; then
    echo "No packages assigned to shard $shard_index/$shard_total"
    return 0
  fi
  echo "$packages"
  printf '%s\n' "$packages" | xargs env "${test_env[@]}" go test -count=1 -timeout="$timeout" "$@"
}

run_test_names() {
  local package="$1"
  local tests
  tests="$(env "${test_env[@]}" go test "$package" -list '^Test' | awk '/^Test/ {print}' | shard_lines)"
  if [[ -z "$tests" ]]; then
    echo "No tests assigned to shard $shard_index/$shard_total for $package"
    return 0
  fi
  local pattern
  pattern="$(printf '%s\n' "$tests" | paste -sd'|' -)"
  echo "$tests"
  env "${test_env[@]}" go test "$package" -count=1 -timeout="$timeout" -run "^(${pattern})$"
}

internal_packages() {
  go list ./internal/...
}

packages_with_tests() {
  local pattern="$1"
  local pkg dir files
  go list -f '{{.ImportPath}} {{.Dir}}' "$pattern" | while read -r pkg dir; do
    files=("$dir"/*_test.go)
    if ((${#files[@]} > 0)) &&
      grep -Eq '^[[:space:]]*func[[:space:]]+Test[A-Za-z0-9_]*[[:space:]]*\(' "${files[@]}"; then
      echo "$pkg"
    fi
  done
}

internal_test_packages() {
  packages_with_tests ./internal/...
}

cmd_test_packages() {
  packages_with_tests ./cmd/...
}

internal_core_packages() {
  internal_test_packages | grep -Ev 'internal/(handler|integrations|nango|rag|storage|tasks)(/|$)'
}

select_internal_core_packages() {
  local all suffix
  all="$(internal_core_packages)"
  for suffix in "$@"; do
    printf '%s\n' "$all" | awk -v suffix="/${suffix}" '$0 ~ suffix "$"'
  done
}

internal_core_shard_packages() {
  if [[ "$shard_total" != "12" ]]; then
    internal_core_packages | shard_lines
    return
  fi

  case "$shard_index" in
    0) select_internal_core_packages auth middleware credentials crypto ;;
    1) select_internal_core_packages billing billing/purchase ;;
    2) select_internal_core_packages billing/fake billing/paystack ;;
    3) select_internal_core_packages bootstrap cache config goroutine system system/tasks logging onboarding ;;
    4) select_internal_core_packages bridge bridgeevents proxy streaming slackapp slackworkflow providerheaders ;;
    5) select_internal_core_packages agentruntime agentprompts sandboxruntime runtimestream ;;
    6) select_internal_core_packages sandbox sandbox/daytona sandbox/docker ;;
    7) select_internal_core_packages connectionname mcp mcp/catalog mcpserver mcpservers skills resources providergroups ;;
    8) select_internal_core_packages agents agentsandbox apps ;;
    9) select_internal_core_packages model registry db migrations testdb counter memory ;;
    10) select_internal_core_packages trigger/dispatch trigger/enrichment trigger/hivy spider firecrawl serper webcrawl enqueue email ;;
    11) select_internal_core_packages evals observability/sentry observe sheets ;;
    *)
      echo "invalid internal-core shard index: $shard_index" >&2
      exit 2
      ;;
  esac
}

internal_extra_packages() {
  printf '%s\n' \
    github.com/usehivy/hivy/internal/agentcatalog \
    github.com/usehivy/hivy/internal/agentrouter \
    github.com/usehivy/hivy/internal/agentschedule \
    github.com/usehivy/hivy/internal/access \
    github.com/usehivy/hivy/internal/automationcatalog \
    github.com/usehivy/hivy/internal/canvasartifact \
    github.com/usehivy/hivy/internal/channelagents \
    github.com/usehivy/hivy/internal/connectionaccess \
    github.com/usehivy/hivy/internal/databaseintegration \
    github.com/usehivy/hivy/internal/keyedlock \
    github.com/usehivy/hivy/internal/microsandbox/api \
    github.com/usehivy/hivy/internal/microsandbox/control \
    github.com/usehivy/hivy/internal/microsandbox/runner \
    github.com/usehivy/hivy/internal/microsandbox/security \
    github.com/usehivy/hivy/internal/netguard \
	github.com/usehivy/hivy/internal/skillaccess \
    github.com/usehivy/hivy/internal/precontext \
    github.com/usehivy/hivy/internal/quiver \
    github.com/usehivy/hivy/internal/railway \
    github.com/usehivy/hivy/internal/reve \
    github.com/usehivy/hivy/internal/sandbox/microsandbox \
    github.com/usehivy/hivy/internal/sandbox/railway \
    github.com/usehivy/hivy/internal/teamprovision \
    github.com/usehivy/hivy/internal/token \
    github.com/usehivy/hivy/internal/transcription
}

# coverage_check_selected prints every package that some CI shard/suite runs.
# It MUST stay in sync with the suite dispatch below: internal-core shards
# 0-11 (forced to shard_total=12), plus the named suites (handler, rag, tasks,
# integrations+nango, storage) and the internal-extra list.
coverage_check_selected() {
  local saved_index="$shard_index" saved_total="$shard_total" i
  shard_total=12
  for i in $(seq 0 11); do
    shard_index="$i"
    internal_core_shard_packages
  done
  shard_index="$saved_index"
  shard_total="$saved_total"

  echo github.com/usehivy/hivy/internal/handler
  internal_test_packages | grep -E 'internal/rag(/|$)'
  echo github.com/usehivy/hivy/internal/tasks
  printf '%s\n' \
    github.com/usehivy/hivy/internal/integrations \
    github.com/usehivy/hivy/internal/nango
  echo github.com/usehivy/hivy/internal/storage
  internal_extra_packages
}

coverage_check() {
  local selected missing
  selected="$(coverage_check_selected | sort -u)"
  missing="$(comm -23 <(internal_test_packages | sort -u) <(printf '%s\n' "$selected"))"
  if [[ -n "$missing" ]]; then
    echo "coverage-check FAILED: packages contain *_test.go but are not run by any CI shard/suite:" >&2
    printf '%s\n' "$missing" >&2
    echo >&2
    echo "Add each package to a shard in internal_core_shard_packages() or to internal_extra_packages() in scripts/ci-test-shard.sh." >&2
    return 1
  fi
  echo "coverage-check OK: all $(internal_test_packages | wc -l | tr -d ' ') internal test packages are covered by a CI shard/suite."
}

case "$suite" in
  coverage-check)
    coverage_check
    ;;
  internal-core)
    run_packages "$(internal_core_shard_packages)"
    ;;
  internal-handler)
    run_test_names ./internal/handler
    ;;
  internal-rag)
    run_packages "$(internal_test_packages | grep -E 'internal/rag(/|$)' | shard_lines)"
    ;;
  internal-tasks)
    run_test_names ./internal/tasks
    ;;
  internal-integrations)
    run_packages "$(printf '%s\n' github.com/usehivy/hivy/internal/integrations github.com/usehivy/hivy/internal/nango | shard_lines)"
    ;;
  internal-storage)
    run_packages "$(printf '%s\n' github.com/usehivy/hivy/internal/storage | shard_lines)"
    ;;
  internal-extra)
    run_packages "$(internal_extra_packages | shard_lines)" -skip '^TestLiveProviderTemplateE2E$'
    ;;
  e2e)
    run_test_names ./e2e
    ;;
  cmd)
    env "${test_env[@]}" go build ./cmd/...
    run_packages "$(cmd_test_packages)"
    ;;
  *)
    echo "unknown suite: $suite" >&2
    exit 2
    ;;
esac
