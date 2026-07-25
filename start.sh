#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_root"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  reset=$'\033[0m'
  bold=$'\033[1m'
  dim=$'\033[2m'
  blue=$'\033[38;5;75m'
  green=$'\033[38;5;78m'
  yellow=$'\033[38;5;220m'
  red=$'\033[38;5;203m'
else
  reset="" bold="" dim="" blue="" green="" yellow="" red=""
fi

phase_number=0
current_phase="preflight"
current_log=""
last_log=""
compose=()
base_compose=()
job_pids=()
job_labels=()
job_logs=()
image_mode="auto"

usage() {
  printf 'Usage: ./start.sh [--build-local-images]\n\n'
  printf '  --build-local-images  Build native sandbox images from this checkout using Docker.\n'
  printf '  --help                Show this help.\n'
}

while (( $# > 0 )); do
  case "$1" in
    --build-local-images)
      image_mode="local"
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

banner() {
  printf '\n%s%s' "$bold" "$blue"
  printf '╭──────────────────────────────────────────────────────────────╮\n'
  printf '│                    HIVY LOCAL DEVELOPMENT                    │\n'
  printf '╰──────────────────────────────────────────────────────────────╯'
  printf '%s\n\n' "$reset"
}

phase() {
  phase_number=$((phase_number + 1))
  current_phase="$1"
  printf '\n%s[%d/7] %s%s\n' "$bold$blue" "$phase_number" "$1" "$reset"
}

detail() {
  printf '      %s%s%s\n' "$dim" "$1" "$reset"
}

ok() {
  printf '  %s✓%s %s\n' "$green" "$reset" "$1"
}

warn() {
  printf '  %s!%s %s\n' "$yellow" "$reset" "$1"
}

fail() {
  printf '  %s✗%s %s\n' "$red" "$reset" "$1" >&2
}

on_error() {
  local exit_code=$?
  local line_number=$1
  fail "Phase failed: ${current_phase} (line ${line_number}, exit ${exit_code})"
  if [[ -n "$current_log" && -s "$current_log" ]]; then
    printf '\n%sCommand output%s\n' "$bold" "$reset" >&2
    tail -80 "$current_log" >&2
  fi
  if (( ${#compose[@]} > 0 )); then
    printf '\n%sContainer status%s\n' "$bold" "$reset" >&2
    "${compose[@]}" ps >&2 || true
    printf '\n%sRecent service logs%s\n' "$bold" "$reset" >&2
    "${compose[@]}" logs --since=5m --tail=60 api worker nango postgres >&2 || true
  fi
  exit "$exit_code"
}
trap 'on_error $LINENO' ERR

run_quiet() {
  local label=$1
  shift
  current_log="$repo_root/tmp/start-${phase_number}-${label}.log"
  last_log="$current_log"
  : >"$current_log"
  if ! "$@" >"$current_log" 2>&1; then
    return 1
  fi
  current_log=""
}

start_job() {
  local label=$1
  local slug=$2
  shift 2
  local log_file="$repo_root/tmp/start-${phase_number}-${slug}.log"
  : >"$log_file"
  "$@" >"$log_file" 2>&1 &
  job_pids+=("$!")
  job_labels+=("$label")
  job_logs+=("$log_file")
}

wait_jobs() {
  local failed=0
  local index=0
  local pid=""
  for pid in "${job_pids[@]}"; do
    if wait "$pid"; then
      ok "${job_labels[$index]}"
    else
      fail "${job_labels[$index]}"
      tail -80 "${job_logs[$index]}" >&2 || true
      failed=1
    fi
    index=$((index + 1))
  done
  job_pids=()
  job_labels=()
  job_logs=()
  (( failed == 0 ))
}

wait_healthy() {
  local service=$1
  local timeout_seconds=${2:-240}
  local started_at=$SECONDS
  local container_id=""
  local health=""
  local state=""

  while (( SECONDS - started_at < timeout_seconds )); do
    container_id=$("${compose[@]}" ps -q "$service" 2>/dev/null || true)
    if [[ -n "$container_id" ]]; then
      health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id" 2>/dev/null || true)
      state=$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)
      if [[ "$health" == "healthy" || ( -z "$health" && "$state" == "running" ) ]]; then
        ok "${service} is ready"
        return 0
      fi
      if [[ "$health" == "unhealthy" || "$state" == "exited" || "$state" == "dead" ]]; then
        fail "${service} entered state ${health:-$state}"
        return 1
      fi
    fi
    sleep 2
  done
  fail "Timed out waiting for ${service}"
  return 1
}

wait_completed() {
  local service=$1
  local timeout_seconds=${2:-180}
  local started_at=$SECONDS
  local container_id=""
  local state=""
  local exit_code=""

  while (( SECONDS - started_at < timeout_seconds )); do
    container_id=$("${compose[@]}" ps -aq "$service" 2>/dev/null || true)
    if [[ -n "$container_id" ]]; then
      state=$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)
      exit_code=$(docker inspect --format '{{.State.ExitCode}}' "$container_id" 2>/dev/null || true)
      if [[ "$state" == "exited" && "$exit_code" == "0" ]]; then
        ok "${service} completed"
        return 0
      fi
      if [[ "$state" == "exited" && "$exit_code" != "0" ]]; then
        fail "${service} exited with code ${exit_code}"
        return 1
      fi
    fi
    sleep 2
  done
  fail "Timed out waiting for ${service}"
  return 1
}

pull_published_image() {
  local ref=$1
  local cached_arch=""
  cached_arch=$(docker image inspect --format '{{.Architecture}}' "$ref" 2>/dev/null || true)
  if [[ "$cached_arch" == "$local_arch" ]]; then
    return
  fi
  docker pull --platform "$local_platform" "$ref"
}

published_platform_available() {
  local ref=$1
  local platform=$2
  local manifest=""
  manifest=$(docker buildx imagetools inspect "$ref" 2>/dev/null) || return 1
  grep -Eq "Platform:[[:space:]]+${platform}$" <<<"$manifest"
}

published_port() {
  local service=$1
  local container_port=$2
  "${compose[@]}" port "$service" "$container_port" | tail -1 | awk -F: '{print $NF}'
}

banner

phase "Preflight"
command -v docker >/dev/null 2>&1 || {
  fail "Docker is required"
  exit 1
}
docker info >/dev/null 2>&1 || {
  fail "Docker is not running"
  exit 1
}
docker compose version >/dev/null 2>&1 || {
  fail "Docker Compose v2 is required"
  exit 1
}
docker_arch=$(docker info --format '{{.Architecture}}')
case "$docker_arch" in
  arm64|aarch64)
    local_arch="arm64"
    local_platform="linux/arm64"
    docker buildx version >/dev/null 2>&1 || {
      fail "Docker Buildx is required on ARM hosts"
      exit 1
    }
    ;;
  amd64|x86_64)
    local_arch="amd64"
    local_platform="linux/amd64"
    ;;
  *)
    fail "Unsupported Docker architecture: ${docker_arch}"
    exit 1
    ;;
