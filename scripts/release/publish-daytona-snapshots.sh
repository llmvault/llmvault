#!/usr/bin/env bash
set -euo pipefail

daytona_max_cpu="${HIVY_DAYTONA_MAX_CPU:-4}"
daytona_max_memory="${HIVY_DAYTONA_MAX_MEMORY_GB:-8}"
daytona_max_disk="${HIVY_DAYTONA_MAX_DISK_GB:-10}"

manifest="${1:?usage: publish-daytona-snapshots.sh <release-manifest.json>}"
: "${HIVY_DAYTONA_API_KEY:?HIVY_DAYTONA_API_KEY is required}"
: "${HIVY_DAYTONA_API_URL:?HIVY_DAYTONA_API_URL is required}"

command -v curl >/dev/null || {
  echo "curl is required" >&2
  exit 1
}
command -v daytona >/dev/null || {
  echo "daytona CLI is required" >&2
  exit 1
}
command -v docker >/dev/null || {
  echo "docker is required" >&2
  exit 1
}
command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}

if [[ ! -f "${manifest}" ]]; then
  echo "manifest not found: ${manifest}" >&2
  exit 1
fi

export DAYTONA_API_KEY="${HIVY_DAYTONA_API_KEY}"
export DAYTONA_API_URL="${HIVY_DAYTONA_API_URL}"
export DAYTONA_TARGET="${HIVY_DAYTONA_TARGET:-}"

release_tag="$(jq -r '.version' "${manifest}")"
release_version="${release_tag#v}"
release_version_dashed="${release_version//./-}"
runtime_image="$(jq -r '.images.sandboxesRuntimeDaytona' "${manifest}")-amd64"
developers_image="$(jq -r '.images.sandboxesRuntimeDevelopersDaytona' "${manifest}")-amd64"
requested_sizes="${DAYTONA_SNAPSHOT_SIZES:-all}"

for value in "${release_tag}" "${runtime_image}" "${developers_image}"; do
  if [[ -z "${value}" || "${value}" == "null" ]]; then
    echo "release manifest is missing Daytona release data" >&2
    exit 1
  fi
done

resolve_sizes() {
  if [[ "${requested_sizes}" == "all" ]]; then
    printf '%s\n' micro nano small medium large xlarge
    return
  fi
  tr ',' '\n' <<<"${requested_sizes}" | sed '/^[[:space:]]*$/d; s/^[[:space:]]*//; s/[[:space:]]*$//'
}

snapshot_resources() {
  case "$1" in
    micro) printf '%s\n' "1 1 5" ;;
    nano) printf '%s\n' "1 1 5" ;;
    small) printf '%s\n' "1 2 10" ;;
    medium) printf '%s\n' "2 4 20" ;;
    large) printf '%s\n' "4 8 40" ;;
    xlarge) printf '%s\n' "8 16 60" ;;
    *)
      echo "unsupported Daytona snapshot size: $1" >&2
      return 1
      ;;
  esac
}

snapshot_state() {
  local name="$1"
  local response_file
  local status
  response_file="$(mktemp)"
  status="$(
    curl --silent --show-error \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header "Authorization: Bearer ${HIVY_DAYTONA_API_KEY}" \
      "${HIVY_DAYTONA_API_URL%/}/snapshots/${name}"
  )"
  case "${status}" in
    200) jq -r '.state // "unknown"' "${response_file}" ;;
    404) printf '%s\n' missing ;;
    *)
      echo "Daytona snapshot lookup failed for ${name} with HTTP ${status}" >&2
      rm -f "${response_file}"
      return 1
      ;;
  esac
  rm -f "${response_file}"
}

wait_for_snapshot() {
  local name="$1"
  local deadline=$((SECONDS + 1800))
  local state
  while ((SECONDS < deadline)); do
    state="$(snapshot_state "${name}")"
    case "${state}" in
      active)
        echo "Daytona snapshot ${name} is active."
        return 0
        ;;
      error | failed)
        echo "Daytona snapshot ${name} entered ${state}." >&2
        return 1
        ;;
      *) sleep 10 ;;
    esac
  done
  echo "Timed out waiting for Daytona snapshot ${name}." >&2
  return 1
}

publish_variant() {
  local image="$1"
  local prefix="$2"
  local minimum_disk="$3"
  local size
  local cpu
  local memory
  local disk
  local requested_cpu
  local requested_memory
  local requested_disk
  local name
  local state

  docker pull --platform linux/amd64 "${image}"
  if [[ "$(docker image inspect --format '{{.Architecture}}' "${image}")" != "amd64" ]]; then
    echo "Daytona image is not linux/amd64: ${image}" >&2
    return 1
  fi

  while IFS= read -r size; do
    read -r cpu memory disk <<<"$(snapshot_resources "${size}")"
    requested_cpu="${cpu}"
    requested_memory="${memory}"
    requested_disk="${disk}"
    if ((disk < minimum_disk)); then
      disk="${minimum_disk}"
    fi
    if ((cpu > daytona_max_cpu)); then
      cpu="${daytona_max_cpu}"
    fi
    if ((memory > daytona_max_memory)); then
      memory="${daytona_max_memory}"
    fi
    if ((disk > daytona_max_disk)); then
      disk="${daytona_max_disk}"
    fi
    name="${prefix}-${release_version_dashed}-${size}-v1"
    if [[ "${size}" == "micro" ]]; then
      echo "Daytona adjusts Hivy micro to its minimum ${cpu} CPU/${memory} GiB memory/${disk} GiB disk allocation."
    elif ((requested_cpu != cpu || requested_memory != memory || requested_disk != disk)); then
      echo "Daytona adjusts Hivy ${size} from ${requested_cpu} CPU/${requested_memory} GiB memory/${requested_disk} GiB disk to ${cpu} CPU/${memory} GiB memory/${disk} GiB disk."
    fi
    state="$(snapshot_state "${name}")"
    if [[ "${state}" == "active" ]]; then
      echo "Daytona snapshot ${name} already active; skipping."
      continue
    fi
    if [[ "${state}" != "missing" ]]; then
      echo "Daytona snapshot ${name} exists in state ${state}; refusing to replace an immutable release snapshot." >&2
      return 1
    fi

    daytona snapshot push "${image}" \
      --name "${name}" \
      --entrypoint /usr/local/bin/hivy-daytona-entrypoint \
      --cpu "${cpu}" \
      --memory "${memory}" \
      --disk "${disk}"
    wait_for_snapshot "${name}"
  done < <(resolve_sizes)
}

if [[ "${DAYTONA_PUBLISH_RUNTIME:-true}" == "true" ]]; then
  publish_variant "${runtime_image}" hivy-sandboxes-runtime-daytona 5
fi
if [[ "${DAYTONA_PUBLISH_DEVELOPERS:-true}" == "true" ]]; then
  publish_variant "${developers_image}" hivy-sandboxes-runtime-developers-daytona 10
fi
