#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ -f "$repo_root/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$repo_root/.env"
  set +a
fi

env_file="${HIVY_NANGO_COMPOSE_ENV_FILE:-$repo_root/tmp/nango-compose.env}"
source_commit="${HIVY_NANGO_SOURCE_COMMIT:-4071623aed1d5b7ae7f9382cd3805313001b7082}"
published_image="${HIVY_NANGO_IMAGE:-ghcr.io/usehivy/integrations:latest}"
local_image="${HIVY_NANGO_LOCAL_IMAGE:-usehivy/integrations:local-arm64-${source_commit}}"
arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"

mkdir -p "$(dirname "$env_file")"

write_env() {
  {
    printf 'HIVY_NANGO_IMAGE=%s\n' "$1"
    printf 'HIVY_NANGO_PLATFORM=%s\n' "$2"
  } > "$env_file"
}

if [ -n "${HIVY_NANGO_IMAGE:-}" ]; then
  case "${HIVY_NANGO_PLATFORM:-}" in
    "")
      case "$arch" in
        aarch64|arm64) platform="linux/arm64" ;;
        *) platform="linux/amd64" ;;
      esac
      ;;
    *) platform="$HIVY_NANGO_PLATFORM" ;;
  esac
  write_env "$HIVY_NANGO_IMAGE" "$platform"
  exit 0
fi

case "$arch" in
  aarch64|arm64)
    if ! docker image inspect "$local_image" >/dev/null 2>&1; then
      echo "Building real Nango for linux/arm64 as $local_image" >&2
      docker buildx build \
        --load \
        --platform linux/arm64 \
        --target self-hosted \
        --build-arg "git_hash=$source_commit" \
        -t "$local_image" \
        "https://github.com/NangoHQ/nango.git#$source_commit"
    fi
    write_env "$local_image" "linux/arm64"
    ;;
  *)
    write_env "$published_image" "${HIVY_NANGO_PLATFORM:-linux/amd64}"
    ;;
esac