esac
ok "Docker is ready (${docker_arch})"
ok "Local Docker sandbox driver selected"

phase "Local configuration"
if [[ ! -f .env ]]; then
  cp .env.example .env
  chmod 600 .env
  ok "Created .env from the local development template"
else
  ok "Using existing .env"
fi
mkdir -p tmp

export HIVY_ENVIRONMENT=development
export HIVY_ADMIN_ENABLED=true
export HIVY_ADMIN_SECRET="${HIVY_ADMIN_SECRET:-local-development-admin}"
export HIVY_SANDBOX_PROVIDER_ID=docker
requested_runtime_tag="${HIVY_DEV_RUNTIME_IMAGE_TAG:-latest}"
requested_app_tag="${HIVY_DEV_APP_IMAGE_TAG:-latest}"
if [[ "$image_mode" == "auto" ]]; then
  if published_platform_available "ghcr.io/usehivy/hivy-sandboxes-runtime:${requested_runtime_tag}" "$local_platform" &&
    published_platform_available "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:${requested_runtime_tag}" "$local_platform" &&
    published_platform_available "ghcr.io/usehivy/hivy-app:${requested_app_tag}" "$local_platform"; then
    image_mode="published"
  else
    image_mode="local"
    detail "Published tags do not support ${local_platform}; using native local builds"
  fi
