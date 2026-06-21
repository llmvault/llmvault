#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$HOME/.cargo/bin:$PATH"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
IMAGE="${HIVY_SANDBOXES_RUNTIME_IMAGE:-ghcr.io/usehivy/hivy-sandboxes-runtime:runtime}"
default_target="x86_64-unknown-linux-gnu"
BINARY="${HIVY_SANDBOXES_RUNTIME_BINARY:-$ROOT/dist/hivy-sandboxes-runtime-$default_target}"
PENPOT_BINARY="${PENPOT_CLI_BINARY:-$ROOT/dist/penpot-linux-amd64}"
PLATFORM="${HIVY_SANDBOXES_RUNTIME_PLATFORM:-}"
DOCKERFILE="${HIVY_SANDBOXES_RUNTIME_DOCKERFILE:-Dockerfile.runtime}"
TMP_CONTEXT="$(mktemp -d)"
trap 'rm -rf "$TMP_CONTEXT"' EXIT

if [[ ! -x "$BINARY" ]]; then
  echo "release binary not found or not executable: $BINARY" >&2
  echo "build it first with: scripts/build_linux_release.sh" >&2
  exit 1
fi

if [[ ! -x "$PENPOT_BINARY" ]]; then
  echo "penpot CLI binary not found or not executable: $PENPOT_BINARY" >&2
  echo "build it first with: go build -o $PENPOT_BINARY ./cmd/penpot" >&2
  exit 1
fi

kind="$(file "$BINARY")"
echo "$kind"
if [[ "$kind" != *"ELF"* ]] || [[ "$kind" != *"Linux"* ]]; then
  echo "refusing to package non-Linux binary into Debian image" >&2
  echo "expected an ELF Linux release binary at: $BINARY" >&2
  exit 1
fi

penpot_kind="$(file "$PENPOT_BINARY")"
echo "$penpot_kind"
if [[ "$penpot_kind" != *"ELF"* ]]; then
  echo "refusing to package non-Linux penpot CLI into Debian image" >&2
  echo "expected an ELF binary at: $PENPOT_BINARY" >&2
  exit 1
fi

if [[ -z "$PLATFORM" ]]; then
  case "$kind" in
    *"x86-64"*)
      PLATFORM="linux/amd64"
      ;;
    *"ARM aarch64"*)
      PLATFORM="linux/arm64"
      ;;
  esac
fi

case "$DOCKERFILE" in
  /*)
    dockerfile_path="$DOCKERFILE"
    ;;
  *)
    dockerfile_path="$ROOT/$DOCKERFILE"
    ;;
esac

if [[ ! -f "$dockerfile_path" ]]; then
  echo "runtime Dockerfile not found: $dockerfile_path" >&2
  exit 1
fi

dockerfile_name="$(basename "$dockerfile_path")"
cp "$dockerfile_path" "$TMP_CONTEXT/$dockerfile_name"
cp "$BINARY" "$TMP_CONTEXT/hivy-sandboxes-runtime"
cp "$PENPOT_BINARY" "$TMP_CONTEXT/penpot"
mkdir -p "$TMP_CONTEXT/docker" "$TMP_CONTEXT/scripts"
cp -R "$ROOT/docker/runtime" "$TMP_CONTEXT/docker/runtime"
cp "$ROOT/scripts/hivy-guardian.sh" "$TMP_CONTEXT/scripts/hivy-guardian.sh"

build_args=()
if [[ -n "$PLATFORM" ]]; then
  build_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" build \
  "${build_args[@]}" \
  -f "$TMP_CONTEXT/$dockerfile_name" \
  -t "$IMAGE" \
  "$TMP_CONTEXT"

echo "built runtime image: $IMAGE (dockerfile=$DOCKERFILE)"
