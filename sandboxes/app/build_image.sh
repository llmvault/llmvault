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
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
IMAGE="${HIVY_APP_IMAGE:-ghcr.io/usehivy/hivy-app:app}"
PLATFORM="${HIVY_APP_PLATFORM:-}"
VERSION="${VERSION:-$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)}"

TMP_CONTEXT="$(mktemp -d)"
trap 'rm -rf "$TMP_CONTEXT"' EXIT

cp "$HERE/Dockerfile" "$TMP_CONTEXT/Dockerfile"
cp "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum" "$TMP_CONTEXT/"

# Copy exactly the first-party Go packages hivy-appd depends on, computed
# from the import graph so the context stays minimal and never drifts.
while IFS= read -r pkg; do
  rel="${pkg#github.com/usehivy/hivy/}"
  mkdir -p "$TMP_CONTEXT/$(dirname "$rel")"
  cp -R "$REPO_ROOT/$rel" "$TMP_CONTEXT/$rel"
done < <(cd "$REPO_ROOT" && go list -deps ./cmd/hivy-appd | grep '^github.com/usehivy/hivy/')

mkdir -p "$TMP_CONTEXT/docker/app"
cp "$HERE/entrypoint" \
  "$HERE/hivy-appd.service" \
  "$HERE/hivy-app.service" \
  "$HERE/hivy-sandbox-env-generator" \
  "$TMP_CONTEXT/docker/app/"

build_args=(--build-arg "VERSION=$VERSION" --build-arg "COMMIT=$COMMIT")
if [[ -n "$PLATFORM" ]]; then
  build_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" build \
  "${build_args[@]}" \
  -f "$TMP_CONTEXT/Dockerfile" \
  -t "$IMAGE" \
  "$TMP_CONTEXT"

echo "built app sandbox image: $IMAGE"
