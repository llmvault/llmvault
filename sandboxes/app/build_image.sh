#!/usr/bin/env bash
# Build the Hivy app sandbox image (hivy-appd + systemd assets).
#
# Mirrors sandboxes/runtime/scripts/build_runtime_image.sh: assemble a
# minimal build context in a temp dir, then docker build. The hivy-appd
# binary is compiled inside the image (multi-stage) from the repo source, so
# no host Go cross-toolchain is required.
#
# Usage:
#   sandboxes/app/build_image.sh
#   HIVY_APP_IMAGE=ghcr.io/usehivy/hivy-app:dev sandboxes/app/build_image.sh
#   HIVY_APP_PLATFORM=linux/amd64 sandboxes/app/build_image.sh
#
# Release/CI: assemble the build context without building, so a caching
# multi-tag pusher (docker/build-push-action) can consume it — the exact
# pattern the runtime image release job uses. This keeps context assembly
# single-sourced (no drift between local builds and the release workflow):
#   HIVY_APP_CONTEXT_OUT=/path/to/context sandboxes/app/build_image.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
IMAGE="${HIVY_APP_IMAGE:-ghcr.io/usehivy/hivy-app:app}"
PLATFORM="${HIVY_APP_PLATFORM:-}"
VERSION="${VERSION:-$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)}"

# When HIVY_APP_CONTEXT_OUT is set, assemble the context into that directory
# and exit (assemble-only). Otherwise use a throwaway temp dir and build.
CONTEXT_OUT="${HIVY_APP_CONTEXT_OUT:-}"
if [[ -n "$CONTEXT_OUT" ]]; then
  mkdir -p "$CONTEXT_OUT"
  CONTEXT="$(cd "$CONTEXT_OUT" && pwd)"
else
  CONTEXT="$(mktemp -d)"
  trap 'rm -rf "$CONTEXT"' EXIT
fi

cp "$HERE/Dockerfile" "$CONTEXT/Dockerfile"
cp "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum" "$CONTEXT/"

# Copy exactly the first-party Go packages hivy-appd depends on, computed
# from the import graph so the context stays minimal and never drifts.
while IFS= read -r pkg; do
  rel="${pkg#github.com/usehivy/hivy/}"
  mkdir -p "$CONTEXT/$(dirname "$rel")"
  cp -R "$REPO_ROOT/$rel" "$CONTEXT/$rel"
done < <(cd "$REPO_ROOT" && go list -deps ./cmd/hivy-appd | grep '^github.com/usehivy/hivy/')

mkdir -p "$CONTEXT/docker/app"
cp "$HERE/entrypoint" \
  "$HERE/hivy-appd.service" \
  "$HERE/hivy-app.service" \
  "$HERE/hivy-sandbox-env-generator" \
  "$CONTEXT/docker/app/"

if [[ -n "$CONTEXT_OUT" ]]; then
  echo "assembled app sandbox build context: $CONTEXT"
  exit 0
fi

build_args=(--build-arg "VERSION=$VERSION" --build-arg "COMMIT=$COMMIT")
if [[ -n "$PLATFORM" ]]; then
  build_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" build \
  "${build_args[@]}" \
  -f "$CONTEXT/Dockerfile" \
  -t "$IMAGE" \
  "$CONTEXT"

echo "built app sandbox image: $IMAGE"
