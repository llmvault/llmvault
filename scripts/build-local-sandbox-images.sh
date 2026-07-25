#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
platform="${HIVY_LOCAL_DOCKER_PLATFORM:?HIVY_LOCAL_DOCKER_PLATFORM is required}"
architecture="${HIVY_LOCAL_DOCKER_ARCH:?HIVY_LOCAL_DOCKER_ARCH is required}"
runtime_image="${HIVY_LOCAL_RUNTIME_IMAGE:-ghcr.io/usehivy/hivy-sandboxes-runtime:local}"
developer_image="${HIVY_LOCAL_DEVELOPER_RUNTIME_IMAGE:-ghcr.io/usehivy/hivy-sandboxes-runtime-developers:local}"
output_dir="$repo_root/tmp/local-sandbox-build/$architecture"

case "$architecture" in
  arm64)
    goarch="arm64"
    ;;
  amd64)
    goarch="amd64"
    ;;
  *)
    printf 'unsupported local sandbox architecture: %s\n' "$architecture" >&2
    exit 1
    ;;
esac

mkdir -p "$output_dir"
rm -f "$output_dir/hivy-sandboxes-runtime" "$output_dir/canvas"

docker volume create usehivycom_local-cargo-registry >/dev/null
docker volume create usehivycom_local-cargo-target-"$architecture" >/dev/null
docker volume create usehivycom_local-go-mod >/dev/null
docker volume create usehivycom_local-go-build-"$architecture" >/dev/null

rust_log="$output_dir/rust-build.log"
canvas_log="$output_dir/canvas-build.log"

docker run --rm --platform "$platform" \
  -e CARGO_TARGET_DIR=/cargo-target \
  -v "$repo_root:/src:ro" \
  -v "$output_dir:/out" \
  -v usehivycom_local-cargo-registry:/usr/local/cargo/registry \
  -v usehivycom_local-cargo-target-"$architecture":/cargo-target \
  -w /src/sandboxes/runtime \
  rust:bookworm \
  bash -c 'cargo build --release --locked -p hivy-sandboxes-runtime && cp /cargo-target/release/hivy-sandboxes-runtime /out/hivy-sandboxes-runtime' \
  >"$rust_log" 2>&1 &
rust_pid=$!

docker run --rm --platform "$platform" \
  -e CGO_ENABLED=0 \
  -e GOOS=linux \
  -e GOARCH="$goarch" \
  -v "$repo_root:/src:ro" \
  -v "$output_dir:/out" \
  -v usehivycom_local-go-mod:/go/pkg/mod \
  -v usehivycom_local-go-build-"$architecture":/root/.cache/go-build \
  -w /src \
  golang:1.25-bookworm \
  bash -c 'go build -trimpath -ldflags="-s -w" -o /out/canvas ./cmd/canvas' \
  >"$canvas_log" 2>&1 &
canvas_pid=$!

compile_failed=0
if ! wait "$rust_pid"; then
  printf 'local Rust runtime compilation failed\n' >&2
  tail -80 "$rust_log" >&2
  compile_failed=1
fi
if ! wait "$canvas_pid"; then
  printf 'local canvas compilation failed\n' >&2
  tail -80 "$canvas_log" >&2
  compile_failed=1
fi
if (( compile_failed != 0 )); then
  exit 1
fi

chmod +x "$output_dir/hivy-sandboxes-runtime" "$output_dir/canvas"
runtime_log="$output_dir/runtime-image.log"
developer_log="$output_dir/developer-image.log"

HIVY_SANDBOXES_RUNTIME_BINARY="$output_dir/hivy-sandboxes-runtime" \
CANVAS_CLI_BINARY="$output_dir/canvas" \
HIVY_SANDBOXES_RUNTIME_IMAGE="$runtime_image" \
HIVY_SANDBOXES_RUNTIME_PLATFORM="$platform" \
bash "$repo_root/sandboxes/runtime/scripts/build_runtime_image.sh" \
  >"$runtime_log" 2>&1 &
runtime_pid=$!

HIVY_SANDBOXES_RUNTIME_BINARY="$output_dir/hivy-sandboxes-runtime" \
CANVAS_CLI_BINARY="$output_dir/canvas" \
HIVY_SANDBOXES_RUNTIME_IMAGE="$developer_image" \
HIVY_SANDBOXES_RUNTIME_PLATFORM="$platform" \
HIVY_SANDBOXES_RUNTIME_DOCKERFILE="Dockerfile.developers" \
bash "$repo_root/sandboxes/runtime/scripts/build_runtime_image.sh" \
  >"$developer_log" 2>&1 &
developer_pid=$!

image_failed=0
if ! wait "$runtime_pid"; then
  printf 'local default runtime image build failed\n' >&2
  tail -80 "$runtime_log" >&2
  image_failed=1
fi
if ! wait "$developer_pid"; then
  printf 'local developer runtime image build failed\n' >&2
  tail -80 "$developer_log" >&2
  image_failed=1
fi
if (( image_failed != 0 )); then
  exit 1
fi

printf 'built native local sandbox runtime images for %s\n' "$platform"
