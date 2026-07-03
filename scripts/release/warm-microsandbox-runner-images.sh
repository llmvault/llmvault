#!/usr/bin/env bash
set -euo pipefail

manifest="${1:?usage: warm-microsandbox-runner-images.sh <release-manifest.json> <runner-url>...}"
shift
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

command -v curl >/dev/null || {
  echo "curl is required" >&2
  exit 1
}
command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}

runtime_tag="$(bash "${script_dir}/runtime-image-tag.sh" "${manifest}")"
app_tag="$(bash "${script_dir}/app-image-tag.sh" "${manifest}")"
runtime_repo="$(jq -r '.images.sandboxesRuntime | sub(":[^/:]+$"; "")' "${manifest}")"
developers_repo="$(jq -r '.images.sandboxesRuntimeDevelopers | sub(":[^/:]+$"; "")' "${manifest}")"
app_repo="$(jq -r '.images.sandboxesApp | sub(":[^/:]+$"; "")' "${manifest}")"

for repo in "${runtime_repo}" "${developers_repo}" "${app_repo}"; do
  if [[ -z "${repo}" || "${repo}" == "null" ]]; then
    echo "release manifest is missing runtime/app image repositories" >&2
    exit 1
  fi
done

images=(
  "${runtime_repo}:${runtime_tag}"
  "${developers_repo}:${runtime_tag}"
  "${app_repo}:${app_tag}"
)

runner_urls=("$@")

if [[ "${#runner_urls[@]}" -eq 0 ]]; then
  echo "no runner URLs provided; skipping runner image cache warm-up"
  exit 0
fi

: "${HIVY_MICROSANDBOX_RUNNER_API_TOKEN:?HIVY_MICROSANDBOX_RUNNER_API_TOKEN is required}"

created=()

sandbox_hash() {
  if command -v sha256sum >/dev/null; then
    printf '%s' "${1}" | sha256sum | awk '{print $1}' | cut -c1-16
    return
  fi
  printf '%s' "${1}" | shasum -a 256 | awk '{print $1}' | cut -c1-16
}

delete_sandbox() {
  local runner_url="${1}"
  local sandbox_id="${2}"
  curl --fail --show-error --silent \
    --retry 3 \
    --retry-delay 5 \
    --connect-timeout 10 \
    --max-time "${HIVY_MICROSANDBOX_RUNNER_DELETE_TIMEOUT_SECONDS:-180}" \
    -X POST "${runner_url}/v1/sandboxes/${sandbox_id}/delete" \
    -H "Authorization: Bearer ${HIVY_MICROSANDBOX_RUNNER_API_TOKEN}" >/dev/null
}

cleanup_created() {
  local item runner_url sandbox_id
  for item in "${created[@]}"; do
    runner_url="${item%%|*}"
    sandbox_id="${item#*|}"
    delete_sandbox "${runner_url}" "${sandbox_id}" || true
  done
}
trap cleanup_created EXIT

for runner_url in "${runner_urls[@]}"; do
  runner_url="${runner_url%/}"
  if [[ -z "${runner_url}" ]]; then
    continue
  fi

  for image in "${images[@]}"; do
    image_hash="$(sandbox_hash "${runner_url}|${image}")"
    sandbox_id="release-warm-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}-${image_hash}"
    payload="$(
      jq -nc \
        --arg id "${sandbox_id}" \
        --arg image "${image}" \
        --arg release "${runtime_tag}" \
        '{
          id: $id,
          name: $id,
          image_ref: $image,
          cpu: 1,
          memory_mb: 2048,
          disk_gb: 8,
          env: {},
          labels: {purpose: "release-cache-warm", release: $release},
          preview_ports: [],
          init: {cmd: "/bin/sleep", args: ["300"]}
        }'
    )"

    echo "Warming ${image} on ${runner_url}"
    created+=("${runner_url}|${sandbox_id}")
    delete_sandbox "${runner_url}" "${sandbox_id}" || true
    curl --fail --show-error --silent \
      --retry 3 \
      --retry-delay 5 \
      --connect-timeout 10 \
      --max-time "${HIVY_MICROSANDBOX_RUNNER_CREATE_TIMEOUT_SECONDS:-900}" \
      -X POST "${runner_url}/v1/sandboxes" \
      -H "Authorization: Bearer ${HIVY_MICROSANDBOX_RUNNER_API_TOKEN}" \
      -H "Content-Type: application/json" \
      --data "${payload}" >/dev/null
    delete_sandbox "${runner_url}" "${sandbox_id}"
  done
done

trap - EXIT