fi
if [[ "$image_mode" == "local" ]]; then
  export HIVY_SANDBOXES_RUNTIME_IMAGE_TAG=local
  export HIVY_SANDBOXES_APP_IMAGE_TAG=local
  detail "Sandbox images will be built locally for ${local_platform}"
else
  export HIVY_SANDBOXES_RUNTIME_IMAGE_TAG="$requested_runtime_tag"
  export HIVY_SANDBOXES_APP_IMAGE_TAG="$requested_app_tag"
  detail "Published sandbox images will use tag ${HIVY_SANDBOXES_RUNTIME_IMAGE_TAG}"
fi
base_compose=(docker compose --env-file .env)

phase "Parallel preparation"
start_job "Real Nango image prepared" nango-image env \
  HIVY_NANGO_COMPOSE_ENV_FILE="$repo_root/tmp/nango-compose.env" \
  ./scripts/ensure-nango-image.sh
start_job "Core infrastructure started" infrastructure \
  "${base_compose[@]}" up -d postgres redis redis-2 redis-3 redis-cluster-init qdrant minio minio-setup
start_job "Hivy development images built" hivy-images \
  "${base_compose[@]}" build api worker

if [[ "${HIVY_DEV_SKIP_RUNTIME_IMAGES:-0}" == "1" ]]; then
  warn "Sandbox image preparation skipped by HIVY_DEV_SKIP_RUNTIME_IMAGES=1"
elif [[ "$image_mode" == "local" ]]; then
  start_job "Native runtime images built" local-runtimes env \
    HIVY_LOCAL_DOCKER_PLATFORM="$local_platform" \
    HIVY_LOCAL_DOCKER_ARCH="$local_arch" \
    ./scripts/build-local-sandbox-images.sh
  start_job "Native app image built" local-app env \
    HIVY_APP_IMAGE=ghcr.io/usehivy/hivy-app:local \
    HIVY_APP_PLATFORM="$local_platform" \
    bash sandboxes/app/build_image.sh
else
  start_job "Default runtime image ready" runtime-image \
    pull_published_image "ghcr.io/usehivy/hivy-sandboxes-runtime:${HIVY_SANDBOXES_RUNTIME_IMAGE_TAG}"
  start_job "Developer runtime image ready" developer-image \
    pull_published_image "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:${HIVY_SANDBOXES_RUNTIME_IMAGE_TAG}"
  start_job "App sandbox image ready" app-image \
    pull_published_image "ghcr.io/usehivy/hivy-app:${HIVY_SANDBOXES_APP_IMAGE_TAG}"
fi
wait_jobs

compose=(docker compose --env-file .env --env-file tmp/nango-compose.env)

phase "Infrastructure"
run_quiet nango "${compose[@]}" up -d nango
wait_healthy postgres
wait_healthy redis
wait_healthy redis-2
wait_healthy redis-3
wait_completed redis-cluster-init
run_quiet redis-cluster \
  "${compose[@]}" exec -T redis sh -ec \
  'redis-cli -p 16279 cluster info | grep -q "cluster_state:ok" &&
    redis-cli -p 16279 cluster nodes |
      awk '\''BEGIN { good=1 } $3 ~ /fail/ || $8 != "connected" { good=0 } END { exit !(good && NR == 3) }'\'''
ok "redis cluster is ready"
wait_healthy nango 300
wait_healthy minio
wait_completed minio-setup
until "${compose[@]}" exec -T minio curl -fsS http://qdrant:6333/readyz >/dev/null 2>&1; do
  sleep 2
done
ok "qdrant is ready"

