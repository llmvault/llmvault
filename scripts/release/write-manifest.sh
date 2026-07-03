#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: write-manifest.sh <tag> <output-path> [commit]}"
out="${2:?usage: write-manifest.sh <tag> <output-path> [commit]}"
commit="${3:-${GITHUB_SHA:-}}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

eval "$(bash "${script_dir}/derive-version.sh" "${tag}" "${commit}")"

latest=false
if [[ "${RELEASE_IS_PRERELEASE}" != "true" ]]; then
  latest=true
fi

cat >"${out}" <<EOF
{
  "version": "${RELEASE_TAG}",
  "semver": "${RELEASE_VERSION}",
  "commit": "${RELEASE_COMMIT}",
  "prerelease": ${RELEASE_IS_PRERELEASE},
  "latest": ${latest},
  "images": {
    "api": "ghcr.io/usehivy/hivy:${RELEASE_TAG}",
    "apiSemver": "ghcr.io/usehivy/hivy:${RELEASE_VERSION}",
    "sandboxesRuntime": "ghcr.io/usehivy/hivy-sandboxes-runtime:${RELEASE_TAG}",
    "sandboxesRuntimeSemver": "ghcr.io/usehivy/hivy-sandboxes-runtime:${RELEASE_VERSION}",
    "sandboxesRuntimeDevelopers": "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:${RELEASE_TAG}",
    "sandboxesRuntimeDevelopersSemver": "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:${RELEASE_VERSION}",
    "sandboxesApp": "ghcr.io/usehivy/hivy-app:${RELEASE_TAG}",
    "sandboxesAppSemver": "ghcr.io/usehivy/hivy-app:${RELEASE_VERSION}"
  },
  "runtimeConfig": {
    "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG": "${RELEASE_TAG}",
    "HIVY_SANDBOXES_APP_IMAGE_TAG": "${RELEASE_TAG}"
  }
}
EOF
