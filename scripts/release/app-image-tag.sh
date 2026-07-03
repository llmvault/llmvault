#!/usr/bin/env bash
set -euo pipefail

manifest="${1:?usage: app-image-tag.sh <release-manifest.json>}"
app_arch_suffix="${HIVY_SANDBOXES_APP_IMAGE_ARCH_SUFFIX:-${HIVY_SANDBOXES_RUNTIME_IMAGE_ARCH_SUFFIX:-amd64}}"

if [[ ! -f "${manifest}" ]]; then
  echo "manifest not found: ${manifest}" >&2
  exit 1
fi

command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}

sandboxes_app_image_tag="$(jq -r '.runtimeConfig.HIVY_SANDBOXES_APP_IMAGE_TAG' "${manifest}")"

if [[ -z "${sandboxes_app_image_tag}" || "${sandboxes_app_image_tag}" == "null" ]]; then
  echo "release manifest is missing runtimeConfig.HIVY_SANDBOXES_APP_IMAGE_TAG" >&2
  exit 1
fi

if [[ -n "${app_arch_suffix}" && "${sandboxes_app_image_tag}" != *"-${app_arch_suffix}" ]]; then
  sandboxes_app_image_tag="${sandboxes_app_image_tag}-${app_arch_suffix}"
fi

printf '%s\n' "${sandboxes_app_image_tag}"
