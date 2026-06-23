#!/usr/bin/env bash
set -euo pipefail

manifest="${1:?usage: runtime-image-tag.sh <release-manifest.json>}"
runtime_arch_suffix="${HIVY_SANDBOXES_RUNTIME_IMAGE_ARCH_SUFFIX:-amd64}"

if [[ ! -f "${manifest}" ]]; then
  echo "manifest not found: ${manifest}" >&2
  exit 1
fi

command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}

sandboxes_runtime_image_tag="$(jq -r '.runtimeConfig.HIVY_SANDBOXES_RUNTIME_IMAGE_TAG' "${manifest}")"

if [[ -n "${runtime_arch_suffix}" && "${sandboxes_runtime_image_tag}" != *"-${runtime_arch_suffix}" ]]; then
  sandboxes_runtime_image_tag="${sandboxes_runtime_image_tag}-${runtime_arch_suffix}"
fi

if [[ -z "${sandboxes_runtime_image_tag}" || "${sandboxes_runtime_image_tag}" == "null" ]]; then
  echo "release manifest is missing runtimeConfig.HIVY_SANDBOXES_RUNTIME_IMAGE_TAG" >&2
  exit 1
fi

printf '%s\n' "${sandboxes_runtime_image_tag}"
