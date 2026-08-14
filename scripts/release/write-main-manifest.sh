#!/usr/bin/env bash
set -euo pipefail

commit="${1:?usage: write-main-manifest.sh <commit> <output-path>}"
out="${2:?usage: write-main-manifest.sh <commit> <output-path>}"

if [[ ! "${commit}" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "error: main image manifest commit must be a 40-character Git SHA" >&2
  exit 1
fi

commit="$(printf '%s' "${commit}" | tr '[:upper:]' '[:lower:]')"
tag="sha-${commit}"

cat >"${out}" <<EOF
{
  "version": "${tag}",
  "commit": "${commit}",
  "prerelease": false,
  "latest": false,
  "images": {
    "api": "ghcr.io/usehivy/hivy:${tag}",
    "sandboxesRuntime": "ghcr.io/usehivy/hivy-sandboxes-runtime:${tag}",
    "sandboxesRuntimeDevelopers": "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:${tag}",
    "sandboxesApp": "ghcr.io/usehivy/hivy-app:${tag}"
  },
  "runtimeConfig": {
    "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG": "${tag}",
    "HIVY_SANDBOXES_APP_IMAGE_TAG": "${tag}",
    "HIVY_SANDBOX_PROVIDER_ID": "microsandbox"
  }
}
EOF
