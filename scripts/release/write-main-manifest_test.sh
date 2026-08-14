#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "${test_dir}"' EXIT

commit="0123456789abcdef0123456789abcdef01234567"
manifest="${test_dir}/main-manifest.json"

bash "${repo_root}/scripts/release/write-main-manifest.sh" "${commit}" "${manifest}"

jq -e --arg commit "${commit}" '
  .version == ("sha-" + $commit)
  and .commit == $commit
  and .latest == false
  and .images.sandboxesRuntime == ("ghcr.io/usehivy/hivy-sandboxes-runtime:sha-" + $commit)
  and .images.sandboxesRuntimeDevelopers == ("ghcr.io/usehivy/hivy-sandboxes-runtime-developers:sha-" + $commit)
  and .images.sandboxesApp == ("ghcr.io/usehivy/hivy-app:sha-" + $commit)
  and .runtimeConfig.HIVY_SANDBOXES_RUNTIME_IMAGE_TAG == ("sha-" + $commit)
  and .runtimeConfig.HIVY_SANDBOXES_APP_IMAGE_TAG == ("sha-" + $commit)
' "${manifest}" >/dev/null

if bash "${repo_root}/scripts/release/write-main-manifest.sh" "sha-${commit}" "${manifest}" >/dev/null 2>&1; then
  echo "write-main-manifest must reject a prefixed commit" >&2
  exit 1
fi

echo "write-main-manifest tests passed"