nango_secret=$("${compose[@]}" exec -T postgres sh -lc \
  'psql -U "$POSTGRES_USER" -d nango -Atc "SELECT secret_key FROM nango._nango_environments WHERE name='\''prod'\'' LIMIT 1"')
[[ -n "$nango_secret" ]] || {
  fail "Nango became healthy without creating its local environment secret"
  exit 1
}
export HIVY_NANGO_SECRET_KEY="$nango_secret"
ok "Real Nango environment is ready (no integrations configured)"

phase "Database migrations"
run_quiet migrations "${compose[@]}" run --rm --no-deps api go run ./cmd/server migrate up
ok "Hivy database is current"

phase "Hivy services"
run_quiet services "${compose[@]}" up -d --no-build --no-deps api worker proxy web
wait_healthy api 300
wait_healthy worker 300
wait_healthy proxy
wait_healthy web 300

phase "Development workspace"
seed_email="${HIVY_DEV_SEED_EMAIL:-dev@hivy.local}"
seed_password="${HIVY_DEV_SEED_PASSWORD:-local-development}"
run_quiet seed "${compose[@]}" run --rm --no-deps \
  -e HIVY_DEV_SEED_ENABLED=true \
  -e HIVY_DEV_SEED_EMAIL="$seed_email" \
  -e HIVY_DEV_SEED_PASSWORD="$seed_password" \
  -e HIVY_DEV_SEED_USER_NAME="${HIVY_DEV_SEED_USER_NAME:-Local Developer}" \
  -e HIVY_DEV_SEED_ORG_NAME="${HIVY_DEV_SEED_ORG_NAME:-Hivy Development}" \
  -e HIVY_DEV_SEED_TEAM_NAME="${HIVY_DEV_SEED_TEAM_NAME:-Development}" \
  api go run ./cmd/dev-seed
seed_summary=$(grep 'development seed ready' "$last_log" | tail -1 || true)
if [[ -n "$seed_summary" ]]; then
  detail "$seed_summary"
fi
ok "Local user, organization, team, and team-scoped Hivy agent are ready"

web_port=$(published_port web 3000)
api_port=$(published_port api 8080)
nango_port=$(published_port nango 3003)
minio_port=$(published_port minio 9001)

printf '\n%s%sLocal Hivy is ready%s\n\n' "$bold" "$green" "$reset"
printf '  %-17s http://localhost:%s\n' "Application" "$web_port"
printf '  %-17s http://localhost:%s\n' "API" "$api_port"
printf '  %-17s http://localhost:%s\n' "Admin panel" "$web_port/admin"
printf '  %-17s http://localhost:%s\n' "Nango" "$nango_port"
printf '  %-17s http://localhost:%s\n' "MinIO console" "$minio_port"
printf '\n%sSeeded login%s\n' "$bold" "$reset"
printf '  %-17s %s\n' "Email" "$seed_email"
if [[ "$seed_password" == "local-development" ]]; then
  printf '  %-17s %s\n' "Password" "$seed_password"
else
  printf '  %-17s %s\n' "Password" "custom value from HIVY_DEV_SEED_PASSWORD"
fi
printf '\n%sBefore starting an agent%s\n' "$bold" "$reset"
printf '  1. Open http://localhost:%s/admin\n' "$web_port"
if [[ "$HIVY_ADMIN_SECRET" == "local-development-admin" ]]; then
  printf '  2. Enter admin secret: %s\n' "$HIVY_ADMIN_SECRET"
else
  printf '  2. Enter the custom HIVY_ADMIN_SECRET supplied to this script\n'
fi
printf '  3. Add at least one LLM system credential from the admin panel\n'
printf '  4. Log in and start Hivy; sandboxes will run through local Docker\n'
printf '\n%sConnections%s\n' "$bold" "$reset"
printf '  No Nango integrations or connections were created. Add them manually in Hivy when needed.\n'
printf '\n%sUseful commands%s\n' "$bold" "$reset"
printf '  docker compose down       Stop services and preserve local data\n'
printf '  docker compose down -v    Delete containers and all development data\n'
printf '  docker compose logs -f api worker web\n\n'
